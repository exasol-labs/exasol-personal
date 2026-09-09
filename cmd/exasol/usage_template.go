// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// nolint: gochecknoinits
func init() {
	cobra.AddTemplateFunc("hasInfrastructureVariableFlags", hasInfrastructureVariableFlags)
	cobra.AddTemplateFunc("infrastructureVariableFlagsTitle", infrastructureVariableFlagsTitle)
	cobra.AddTemplateFunc("infrastructureVariableFlagUsages", infrastructureVariableFlagUsages)
	cobra.AddTemplateFunc("hasInstallationVariableFlags", hasInstallationVariableFlags)
	cobra.AddTemplateFunc("installationVariableFlagsTitle", installationVariableFlagsTitle)
	cobra.AddTemplateFunc("installationVariableFlagUsages", installationVariableFlagUsages)
	cobra.AddTemplateFunc("hasBackendOptionFlags", hasBackendOptionFlags)
	cobra.AddTemplateFunc("backendOptionFlagsTitle", backendOptionFlagsTitle)
	cobra.AddTemplateFunc("backendOptionFlagUsages", backendOptionFlagUsages)
	cobra.AddTemplateFunc("hasGlobalFlags", hasGlobalFlags)
	cobra.AddTemplateFunc("globalFlagUsages", globalFlagUsages)
	cobra.AddTemplateFunc("commandsInGroup", commandsInGroup)
	cobra.AddTemplateFunc("ungroupedCommands", ungroupedCommands)
	cobra.AddTemplateFunc("hasOtherLocalFlags", hasOtherLocalFlags)
	cobra.AddTemplateFunc("otherLocalFlagUsages", otherLocalFlagUsages)
}

func hasGlobalFlags(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	// Show inherited persistent flags (for subcommands) plus the local --help flag.
	if cmd.HasAvailableInheritedFlags() {
		return true
	}

	return cmd.Flags().Lookup("help") != nil
}

func globalFlagUsages(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}

	flagset := pflag.NewFlagSet("global", pflag.ContinueOnError)

	cmd.InheritedFlags().VisitAll(func(flag *pflag.Flag) {
		if flag == nil {
			return
		}
		flagset.AddFlag(flag)
	})

	if helpFlag := cmd.Flags().Lookup("help"); helpFlag != nil {
		flagset.AddFlag(helpFlag)
	}

	return strings.TrimRight(flagset.FlagUsages(), "\n")
}

func isInfrastructureVariableFlagName(flagName string) bool {
	if strings.TrimSpace(flagName) == "" {
		return false
	}
	_, ok := infraFlagToVarName[flagName]

	return ok
}

func isInstallationVariableFlagName(flagName string) bool {
	if strings.TrimSpace(flagName) == "" {
		return false
	}
	_, ok := installFlagToVarName[flagName]

	return ok
}

func hasInfrastructureVariableFlags(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	// Only consider non-persistent local flags so we don't accidentally pull in
	// root persistent flags.
	for flagName := range infraFlagToVarName {
		if cmd.LocalNonPersistentFlags().Lookup(flagName) != nil {
			return true
		}
	}

	return false
}

func infrastructureVariableFlagsTitle(cmd *cobra.Command) string {
	label := ""
	if cmd != nil && cmd.Annotations != nil {
		label = strings.TrimSpace(cmd.Annotations[infraPresetLabelAnnotationKey])
	}
	if label == "" {
		return "Infrastructure variable flags:"
	}

	return fmt.Sprintf("Infrastructure variable flags of preset `%s`:", label)
}

func infrastructureVariableFlagUsages(cmd *cobra.Command) string {
	flagset := pflag.NewFlagSet("infrastructure-variables", pflag.ContinueOnError)
	for flagName := range infraFlagToVarName {
		if f := cmd.LocalNonPersistentFlags().Lookup(flagName); f != nil {
			flagset.AddFlag(f)
		}
	}

	return strings.TrimRight(flagset.FlagUsages(), "\n")
}

func hasInstallationVariableFlags(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	for flagName := range installFlagToVarName {
		if cmd.LocalNonPersistentFlags().Lookup(flagName) != nil {
			return true
		}
	}

	return false
}

func installationVariableFlagsTitle(cmd *cobra.Command) string {
	label := ""
	if cmd != nil && cmd.Annotations != nil {
		label = strings.TrimSpace(cmd.Annotations[installPresetLabelAnnotationKey])
	}
	if label == "" {
		return "Installation variable flags:"
	}

	return fmt.Sprintf("Installation variable flags of preset `%s`:", label)
}

func installationVariableFlagUsages(cmd *cobra.Command) string {
	flagset := pflag.NewFlagSet("installation-variables", pflag.ContinueOnError)
	for flagName := range installFlagToVarName {
		if f := cmd.LocalNonPersistentFlags().Lookup(flagName); f != nil {
			flagset.AddFlag(f)
		}
	}

	return strings.TrimRight(flagset.FlagUsages(), "\n")
}

func hasBackendOptionFlags(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	for _, flagName := range backendOptionFlagNames(cmd) {
		if flag := cmd.LocalNonPersistentFlags().Lookup(flagName); flag != nil && !flag.Hidden {
			return true
		}
	}

	return false
}

func backendOptionFlagsTitle(cmd *cobra.Command) string {
	label := ""
	if cmd != nil && cmd.Annotations != nil {
		label = strings.TrimSpace(cmd.Annotations[backendOptionPresetLabelAnnotationKey])
	}
	if label == "" {
		return "Backend option flags:"
	}

	return fmt.Sprintf("Backend option flags of preset `%s`:", label)
}

func backendOptionFlagUsages(cmd *cobra.Command) string {
	flagset := pflag.NewFlagSet("backend-options", pflag.ContinueOnError)
	for _, flagName := range backendOptionFlagNames(cmd) {
		if f := cmd.LocalNonPersistentFlags().Lookup(flagName); f != nil {
			flagset.AddFlag(f)
		}
	}

	return strings.TrimRight(flagset.FlagUsages(), "\n")
}

func commandsInGroup(cmd *cobra.Command, groupID string) []*cobra.Command {
	if cmd == nil {
		return nil
	}

	cmds := make([]*cobra.Command, 0, len(cmd.Commands()))
	for _, cmd := range cmd.Commands() {
		if cmd == nil {
			continue
		}
		if !cmd.IsAvailableCommand() || cmd.IsAdditionalHelpTopicCommand() {
			continue
		}
		if cmd.GroupID != groupID {
			continue
		}
		cmds = append(cmds, cmd)
	}

	return cmds
}

func ungroupedCommands(cmd *cobra.Command) []*cobra.Command {
	if cmd == nil {
		return nil
	}

	cmds := make([]*cobra.Command, 0, len(cmd.Commands()))
	for _, cmd := range cmd.Commands() {
		if cmd == nil {
			continue
		}
		if !cmd.IsAvailableCommand() || cmd.IsAdditionalHelpTopicCommand() {
			continue
		}
		if cmd.GroupID != "" {
			continue
		}
		cmds = append(cmds, cmd)
	}

	return cmds
}

func hasOtherLocalFlags(cmd *cobra.Command) bool {
	for _, f := range otherLocalFlags(cmd) {
		if f != nil {
			return true
		}
	}

	return false
}

func otherLocalFlagUsages(cmd *cobra.Command) string {
	fs := pflag.NewFlagSet("other", pflag.ContinueOnError)
	for _, f := range otherLocalFlags(cmd) {
		fs.AddFlag(f)
	}

	return strings.TrimRight(fs.FlagUsages(), "\n")
}

func otherLocalFlags(cmd *cobra.Command) []*pflag.Flag {
	flags := []*pflag.Flag{}
	// Use LocalNonPersistentFlags so we don't include persistent ("global") flags
	// such as --log-level in the root command's local flags.
	cmd.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
		if flag == nil {
			return
		}
		if flag.Name == "help" {
			// Render --help under Global Flags.
			return
		}
		if isInfrastructureVariableFlagName(flag.Name) {
			return
		}
		if isInstallationVariableFlagName(flag.Name) {
			return
		}
		if isBackendOptionFlagName(cmd, flag.Name) {
			return
		}
		flags = append(flags, flag)
	})

	return flags
}

// nolint: revive, lll
const customUsageTemplate = `Usage:
	{{if .HasAvailableSubCommands}}{{.CommandPath}} [command] [flags]{{else}}{{.UseLine}}{{end}}

{{ if .HasAvailableSubCommands }}

{{- $cmd := . }}
{{- $np := .NamePadding }}
{{- range $g := $cmd.Groups }}
{{- $cmds := commandsInGroup $cmd $g.ID }}
{{- if gt (len $cmds) 0 }}{{$g.Title}}
{{- range $cmds }}
	{{rpad .Name $np }} {{.Short}}
{{- end }}
{{ "\n" }}

{{- end }}
{{- end }}

{{- $other := ungroupedCommands $cmd }}
{{- if gt (len $other) 0 }}Additional Commands:
{{- range $other }}
	{{rpad .Name $np }} {{.Short}}
{{- end }}
{{ "\n" }}

{{- end }}
{{- end }}

{{- if hasGlobalFlags .}}Global Flags:
{{globalFlagUsages . | trimTrailingWhitespaces}}

{{end}}{{if hasInfrastructureVariableFlags .}}{{infrastructureVariableFlagsTitle .}}
{{infrastructureVariableFlagUsages . | trimTrailingWhitespaces}}

{{end}}{{if hasInstallationVariableFlags .}}{{installationVariableFlagsTitle .}}
{{installationVariableFlagUsages . | trimTrailingWhitespaces}}

{{end}}{{if hasBackendOptionFlags .}}{{backendOptionFlagsTitle .}}
{{backendOptionFlagUsages . | trimTrailingWhitespaces}}

{{end}}{{if hasOtherLocalFlags .}}Flags:
{{otherLocalFlagUsages . | trimTrailingWhitespaces}}

{{end}}{{if .HasExample}}Examples:
{{.Example}}

{{end}}{{if .HasAvailableSubCommands}}Use "{{.CommandPath}} [command] --help" for more information about a command.
{{end}}`
