// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/util"
	"github.com/spf13/cobra"
)

var slcInstallOpts = struct {
	AutoApprove bool
	NoRestart   bool
}{}

var slcUpdateOpts = struct {
	AutoApprove bool
	NoRestart   bool
}{}

var slcRemoveOpts = struct {
	AutoApprove bool
	NoRestart   bool
}{}

const slcOperationAbortedMessage = "Aborted; no changes were made."

const slcAutoApproveFlagName = "auto-approve"

const slcAutoApproveFlagDesc = "Do not prompt for confirmation before restarting the database"

const slcNoRestartFlagName = "no-restart"

// slcInstalledVerb opens the message for a first install, as opposed to a replacement.
const slcInstalledVerb = "Installed"

var slcCustomInstallOpts = struct {
	AutoApprove bool
	NoRestart   bool
	Source      string
	Alias       string
	Language    string
}{}

var slcCustomUpdateOpts = struct {
	AutoApprove bool
	NoRestart   bool
	Source      string
	Alias       string
	Language    string
}{}

const slcCustomSourceFlagName = "source"

const slcCmdShortDesc = "Manage script language containers (SLCs)"

const slcNoneAvailableText = "No script language containers are available for this platform."

const slcCmdLongDesc = slcCmdShortDesc + `

Script language containers provide the language runtimes used by UDFs.
Local deployments ship without any SLC installed; install one to run UDFs in that language.
`

var slcCmd = &cobra.Command{
	Use:   "slc",
	Short: slcCmdShortDesc,
	Long:  slcCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var slcInstallCmd = &cobra.Command{
	Use:   "install <alias-or-flavor>",
	Short: "Install an official script language container",
	Long: "Install an official script language container by alias or flavor.\n" +
		"Prefer aliases because they remain stable when the catalog flavor changes.\n\n" +
		"The alias `rust` is special: it is not part of the official catalogue. It installs\n" +
		"the latest release of exasol-labs/language-container-rs that matches this machine's\n" +
		"CPU architecture. To install a specific Rust build instead, use\n" +
		"`exasol slc custom install --language rust --source <path-or-url>`.\n\n" +
		"For any other user-supplied container, use `exasol slc custom install`.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		alias := args[0]
		if strings.EqualFold(alias, deploy.RustSLCAlias) {
			return runSLCInstallRust(cmd)
		}

		result, err := deploy.InstallSLC(
			cmd.Context(),
			commonFlags.Deployment(),
			alias,
			commonFlags.DeployVerbose,
			!slcInstallOpts.NoRestart,
			slcConfirmFunc(cmd, slcInstallOpts.AutoApprove, fmt.Sprintf("Installing %q", alias)),
		)
		if errors.Is(err, deploy.ErrSLCOperationCancelled) {
			addTerminalNotice(slcOperationAbortedMessage)

			return nil
		}
		if err != nil {
			return err
		}

		if commonFlags.OutputJson {
			return renderSLCCommandJSON(result)
		}

		if result.AlreadyInstalled {
			addTerminalOutput(fmt.Sprintf(
				"%s is already installed and up to date (version %s, aliases: %s). Nothing to do.",
				result.Entry.Flavor,
				result.Entry.Version,
				strings.Join(result.Entry.Aliases, ", "),
			))

			return nil
		}

		printSLCInstallResult(result)

		return nil
	},
}

var slcUpdateCmd = &cobra.Command{
	Use:   "update <alias-or-flavor>",
	Short: "Update an installed official script language container",
	Long: "Update an installed official script language container by alias or flavor.\n" +
		"Prefer aliases because they remain stable when the catalog flavor changes.\n\n" +
		"The alias `rust` is special: it re-resolves the latest release of\n" +
		"exasol-labs/language-container-rs for this machine's CPU architecture and applies it\n" +
		"if it differs from the installed container. To pin a specific Rust build, use\n" +
		"`exasol slc custom install --language rust --source <path-or-url>`.\n\n" +
		"For any other user-supplied container, use `exasol slc custom update`.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		alias := args[0]
		if strings.EqualFold(alias, deploy.RustSLCAlias) {
			return runSLCUpdateRust(cmd)
		}

		result, err := deploy.UpdateSLC(
			cmd.Context(),
			commonFlags.Deployment(),
			alias,
			commonFlags.DeployVerbose,
			!slcUpdateOpts.NoRestart,
			slcConfirmFunc(cmd, slcUpdateOpts.AutoApprove, fmt.Sprintf("Updating %q", alias)),
		)
		if errors.Is(err, deploy.ErrSLCOperationCancelled) {
			addTerminalNotice(slcOperationAbortedMessage)

			return nil
		}
		if err != nil {
			return err
		}

		if commonFlags.OutputJson {
			return renderSLCCommandJSON(result)
		}

		if !result.Found {
			addTerminalOutput(fmt.Sprintf(
				"No SLC matching %q is installed, so there is nothing to update.",
				alias,
			))
			addTerminalCallToAction(
				fmt.Sprintf("Run `exasol slc install %s` to install it.", alias),
			)

			return nil
		}
		if result.Unchanged {
			addTerminalOutput(fmt.Sprintf(
				"%s is already up to date (version %s). Nothing to do.",
				result.Entry.Flavor,
				result.Entry.Version,
			))

			return nil
		}

		printSLCUpdateResult(result)

		return nil
	},
}

var slcRemoveCmd = &cobra.Command{
	Use:   "remove <alias>",
	Short: "Remove a script language container",
	Long: "Remove an installed script language container by alias.\n\n" +
		"Handles both official and user-supplied (custom) containers.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		alias := args[0]

		isCustom, err := deploy.IsCustomSLCAlias(commonFlags.Deployment(), alias)
		if err != nil {
			return err
		}
		if isCustom {
			return runSLCCustomRemove(cmd, alias)
		}

		result, err := deploy.RemoveSLC(
			cmd.Context(),
			commonFlags.Deployment(),
			alias,
			commonFlags.DeployVerbose,
			!slcRemoveOpts.NoRestart,
			slcConfirmFunc(cmd, slcRemoveOpts.AutoApprove, fmt.Sprintf("Removing %q", alias)),
		)
		if errors.Is(err, deploy.ErrSLCOperationCancelled) {
			addTerminalNotice(slcOperationAbortedMessage)

			return nil
		}
		if err != nil {
			return err
		}

		if commonFlags.OutputJson {
			return renderSLCCommandJSON(result)
		}

		if !result.Found {
			addTerminalOutput(fmt.Sprintf(
				"No SLC matching %q is installed, so there is nothing to remove.",
				alias,
			))

			return nil
		}

		printSLCRemoveResult(result)

		return nil
	},
}

var slcCustomCmd = &cobra.Command{
	Use:   "custom",
	Short: "Manage user-supplied (custom) script language containers",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var slcCustomInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install a custom script language container",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		return runSLCCustomInstall(cmd)
	},
}

var slcCustomUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Replace an installed custom script language container",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		return runSLCCustomUpdate(cmd)
	},
}

var slcCustomRemoveCmd = &cobra.Command{
	Use:   "remove <alias>",
	Short: "Remove an installed custom script language container",
	Long: "Remove an installed custom script language container.\n\n" +
		"`exasol slc remove <alias>` removes either kind and is the preferred command.",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		return runSLCCustomRemove(cmd, args[0])
	},
}

func runSLCCustomInstall(cmd *cobra.Command) error {
	result, err := deploy.InstallCustomSLC(
		cmd.Context(),
		commonFlags.Deployment(),
		deploy.CustomSLCInstallOpts{
			Alias:    slcCustomInstallOpts.Alias,
			Language: slcCustomInstallOpts.Language,
			Source:   slcCustomInstallOpts.Source,
		},
		commonFlags.DeployVerbose,
		!slcCustomInstallOpts.NoRestart,
		customSLCConfirmFunc(cmd, slcCustomInstallOpts.AutoApprove),
	)
	if errors.Is(err, deploy.ErrSLCOperationCancelled) {
		addTerminalNotice(slcOperationAbortedMessage)

		return nil
	}
	if err != nil {
		return err
	}

	if commonFlags.OutputJson {
		return renderSLCCommandJSON(result)
	}

	if result.AlreadyInstalled {
		addTerminalOutput(
			result.Alias + " is already installed with this container. Nothing to do.",
		)

		return nil
	}

	verb := slcInstalledVerb
	if result.Replaced {
		verb = "Replaced"
	}
	addTerminalOutput(fmt.Sprintf(
		"%s custom SLC %q (language: %s).%s",
		verb, result.Alias, result.Language, customSLCOutcomeSuffix(result.Outcome),
	))

	return nil
}

func runSLCCustomUpdate(cmd *cobra.Command) error {
	result, err := deploy.UpdateCustomSLC(
		cmd.Context(),
		commonFlags.Deployment(),
		deploy.CustomSLCInstallOpts{
			Alias:    slcCustomUpdateOpts.Alias,
			Language: slcCustomUpdateOpts.Language,
			Source:   slcCustomUpdateOpts.Source,
		},
		commonFlags.DeployVerbose,
		!slcCustomUpdateOpts.NoRestart,
		customSLCConfirmFunc(cmd, slcCustomUpdateOpts.AutoApprove),
	)
	if errors.Is(err, deploy.ErrSLCOperationCancelled) {
		addTerminalNotice(slcOperationAbortedMessage)

		return nil
	}
	if err != nil {
		return err
	}

	if commonFlags.OutputJson {
		return renderSLCCommandJSON(result)
	}

	if !result.Found {
		addTerminalOutput(fmt.Sprintf(
			"No custom SLC matching %q is installed, so there is nothing to update.", result.Alias,
		))

		return nil
	}
	if result.Unchanged {
		addTerminalOutput(fmt.Sprintf(
			"Custom SLC %q is already up to date. Nothing to do.", result.Alias,
		))

		return nil
	}

	addTerminalOutput(fmt.Sprintf(
		"Updated custom SLC %q.%s", result.Alias, customSLCOutcomeSuffix(result.Outcome),
	))

	return nil
}

// runSLCInstallRust handles `exasol slc install rust`.
func runSLCInstallRust(cmd *cobra.Command) error {
	return runSLCRust(
		cmd,
		slcInstallOpts.AutoApprove,
		!slcInstallOpts.NoRestart,
		"The Rust SLC is already installed with this container. Nothing to do.",
		"Replaced",
	)
}

// runSLCUpdateRust handles `exasol slc update rust`. The Rust SLC always tracks the latest
// release, so updating it means resolving that release again and installing it; an unchanged
// container is a no-op, which is why this shares the install path instead of an update function.
func runSLCUpdateRust(cmd *cobra.Command) error {
	return runSLCRust(
		cmd,
		slcUpdateOpts.AutoApprove,
		!slcUpdateOpts.NoRestart,
		"The Rust SLC is already up to date. Nothing to do.",
		"Updated",
	)
}

// installRustSLCFn is swapped in tests so the dispatch and rendering logic below can be
// exercised without a live deployment or a network call to resolve the latest release.
var installRustSLCFn = deploy.InstallRustSLC

// runSLCRust installs the Rust SLC and reports the outcome. `rust` is not an official-catalogue
// alias, so both `slc install rust` and `slc update rust` route here; they differ only in the
// wording for an unchanged container and for a replaced one.
func runSLCRust(
	cmd *cobra.Command,
	autoApprove bool,
	restart bool,
	unchangedMessage string,
	replacedVerb string,
) error {
	result, err := installRustSLCFn(
		cmd.Context(),
		commonFlags.Deployment(),
		deploy.RustSLCInstallOpts{},
		commonFlags.DeployVerbose,
		restart,
		customSLCConfirmFunc(cmd, autoApprove),
	)

	return reportRustSLCResult(result, err, unchangedMessage, replacedVerb)
}

// reportRustSLCResult renders the outcome of a Rust SLC install/update call. Split from
// runSLCRust so this branching is unit-testable with a synthetic result, without touching the
// deploy layer.
func reportRustSLCResult(
	result *deploy.CustomSLCInstallResult,
	err error,
	unchangedMessage string,
	replacedVerb string,
) error {
	if errors.Is(err, deploy.ErrSLCOperationCancelled) {
		addTerminalNotice(slcOperationAbortedMessage)

		return nil
	}
	if err != nil {
		return err
	}

	if commonFlags.OutputJson {
		return renderSLCCommandJSON(result)
	}

	if result.AlreadyInstalled {
		addTerminalOutput(unchangedMessage)

		return nil
	}

	verb := slcInstalledVerb
	if result.Replaced {
		verb = replacedVerb
	}
	addTerminalOutput(fmt.Sprintf(
		"%s the Rust SLC under alias %q (language: %s).%s",
		verb, result.Alias, result.Language, customSLCOutcomeSuffix(result.Outcome),
	))

	return nil
}

func customSLCOutcomeSuffix(outcome deploy.SLCApplyOutcome) string {
	switch outcome {
	case deploy.SLCApplyNone:
		return ""
	case deploy.SLCApplyRestarted:
		return " Database restarted; the language is ready to use."
	case deploy.SLCApplyStarted:
		return " Database started; the language is ready to use."
	case deploy.SLCApplyDeferred:
		return " It will become available on the next start."
	default:
		return ""
	}
}

func runSLCCustomRemove(cmd *cobra.Command, alias string) error {
	result, err := deploy.RemoveCustomSLC(cmd.Context(), commonFlags.Deployment(), alias)
	if err != nil {
		return err
	}

	if commonFlags.OutputJson {
		return renderSLCCommandJSON(result)
	}

	if !result.Found {
		addTerminalOutput(fmt.Sprintf(
			"No SLC matching %q is installed, so there is nothing to remove.", alias,
		))

		return nil
	}

	addTerminalOutput(fmt.Sprintf(
		"Removed custom SLC %q. It is no longer available to new sessions.", result.Alias,
	))

	return nil
}

// Refuses non-interactively so scripts never override an SLC silently.
//
//nolint:revive // autoApprove reflects the user's flag, not internal control coupling.
func customSLCConfirmFunc(cmd *cobra.Command, autoApprove bool) deploy.CustomSLCConfirm {
	if autoApprove {
		return nil
	}

	return func(prompt string) (bool, error) {
		if !util.IsInteractiveStdin() {
			return false, errors.New(prompt + "; re-run with --auto-approve to proceed")
		}

		// stderr, so an interactive prompt never lands in the middle of --json output.
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s.\nContinue? [y/N]: ", prompt)

		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}

		return confirmYes(line), nil
	}
}

// slcConfirmFunc returns a confirmation callback for a database-restarting SLC operation.
// It returns nil when the user passed --auto-approve (pre-approved). Otherwise it warns and
// prompts interactively, and refuses non-interactively so scripts never trigger a silent
// restart.
//
//nolint:revive // autoApprove reflects the user's flag, not internal control coupling.
func slcConfirmFunc(cmd *cobra.Command, autoApprove bool, action string) deploy.ConfirmFunc {
	if autoApprove {
		return nil
	}

	return func() (bool, error) {
		if !util.IsInteractiveStdin() {
			return false, errors.New(
				"this restarts the database; re-run with --auto-approve to confirm, " +
					"or --no-restart to apply on the next start",
			)
		}

		_, _ = fmt.Fprintf(
			cmd.ErrOrStderr(),
			"%s will restart the database. Open connections will be dropped and running "+
				"statements aborted.\nContinue? [y/N]: ",
			action,
		)

		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}

		return confirmYes(line), nil
	}
}

var slcListCmd = &cobra.Command{
	Use:   commandList,
	Short: "List available script language containers and which are installed",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		statuses, err := deploy.SLCStatuses(commonFlags.Deployment())
		if err != nil {
			return err
		}
		customs, err := deploy.CustomSLCStatuses(commonFlags.Deployment())
		if err != nil {
			return err
		}

		if commonFlags.OutputJson {
			return queueSLCListJSON(statuses, customs)
		}

		renderSLCListText(statuses, customs)
		if len(statuses) > 0 {
			addTerminalCallToAction(
				"Prefer an alias from the ALIASES column when installing an SLC.",
			)
		}

		return nil
	},
}

type slcListJSONItem struct {
	Type      string   `json:"type"`
	Language  string   `json:"language"`
	Flavor    string   `json:"flavor,omitempty"`
	Version   string   `json:"version,omitempty"`
	Aliases   []string `json:"aliases,omitempty"`
	Alias     string   `json:"alias,omitempty"`
	Source    string   `json:"source,omitempty"`
	Installed bool     `json:"installed"`
	Available *bool    `json:"available,omitempty"`
}

func renderSLCListJSON(
	writer io.Writer,
	statuses []deploy.SLCStatus,
	customs []deploy.CustomSLCStatus,
) error {
	content, err := formatSLCListJSON(statuses, customs)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(writer, content)

	return err
}

func queueSLCListJSON(statuses []deploy.SLCStatus, customs []deploy.CustomSLCStatus) error {
	content, err := formatSLCListJSON(statuses, customs)
	if err != nil {
		return err
	}
	addTerminalOutput(content)

	return nil
}

func formatSLCListJSON(
	statuses []deploy.SLCStatus,
	customs []deploy.CustomSLCStatus,
) (string, error) {
	items := make([]slcListJSONItem, 0, len(statuses)+len(customs))
	for _, status := range statuses {
		items = append(items, slcListJSONItem{
			Type:      "official",
			Language:  status.Language,
			Flavor:    status.Flavor,
			Version:   status.Version,
			Aliases:   status.Aliases,
			Installed: status.Installed,
		})
	}
	for _, custom := range customs {
		available := custom.Available
		items = append(items, slcListJSONItem{
			Type:      "custom",
			Language:  custom.Language,
			Alias:     custom.Alias,
			Source:    custom.Source,
			Installed: true,
			Available: &available,
		})
	}

	content, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func renderSLCListText(statuses []deploy.SLCStatus, customs []deploy.CustomSLCStatus) {
	addTerminalOutput(formatSLCListText(statuses, customs))
}

func formatSLCListText(statuses []deploy.SLCStatus, customs []deploy.CustomSLCStatus) string {
	if len(statuses) == 0 && len(customs) == 0 {
		return slcNoneAvailableText
	}

	var builder strings.Builder
	const columnPadding = 2
	writer := tabwriter.NewWriter(&builder, 0, 0, columnPadding, ' ', 0)
	writeOfficialSLCRows(writer, statuses)
	writeCustomSLCRows(writer, customs, len(statuses) > 0)
	_ = writer.Flush()

	return strings.TrimRight(builder.String(), "\n")
}

func writeOfficialSLCRows(writer io.Writer, statuses []deploy.SLCStatus) {
	if len(statuses) == 0 {
		return
	}

	_, _ = fmt.Fprintln(writer, "ALIASES\tFLAVOR\tVERSION\tINSTALLED")
	for _, status := range statuses {
		installed := "no"
		if status.Installed {
			installed = "yes"
		}
		_, _ = fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%s\n",
			strings.Join(status.Aliases, ", "),
			status.Flavor,
			status.Version,
			installed,
		)
	}
}

//nolint:revive // afterOfficial reports what was already written, not a control flag.
func writeCustomSLCRows(
	writer io.Writer,
	customs []deploy.CustomSLCStatus,
	afterOfficial bool,
) {
	if len(customs) == 0 {
		return
	}

	if afterOfficial {
		_, _ = fmt.Fprintln(writer)
	}
	_, _ = fmt.Fprintln(writer, "CUSTOM ALIAS\tLANGUAGE\tSTATUS\tSOURCE")
	for _, custom := range customs {
		status := "pending"
		if custom.Available {
			status = "active"
		}
		_, _ = fmt.Fprintf(
			writer, "%s\t%s\t%s\t%s\n",
			custom.Alias, custom.Language, status, custom.Source,
		)
	}
}

func printSLCInstallResult(result *deploy.SLCInstallResult) {
	verb := slcInstalledVerb
	if result.Replaced {
		verb = "Updated"
	}
	message := fmt.Sprintf(
		"%s %s (aliases: %s).",
		verb,
		result.Entry.Flavor,
		strings.Join(result.Entry.Aliases, ", "),
	)

	switch result.Outcome {
	case deploy.SLCApplyNone:
	case deploy.SLCApplyRestarted:
		message += " Database restarted; the language is ready to use."
	case deploy.SLCApplyStarted:
		message += " Database started; the language is ready to use."
	case deploy.SLCApplyDeferred:
		message += " It will become available on the next start."
	default:
	}

	addTerminalOutput(message)
}

func printSLCUpdateResult(result *deploy.SLCUpdateResult) {
	message := fmt.Sprintf("Updated %s.", result.Entry.Flavor)
	if result.FromFlavor != "" && result.FromFlavor != result.Entry.Flavor {
		message = fmt.Sprintf("Updated %s to %s.", result.FromFlavor, result.Entry.Flavor)
	}

	switch result.Outcome {
	case deploy.SLCApplyNone:
	case deploy.SLCApplyRestarted:
		message += " Database restarted; the new version is ready to use."
	case deploy.SLCApplyStarted:
		message += " Database started; the new version is ready to use."
	case deploy.SLCApplyDeferred:
		message += " It will take effect on the next start."
	default:
	}

	addTerminalOutput(message)
}

func printSLCRemoveResult(result *deploy.SLCRemoveResult) {
	message := fmt.Sprintf("Removed %s.", result.Entry.Flavor)

	switch result.Outcome {
	case deploy.SLCApplyNone:
	case deploy.SLCApplyRestarted:
		message += " Database restarted; the language is no longer available."
	case deploy.SLCApplyStarted:
		message += " Database started."
	case deploy.SLCApplyDeferred:
		message += " It will no longer be loaded on the next start."
	default:
	}

	addTerminalOutput(message)
}

func renderSLCCommandJSON(value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	addTerminalOutput(string(content))

	return nil
}

// nolint: gochecknoinits
func init() {
	requireDefaultDeploymentCompatibility(slcInstallCmd)
	requireInitializedDeploymentDir(slcInstallCmd)
	registerDeploymentDirFlag(slcInstallCmd, commonFlags)
	registerOutputFlags(slcInstallCmd, commonFlags)
	registerVerboseFlag(slcInstallCmd, commonFlags)
	slcInstallCmd.Flags().BoolVar(&slcInstallOpts.AutoApprove, slcAutoApproveFlagName, false,
		slcAutoApproveFlagDesc)
	slcInstallCmd.Flags().BoolVar(&slcInstallOpts.NoRestart, slcNoRestartFlagName, false,
		"Record the SLC without restarting; it activates on the next start")

	requireDefaultDeploymentCompatibility(slcUpdateCmd)
	requireInitializedDeploymentDir(slcUpdateCmd)
	registerDeploymentDirFlag(slcUpdateCmd, commonFlags)
	registerOutputFlags(slcUpdateCmd, commonFlags)
	registerVerboseFlag(slcUpdateCmd, commonFlags)
	slcUpdateCmd.Flags().BoolVar(&slcUpdateOpts.AutoApprove, slcAutoApproveFlagName, false,
		slcAutoApproveFlagDesc)
	slcUpdateCmd.Flags().BoolVar(&slcUpdateOpts.NoRestart, slcNoRestartFlagName, false,
		"Record the update without restarting; it applies on the next start")

	requireDefaultDeploymentCompatibility(slcRemoveCmd)
	requireInitializedDeploymentDir(slcRemoveCmd)
	registerDeploymentDirFlag(slcRemoveCmd, commonFlags)
	registerOutputFlags(slcRemoveCmd, commonFlags)
	registerVerboseFlag(slcRemoveCmd, commonFlags)
	slcRemoveCmd.Flags().BoolVar(&slcRemoveOpts.AutoApprove, slcAutoApproveFlagName, false,
		slcAutoApproveFlagDesc)
	slcRemoveCmd.Flags().BoolVar(&slcRemoveOpts.NoRestart, slcNoRestartFlagName, false,
		"Record the removal without restarting; it applies on the next start")

	registerDeploymentDirFlag(slcListCmd, commonFlags)
	registerOutputFlags(slcListCmd, commonFlags)

	requireDefaultDeploymentCompatibility(slcCustomInstallCmd)
	requireInitializedDeploymentDir(slcCustomInstallCmd)
	registerDeploymentDirFlag(slcCustomInstallCmd, commonFlags)
	registerOutputFlags(slcCustomInstallCmd, commonFlags)
	registerVerboseFlag(slcCustomInstallCmd, commonFlags)
	slcCustomInstallCmd.Flags().BoolVar(&slcCustomInstallOpts.AutoApprove,
		slcAutoApproveFlagName, false,
		"Do not prompt before restarting the database, overriding a built-in alias, "+
			"or replacing a custom SLC")
	slcCustomInstallCmd.Flags().BoolVar(&slcCustomInstallOpts.NoRestart,
		slcNoRestartFlagName, false,
		"Record the custom SLC without restarting; it activates on the next start")
	slcCustomInstallCmd.Flags().StringVar(&slcCustomInstallOpts.Source,
		slcCustomSourceFlagName, "",
		"Container tarball to install: a local path or an HTTPS URL")
	slcCustomInstallCmd.Flags().StringVar(&slcCustomInstallOpts.Alias, "alias", "",
		"Alias for the custom SLC (used in CREATE <alias> SCALAR SCRIPT)")
	slcCustomInstallCmd.Flags().StringVar(&slcCustomInstallOpts.Language, "language", "",
		"Language identifier provided by the custom SLC client (normalized to lowercase)")

	requireDefaultDeploymentCompatibility(slcCustomUpdateCmd)
	requireInitializedDeploymentDir(slcCustomUpdateCmd)
	registerDeploymentDirFlag(slcCustomUpdateCmd, commonFlags)
	registerOutputFlags(slcCustomUpdateCmd, commonFlags)
	registerVerboseFlag(slcCustomUpdateCmd, commonFlags)
	slcCustomUpdateCmd.Flags().BoolVar(&slcCustomUpdateOpts.AutoApprove,
		slcAutoApproveFlagName, false, slcAutoApproveFlagDesc)
	slcCustomUpdateCmd.Flags().BoolVar(&slcCustomUpdateOpts.NoRestart,
		slcNoRestartFlagName, false,
		"Record the update without restarting; it applies on the next start")
	slcCustomUpdateCmd.Flags().StringVar(&slcCustomUpdateOpts.Source,
		slcCustomSourceFlagName, "",
		"Container tarball to update to: a local path or an HTTPS URL")
	slcCustomUpdateCmd.Flags().StringVar(&slcCustomUpdateOpts.Alias, "alias", "",
		"Alias of the custom SLC to update")
	slcCustomUpdateCmd.Flags().StringVar(&slcCustomUpdateOpts.Language, "language", "",
		"Override the custom SLC language identifier (normalized to lowercase)")

	requireDefaultDeploymentCompatibility(slcCustomRemoveCmd)
	requireInitializedDeploymentDir(slcCustomRemoveCmd)
	registerDeploymentDirFlag(slcCustomRemoveCmd, commonFlags)
	registerOutputFlags(slcCustomRemoveCmd, commonFlags)

	slcCustomCmd.AddCommand(slcCustomInstallCmd)
	slcCustomCmd.AddCommand(slcCustomUpdateCmd)
	slcCustomCmd.AddCommand(slcCustomRemoveCmd)

	slcCmd.AddCommand(slcInstallCmd)
	slcCmd.AddCommand(slcListCmd)
	slcCmd.AddCommand(slcUpdateCmd)
	slcCmd.AddCommand(slcRemoveCmd)
	slcCmd.AddCommand(slcCustomCmd)
	rootCmd.AddCommand(slcCmd)
}
