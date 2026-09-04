// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestAutoApproveFlagReachesNestedSubcommands(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"before the subcommands": {"--auto-approve", "child", "grandchild"},
		"after the subcommands":  {"child", "grandchild", "--auto-approve"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Given
			state := &RootFlags{}
			root := &cobra.Command{Use: "exasol"}
			child := &cobra.Command{Use: "child"}
			grandchild := &cobra.Command{
				Use:  "grandchild",
				RunE: func(*cobra.Command, []string) error { return nil },
			}
			child.AddCommand(grandchild)
			root.AddCommand(child)
			registerAutoApproveFlag(root, state)

			// When
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("expected the command to accept --auto-approve: %v", err)
			}

			// Then
			if !state.AutoApprove {
				t.Error("expected --auto-approve to set the root flag state")
			}
		})
	}
}

func TestAutoApproveFlagDefaultsToRequiringApproval(t *testing.T) {
	t.Parallel()

	// Given
	state := &RootFlags{}
	root := &cobra.Command{Use: "exasol"}
	child := &cobra.Command{
		Use:  "child",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	root.AddCommand(child)
	registerAutoApproveFlag(root, state)

	// When
	root.SetArgs([]string{"child"})
	if err := root.Execute(); err != nil {
		t.Fatalf("expected the command to run: %v", err)
	}

	// Then
	if state.AutoApprove {
		t.Error("expected approval to be required when --auto-approve is absent")
	}
}

// A command declaring its own --auto-approve shadows the global flag, so the
// global one would silently stop applying to exactly the commands that ask
// for confirmation.
//
// Walking the shared command tree mutates it: cobra sorts and caches a
// command's children on first access, and reading local flag sets merges
// persistent flags into them. This test therefore stays sequential.
//
//nolint:paralleltest // Mutates the shared command tree, see above.
func TestCommandsDoNotShadowGlobalAutoApproveFlag(t *testing.T) {
	// Given
	var shadowing []string

	// When
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.LocalNonPersistentFlags().Lookup("auto-approve") != nil {
			shadowing = append(shadowing, cmd.CommandPath())
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(rootCmd)

	// Then
	if len(shadowing) > 0 {
		t.Fatalf("commands declare their own --auto-approve instead of using the "+
			"global flag: %v", shadowing)
	}
}
