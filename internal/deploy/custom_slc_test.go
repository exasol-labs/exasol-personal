// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/customslc"
	"github.com/exasol/exasol-personal/internal/directorymutex"
	"github.com/exasol/exasol-personal/internal/slc"
)

func TestValidateCustomAlias(t *testing.T) {
	t.Parallel()

	// Given
	for _, testCase := range []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"mypy3", "MYPY3", false},
		{" my_py3 ", "MY_PY3", false},
		{"", "", true},
		{"has space", "", true},
		{"../evil", "", true},
		{"semi;colon", "", true},
		{"123py", "", true},
		{"_foo", "", true},
		{strings.Repeat("a", 129), "", true},
		{strings.Repeat("a", 128), strings.Repeat("A", 128), false},
	} {
		// When
		got, err := validateCustomAlias(testCase.in)
		// Then
		if testCase.wantErr {
			if err == nil {
				t.Fatalf("validateCustomAlias(%q): expected error", testCase.in)
			}

			continue
		}
		if err != nil {
			t.Fatalf("validateCustomAlias(%q): unexpected error %v", testCase.in, err)
		}
		if got != testCase.want {
			t.Fatalf("validateCustomAlias(%q) = %q, want %q", testCase.in, got, testCase.want)
		}
	}
}

func TestValidateCustomSourceRequiresAValue(t *testing.T) {
	t.Parallel()

	// When / Then
	if _, err := validateCustomSource(""); err == nil {
		t.Fatal("expected an error when no source is set")
	}
	if _, err := validateCustomSource("   "); err == nil {
		t.Fatal("expected an error for a blank source")
	}
	if got, err := validateCustomSource(" a.tar.gz "); err != nil || got != "a.tar.gz" {
		t.Fatalf("expected a trimmed file source, got %q err %v", got, err)
	}
}

func TestValidateCustomSourceEnforcesHTTPSForURLs(t *testing.T) {
	t.Parallel()

	// When / Then
	if _, err := validateCustomSource("https://example.com/c.tar.gz"); err != nil {
		t.Fatalf("an https URL should be accepted, got %v", err)
	}
	if _, err := validateCustomSource("http://example.com/c.tar.gz"); err == nil {
		t.Fatal("a plaintext http URL must be rejected")
	}

	// A path is not a URL: "https:foo" has no host, so it is a file name, not a rejected URL.
	if got, err := validateCustomSource("/tmp/c.tar.gz"); err != nil || got != "/tmp/c.tar.gz" {
		t.Fatalf("expected a local path to be accepted, got %q err %v", got, err)
	}
}

func TestIsURLSourceDistinguishesPathsFromURLs(t *testing.T) {
	t.Parallel()

	// When / Then
	if !isURLSource("https://example.com/c.tar.gz") {
		t.Fatal("an https URL must be treated as a download")
	}
	for _, source := range []string{"/tmp/c.tar.gz", "c.tar.gz", "./c.tar.gz", "https:foo"} {
		if isURLSource(source) {
			t.Fatalf("%q must be treated as a local file", source)
		}
	}
}

func TestOfficialOwnerMatchesAliasCaseInsensitively(t *testing.T) {
	t.Parallel()

	// Given
	const flavor = "python-3.12"
	installed := []config.InstalledSLC{
		{Flavor: flavor, Aliases: []string{"PYTHON3", "PYTHON312"}},
	}

	// When / Then
	if got := officialOwner(installed, "python3"); got != flavor {
		t.Fatalf("expected %s, got %q", flavor, got)
	}
	if got := officialOwner(installed, "JAVA"); got != "" {
		t.Fatalf("expected no owner for JAVA, got %q", got)
	}
}

func TestCheckOfficialAliasNotHeldByCustom(t *testing.T) {
	t.Parallel()

	// Given
	customs := []config.InstalledCustomSLC{{Alias: "PYTHON3", Language: "python"}}

	// When
	err := checkOfficialAliasNotHeldByCustom(customs, []string{"PYTHON3", "PYTHON312"})

	// Then
	if err == nil {
		t.Fatal("expected a collision error when a custom SLC owns the alias")
	}
	if !strings.Contains(err.Error(), "exasol slc remove PYTHON3") {
		t.Fatalf("expected the error to guide removal, got %v", err)
	}

	if err := checkOfficialAliasNotHeldByCustom(customs, []string{"JAVA"}); err != nil {
		t.Fatalf("expected no collision for a disjoint alias, got %v", err)
	}
}

func TestCheckCustomSLCInternalAliasConflicts(t *testing.T) {
	t.Parallel()

	// Given
	state := &config.ExasolPersonalState{
		InstalledSLCs: []config.InstalledSLC{{Flavor: "python-3.12", Aliases: []string{"PYTHON3"}}},
		InstalledCustomSLCs: []config.InstalledCustomSLC{
			{Alias: "FIRST", PackageAliases: []string{"JAVA"}},
			{Alias: "OTHER", PackageAliases: []string{"RUST"}},
		},
	}
	deployment := config.NewDeploymentDir(t.TempDir())

	// When / Then
	err := checkCustomSLCInternalAliasConflicts(
		deployment, state, []string{"rust"}, -1, "install", "NEW",
	)
	if err == nil {
		t.Fatal("expected conflict with another custom SLC")
	}
	want := `cannot install custom SLC "NEW": an alias defined in this package, "RUST"`
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("unexpected conflict message %q", err)
	}
	if err := checkCustomSLCInternalAliasConflicts(
		deployment, state, []string{"rust"}, 1, "update", "OTHER",
	); err != nil {
		t.Fatalf("expected update to retain its own alias, got %v", err)
	}
	err = checkCustomSLCInternalAliasConflicts(
		deployment, state, []string{"rust"}, 0, "update", "FIRST",
	)
	if err == nil {
		t.Fatal("expected update conflict with another custom SLC")
	}
	if want := `cannot update custom SLC "FIRST"`; !strings.Contains(err.Error(), want) {
		t.Fatalf("unexpected update conflict message %q", err)
	}
	if err := checkCustomSLCInternalAliasConflicts(
		deployment, state, []string{"rust"}, 1, "install", "OTHER",
	); err != nil {
		t.Fatalf("expected install replacement to retain its own alias, got %v", err)
	}
	if err := checkCustomSLCInternalAliasConflicts(
		deployment, state, []string{"unique"}, -1, "install", "NEW",
	); err != nil {
		t.Fatalf("expected unique alias to pass, got %v", err)
	}
}

func TestCustomSLCNamesAreDerivedFromAliasAndDigest(t *testing.T) {
	t.Parallel()

	// Given
	const shaA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const shaB = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

	// When / Then
	if got := customSLCDir("MyPy3"); got != "custom-mypy3" {
		t.Fatalf("dir = %q, want custom-mypy3", got)
	}
	if got := customSLCTarget("MyPy3"); got != slc.SLCMountRoot+"/custom-mypy3" {
		t.Fatalf("target = %q, want a path under the SLC mount root", got)
	}
	if got := customSLCImage("MyPy3", shaA); got != customSLCImageRepo+":mypy3-0123456789abcdef" {
		t.Fatalf("image = %q, want the 16-char digest tag", got)
	}
	if got := customSLCPackageName("MyPy3", shaA); got != "custom-mypy3-0123456789abcdef.tar.gz" {
		t.Fatalf("package = %q, want the digest-suffixed package name", got)
	}

	if customSLCImage("MyPy3", shaA) == customSLCImage("MyPy3", shaB) {
		t.Fatal("distinct content must yield distinct image references")
	}
	if customSLCPackageName("MyPy3", shaA) == customSLCPackageName("MyPy3", shaB) {
		t.Fatal("distinct content must yield distinct package names")
	}
}

func TestCustomSLCTargetNeverCollidesWithOfficialFlavors(t *testing.T) {
	t.Parallel()

	// Given
	catalog, err := slc.Load(resources.SLCCatalogYAML)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := catalog.List(runtime.GOARCH)
	if err != nil {
		t.Skipf("no catalog entries for %s: %v", runtime.GOARCH, err)
	}

	// When / Then
	for _, entry := range entries {
		if customSLCTarget(entry.Flavor) == entry.Target {
			t.Fatalf("custom target collides with official flavor %q", entry.Flavor)
		}
	}
}

func TestCustomSLCOperationsRequireDeploymentLock(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	mutex, err := directorymutex.New(deployment.Root())
	if err != nil {
		t.Fatal(err)
	}

	opts := CustomSLCInstallOpts{Alias: "MYPY3", Language: "python", Source: "container.tar.gz"}
	ops := []struct {
		name string
		call func(context.Context) error
	}{
		{"install", func(ctx context.Context) error {
			_, err := InstallCustomSLC(ctx, deployment, opts, false, true, nil)

			return err
		}},
		{"update", func(ctx context.Context) error {
			_, err := UpdateCustomSLC(ctx, deployment, opts, false, true, nil)

			return err
		}},
		{"remove", func(ctx context.Context) error {
			_, err := RemoveCustomSLC(ctx, deployment, "MYPY3")

			return err
		}},
	}

	for _, operation := range ops {
		held := mutex.WithExclusive(context.Background(), nil, func(any) error {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			// When
			err := operation.call(ctx)
			// Then
			if !errors.Is(err, ErrDeploymentDirectoryLocked) {
				t.Fatalf("%s: expected ErrDeploymentDirectoryLocked, got %v", operation.name, err)
			}

			return nil
		})
		if held != nil {
			t.Fatalf("%s: holding the lock failed: %v", operation.name, held)
		}
	}
}

func TestAliasResolvesTo(t *testing.T) {
	t.Parallel()

	// Given
	entries := customslc.ParseScriptLanguages(
		"MYPY3=localzmq+protobuf:///a?lang=python#a PYTHON3=builtin_python3",
	)

	// When / Then
	if !aliasResolvesTo(entries, "mypy3", "localzmq+protobuf:///a?lang=python#a") {
		t.Fatal("the exact recorded URI must match")
	}
	if aliasResolvesTo(entries, "mypy3", "localzmq+protobuf:///b?lang=python#b") {
		t.Fatal("a stale URI must not be treated as a match")
	}
	if aliasResolvesTo(entries, "absent", "x") {
		t.Fatal("an absent alias must not match")
	}
}

func TestCustomSLCUnchanged(t *testing.T) {
	t.Parallel()

	// Given
	recorded := config.InstalledCustomSLC{Sha256: "abc", Language: "python", Activated: true}

	// When / Then
	if !customSLCUnchanged(recorded, "abc", customslc.Language("python")) {
		t.Fatal("same digest and language must be a no-op")
	}
	if customSLCUnchanged(recorded, "abc", customslc.Language("java")) {
		t.Fatal("a language-only change must not be treated as unchanged")
	}
	if customSLCUnchanged(recorded, "def", customslc.Language("python")) {
		t.Fatal("a content change must not be treated as unchanged")
	}

	pending := config.InstalledCustomSLC{Sha256: "abc", Language: "python"}
	if customSLCUnchanged(pending, "abc", customslc.Language("python")) {
		t.Fatal("an inactive container must be retried, not treated as unchanged")
	}
}

func TestCarriedDisplacedURIPreservesTheOriginalBuiltin(t *testing.T) {
	t.Parallel()

	// Given
	installed := []config.InstalledCustomSLC{
		{Alias: "PYTHON3", DisplacedURI: "builtin_python3"},
		{Alias: "MYPY3"},
	}

	// When / Then
	if got := carriedDisplacedURI(installed, "python3"); got != "builtin_python3" {
		t.Fatalf("replace: got %q, want the preserved builtin_python3", got)
	}
	if got := carriedDisplacedURI(installed, "MYPY3"); got != "" {
		t.Fatalf("a container that displaced nothing must carry nothing, got %q", got)
	}
	if got := carriedDisplacedURI(nil, "PYTHON3"); got != "" {
		t.Fatalf("fresh alias: got %q, want empty", got)
	}
}

func TestSupersededCustomSLCPackage(t *testing.T) {
	t.Parallel()

	// Given
	installed := []config.InstalledCustomSLC{{Alias: "MYPY3", Package: "old.tar.gz"}}

	// When / Then
	replacement := config.InstalledCustomSLC{Alias: "MYPY3", Package: "new.tar.gz"}
	if got := supersededCustomSLCPackage(installed, replacement); got != "old.tar.gz" {
		t.Fatalf("expected the replaced package, got %q", got)
	}

	same := config.InstalledCustomSLC{Alias: "MYPY3", Package: "old.tar.gz"}
	if got := supersededCustomSLCPackage(installed, same); got != "" {
		t.Fatalf("the live package must never be deleted, got %q", got)
	}

	fresh := config.InstalledCustomSLC{Alias: "MYR", Package: "r.tar.gz"}
	if got := supersededCustomSLCPackage(installed, fresh); got != "" {
		t.Fatalf("a first install supersedes nothing, got %q", got)
	}
}

func TestUpsertInstalledCustomSLCReplacesAndSorts(t *testing.T) {
	t.Parallel()

	// Given
	existing := []config.InstalledCustomSLC{{Alias: "MYPY3", Sha256: "old"}}

	// When / Then
	withR := upsertInstalledCustomSLC(
		existing,
		config.InstalledCustomSLC{Alias: "MYR", Sha256: "r"},
	)
	if len(withR) != 2 || withR[0].Alias != "MYPY3" || withR[1].Alias != "MYR" {
		t.Fatalf("expected sorted [MYPY3 MYR], got %+v", withR)
	}

	replaced := upsertInstalledCustomSLC(
		withR,
		config.InstalledCustomSLC{Alias: "MYPY3", Sha256: "new"},
	)
	if len(replaced) != 2 {
		t.Fatalf("expected replace to keep length 2, got %d", len(replaced))
	}
	if idx := findInstalledCustomSLC(replaced, "mypy3"); idx < 0 || replaced[idx].Sha256 != "new" {
		t.Fatalf("expected MYPY3 replaced with new digest, got %+v", replaced)
	}
}

func TestConfirmOfficialAliasReuseBlocksInstalledOfficial(t *testing.T) {
	t.Parallel()

	// Given
	state := &config.ExasolPersonalState{
		InstalledSLCs: []config.InstalledSLC{
			{Flavor: "python-3.12", Aliases: []string{"PYTHON3", "PYTHON312"}},
		},
	}
	confirm := func(string) (bool, error) {
		t.Fatal("confirm must not be called when an official SLC owns the alias")

		return false, nil
	}

	// When
	err := confirmOfficialAliasReuse(state, nil, "PYTHON3", confirm)
	// Then
	if err == nil || !strings.Contains(err.Error(), "python-3.12") {
		t.Fatalf("expected a block naming the official flavor, got %v", err)
	}
}

func TestConfirmOfficialAliasReuseConfirmsBuiltinOverride(t *testing.T) {
	t.Parallel()

	// Given
	entries := []customslc.AliasEntry{{Alias: "PYTHON3", URI: "builtin_python3"}}

	// When / Then
	declined := confirmOfficialAliasReuse(
		&config.ExasolPersonalState{}, entries, "PYTHON3",
		func(string) (bool, error) { return false, nil },
	)
	if !errors.Is(declined, ErrSLCOperationCancelled) {
		t.Fatalf("expected cancellation when the user declines, got %v", declined)
	}

	accepted := confirmOfficialAliasReuse(
		&config.ExasolPersonalState{}, entries, "PYTHON3",
		func(string) (bool, error) { return true, nil },
	)
	if accepted != nil {
		t.Fatalf("expected success when the user confirms, got %v", accepted)
	}
}

func TestConfirmOfficialAliasReuseAllowsFreeAlias(t *testing.T) {
	t.Parallel()

	// When
	err := confirmOfficialAliasReuse(
		&config.ExasolPersonalState{}, nil, "MYPY3",
		func(string) (bool, error) {
			t.Fatal("confirm must not be called for a non-official alias")

			return false, nil
		},
	)
	// Then
	if err != nil {
		t.Fatalf("expected nil for a free alias, got %v", err)
	}
}

func TestHashFileMatchesSHA256(t *testing.T) {
	t.Parallel()

	// Given
	path := filepath.Join(t.TempDir(), "container.bin")
	content := []byte("hello slc")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	// When
	got, err := hashFile(path)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("%x", sha256.Sum256(content)); got != want {
		t.Fatalf("hashFile = %s, want %s", got, want)
	}
}

func TestRejectNonHTTPSRedirect(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()

	// When / Then
	httpsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := rejectNonHTTPSRedirect(httpsReq, nil); err != nil {
		t.Fatalf("an https redirect should be allowed: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := rejectNonHTTPSRedirect(httpReq, nil); err == nil {
		t.Fatal("a redirect to a non-https URL must be rejected")
	}
}

func TestAcquireCustomTarballFromURLEvictsAfterCleanup(t *testing.T) {
	t.Parallel()

	// Given
	body := []byte("tarball-bytes")
	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write(body)
		}),
	)
	defer server.Close()
	deployment := config.NewDeploymentDir(t.TempDir())

	// When
	tarball, err := acquireCustomTarball(context.Background(), deployment, server.URL)
	// Then
	if err != nil {
		t.Fatal(err)
	}

	if want := fmt.Sprintf("%x", sha256.Sum256(body)); tarball.sha256 != want {
		t.Fatalf("digest = %s, want %s", tarball.sha256, want)
	}
	got, err := os.ReadFile(tarball.path)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("downloaded content mismatch: %q err %v", got, err)
	}

	tarball.cleanup()
	if _, err := os.Stat(tarball.path); !os.IsNotExist(err) {
		t.Fatalf("expected the downloaded temp file removed after cleanup, got %v", err)
	}
}

func TestAcquireCustomTarballURLFailsOnNon200(t *testing.T) {
	t.Parallel()

	// Given
	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusNotFound)
		}),
	)
	defer server.Close()

	// When
	_, err := acquireCustomTarball(
		context.Background(), config.NewDeploymentDir(t.TempDir()), server.URL,
	)
	// Then
	if err == nil {
		t.Fatal("expected an error on a non-200 response")
	}
}

func TestAcquireCustomTarballFromFileSnapshotsAndCleansUp(t *testing.T) {
	t.Parallel()

	// Given
	path := filepath.Join(t.TempDir(), "c.tar.gz")
	content := []byte("file-bytes")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	// When
	tarball, err := acquireCustomTarball(
		context.Background(), config.NewDeploymentDir(t.TempDir()), path,
	)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	if tarball.path == path {
		t.Fatal("expected the local source to be copied to a snapshot")
	}
	if want := fmt.Sprintf("%x", sha256.Sum256(content)); tarball.sha256 != want {
		t.Fatalf("digest = %s, want %s", tarball.sha256, want)
	}

	tarball.cleanup()
	if _, err := os.Stat(tarball.path); !os.IsNotExist(err) {
		t.Fatalf("expected the snapshot removed after cleanup, got %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cleanup must not delete a user-supplied source: %v", err)
	}
}

func TestReconcileCustomSLCActivationSkipsWhenNothingIsPending(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{
		DeploymentVersion: "0.0.0",
		InstalledCustomSLCs: []config.InstalledCustomSLC{
			{Alias: "MYPY3", Image: "custom:mypy3-abc", Activated: true},
		},
	}
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatal(err)
	}

	// When / Then: no connection info exists, so any database access would fail.
	if err := reconcileCustomSLCActivation(context.Background(), deployment); err != nil {
		t.Fatalf("expected a no-op, got %v", err)
	}
}

func TestReconcileCustomSLCActivationSkipsUnavailableContainers(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{
		DeploymentVersion: "0.0.0",
		InstalledCustomSLCs: []config.InstalledCustomSLC{
			{Alias: "MYPY3", Image: "custom:mypy3-abc", Language: "python"},
		},
	}
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatal(err)
	}
	writeSLCStatusReport(
		t, deployment, `{"slc":[{"image":"custom:mypy3-abc","state":"import-failed"}]}`,
	)

	// When
	err := reconcileCustomSLCActivation(context.Background(), deployment)
	// Then
	if err != nil {
		t.Fatalf("expected the unavailable container to be skipped, got %v", err)
	}

	reread, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatal(err)
	}
	if reread.InstalledCustomSLCs[0].Activated {
		t.Fatal("an unavailable container must not be recorded as activated")
	}
}

func TestAcquireCustomTarballDownloadsIntoTheStagingDirectory(t *testing.T) {
	t.Parallel()

	// Given
	body := []byte("tarball-bytes")
	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write(body)
		}),
	)
	defer server.Close()
	deployment := config.NewDeploymentDir(t.TempDir())

	// When
	tarball, err := acquireCustomTarball(context.Background(), deployment, server.URL)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	if !tarball.staged {
		t.Fatal("a downloaded container must be reported as already staged")
	}
	if dir := filepath.Dir(tarball.path); dir != customSLCStagingDir(deployment) {
		t.Fatalf("downloaded to %s, want the staging directory", dir)
	}

	// And: promoting it moves the file rather than copying it.
	if err := promoteCustomSLCPackage(deployment, tarball.path, "custom-x-abc.tar.gz"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tarball.path); !os.IsNotExist(err) {
		t.Fatalf("expected the temporary download to be moved away, got %v", err)
	}
	got, err := os.ReadFile( //nolint:gosec // test-owned path
		filepath.Join(customSLCStagingDir(deployment), "custom-x-abc.tar.gz"),
	)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("promoted content mismatch: %q err %v", got, err)
	}
}

func TestVerifyCustomSLCApplied(t *testing.T) {
	t.Parallel()

	staged := config.InstalledCustomSLC{
		Alias:    "MYPY3",
		Language: "python",
		Image:    "custom:mypy3-abc",
		Sha256:   "abc",
	}

	for _, testCase := range []struct {
		name     string
		recorded []config.InstalledCustomSLC
		wantErr  string
	}{
		{
			name:     "removed after apply",
			recorded: nil,
			wantErr:  "was removed by another operation",
		},
		{
			name: "replaced with a different digest",
			recorded: []config.InstalledCustomSLC{
				{Alias: "MYPY3", Language: "python", Sha256: "def", Activated: true},
			},
			wantErr: "was replaced by another operation",
		},
		{
			name: "same digest but a different language",
			recorded: []config.InstalledCustomSLC{
				{Alias: "MYPY3", Language: "java", Sha256: "abc", Activated: true},
			},
			wantErr: "was replaced by another operation",
		},
		{
			name: "present but inactive",
			recorded: []config.InstalledCustomSLC{
				{Alias: "MYPY3", Language: "python", Sha256: "abc"},
			},
			wantErr: "recorded but not active",
		},
		{
			name: "unchanged and active",
			recorded: []config.InstalledCustomSLC{
				{Alias: "MYPY3", Language: "python", Sha256: "abc", Activated: true},
			},
		},
	} {
		// Given
		deployment := config.NewDeploymentDir(t.TempDir())
		state := &config.ExasolPersonalState{
			DeploymentVersion:   "0.0.0",
			InstalledCustomSLCs: testCase.recorded,
		}
		if err := config.WriteExasolPersonalState(state, deployment); err != nil {
			t.Fatal(err)
		}

		// When
		err := verifyCustomSLCApplied(deployment, staged)

		// Then
		if testCase.wantErr == "" {
			if err != nil {
				t.Fatalf("%s: expected success, got %v", testCase.name, err)
			}

			continue
		}
		if err == nil {
			t.Fatalf("%s: expected an error", testCase.name)
		}
		if !strings.Contains(err.Error(), testCase.wantErr) {
			t.Fatalf("%s: expected %q, got %v", testCase.name, testCase.wantErr, err)
		}
	}
}

func TestCustomSLCUnavailableErrorUsesReadableReasons(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	entry := config.InstalledCustomSLC{Alias: "MYPY3", Image: "custom:mypy3-abc"}
	writeSLCStatusReport(
		t, deployment, `{"slc":[{"image":"custom:mypy3-abc","state":"package-missing"}]}`,
	)

	// When
	err := customSLCUnavailableError(deployment, entry)

	// Then
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "its staged container package is missing") {
		t.Fatalf("expected the readable reason, got %v", err)
	}
	if strings.Contains(err.Error(), "package-missing") {
		t.Fatalf("expected the raw state name to be hidden, got %v", err)
	}
}

func TestReconcileWarnsAboutAnActiveButUnavailableContainer(t *testing.T) {
	t.Parallel()

	// Given: an activated container whose image the runtime reports as unavailable.
	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{
		DeploymentVersion: "0.0.0",
		InstalledCustomSLCs: []config.InstalledCustomSLC{{
			Alias:     "MYPY3",
			Language:  "python",
			Image:     "custom:mypy3-abc",
			Activated: true,
		}},
	}
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatal(err)
	}
	writeSLCStatusReport(
		t, deployment, `{"slc":[{"image":"custom:mypy3-abc","state":"package-missing"}]}`,
	)

	// When / Then: no database is reachable, so a nil error proves nothing was activated and
	// the entry was reported instead.
	if err := reconcileCustomSLCActivation(context.Background(), deployment); err != nil {
		t.Fatalf("expected the unavailable container to be reported, not activated: %v", err)
	}

	reread, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatal(err)
	}
	if !reread.InstalledCustomSLCs[0].Activated {
		t.Fatal("an already-active entry must not be de-activated by the reconcile")
	}
}
