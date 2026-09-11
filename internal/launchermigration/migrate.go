// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package launchermigration

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/util"
)

const stagingSuffix = ".migrating"

// ErrDestinationExists is returned by Migrate when destination already
// exists; callers own collision handling.
var ErrDestinationExists = errors.New("migration destination already exists")

// Migrate moves source to destination. A pre-existing destination is accepted
// only when the staging marker proves this migration already published it.
// A cross-filesystem move can't be one atomic rename, so it stages the copy
// and only removes source after a completion marker confirms the copy
// landed; an interruption before that marker leaves source untouched and
// gets retried from scratch.
func Migrate(source, destination string) error {
	if _, err := os.Lstat(destination); err == nil {
		resumed, resumeErr := finishPublishedMigration(source, destination)
		if resumeErr != nil {
			return resumeErr
		}
		if resumed {
			return nil
		}

		return fmt.Errorf("%w: %s", ErrDestinationExists, destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect migration destination %s: %w", destination, err)
	}

	if err := os.MkdirAll(filepath.Dir(destination), markerDirPerm); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", destination, err)
	}

	if err := os.Rename(source, destination); err == nil {
		return nil
	}

	return migrateViaStaging(source, destination)
}

func migrateViaStaging(source, destination string) error {
	staging := destination + stagingSuffix
	marker := migrationMarker(destination)

	done, err := marker.Done()
	if err != nil {
		return err
	}
	if !done {
		if err := stageDurableCopy(source, staging, marker); err != nil {
			return err
		}
	}

	if err := os.Rename(staging, destination); err != nil {
		return fmt.Errorf("publish migrated %s to %s: %w", staging, destination, err)
	}
	// The publish is a directory-entry change, so it has to be durable before
	// the source goes; otherwise an interruption here could leave neither
	// location holding the data.
	if err := syncFile(filepath.Dir(destination)); err != nil {
		return err
	}
	if err := os.RemoveAll(source); err != nil {
		return fmt.Errorf("remove migrated source %s: %w", source, err)
	}
	if err := os.Remove(marker.Path()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove migration marker %s: %w", marker.Path(), err)
	}

	return nil
}

// stageDurableCopy leaves staging holding a complete copy of source, writing
// the marker only once the staged bytes and the directory entries recording
// them are both on disk. The marker means "this copy is durable, safe to
// publish", so anything it vouches for has to survive a power loss.
func stageDurableCopy(source, staging string, marker Marker) error {
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("reset incomplete migration staging %s: %w", staging, err)
	}
	if err := copyPath(source, staging); err != nil {
		return fmt.Errorf("copy %s to staging %s: %w", source, staging, err)
	}
	if err := syncPath(staging); err != nil {
		return err
	}
	if err := marker.Write(); err != nil {
		return err
	}

	return syncFile(filepath.Dir(staging))
}

func finishPublishedMigration(source, destination string) (bool, error) {
	marker := migrationMarker(destination)
	done, err := marker.Done()
	if err != nil || !done {
		return false, err
	}

	staging := destination + stagingSuffix
	if _, err := os.Lstat(staging); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect migration staging %s: %w", staging, err)
	}

	if err := os.RemoveAll(source); err != nil {
		return true, fmt.Errorf("remove migrated source %s: %w", source, err)
	}
	if err := os.Remove(marker.Path()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return true, fmt.Errorf("remove migration marker %s: %w", marker.Path(), err)
	}

	return true, nil
}

func migrationMarker(destination string) Marker {
	return NewMarker(destination + stagingSuffix + ".complete")
}

func copyPath(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", source, err)
	}
	if info.IsDir() {
		return copyDirectory(source, destination)
	}

	return copyFile(source, destination, info.Mode())
}

// The staging directory starts private so a half-copied tree is never
// world-readable; util.CopyDir restores the source's own modes afterwards.
func copyDirectory(source, destination string) error {
	if err := os.Mkdir(destination, markerDirPerm); err != nil {
		return fmt.Errorf("create private staging directory %s: %w", destination, err)
	}

	return util.CopyDir(source, destination)
}

// syncPath flushes a staged file, or every file and directory entry of a
// staged tree, so the copy survives a system failure and not merely a
// process exit.
func syncPath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect staged %s: %w", path, err)
	}
	if !info.IsDir() {
		return syncFile(path)
	}

	if err := filepath.Walk(path, func(walked string, walkedInfo os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if walkedInfo.IsDir() || walkedInfo.Mode().IsRegular() {
			return syncFile(walked)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("flush staged %s: %w", path, err)
	}

	return nil
}

func syncFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open staged %s for flushing: %w", path, err)
	}
	defer file.Close()

	if err := file.Sync(); err != nil {
		return fmt.Errorf("flush staged %s: %w", path, err)
	}

	return nil
}

func copyFile(source, destination string, mode os.FileMode) error {
	sourceFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open %s: %w", source, err)
	}
	defer sourceFile.Close()

	destFile, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return fmt.Errorf("create %s: %w", destination, err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("copy %s to %s: %w", source, destination, err)
	}
	if err := destFile.Chmod(mode.Perm()); err != nil {
		return fmt.Errorf("preserve permissions from %s on %s: %w", source, destination, err)
	}

	return destFile.Sync()
}
