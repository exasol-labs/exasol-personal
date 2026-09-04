// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package approval

import "testing"

func TestResolve(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		autoApprove bool
		interactive bool
		expected    Mode
	}{
		"flag set with a terminal": {
			autoApprove: true, interactive: true, expected: ModeApprove,
		},
		"flag set without a terminal": {
			autoApprove: true, interactive: false, expected: ModeApprove,
		},
		"no flag with a terminal": {
			autoApprove: false, interactive: true, expected: ModePrompt,
		},
		"no flag and no terminal": {
			autoApprove: false, interactive: false, expected: ModeNonInteractive,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// When
			mode := Resolve(testCase.autoApprove, testCase.interactive)

			// Then
			if mode != testCase.expected {
				t.Fatalf("expected %q, got %q", testCase.expected, mode)
			}
		})
	}
}
