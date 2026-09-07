// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package slc_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/slc"
)

const testCatalog = `
registry: docker.io/exasol/script-language-container
tag_template: "standard-EXASOL-all-{flavor}-release_{arch}_{hash}"
architectures:
  arm64:
    image_architecture: arm64
    default_version: "11.2.0"
    versions:
      "11.2.0":
        languages:
          python:
            flavor: python-3.12
            hash: PYHASH
            aliases: [PYTHON3, PYTHON312]
          java:
            flavor: java-17
            hash: JAVAHASH
            aliases: [JAVA, JAVA17]
          r:
            flavor: r-4.4
            hash: RHASH
            aliases: [R, R44]
`

func mustLoad(t *testing.T) *slc.Catalog {
	t.Helper()
	catalog, err := slc.Load([]byte(testCatalog))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}

	return catalog
}

func TestResolveResolvesAliasToImageAndTarget(t *testing.T) {
	t.Parallel()

	catalog := mustLoad(t)

	entry, err := catalog.Resolve("python3", "arm64")
	if err != nil {
		t.Fatalf("Resolve returned unexpected error: %v", err)
	}

	wantImage := "docker.io/exasol/script-language-container:" +
		"standard-EXASOL-all-python-3.12-release_arm64_PYHASH"
	if entry.Image != wantImage {
		t.Errorf("Image = %q, want %q", entry.Image, wantImage)
	}
	if entry.Target != "/exa/slc/python-3.12" {
		t.Errorf("Target = %q, want %q", entry.Target, "/exa/slc/python-3.12")
	}
	if entry.Flavor != "python-3.12" {
		t.Errorf("Flavor = %q, want %q", entry.Flavor, "python-3.12")
	}
}

func TestResolveIsCaseInsensitiveAndMatchesVersionedAlias(t *testing.T) {
	t.Parallel()

	catalog := mustLoad(t)

	lower, err := catalog.Resolve("python3", "arm64")
	if err != nil {
		t.Fatalf("Resolve(python3) error: %v", err)
	}
	upper, err := catalog.Resolve("PYTHON3", "arm64")
	if err != nil {
		t.Fatalf("Resolve(PYTHON3) error: %v", err)
	}
	versioned, err := catalog.Resolve("python312", "arm64")
	if err != nil {
		t.Fatalf("Resolve(python312) error: %v", err)
	}

	if lower.Flavor != upper.Flavor || lower.Flavor != versioned.Flavor {
		t.Errorf("expected python3/PYTHON3/python312 to resolve to the same flavor, got %q/%q/%q",
			lower.Flavor, upper.Flavor, versioned.Flavor)
	}
}

func TestResolveMatchesFlavorCaseInsensitively(t *testing.T) {
	t.Parallel()

	catalog := mustLoad(t)

	entry, err := catalog.Resolve("JAVA-17", "arm64")
	if err != nil {
		t.Fatalf("Resolve(JAVA-17) error: %v", err)
	}
	if entry.Flavor != "java-17" {
		t.Errorf("Flavor = %q, want %q", entry.Flavor, "java-17")
	}
}

func TestResolveUnknownAliasListsValidAliases(t *testing.T) {
	t.Parallel()

	catalog := mustLoad(t)

	_, err := catalog.Resolve("nodejs", "arm64")

	var unknown *slc.UnknownAliasError
	if !errors.As(err, &unknown) {
		t.Fatalf("expected UnknownAliasError, got %v", err)
	}
	if len(unknown.ValidAliases) == 0 {
		t.Error("expected UnknownAliasError to list valid aliases")
	}
	if !strings.Contains(err.Error(), "unknown SLC identifier") {
		t.Errorf("expected an identifier error, got %q", err)
	}
}

func TestResolveRejectsEmptyIdentifier(t *testing.T) {
	t.Parallel()

	catalog := mustLoad(t)

	_, err := catalog.Resolve("  ", "arm64")
	if err == nil || err.Error() != "no SLC identifier provided" {
		t.Errorf("expected an empty identifier error, got %v", err)
	}
}

func TestResolveUnsupportedArchitecture(t *testing.T) {
	t.Parallel()

	catalog := mustLoad(t)

	_, err := catalog.Resolve("python3", "s390x")
	if err == nil {
		t.Fatal("expected an error resolving on an unsupported architecture")
	}
	if !errors.Is(err, slc.ErrArchitectureUnsupported) {
		t.Errorf("expected ErrArchitectureUnsupported, got %v", err)
	}
}

func TestListUnsupportedArchitectureReturnsSentinel(t *testing.T) {
	t.Parallel()

	catalog := mustLoad(t)

	_, err := catalog.List("s390x")
	if !errors.Is(err, slc.ErrArchitectureUnsupported) {
		t.Errorf("expected ErrArchitectureUnsupported, got %v", err)
	}
}

func TestListReturnsAllFlavorsSorted(t *testing.T) {
	t.Parallel()

	catalog := mustLoad(t)

	entries, err := catalog.List("arm64")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Language != "java" || entries[1].Language != "python" ||
		entries[2].Language != "r" {
		t.Errorf("expected entries sorted java,python,r; got %q,%q,%q",
			entries[0].Language, entries[1].Language, entries[2].Language)
	}
}

func TestEmbeddedCatalogResolvesPython3(t *testing.T) {
	t.Parallel()

	catalog, err := slc.Load(resources.SLCCatalogYAML)
	if err != nil {
		t.Fatalf("failed to load embedded SLC catalog: %v", err)
	}

	entry, err := catalog.Resolve("python3", "arm64")
	if err != nil {
		t.Fatalf("failed to resolve python3 against embedded catalog: %v", err)
	}
	if entry.Flavor != "python-3.12" {
		t.Errorf("embedded catalog python3 Flavor = %q, want python-3.12", entry.Flavor)
	}
}

func TestEmbeddedCatalogMapsAMD64ToX64ImageTags(t *testing.T) {
	t.Parallel()

	catalog, err := slc.Load(resources.SLCCatalogYAML)
	if err != nil {
		t.Fatalf("failed to load embedded SLC catalog: %v", err)
	}

	entry, err := catalog.Resolve("python3", "amd64")
	if err != nil {
		t.Fatalf("failed to resolve amd64 python3: %v", err)
	}
	want := "release_x64_GM7DI5JDDTJRRDVV2QPRSNDRDT5CCYHPPEZO7HPJSIX4B5ICISZQ"
	if !strings.Contains(entry.Image, want) {
		t.Errorf("amd64 image = %q, want tag containing %q", entry.Image, want)
	}
}

func TestCheckInstallableRejectsSharedAliasAcrossFlavors(t *testing.T) {
	t.Parallel()

	installed := []slc.Entry{
		{Flavor: "python-3.12", Aliases: []string{"PYTHON3", "PYTHON312"}},
	}
	candidate := slc.Entry{Flavor: "python-3.10", Aliases: []string{"PYTHON3", "PYTHON310"}}

	if _, err := slc.CheckInstallable(installed, candidate); err == nil {
		t.Error("expected collision error for shared PYTHON3 alias across flavors")
	}
}

func TestCheckInstallableRejectsSharedVersionedAlias(t *testing.T) {
	t.Parallel()

	installed := []slc.Entry{
		{Flavor: "python-3.12", Aliases: []string{"PYTHON312"}},
	}
	candidate := slc.Entry{Flavor: "other-flavor", Aliases: []string{"PYTHON312"}}

	if _, err := slc.CheckInstallable(installed, candidate); err == nil {
		t.Error("expected collision error for a shared versioned alias, not only unversioned")
	}
}

func TestCheckInstallableSameFlavorReplaces(t *testing.T) {
	t.Parallel()

	installed := []slc.Entry{
		{Flavor: "python-3.12", Aliases: []string{"PYTHON3", "PYTHON312"}},
	}
	candidate := slc.Entry{Flavor: "python-3.12", Aliases: []string{"PYTHON3", "PYTHON312"}}

	replaces, err := slc.CheckInstallable(installed, candidate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !replaces {
		t.Error("expected replacesFlavor=true when reinstalling the same flavor")
	}
}

func TestCheckInstallableDisjointIsAllowed(t *testing.T) {
	t.Parallel()

	installed := []slc.Entry{
		{Flavor: "python-3.12", Aliases: []string{"PYTHON3", "PYTHON312"}},
	}
	candidate := slc.Entry{Flavor: "java-17", Aliases: []string{"JAVA", "JAVA17"}}

	replaces, err := slc.CheckInstallable(installed, candidate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if replaces {
		t.Error("expected replacesFlavor=false for a disjoint new flavor")
	}
}
