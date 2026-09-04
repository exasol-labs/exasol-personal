// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/approval"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/exasol/exasol-personal/internal/util"
	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const rootCmdShortDesc = `Exasol Personal: https://github.com/exasol/exasol-personal`

// rootCmdLongDesc is built at startup (not a const) because it interpolates
// the launcher's default and named deployment directory paths using the
// current platform's real path conventions.
var rootCmdLongDesc = rootCmdShortDesc + fmt.Sprintf(`

Getting Started:
	To create and run an Exasol deployment, run "exasol install <infra preset name-or-path>".
	This single command initializes your deployment directory, prepares the selected infrastructure,
	and installs the database. It uses either a built-in infrastructure preset or a custom preset
	at a path you provide. Built-in presets are: local, aws, azure, exoscale, and stackit.

	Quick start: exasol install local
	Deployment lifecycle: install -> status -> connect -> stop -> start

	If you do not pass --deployment-dir and are not already inside a deployment directory,
	Exasol Personal uses %s. Pass --deployment-dir
	to override the active deployment directory, or pass --deployment <name> (-d <name>) to use
	%s%c<name> instead, for running more than one deployment
	side by side. --deployment-dir and --deployment cannot be used together.

	Note: Cloud presets require provider credentials in your environment.
	Use "exasol init --help", "exasol install --help", or "exasol presets list" to see the preset
	compatibility matrix.

	AI agent skills: https://github.com/exasol-labs/exasol-agent-skills`,
	defaultDeploymentDirDisplayPath(), deploymentsRootDisplayPath(), os.PathSeparator)

const (
	rootCmdExample = `  exasol install local`

	rootCmdGroupEssential = "essential"
	rootCmdGroupLifecycle = "lifecycle"
	commandInfo           = "info"
	commandList           = "list"
	logLevelInfo          = "info"
)

var logLevelMap = map[string]slog.Level{
	"debug":      slog.LevelDebug,
	logLevelInfo: slog.LevelInfo,
	"warn":       slog.LevelWarn,
	"error":      slog.LevelError,
	"":           slog.LevelInfo, // default to info
}

var ErrInvalidLogLevel = errors.New("invalid log level")

// RootFlags contains flag values that apply to every command.
//
// State is kept separate from registration, like CommonFlags, so tests can
// register against a throwaway command tree instead of the shared globals.
type RootFlags struct {
	AutoApprove bool
}

var rootOpts = &RootFlags{}

// ApprovalMode resolves the flag together with terminal availability, so each
// command can respond to "nobody can be asked" on its own terms rather than
// having it collapsed into a plain refusal here.
func (flags *RootFlags) ApprovalMode() approval.Mode {
	return approval.Resolve(flags.AutoApprove, util.IsInteractiveStdin())
}

// registerAutoApproveFlag makes approval a single cross-cutting choice rather
// than a per-command one. Commands read rootOpts.ApprovalMode() instead of
// declaring their own flag, which would shadow this one where it matters.
func registerAutoApproveFlag(root *cobra.Command, state *RootFlags) {
	root.PersistentFlags().BoolVar(
		&state.AutoApprove,
		"auto-approve",
		false,
		"Approve confirmation prompts, which a run with no terminal also does "+
			"automatically except for local runtime host preparation. Input "+
			"validation and other safety checks still apply",
	)
}

var rootCmd = &cobra.Command{
	Use:           "exasol",
	Short:         rootCmdShortDesc,
	Long:          rootCmdLongDesc,
	Example:       rootCmdExample,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		// Root-level pre-run is the single place where we enforce cross-cutting concerns.
		// Design decision: keep this centralized so individual commands don't have to
		// remember to repeat it (and so user-visible behavior stays consistent).
		//
		// Cobra only validates flag groups (e.g. MarkFlagsMutuallyExclusive) after
		// PersistentPreRunE returns, so an invalid combination would otherwise reach
		// resolution, compatibility enforcement, deployment logging, and the
		// version-update check before being rejected. Validate here first so a
		// rejected command has no side effects. Cobra's own later call becomes a
		// harmless no-op on the success path.
		if err := cmd.ValidateFlagGroups(); err != nil {
			return err
		}
		if err := setupLogging(); err != nil {
			return err
		}
		if err := resolveDeploymentDirForCommand(cmd, commonFlags); err != nil {
			return err
		}
		deployment := commonFlags.Deployment()

		// Deployment-directory compatibility is enforced centrally and only for commands
		// that declare it via annotations.
		err := enforceDeploymentDirectoryCompatibility(cmd, deployment)
		if err != nil {
			return err
		}

		if !deploymentLogSessionStartsAfterInit(cmd) {
			if err := setupDeploymentLogSession(cmd, deployment); err != nil {
				return err
			}
		}

		// Best-effort version update hint (non-blocking; terminal-only when available).
		// Design decision: never block commands on this.
		if cmd.Name() != "version" && !cmd.Hidden {
			maybeAddVersionUpdateHint(cmd, deployment)
		}

		return nil
	},
}

func setupLogging() error {
	var terminalHandler slog.Handler

	selectedLevel, ok := logLevelMap[commonFlags.LogLevel]
	if !ok {
		return fmt.Errorf("%w: \"%s\"", ErrInvalidLogLevel, commonFlags.LogLevel)
	}

	if term.IsTerminal(int(os.Stderr.Fd())) {
		levelVar := slog.LevelVar{}
		levelVar.Set(selectedLevel)
		// Design decision: when attached to a terminal, prefer human-friendly logs.
		terminalHandler = tint.NewTextHandler(os.Stderr, &tint.Options{
			Level: &levelVar, TimeFormat: time.DateTime,
		})
	} else {
		terminalHandler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: selectedLevel,
		})
	}

	slog.SetDefault(slog.New(newRoutingHandler(terminalHandler, globalDeploymentFileSink)))

	slog.Debug(
		"using log level",
		"log_level", commonFlags.LogLevel,
		"level", logLevelMap[commonFlags.LogLevel],
	)

	return nil
}

// addHelpFlag adds the help flag to the command and all its children.
func addHelpFlag(cmd *cobra.Command) {
	cmd.Flags().BoolP("help", "h", false, "Help for "+cmd.Name())

	for _, child := range cmd.Commands() {
		addHelpFlag(child)
	}
}

func Execute() error {
	resetTerminalMessages()
	registerLogLevelFlag(rootCmd, commonFlags)
	registerAutoApproveFlag(rootCmd, rootOpts)

	// One resource manager for the whole process, attached to the root
	// context so every command reaches it via cmd.Context() instead of each
	// building (and caching against) its own.
	manager, err := runtimeartifacts.NewResourceManagerWithSpec(resources.ResourcesYAML)
	if err != nil {
		return err
	}
	ctx := runtimeartifacts.NewContext(context.Background(), manager)

	// Register infrastructure variable flags only for commands that need them.
	// This must happen before Cobra parses arguments.
	if err := prepareInfrastructureVariableFlags(ctx, os.Args[1:]); err != nil {
		return err
	}
	if err := prepareInstallationVariableFlags(ctx, os.Args[1:]); err != nil {
		return err
	}

	// Record the preset selection that preset-specific help describes; scans only when
	// help is requested.
	preparePresetHelpSelection(ctx, os.Args[1:])

	// Customize usage/help formatting.
	rootCmd.SetUsageTemplate(customUsageTemplate)
	rootCmd.SetHelpTemplate(
		"{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}\n\n{{end}}{{.UsageString}}",
	)

	rootCmd.AddGroup(&cobra.Group{
		ID:    rootCmdGroupEssential,
		Title: "Essential Commands:",
	})

	rootCmd.AddGroup(&cobra.Group{
		ID:    rootCmdGroupLifecycle,
		Title: "Lifecycle Commands:",
	})

	// We add the help flag explicitly because we
	// want to have the "Usage" text be capitalized.
	addHelpFlag(rootCmd)

	err = rootCmd.ExecuteContext(ctx)
	runDeploymentLogCleanup()
	if err == nil {
		printTerminalMessages()
	}

	return err
}

func maybeAddVersionUpdateHint(cmd *cobra.Command, deployment config.DeploymentDir) {
	result, err := deploy.PerformSilentVersionCheck(
		cmd.Context(),
		deployment,
		CurrentLauncherVersion,
	)
	if err != nil {
		slog.Debug("launcher version update check failed", "error", err)
		return
	}
	if !result.Checked || !result.UpdateAvailable {
		return
	}

	addTerminalCallToAction(fmt.Sprintf(
		"A new version of Exasol Personal is available: %s. "+
			"Run `exasol version --latest` for more details.",
		result.LatestVersion,
	))
}
