// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package launchermigration provides the primitives startup path migration
// is built from: a completion marker and a staging/resume helper.
package launchermigration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	markerDirPerm  = 0o700
	markerFilePerm = 0o600
	markerContents = "1\n"
)

// Marker records completion of a one-time step as a plain file.
type Marker struct {
	path string
}

func NewMarker(path string) Marker {
	return Marker{path: path}
}

func (m Marker) Path() string {
	return m.path
}

func (m Marker) Done() (bool, error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("read migration marker %s: %w", m.path, err)
	}

	return string(data) == markerContents, nil
}

func (m Marker) Write() error {
	if err := os.MkdirAll(filepath.Dir(m.path), markerDirPerm); err != nil {
		return fmt.Errorf("create directory for migration marker %s: %w", m.path, err)
	}
	if err := os.WriteFile(m.path, []byte(markerContents), markerFilePerm); err != nil {
		return fmt.Errorf("write migration marker %s: %w", m.path, err)
	}

	return nil
}
