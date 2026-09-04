// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package localports defines errors and bind-failure classification shared by
// local runtime and installation implementations.
package localports

import (
	"errors"
	"fmt"
	"strings"
)

// UnavailableError reports that a runtime could not bind one configured local
// service port. Cause remains available through errors.Is/errors.As.
type UnavailableError struct {
	Service string
	Port    int
	Cause   error
}

func (err *UnavailableError) Error() string {
	return fmt.Sprintf(
		"local service %q cannot bind configured host port %d: %v",
		err.Service,
		err.Port,
		err.Cause,
	)
}

func (err *UnavailableError) Unwrap() error { return err.Cause }

// ClassifyBindFailure returns a typed error only when command diagnostics name
// a bind conflict. Other failures retain their original identity unchanged.
func ClassifyBindFailure(
	service string,
	port int,
	cause error,
	diagnostic string,
) error {
	if cause == nil {
		return nil
	}
	if bindConflictDiagnostic(diagnostic) {
		return &UnavailableError{Service: service, Port: port, Cause: cause}
	}

	return cause
}

func bindConflictDiagnostic(diagnostic string) bool {
	diagnostic = strings.ToLower(diagnostic)
	for _, marker := range []string{
		"address already in use",
		"bind: address in use",
		"port is already allocated",
		"port already in use",
	} {
		if strings.Contains(diagnostic, marker) {
			return true
		}
	}

	return false
}

// AsUnavailable extracts an unavailable-port error from an error chain.
func AsUnavailable(err error) (*UnavailableError, bool) {
	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) {
		return nil, false
	}

	return unavailable, true
}
