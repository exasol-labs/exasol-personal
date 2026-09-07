// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/slc"
)

func TestInstalledFlavors(t *testing.T) {
	t.Parallel()

	t.Run("missing state file is tolerated as nothing installed", func(t *testing.T) {
		t.Parallel()

		got, err := installedFlavors(config.NewDeploymentDir(t.TempDir()))
		if err != nil {
			t.Fatalf("expected no error for a missing state file, got %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected an empty set, got %v", got)
		}
	})

	t.Run("installed flavors are reported lower-cased", func(t *testing.T) {
		t.Parallel()

		deployment := config.NewDeploymentDir(t.TempDir())
		state := &config.ExasolPersonalState{
			DeploymentVersion: "0.0.0",
			InstalledSLCs:     []config.InstalledSLC{{Flavor: "Python-3.12"}},
		}
		if err := config.WriteExasolPersonalState(state, deployment); err != nil {
			t.Fatalf("failed to write state: %v", err)
		}

		got, err := installedFlavors(deployment)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got["python-3.12"] {
			t.Fatalf("expected python-3.12 to be reported installed, got %v", got)
		}
	})

	t.Run("corrupt state file surfaces an error", func(t *testing.T) {
		t.Parallel()

		deployment := config.NewDeploymentDir(t.TempDir())
		path := deployment.ExasolPersonalStatePath()
		if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
			t.Fatalf("failed to write corrupt state: %v", err)
		}

		if _, err := installedFlavors(deployment); err == nil {
			t.Fatal("expected an error for a corrupt state file, got nil")
		}
	})
}

// requireDeploymentPresent guards SLC change operations: a deployment that has only been
// initialized (never deployed) must be rejected with ErrDeploymentNotPresent, and only a
// deployed deployment is allowed to proceed.
func TestRequireDeploymentPresent(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		state   any
		wantErr bool
	}{
		"initialized but not deployed": {state: &config.WorkflowStateInitialized{}, wantErr: true},
		"deployed and stopped":         {state: &config.WorkflowStateStopped{}, wantErr: false},
		"deployed and running":         {state: &config.WorkflowStateRunning{}, wantErr: false},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			deployment := config.NewDeploymentDir(t.TempDir())
			state := &config.ExasolPersonalState{DeploymentVersion: "0.0.0"}
			if err := state.SetWorkflowStateAndWrite(test.state, deployment); err != nil {
				t.Fatalf("failed to write launcher state: %v", err)
			}

			err := requireDeploymentPresent(deployment)
			if test.wantErr && !errors.Is(err, ErrDeploymentNotPresent) {
				t.Fatalf("expected ErrDeploymentNotPresent, got %v", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestUpsertInstalledSLCAppendsSortedAndReplacesSameFlavor(t *testing.T) {
	t.Parallel()

	existing := []config.InstalledSLC{
		{Language: "python", Flavor: "python-3.12", Image: "img:old", Aliases: []string{"PYTHON3"}},
	}

	withJava := upsertInstalledSLC(existing, config.InstalledSLC{
		Language: "java", Flavor: "java-17", Image: "img:java", Aliases: []string{"JAVA"},
	})
	if len(withJava) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(withJava))
	}
	if withJava[0].Flavor != "java-17" || withJava[1].Flavor != "python-3.12" {
		t.Errorf("expected sorted [java-17, python-3.12], got [%s, %s]",
			withJava[0].Flavor, withJava[1].Flavor)
	}

	replaced := upsertInstalledSLC(withJava, config.InstalledSLC{
		Language: "python", Flavor: "python-3.12", Image: "img:new",
		Aliases: []string{"PYTHON3", "PYTHON312"},
	})
	if len(replaced) != 2 {
		t.Fatalf("expected replace to keep 2 entries, got %d", len(replaced))
	}
	for _, entry := range replaced {
		if entry.Flavor == "python-3.12" && entry.Image != "img:new" {
			t.Errorf("expected python-3.12 image to be replaced with img:new, got %q", entry.Image)
		}
	}
}

func TestFindInstalledSLCMatchesAliasLanguageAndFlavor(t *testing.T) {
	t.Parallel()

	installed := []config.InstalledSLC{
		{Language: "python", Flavor: "python-3.12", Aliases: []string{"PYTHON3", "PYTHON312"}},
	}

	cases := map[string]int{
		"python3":     0,
		"PYTHON312":   0,
		"python":      0,
		"python-3.12": 0,
		"java":        -1,
	}
	for needle, want := range cases {
		if got := findInstalledSLC(installed, needle); got != want {
			t.Errorf("findInstalledSLC(%q) = %d, want %d", needle, got, want)
		}
	}
}

func TestUpdateSLCFindsInstalledEntryAfterFlavorChange(t *testing.T) {
	t.Parallel()

	// Given: a stopped local deployment with the previous flavor of the current Python SLC.
	catalog, err := slc.Load(resources.SLCCatalogYAML)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	current, err := catalog.Resolve("PYTHON3", runtime.GOARCH)
	if err != nil {
		t.Fatalf("resolve Python SLC: %v", err)
	}
	previous := toInstalledSLC(current)
	previous.Flavor += "-previous"
	previous.Image += "-previous"

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o750); err != nil {
		t.Fatalf("create infrastructure directory: %v", err)
	}
	if err := os.WriteFile(
		deployment.InfrastructureManifestPath(), []byte("backend: local\n"), 0o600,
	); err != nil {
		t.Fatalf("write infrastructure manifest: %v", err)
	}
	state := &config.ExasolPersonalState{
		DeploymentVersion: "0.0.0",
		InstalledSLCs:     []config.InstalledSLC{previous},
	}
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateStopped{}, deployment,
	); err != nil {
		t.Fatalf("write deployment state: %v", err)
	}

	// When: updating by the new flavor advertised by the catalog.
	result, err := UpdateSLC(
		context.Background(), deployment, current.Flavor, false, false, nil,
	)
	// Then: the previous flavor is found and replaced by the current entry.
	if err != nil {
		t.Fatalf("UpdateSLC returned an error: %v", err)
	}
	if !result.Found || !result.Changed {
		t.Fatalf("expected an installed SLC update, got %#v", result)
	}
	if result.FromFlavor != previous.Flavor || result.Entry.Flavor != current.Flavor {
		t.Fatalf("unexpected flavor transition: %#v", result)
	}
}

func TestFindInstalledByImage(t *testing.T) {
	t.Parallel()

	installed := []config.InstalledSLC{
		{Flavor: "python-3.12", Image: "docker.io/x:pytag"},
		{Flavor: "java-17", Image: "docker.io/x:javatag"},
	}

	if got := findInstalledByImage(installed, "docker.io/x:javatag"); got != 1 {
		t.Errorf("findInstalledByImage(existing) = %d, want 1", got)
	}
	if got := findInstalledByImage(installed, "docker.io/x:missing"); got != -1 {
		t.Errorf("findInstalledByImage(missing) = %d, want -1", got)
	}
}
