// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/exasol/exasol-personal/internal/approval"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

// UserConfirmationValidator is a function that takes an user input as a String,
// and returns if its a possitive confirmation from the user.
type UserConfirmationValidator func(input string) bool

// confirmYes is an implementation of the UserConfirmationValidator type that simply returns true
// if the user answered y or yes.
func confirmYes(input string) bool {
	input = strings.ToLower(strings.TrimSpace(input))
	return input == "y" || input == "yes"
}

// confirmAction applies the approval mode to an action that needs confirmation.
// A run with no terminal proceeds as though approval had been granted, so it
// never blocks on a prompt nobody can answer. Host preparation is the
// exception and refuses instead, because it mutates state shared beyond the
// deployment.
func confirmAction(mode approval.Mode, ask func() bool) (bool, error) {
	switch mode {
	case approval.ModeApprove, approval.ModeNonInteractive:
		return true, nil
	case approval.ModePrompt:
		return ask(), nil
	default:
		return false, fmt.Errorf("unrecognised approval mode %q", mode)
	}
}

// askForUserConfirmation asks the user for confirmation via stderr, and uses validators
// (Currently only accounting the first one on the variadic) to
// check if the confirmation is positive. If prompt is an empty string, a default prompt
// is emitted.
func askForUserConfirmation(prompt string, validators ...UserConfirmationValidator) bool {
	validate := confirmYes
	if len(validators) > 0 {
		validate = validators[0]
	}
	if prompt == "" {
		prompt = "Continue? [y/N]"
	}

	reader := bufio.NewReader(os.Stdin)
	_, _ = fmt.Fprintf(os.Stderr, "%s: ", prompt)

	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))

	return validate(response)
}

// Note: orderedInfrastructureVariables and the custom help wrapper were removed when
// infrastructure variables became regular Cobra flags.

func looksLikePathPresetArg(arg string) bool {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return false
	}
	// We treat anything path-like as a filesystem path to avoid ambiguity when a folder
	// named like an embedded preset happens to exist in the current working directory.
	//
	// Users can always force "name" selection by passing a plain identifier like "aws".
	return strings.HasPrefix(arg, ".") || strings.HasPrefix(arg, "~") ||
		strings.ContainsAny(arg, `/\\`)
}

// resolvePresetRef resolves a preset argument to a PresetRef.
// Plain names (no path separators or URI scheme) are returned as embedded preset
// names. Everything else is resolved as a runtime artifact.
func resolvePresetRef(
	ctx context.Context,
	arg string,
	presetType string,
) (deploy.PresetRef, error) {
	arg = strings.TrimSpace(arg)
	if !deploy.IsExternalPresetURI(arg) && !looksLikePathPresetArg(arg) {
		return deploy.PresetRef{Name: arg}, nil
	}

	manager := runtimeartifacts.FromContext(ctx)
	resolvedPath, err := deploy.ResolvePreset(ctx, manager, arg, presetType)
	if err != nil {
		return deploy.PresetRef{}, err
	}

	return deploy.PresetRef{Path: resolvedPath}, nil
}

func resolveInstallationPresetRef(
	ctx context.Context,
	args []string,
	index int,
	infrastructurePreset deploy.PresetRef,
) (deploy.PresetRef, error) {
	if index >= 0 && index < len(args) && strings.TrimSpace(args[index]) != "" {
		return resolvePresetRef(ctx, args[index], presets.PresetTypeInstallation)
	}

	return deploy.ResolveDefaultInstallationPreset(ctx, infrastructurePreset)
}

func presetNamesForHelp(listname string, names []string) string {
	namesList := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		namesList = append(namesList, name)
	}
	slices.Sort(namesList)
	if len(namesList) == 0 {
		return "(none)"
	}

	return "Available " + listname + " presets: " + strings.Join(namesList, ", ")
}
