// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const (
	configCmdShortDesc = `Manage deployment configuration`
	configCmdLongDesc  = configCmdShortDesc + `

	Inspect, patch, or reset configuration parameters for the presets already
	initialized in the deployment directory. Configuration changes preserve local
	infrastructure state. Run exasol deploy after changing configuration to apply
	the changes to the deployment.
`
)

var configResetAll bool

var configCmd = &cobra.Command{
	Use:     "config",
	Short:   configCmdShortDesc,
	Long:    configCmdLongDesc,
	GroupID: rootCmdGroupEssential,
}

var configGetCmd = &cobra.Command{
	Use:   "get [option-name...]",
	Short: "Show active deployment configuration",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		values, err := deploy.GetDeploymentConfiguration(
			cmd.Context(),
			commonFlags.Deployment(),
			args,
		)
		if err != nil {
			return fmt.Errorf("failed to read configuration: %w", err)
		}

		if commonFlags.OutputJson {
			return printConfigurationJSON(values)
		}
		addTerminalOutput(formatConfigurationValues(values))

		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Update deployment configuration options",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		infraVars := collectInfrastructureVariableOverrides(cmd)
		installVars := collectInstallationVariableOverrides(cmd)

		values, err := deploy.SetDeploymentConfiguration(
			cmd.Context(),
			infraVars,
			installVars,
			commonFlags.Deployment(),
		)
		if err != nil {
			return fmt.Errorf("configuration failed: %w", err)
		}

		addConfigurationChangedOutput(values)

		return nil
	},
}

var configResetCmd = &cobra.Command{
	Use:   "reset [option-name...]",
	Short: "Reset deployment configuration options to defaults",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		values, err := deploy.ResetDeploymentConfiguration(
			cmd.Context(),
			commonFlags.Deployment(),
			args,
			configResetAll,
		)
		if err != nil {
			return fmt.Errorf("configuration reset failed: %w", err)
		}

		addConfigurationChangedOutput(values)

		return nil
	},
}

// nolint: gochecknoinits
func init() {
	for _, cmd := range []*cobra.Command{configCmd, configGetCmd, configSetCmd, configResetCmd} {
		requireDefaultDeploymentCompatibility(cmd)
		requireInitializedDeploymentDir(cmd)
		requireDeploymentFileLogging(cmd)
	}
	registerDeploymentDirFlag(configGetCmd, commonFlags)
	registerDeploymentDirFlag(configSetCmd, commonFlags)
	registerDeploymentDirFlag(configResetCmd, commonFlags)

	registerOutputFlags(configGetCmd, commonFlags)
	configResetCmd.Flags().BoolVar(&configResetAll, "all", false, "Reset all options to defaults")

	configCmd.AddCommand(configGetCmd, configSetCmd, configResetCmd)
	rootCmd.AddCommand(configCmd)
}

func printConfigurationJSON(configuration deploy.DeploymentConfiguration) error {
	return addJSONTerminalOutput(configurationJSON(configuration))
}

func configurationJSON(configuration deploy.DeploymentConfiguration) map[string]any {
	return map[string]any{
		deploy.ConfigScopeInfrastructure: configurationSectionJSON(configuration.Infrastructure),
		deploy.ConfigScopeInstallation:   configurationSectionJSON(configuration.Installation),
	}
}

func configurationSectionJSON(section deploy.DeploymentConfigurationSection) map[string]any {
	return map[string]any{
		"identity": presetIdentityJSON(section.Identity),
		"options":  configurationOptionsJSON(section.Options),
	}
}

func presetIdentityJSON(identity deploy.PresetIdentityInfo) map[string]string {
	result := map[string]string{}
	if strings.TrimSpace(identity.Selector) != "" {
		result["selector"] = identity.Selector
	}
	if strings.TrimSpace(identity.Kind) != "" {
		result["kind"] = identity.Kind
	}
	if strings.TrimSpace(identity.Name) != "" {
		result["name"] = identity.Name
	}
	if strings.TrimSpace(identity.Path) != "" {
		result["path"] = identity.Path
	}
	if strings.TrimSpace(identity.DisplayName) != "" {
		result["displayName"] = identity.DisplayName
	}
	if strings.TrimSpace(identity.Description) != "" {
		result["description"] = identity.Description
	}

	return result
}

func configurationOptionsJSON(values []deploy.DeploymentConfigValue) map[string]any {
	result := map[string]any{}
	for _, value := range values {
		result[value.DisplayName()] = value.Value
	}

	return result
}

const configurationApplyGuidance = "configuration updated locally; " +
	"run `exasol deploy` to apply these changes"

const configurationRestartGuidance = "configuration updated locally; " +
	"run `exasol start` to apply these changes"

func addConfigurationChangedOutput(configuration deploy.DeploymentConfiguration) {
	addTerminalOutput(formatConfigurationValues(configuration))
	guidance := configurationApplyGuidance
	if configuration.ApplyCommand == "start" {
		guidance = configurationRestartGuidance
	}
	addTerminalCallToAction(guidance)
}

func formatConfigurationValues(configuration deploy.DeploymentConfiguration) string {
	if len(configuration.Infrastructure.Options) == 0 &&
		len(configuration.Installation.Options) == 0 &&
		strings.TrimSpace(configuration.Infrastructure.Identity.Selector) == "" &&
		strings.TrimSpace(configuration.Installation.Identity.Selector) == "" {
		return "Active configuration:\n  (no configurable options)\n"
	}

	var builder strings.Builder
	_, _ = builder.WriteString("Active configuration:\n")
	writeConfigurationSection(
		&builder,
		"Infrastructure",
		configuration.Infrastructure,
	)
	writeConfigurationSection(
		&builder,
		"Installation",
		configuration.Installation,
	)

	return builder.String()
}

func writeConfigurationSection(
	builder *strings.Builder,
	title string,
	section deploy.DeploymentConfigurationSection,
) {
	_, _ = builder.WriteString("  ")
	_, _ = builder.WriteString(title)
	if strings.TrimSpace(section.Identity.DisplayName) != "" {
		_, _ = builder.WriteString(" (")
		_, _ = builder.WriteString(section.Identity.DisplayName)
		_, _ = builder.WriteString(")")
	}
	_, _ = builder.WriteString(":\n")
	writeIdentityLine(builder, "Identity", section.Identity.Selector)
	writeIdentityLine(builder, "Description", section.Identity.Description)
	_, _ = builder.WriteString("    Options:\n")
	if len(section.Options) == 0 {
		_, _ = builder.WriteString("      (no configurable options)\n")

		return
	}
	for _, value := range section.Options {
		_, _ = builder.WriteString("      ")
		_, _ = builder.WriteString(value.DisplayName())
		_, _ = builder.WriteString(" = ")
		_, _ = builder.WriteString(formatConfigurationScalar(value.Value))
		_, _ = builder.WriteString("\n")
	}
}

func writeIdentityLine(builder *strings.Builder, label string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	_, _ = builder.WriteString("    ")
	_, _ = builder.WriteString(label)
	_, _ = builder.WriteString(": ")
	_, _ = builder.WriteString(value)
	_, _ = builder.WriteString("\n")
}

func formatConfigurationScalar(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		if value == nil {
			return ""
		}

		return fmt.Sprint(value)
	}
}

func hasConfigurationVariableOverrides(
	infraVars, installVars map[string]string,
) bool {
	return len(infraVars) > 0 || len(installVars) > 0
}

func applyConfigurationPatchIfProvided(
	cmd *cobra.Command,
	infraVars map[string]string,
	installVars map[string]string,
) (deploy.DeploymentConfiguration, bool, error) {
	if !hasConfigurationVariableOverrides(infraVars, installVars) {
		return deploy.DeploymentConfiguration{}, false, nil
	}

	configuration, err := deploy.SetDeploymentConfiguration(
		cmd.Context(),
		infraVars,
		installVars,
		commonFlags.Deployment(),
	)
	if err != nil {
		return deploy.DeploymentConfiguration{}, false, fmt.Errorf("configuration failed: %w", err)
	}

	return configuration, true, nil
}
