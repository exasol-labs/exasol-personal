// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"

	"github.com/exasol/exasol-personal/internal/deploy"
)

// localRetryGuidance is offered whenever a local lifecycle operation fails and
// leaves a deployment that can be retried, on every platform and whether or not
// the launcher identified the cause. It names the port commands without
// claiming a port caused the failure, because an unavailable port is both the
// most common fixable cause and the one the launcher cannot always recognize.
const localRetryGuidance = "The deployment is stopped, so you can fix the reported problem " +
	"and retry:\n" +
	"  exasol start\n" +
	"If the database could not claim its configured port, select another one first:\n" +
	"  exasol config set --ports db:<available-port>\n" +
	"  exasol config set --ports auto"

func addLocalPortRecoveryCallToAction(err error) {
	if recovery, ok := errors.AsType[*deploy.LocalPortRecoveryError](err); ok {
		addTerminalCallToAction(fmt.Sprintf(
			"Select a replacement port for local service %q, then retry:\n"+
				"  exasol config set --ports %s:<available-port>\n"+
				"  exasol config set --ports auto",
			recovery.Service,
			recovery.Service,
		))

		return
	}

	if _, ok := errors.AsType[*deploy.LocalRetryableFailureError](err); ok {
		addTerminalCallToAction(localRetryGuidance)
	}
}
