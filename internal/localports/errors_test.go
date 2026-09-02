// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localports

import (
	"errors"
	"testing"
)

func TestClassifyBindFailureUsesDiagnosticsAndAvailability(t *testing.T) {
	t.Parallel()

	commandErr := errors.New("command failed")
	tests := []struct {
		name       string
		diagnostic string
		available  bool
		wantTyped  bool
	}{
		{
			name: "diagnostic", diagnostic: "Error: address already in use",
			available: true, wantTyped: true,
		},
		{name: "availability", diagnostic: "runner failed", available: false, wantTyped: true},
		{name: "unrelated", diagnostic: "image is corrupt", available: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ClassifyBindFailure(
				"db", 28563, commandErr, test.diagnostic, func() bool { return test.available },
			)
			unavailable, typed := AsUnavailable(err)
			if typed != test.wantTyped {
				t.Fatalf("typed=%t, want %t: %v", typed, test.wantTyped, err)
			}
			if !errors.Is(err, commandErr) {
				t.Fatalf("expected command error identity, got %v", err)
			}
			if typed && (unavailable.Service != "db" || unavailable.Port != 28563) {
				t.Fatalf("unexpected unavailable-port details: %#v", unavailable)
			}
		})
	}
}
