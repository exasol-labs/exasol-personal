// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"os"

	// Activates embedded runtime resources for this binary.
	_ "github.com/exasol/exasol-personal/assets/resources/generated"
	"github.com/exasol/exasol-personal/internal/util"
)

type cliSilentError interface {
	SuppressCLIError() bool
}

func main() {
	util.StartSignalHandler(func(_ os.Signal) {
		os.Exit(1)
	})

	if err := Execute(); err != nil {
		var silentErr cliSilentError
		if errors.As(err, &silentErr) && silentErr.SuppressCLIError() {
			os.Exit(1)
		}

		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printTerminalCallsToActionAfterError()
		os.Exit(1)
	}
}
