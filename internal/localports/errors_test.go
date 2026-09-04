// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localports

import (
	"errors"
	"testing"
)

func TestClassifyBindFailureUsesDiagnostics(t *testing.T) {
	t.Parallel()

	commandErr := errors.New("command failed")
	tests := []struct {
		name       string
		diagnostic string
		wantTyped  bool
	}{
		{
			name:       "address already in use",
			diagnostic: "Error: address already in use",
			wantTyped:  true,
		},
		{name: "bind address in use", diagnostic: "bind: address in use", wantTyped: true},
		{name: "port is allocated", diagnostic: "Port is already allocated", wantTyped: true},
		{name: "port already in use", diagnostic: "port already in use", wantTyped: true},
		{name: "broad bind failure", diagnostic: "failed to bind socket"},
		{name: "unrelated", diagnostic: "image is corrupt"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ClassifyBindFailure("db", 28563, commandErr, test.diagnostic)
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

func TestClassifyBindFailureKeepsNilCause(t *testing.T) {
	t.Parallel()

	if err := ClassifyBindFailure("db", 28563, nil, "address already in use"); err != nil {
		t.Fatalf("expected nil cause to remain nil, got %v", err)
	}
}
