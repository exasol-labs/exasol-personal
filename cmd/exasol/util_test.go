// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/approval"
	"github.com/exasol/exasol-personal/internal/deploy"
)

func TestLooksLikePathPresetArg(t *testing.T) {
	t.Parallel()

	cases := []struct {
		arg      string
		wantPath bool
	}{
		{"aws", false},
		{"ubuntu", false},
		{"my-preset", false},
		{"./local", true},
		{"/abs/path", true},
		{"~/home", true},
		{`C:\Windows\path`, true},
	}
	for _, tc := range cases {
		got := looksLikePathPresetArg(tc.arg)
		if got != tc.wantPath {
			t.Errorf("looksLikePathPresetArg(%q) = %v, want %v", tc.arg, got, tc.wantPath)
		}
	}
}

func TestLooksLikeExternalPresetURI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		arg  string
		want bool
	}{
		{"file:///path", true},
		{"https://example.com/repo.git", true},
		{"http://example.com/repo.git", true},
		{"git://example.com/repo.git", true},
		{"git@github.com:org/repo.git", true},
		{"aws", false},
		{"./local", false},
		{"/abs/path", false},
		{"ubuntu", false},
	}
	for _, tc := range cases {
		got := deploy.IsExternalPresetURI(tc.arg)
		if got != tc.want {
			t.Errorf("IsExternalPresetURI(%q) = %v, want %v", tc.arg, got, tc.want)
		}
	}
}

// A run with no terminal proceeds without asking, so an unattended caller is
// never blocked on a prompt nobody can answer. Only a reachable user can
// decline.
func TestConfirmActionPerApprovalMode(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		mode         approval.Mode
		answer       bool
		expectAsked  bool
		expectGo     bool
		expectErrMsg string
	}{
		"pre-approved does not ask": {
			mode: approval.ModeApprove, expectGo: true,
		},
		"reachable user accepts": {
			mode: approval.ModePrompt, answer: true, expectAsked: true, expectGo: true,
		},
		"reachable user declines without an error": {
			mode: approval.ModePrompt, answer: false, expectAsked: true,
		},
		"no terminal proceeds without asking": {
			mode: approval.ModeNonInteractive, expectGo: true,
		},
		"unrecognised mode is an error": {
			mode: approval.Mode("bogus"), expectErrMsg: "unrecognised approval mode",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Given
			asked := false
			ask := func() bool {
				asked = true

				return testCase.answer
			}

			// When
			proceed, err := confirmAction(testCase.mode, ask)

			// Then
			if testCase.expectErrMsg == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected an error mentioning %q", testCase.expectErrMsg)
				}
				if !strings.Contains(err.Error(), testCase.expectErrMsg) {
					t.Fatalf("expected %q in error, got %v", testCase.expectErrMsg, err)
				}
			}
			if proceed != testCase.expectGo {
				t.Errorf("expected proceed=%v, got %v", testCase.expectGo, proceed)
			}
			if asked != testCase.expectAsked {
				t.Errorf("expected asked=%v, got %v", testCase.expectAsked, asked)
			}
		})
	}
}
