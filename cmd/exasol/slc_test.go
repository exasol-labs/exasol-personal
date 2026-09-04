// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/approval"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestSLCCustomSubcommandsExist(t *testing.T) {
	t.Parallel()

	// When / Then
	for _, name := range []string{"install", "update"} {
		if !hasSubcommand(slcCustomCmd, name) {
			t.Fatalf("slc custom is missing the %q subcommand", name)
		}
	}
}

func TestSLCCustomRemoveIsHiddenInFavourOfUnifiedRemove(t *testing.T) {
	t.Parallel()

	// When / Then
	if !slcCustomRemoveCmd.Hidden {
		t.Fatal("slc custom remove must be hidden in favour of the unified slc remove")
	}
}

// `slc install rust` and `slc update rust` are dispatched by alias, so they must not have grown
// custom-SLC flags: the shared command keeps one uniform contract for every alias.
func TestSLCCustomOnlyFlagsAreNotOnOfficialCommands(t *testing.T) {
	t.Parallel()

	// Given
	official := map[string]*cobra.Command{"install": slcInstallCmd, "update": slcUpdateCmd}

	// When / Then
	for _, flag := range []string{"source", "alias", "language"} {
		if slcCustomInstallCmd.Flags().Lookup(flag) == nil {
			t.Fatalf("slc custom install is missing --%s", flag)
		}
		for name, cmd := range official {
			if cmd.Flags().Lookup(flag) != nil {
				t.Fatalf("official %s must not carry --%s", name, flag)
			}
		}
	}
}

func TestSLCCustomInstallAndUpdateCarryRestartFlags(t *testing.T) {
	t.Parallel()

	// Given
	commands := map[string]*cobra.Command{
		"install": slcCustomInstallCmd,
		"update":  slcCustomUpdateCmd,
	}

	// When / Then
	for name, cmd := range commands {
		if cmd.Flags().Lookup("no-restart") == nil {
			t.Fatalf("slc custom %s is missing --no-restart", name)
		}
		// Approval is a global flag, so the command must not declare its own.
		if cmd.Flags().Lookup("auto-approve") != nil {
			t.Fatalf("slc custom %s declares its own --auto-approve", name)
		}
	}
	if slcInstallCmd.Flags().Lookup("no-restart") == nil {
		t.Fatal("official install must keep --no-restart")
	}
}

// Rust support must not widen the flag surface of the shared install/update commands, so the
// expected flag set is asserted exactly rather than only checking for known additions.
func TestSLCInstallAndUpdateFlagSurfaceIsUnchangedByRustSupport(t *testing.T) {
	t.Parallel()

	// Given: --auto-approve is global, so it must not appear in the local surface.
	want := []string{
		"no-restart", "deployment", "deployment-dir", "json", "verbose",
	}

	// When / Then
	for name, cmd := range map[string]*cobra.Command{
		"install": slcInstallCmd,
		"update":  slcUpdateCmd,
	} {
		for _, flag := range want {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("slc %s is missing --%s", name, flag)
			}
		}
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			if !slices.Contains(want, flag.Name) {
				t.Errorf("slc %s carries unexpected flag --%s", name, flag.Name)
			}
		})
	}
}

// The `rust` alias is a documented part of the shared commands, so dropping it from the help
// would silently hide the only way to discover it.
func TestSLCInstallAndUpdateDocumentTheRustAlias(t *testing.T) {
	t.Parallel()

	// When / Then
	for name, long := range map[string]string{
		"install": slcInstallCmd.Long,
		"update":  slcUpdateCmd.Long,
	} {
		for _, want := range []string{
			"`rust`",
			"exasol-labs/language-container-rs",
			"exasol slc custom install --language rust --source",
		} {
			if !strings.Contains(long, want) {
				t.Errorf("slc %s help does not mention %q", name, want)
			}
		}
	}
}

// The alias dispatch is the only thing wiring the Rust SLC into the shared commands; keep both
// entry points at the signature the RunE handlers call them with.
func TestSLCRustDispatchFunctionsAreWired(t *testing.T) {
	t.Parallel()

	// Given: the dispatch targets, typed as the RunE handlers call them.
	dispatch := map[string]func(*cobra.Command) error{
		"install": runSLCInstallRust,
		"update":  runSLCUpdateRust,
	}

	// When / Then
	for name, run := range dispatch {
		if run == nil {
			t.Errorf("slc %s has no Rust dispatch target", name)
		}
	}

	if !strings.EqualFold("rust", deploy.RustSLCAlias) {
		t.Fatalf("the documented alias `rust` does not match deploy.RustSLCAlias (%q)",
			deploy.RustSLCAlias)
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestReportRustSLCResultCancelledOperationIsNotAnError(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	// When
	err := reportRustSLCResult(nil, deploy.ErrSLCOperationCancelled, "unchanged", "Replaced")
	// Then
	if err != nil {
		t.Fatalf("expected no error for a cancelled operation, got %v", err)
	}

	stdout, stderr := renderedTerminalOutput(t)
	if stdout != "" {
		t.Fatalf("expected no stdout output, got %q", stdout)
	}
	if !strings.Contains(stderr, slcOperationAbortedMessage) {
		t.Fatalf("expected the abort notice on stderr, got %q", stderr)
	}
}

func TestReportRustSLCResultPropagatesOtherErrors(t *testing.T) {
	t.Parallel()

	// Given
	installErr := errors.New("could not reach exasol-labs/language-container-rs")

	// When
	err := reportRustSLCResult(nil, installErr, "unchanged", "Replaced")

	// Then
	if !errors.Is(err, installErr) {
		t.Fatalf("expected the install error to propagate, got %v", err)
	}
}

//nolint:paralleltest // mutates commonFlags.OutputJson and shared terminal message queues
func TestReportRustSLCResultRendersJSON(t *testing.T) {
	commonFlags.OutputJson = true
	defer func() { commonFlags.OutputJson = false }()
	resetTerminalMessages()
	defer resetTerminalMessages()

	// Given
	result := &deploy.CustomSLCInstallResult{
		Alias: "RUST", Language: "rust", Outcome: deploy.SLCApplyRestarted,
	}

	// When
	if err := reportRustSLCResult(result, nil, "unchanged", "Replaced"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then
	stdout, _ := renderedTerminalOutput(t)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(stdout), &decoded); err != nil {
		t.Fatalf("stdout is not parseable JSON: %v\n%s", err, stdout)
	}
	if decoded["alias"] != "RUST" || decoded["outcome"] != "restarted" {
		t.Fatalf("unexpected JSON contents: %v", decoded)
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestReportRustSLCResultAlreadyInstalledIsANoOp(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	// Given
	result := &deploy.CustomSLCInstallResult{AlreadyInstalled: true}

	// When
	if err := reportRustSLCResult(result, nil, "nothing changed", "Replaced"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then
	stdout, _ := renderedTerminalOutput(t)
	if stdout != "nothing changed" {
		t.Fatalf("expected the unchanged message, got %q", stdout)
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestReportRustSLCResultUsesInstalledOrReplacedVerb(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		replaced bool
		wantVerb string
	}{
		{"first install", false, "Installed"},
		{"replace", true, "Replaced"},
	} {
		resetTerminalMessages()

		// Given
		result := &deploy.CustomSLCInstallResult{
			Alias: "RUST", Language: "rust", Replaced: testCase.replaced,
		}

		// When
		if err := reportRustSLCResult(result, nil, "unchanged", "Replaced"); err != nil {
			t.Fatalf("%s: unexpected error: %v", testCase.name, err)
		}

		// Then
		stdout, _ := renderedTerminalOutput(t)
		if !strings.HasPrefix(stdout, testCase.wantVerb) {
			t.Fatalf("%s: expected output to start with %q, got %q",
				testCase.name, testCase.wantVerb, stdout)
		}
		resetTerminalMessages()
	}
}

func stubInstallRustSLC(
	t *testing.T,
	stub func(
		context.Context, config.DeploymentDir, deploy.RustSLCInstallOpts, bool, bool,
		deploy.CustomSLCConfirm,
	) (*deploy.CustomSLCInstallResult, error),
) {
	t.Helper()
	original := installRustSLCFn
	installRustSLCFn = stub
	t.Cleanup(func() { installRustSLCFn = original })
}

// renderedTerminalOutput flushes the queued terminal messages and returns stdout/stderr,
// trimmed, so tests can assert on the queued content without reimplementing the flush.
//
//nolint:revive // confusing-results: stdout/stderr, not two interchangeable values.
func renderedTerminalOutput(t *testing.T) (string, string) {
	t.Helper()

	var out, errBuf bytes.Buffer
	writeTerminalMessages(terminalConfig{stdout: &out, stderr: &errBuf, showCallsToAction: true})

	return strings.TrimRight(out.String(), "\n"), strings.TrimRight(errBuf.String(), "\n")
}

//nolint:paralleltest // mutates commonFlags and shared package-level Cobra command state
func TestSLCInstallDispatchesRustWithoutNetworkOrLiveDeployment(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	// Given: a stub that proves the dispatch reached the Rust path with the right arguments,
	// instead of the official catalog lookup.
	var gotRestart bool
	var calledWithEmptySource bool
	stubInstallRustSLC(t, func(
		_ context.Context, _ config.DeploymentDir, opts deploy.RustSLCInstallOpts,
		_ bool, restart bool, _ deploy.CustomSLCConfirm,
	) (*deploy.CustomSLCInstallResult, error) {
		gotRestart = restart
		calledWithEmptySource = opts.Source == ""

		return &deploy.CustomSLCInstallResult{Alias: "RUST", Language: "rust"}, nil
	})
	rootOpts.AutoApprove = true
	slcInstallOpts.NoRestart = false
	defer func() {
		rootOpts.AutoApprove = false
		slcInstallOpts = struct{ NoRestart bool }{}
	}()

	// When
	if err := slcInstallCmd.RunE(slcInstallCmd, []string{"rust"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then
	if !calledWithEmptySource {
		t.Fatal("expected slc install rust to resolve the latest release, not a fixed source")
	}
	if !gotRestart {
		t.Fatal("expected a restart, since --no-restart was not set")
	}
	stdout, _ := renderedTerminalOutput(t)
	if !strings.HasPrefix(stdout, "Installed") {
		t.Fatalf("expected an install message, got %q", stdout)
	}
}

//nolint:paralleltest // mutates commonFlags and shared package-level Cobra command state
func TestSLCUpdateDispatchesRustWithoutNetworkOrLiveDeployment(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	// Given
	var gotRestart bool
	stubInstallRustSLC(t, func(
		_ context.Context, _ config.DeploymentDir, _ deploy.RustSLCInstallOpts,
		_ bool, restart bool, _ deploy.CustomSLCConfirm,
	) (*deploy.CustomSLCInstallResult, error) {
		gotRestart = restart

		return &deploy.CustomSLCInstallResult{
			Alias: "RUST", Language: "rust", AlreadyInstalled: true,
		}, nil
	})
	rootOpts.AutoApprove = true
	slcUpdateOpts.NoRestart = true
	defer func() {
		rootOpts.AutoApprove = false
		slcUpdateOpts = struct{ NoRestart bool }{}
	}()

	// When
	if err := slcUpdateCmd.RunE(slcUpdateCmd, []string{"RUST"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then
	if gotRestart {
		t.Fatal("expected no restart, since --no-restart was set")
	}
	stdout, _ := renderedTerminalOutput(t)
	if stdout != "The Rust SLC is already up to date. Nothing to do." {
		t.Fatalf("expected the up-to-date message, got %q", stdout)
	}
}

func hasSubcommand(parent *cobra.Command, name string) bool {
	for _, sub := range parent.Commands() {
		if sub.Name() == name {
			return true
		}
	}

	return false
}

func TestRenderSLCListTextNoneAvailable(t *testing.T) {
	t.Parallel()

	// Given
	official := []deploy.SLCStatus{}
	custom := []deploy.CustomSLCStatus{}

	// When
	output := formatSLCListText(official, custom)

	// Then
	if strings.TrimSpace(
		output,
	) != "No script language containers are available for this platform." {
		t.Fatalf("unexpected text output: %q", output)
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestRenderSLCListTextQueuesPrimaryOutput(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	renderSLCListText([]deploy.SLCStatus{}, []deploy.CustomSLCStatus{})

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	writeTerminalMessages(terminalConfig{stdout: &stdout, stderr: &stderr, showCallsToAction: true})

	if strings.TrimSpace(stdout.String()) !=
		"No script language containers are available for this platform." {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestRenderSLCListJSONEmptyIsArray(t *testing.T) {
	t.Parallel()

	// Given
	var buf bytes.Buffer

	// When
	err := renderSLCListJSON(&buf, []deploy.SLCStatus{}, []deploy.CustomSLCStatus{})
	// Then
	if err != nil {
		t.Fatalf("expected json render to succeed, got %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Fatalf("expected empty JSON array, got %q", got)
	}
}

func TestRenderSLCListTextIncludesCustom(t *testing.T) {
	t.Parallel()

	// Given
	custom := []deploy.CustomSLCStatus{{Alias: "MYPY3", Language: "python", Source: "my.tar.gz"}}

	// When
	output := formatSLCListText([]deploy.SLCStatus{}, custom)

	// Then
	if !strings.Contains(output, "CUSTOM ALIAS") || !strings.Contains(output, "MYPY3") {
		t.Fatalf("expected custom section with the alias, got %q", output)
	}
}

func TestRenderSLCListJSONIncludesCustomType(t *testing.T) {
	t.Parallel()

	// Given
	var buf bytes.Buffer
	custom := []deploy.CustomSLCStatus{{Alias: "MYPY3", Language: "python", Source: "my.tar.gz"}}

	// When
	err := renderSLCListJSON(&buf, []deploy.SLCStatus{}, custom)
	// Then
	if err != nil {
		t.Fatalf("expected json render to succeed, got %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"type": "custom"`) || !strings.Contains(got, `"alias": "MYPY3"`) {
		t.Fatalf("expected custom-typed JSON item, got %q", got)
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestRenderSLCCommandJSONQueuesParseablePrimaryOutput(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	result := &deploy.SLCInstallResult{
		Operation: deploy.SLCOperationInstall,
		Entry: config.InstalledSLC{
			Language: "python",
			Flavor:   "python-3.12",
			Version:  "3.12",
			Image:    "docker.io/exasol/script-language-container:python-3.12",
			Target:   "/exa/slc/python-3.12",
			Aliases:  []string{"PYTHON3", "PYTHON312"},
		},
		Changed: true,
		Outcome: deploy.SLCApplyDeferred,
	}

	if err := renderSLCCommandJSON(result); err != nil {
		t.Fatalf("failed to render JSON: %v", err)
	}

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	writeTerminalMessages(terminalConfig{stdout: &stdout, stderr: &stderr, showCallsToAction: true})

	if stderr.String() != "" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not parseable JSON: %v\n%s", err, stdout.String())
	}
	if decoded["operation"] != "install" {
		t.Fatalf("unexpected operation: %v", decoded["operation"])
	}
	if decoded["outcome"] != "deferred" {
		t.Fatalf("unexpected outcome: %v", decoded["outcome"])
	}
}

func TestSLCApplyOutcomeStringReportsNoOp(t *testing.T) {
	t.Parallel()

	if got := deploy.SLCApplyNone.String(); got != "none" {
		t.Fatalf("expected no-op outcome, got %q", got)
	}
}

//nolint:paralleltest // reads package-global Cobra commands
func TestSLCMutationCommandsRegisterJSONFlag(t *testing.T) {
	// Given
	for _, cmd := range []struct {
		name string
		cmd  interface{ Flag(name string) *pflag.Flag }
	}{
		{name: "install", cmd: slcInstallCmd},
		{name: "update", cmd: slcUpdateCmd},
		{name: "remove", cmd: slcRemoveCmd},
		{name: "custom install", cmd: slcCustomInstallCmd},
		{name: "custom update", cmd: slcCustomUpdateCmd},
		{name: "custom remove", cmd: slcCustomRemoveCmd},
	} {
		// When / Then
		if cmd.cmd.Flag("json") == nil {
			t.Fatalf("expected slc %s to register --json", cmd.name)
		}
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestRenderCustomSLCCommandJSONCarriesTheContract(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	// Given
	result := &deploy.CustomSLCInstallResult{
		Operation: deploy.SLCOperationInstall,
		Alias:     "MYPY3",
		Language:  "python",
		Changed:   true,
		Outcome:   deploy.SLCApplyRestarted,
	}

	// When
	if err := renderSLCCommandJSON(result); err != nil {
		t.Fatalf("failed to render JSON: %v", err)
	}

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	writeTerminalMessages(terminalConfig{stdout: &stdout, stderr: &stderr, showCallsToAction: true})

	// Then
	if stderr.String() != "" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not parseable JSON: %v\n%s", err, stdout.String())
	}
	for key, want := range map[string]any{
		"operation": "install",
		"alias":     "MYPY3",
		"outcome":   "restarted",
	} {
		if decoded[key] != want {
			t.Fatalf("%s = %v, want %v", key, decoded[key], want)
		}
	}
}

// A prompt on stdout would corrupt --json output, so both confirmation paths must use stderr.
func TestSLCConfirmPromptsGoToStderr(t *testing.T) {
	t.Parallel()

	for name, prompt := range map[string]func(*cobra.Command) error{
		"custom": func(cmd *cobra.Command) error {
			_, err := customSLCConfirmFunc(cmd, approval.ModePrompt)("overriding a built-in alias")

			return err
		},
		"official": func(cmd *cobra.Command) error {
			_, err := slcConfirmFunc(cmd, approval.ModePrompt, "Installing")()

			return err
		},
	} {
		// Given: a command whose streams are captured, and a declining answer on stdin.
		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}
		cmd := &cobra.Command{}
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetIn(strings.NewReader("n\n"))

		// When
		_ = prompt(cmd)

		// Then
		if stdout.Len() != 0 {
			t.Fatalf("%s: prompt must not write to stdout, got %q", name, stdout.String())
		}
		if stderr.Len() == 0 {
			t.Fatalf("%s: expected the prompt on stderr", name)
		}
	}
}

// The blank line is the one thing the per-table split could have changed.
func TestFormatSLCListTextSeparatesOfficialFromCustom(t *testing.T) {
	t.Parallel()

	// Given
	official := []deploy.SLCStatus{
		{Language: "python", Flavor: "python-3.12", Version: "3.12", Aliases: []string{"PYTHON3"}},
	}
	custom := []deploy.CustomSLCStatus{{Alias: "MYPY3", Language: "python", Source: "my.tar.gz"}}

	// When
	output := formatSLCListText(official, custom)

	// Then
	if !strings.Contains(output, "FLAVOR") || !strings.Contains(output, "CUSTOM ALIAS") {
		t.Fatalf("expected both tables, got %q", output)
	}
	if !strings.Contains(output, "\n\n") {
		t.Fatalf("expected a blank line between the two tables, got %q", output)
	}

	// And: a custom-only listing must not start with a blank line.
	if got := formatSLCListText(nil, custom); strings.HasPrefix(got, "\n") {
		t.Fatalf("a custom-only listing must not be preceded by a blank line, got %q", got)
	}
}

// A restart needs confirming only when someone can answer. Both an approved
// run and one with no terminal hand over no callback at all, which is how the
// SLC operations proceed without asking.
func TestSLCConfirmFuncsRespondPerApprovalMode(t *testing.T) {
	t.Parallel()

	for name, mode := range map[string]approval.Mode{
		"approved":    approval.ModeApprove,
		"no terminal": approval.ModeNonInteractive,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if slcConfirmFunc(&cobra.Command{}, mode, "Installing") != nil {
				t.Errorf("expected no confirmation callback when %s", name)
			}
			if customSLCConfirmFunc(&cobra.Command{}, mode) != nil {
				t.Errorf("expected no custom confirmation callback when %s", name)
			}
		})
	}

	if slcConfirmFunc(&cobra.Command{}, approval.ModePrompt, "Installing") == nil {
		t.Error("expected a confirmation callback when a user can be asked")
	}
	if customSLCConfirmFunc(&cobra.Command{}, approval.ModePrompt) == nil {
		t.Error("expected a custom confirmation callback when a user can be asked")
	}

	// A nil callback grants the restart, so an unrecognised mode must refuse
	// rather than hand one back.
	bogus := approval.Mode("bogus")
	confirm := slcConfirmFunc(&cobra.Command{}, bogus, "Installing")
	if confirm == nil {
		t.Fatal("expected an unrecognised mode to withhold pre-approval")
	}
	if approved, err := confirm(); err == nil || approved {
		t.Errorf("expected an unrecognised mode to refuse, got approved=%v err=%v", approved, err)
	}

	customConfirm := customSLCConfirmFunc(&cobra.Command{}, bogus)
	if customConfirm == nil {
		t.Fatal("expected an unrecognised mode to withhold custom pre-approval")
	}
	if approved, err := customConfirm("overriding"); err == nil || approved {
		t.Errorf("expected an unrecognised custom mode to refuse, got approved=%v err=%v",
			approved, err)
	}
}
