// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package localports defines errors and conservative bind-failure
// classification shared by local runtime and installation implementations.
package localports

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
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
// a bind conflict or a post-failure probe confirms that the requested port is
// unavailable. Other failures retain their original identity unchanged.
func ClassifyBindFailure(
	service string,
	port int,
	cause error,
	diagnostic string,
	available func() bool,
) error {
	if cause == nil {
		return nil
	}
	if bindConflictDiagnostic(diagnostic) || (available != nil && !available()) {
		return &UnavailableError{Service: service, Port: port, Cause: cause}
	}

	return cause
}

// IsAvailable reports whether a TCP listener can currently bind host and port.
func IsAvailable(host string, port int) bool {
	listener, err := (&net.ListenConfig{}).Listen(
		context.Background(),
		"tcp",
		net.JoinHostPort(host, strconv.Itoa(port)),
	)
	if err != nil {
		return false
	}
	_ = listener.Close()

	return true
}

func bindConflictDiagnostic(diagnostic string) bool {
	diagnostic = strings.ToLower(diagnostic)
	for _, marker := range []string{
		"address already in use",
		"port is already allocated",
		"port already in use",
		"failed to bind",
		"bind: address in use",
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
