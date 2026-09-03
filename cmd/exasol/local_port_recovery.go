// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"

	"github.com/exasol/exasol-personal/internal/deploy"
)

func addLocalPortRecoveryCallToAction(err error) {
	var recovery *deploy.LocalPortRecoveryError
	if !errors.As(err, &recovery) {
		return
	}

	addTerminalCallToAction(fmt.Sprintf(
		"Select a replacement port for local service %q, then retry:\n"+
			"  exasol config set --ports %s:<available-port>\n"+
			"  exasol config set --ports auto",
		recovery.Service,
		recovery.Service,
	))
}
