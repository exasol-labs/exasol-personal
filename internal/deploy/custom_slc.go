// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/connect"
	connecttypes "github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/exasol/exasol-personal/internal/customslc"
	"github.com/exasol/exasol-personal/internal/slc"
)

const (
	customSLCDirPrefix = "custom-"

	customSLCImageRepo = "exasol-personal/custom-slc"

	customSLCDigestLen = 16

	scriptLanguagesQuery = "SELECT SYSTEM_VALUE FROM EXA_PARAMETERS " +
		"WHERE PARAMETER_NAME='SCRIPT_LANGUAGES'"
)

var ErrCustomSLCDatabaseNotRunning = errors.New(
	"the database must be running to remove a custom SLC; run `exasol start` first",
)

// ASCII-only because the alias is also a mount directory name and a URI component.
var customAliasPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,127}$`)

type CustomSLCInstallOpts struct {
	Alias    string
	Language string
	Source   string
}

type CustomSLCConfirm func(prompt string) (bool, error)

type CustomSLCInstallResult struct {
	Operation        string          `json:"operation"`
	Alias            string          `json:"alias"`
	Language         string          `json:"language,omitempty"`
	AlreadyInstalled bool            `json:"alreadyInstalled"`
	Replaced         bool            `json:"replaced"`
	Changed          bool            `json:"changed"`
	Outcome          SLCApplyOutcome `json:"outcome"`
}

type CustomSLCUpdateResult struct {
	Operation string          `json:"operation"`
	Alias     string          `json:"alias"`
	Found     bool            `json:"found"`
	Unchanged bool            `json:"unchanged"`
	Changed   bool            `json:"changed"`
	Outcome   SLCApplyOutcome `json:"outcome"`
}

type CustomSLCRemoveResult struct {
	Operation string `json:"operation"`
	Alias     string `json:"alias"`
	Found     bool   `json:"found"`
	Changed   bool   `json:"changed"`
}

type CustomSLCStatus struct {
	Alias     string
	Language  string
	Source    string
	Available bool
}

//nolint:revive // restart is a user-controlled flag (--no-restart), not internal control coupling.
func InstallCustomSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	opts CustomSLCInstallOpts,
	verbose bool,
	restart bool,
	confirm CustomSLCConfirm,
) (*CustomSLCInstallResult, error) {
	var result *CustomSLCInstallResult
	var staged *config.InstalledCustomSLC

	err := withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			var err error
			result, staged, err = stageCustomSLCInstall(ctx, deployment, opts, restart, confirm)

			return err
		})
	if err != nil || staged == nil {
		return result, err
	}

	outcome, err := applyStagedCustomSLC(ctx, deployment, *staged, verbose)
	if err != nil {
		return nil, err
	}
	result.Outcome = outcome

	slog.Info(
		"custom script language container installed",
		"alias", result.Alias,
		"language", result.Language,
		"outcome", result.Outcome.String(),
	)

	return result, nil
}

// Applying is left to the caller: Start/Stop take the same lock this runs under.
//
//nolint:revive // restart is a user-controlled flag (--no-restart), not internal control coupling.
func stageCustomSLCInstall(
	ctx context.Context,
	deployment config.DeploymentDir,
	opts CustomSLCInstallOpts,
	restart bool,
	confirm CustomSLCConfirm,
) (*CustomSLCInstallResult, *config.InstalledCustomSLC, error) {
	request, err := validateCustomSLCOpts(opts)
	if err != nil {
		return nil, nil, err
	}
	if err := requireCustomSLCPreconditions(deployment); err != nil {
		return nil, nil, err
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return nil, nil, err
	}

	replaced, err := confirmCustomSLCInstall(ctx, deployment, state, request, confirm)
	if err != nil {
		return nil, nil, err
	}

	// Confirmed before fetching, so a declined install never downloads a large archive.
	if err := confirmRestartIfRunning(ctx, deployment, restart, customRestartConfirm(
		confirm, "installing a custom SLC restarts the database, dropping open connections",
	)); err != nil {
		return nil, nil, err
	}

	tarball, err := acquireCustomTarball(ctx, deployment, request.source)
	if err != nil {
		return nil, nil, err
	}
	defer tarball.cleanup()

	idx := findInstalledCustomSLC(state.InstalledCustomSLCs, request.alias)
	if idx >= 0 && customSLCUnchanged(
		state.InstalledCustomSLCs[idx], tarball.sha256, request.language,
	) {
		return &CustomSLCInstallResult{
			Operation:        SLCOperationInstall,
			Alias:            request.alias,
			AlreadyInstalled: true,
			Outcome:          SLCApplyNone,
		}, nil, nil
	}
	aliases, err := readCustomSLCAliases(tarball.path)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid custom SLC container: %w", err)
	}
	if err := checkCustomSLCInternalAliasConflicts(
		deployment, state, aliases, idx, "install", request.alias,
	); err != nil {
		return nil, nil, err
	}

	entry, err := recordCustomSLC(deployment, state, request, tarball, aliases)
	if err != nil {
		return nil, nil, err
	}

	result := &CustomSLCInstallResult{
		Operation: SLCOperationInstall,
		Alias:     request.alias,
		Language:  string(request.language),
		Replaced:  replaced,
		Changed:   true,
		Outcome:   SLCApplyDeferred,
	}
	if !restart {
		slog.Info(
			"custom script language container install recorded",
			"alias", request.alias, "activation", "next_start",
		)

		return result, nil, nil
	}

	return result, &entry, nil
}

func UpdateCustomSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	opts CustomSLCInstallOpts,
	verbose bool,
	restart bool,
	confirm CustomSLCConfirm,
) (*CustomSLCUpdateResult, error) {
	var result *CustomSLCUpdateResult
	var staged *config.InstalledCustomSLC

	err := withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			var err error
			result, staged, err = stageCustomSLCUpdate(ctx, deployment, opts, restart, confirm)

			return err
		})
	if err != nil || staged == nil {
		return result, err
	}

	outcome, err := applyStagedCustomSLC(ctx, deployment, *staged, verbose)
	if err != nil {
		return nil, err
	}
	result.Outcome = outcome

	slog.Info(
		"custom script language container updated",
		"alias", result.Alias,
		"outcome", result.Outcome.String(),
	)

	return result, nil
}

//nolint:revive // restart is a user-controlled flag (--no-restart), not internal control coupling.
func stageCustomSLCUpdate(
	ctx context.Context,
	deployment config.DeploymentDir,
	opts CustomSLCInstallOpts,
	restart bool,
	confirm CustomSLCConfirm,
) (*CustomSLCUpdateResult, *config.InstalledCustomSLC, error) {
	alias, err := validateCustomAlias(opts.Alias)
	if err != nil {
		return nil, nil, err
	}
	source, err := validateCustomSource(opts.Source)
	if err != nil {
		return nil, nil, err
	}
	if err := requireCustomSLCPreconditions(deployment); err != nil {
		return nil, nil, err
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return nil, nil, err
	}
	idx := findInstalledCustomSLC(state.InstalledCustomSLCs, alias)
	if idx < 0 {
		return &CustomSLCUpdateResult{
			Operation: SLCOperationUpdate,
			Alias:     alias,
		}, nil, nil
	}

	languageInput := opts.Language
	if strings.TrimSpace(languageInput) == "" {
		languageInput = state.InstalledCustomSLCs[idx].Language
	}
	language, err := customslc.NormalizeLanguage(languageInput)
	if err != nil {
		return nil, nil, err
	}

	if err := confirmRestartIfRunning(ctx, deployment, restart, customRestartConfirm(
		confirm, "updating a custom SLC restarts the database, dropping open connections",
	)); err != nil {
		return nil, nil, err
	}

	tarball, err := acquireCustomTarball(ctx, deployment, source)
	if err != nil {
		return nil, nil, err
	}
	defer tarball.cleanup()

	if customSLCUnchanged(state.InstalledCustomSLCs[idx], tarball.sha256, language) {
		return &CustomSLCUpdateResult{
			Operation: SLCOperationUpdate,
			Alias:     alias,
			Found:     true,
			Unchanged: true,
			Outcome:   SLCApplyNone,
		}, nil, nil
	}
	aliases, err := readCustomSLCAliases(tarball.path)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid custom SLC container: %w", err)
	}
	if err := checkCustomSLCInternalAliasConflicts(
		deployment, state, aliases, idx, "update", alias,
	); err != nil {
		return nil, nil, err
	}

	request := customSLCRequest{alias: alias, language: language, source: source}
	entry, err := recordCustomSLC(deployment, state, request, tarball, aliases)
	if err != nil {
		return nil, nil, err
	}

	result := &CustomSLCUpdateResult{
		Operation: SLCOperationUpdate,
		Alias:     alias,
		Found:     true,
		Changed:   true,
		Outcome:   SLCApplyDeferred,
	}
	if !restart {
		slog.Info(
			"custom script language container update recorded",
			"alias", alias, "activation", "next_start",
		)

		return result, nil, nil
	}

	return result, &entry, nil
}

func recordCustomSLC(
	deployment config.DeploymentDir,
	state *config.ExasolPersonalState,
	request customSLCRequest,
	tarball acquiredTarball,
	aliases []string,
) (config.InstalledCustomSLC, error) {
	file, err := os.Open(
		tarball.path,
	) //nolint:gosec // path is launcher-owned (download temp or user file)
	if err != nil {
		return config.InstalledCustomSLC{}, err
	}
	defer file.Close()

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return config.InstalledCustomSLC{}, err
	}

	entry := config.InstalledCustomSLC{
		Alias:          request.alias,
		PackageAliases: aliases,
		Language:       string(request.language),
		Image:          customSLCImage(request.alias, tarball.sha256),
		Target:         customSLCTarget(request.alias),
		Package:        customSLCPackageName(request.alias, tarball.sha256),
		Sha256:         tarball.sha256,
		Source:         request.source,
		DisplacedURI:   carriedDisplacedURI(state.InstalledCustomSLCs, request.alias),
	}

	if err := placeCustomSLCPackage(deployment, entry.Package, tarball, file); err != nil {
		return config.InstalledCustomSLC{}, err
	}

	superseded := supersededCustomSLCPackage(state.InstalledCustomSLCs, entry)
	state.InstalledCustomSLCs = upsertInstalledCustomSLC(state.InstalledCustomSLCs, entry)
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		return config.InstalledCustomSLC{}, err
	}

	if superseded != "" {
		if err := removeCustomSLCPackage(deployment, superseded); err != nil {
			slog.Warn(
				"failed to remove the superseded custom SLC package; delete it manually if needed",
				"package", superseded, "error", err,
			)
		}
	}

	return entry, nil
}

func readCustomSLCAliases(packagePath string) ([]string, error) {
	file, err := os.Open(packagePath) //nolint:gosec // path is launcher-owned
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return customslc.ReadAliases(file)
}

func checkCustomSLCInternalAliasConflicts(
	deployment config.DeploymentDir,
	state *config.ExasolPersonalState,
	candidate []string,
	excludeCustomIndex int,
	operation string,
	targetAlias string,
) error {
	for _, alias := range candidate {
		for idx, installed := range state.InstalledCustomSLCs {
			if idx == excludeCustomIndex {
				continue
			}
			aliases, err := installedCustomSLCAliases(deployment, installed)
			if err != nil {
				return fmt.Errorf(
					"cannot inspect installed custom SLC %q: %w", installed.Alias, err,
				)
			}
			for _, declared := range aliases {
				if customslc.NormalizeAlias(alias) == customslc.NormalizeAlias(declared) {
					return fmt.Errorf(
						"cannot %s custom SLC %q: an alias defined in this package, %q, is "+
							"already provided by installed custom SLC %q; remove %q before "+
							"proceeding, or use a package with different aliases",
						operation,
						targetAlias,
						customslc.NormalizeAlias(alias),
						installed.Alias,
						installed.Alias,
					)
				}
			}
		}
	}

	return nil
}

func installedCustomSLCAliases(
	deployment config.DeploymentDir,
	installed config.InstalledCustomSLC,
) ([]string, error) {
	if len(installed.PackageAliases) > 0 {
		return installed.PackageAliases, nil
	}

	return readCustomSLCAliases(filepath.Join(customSLCStagingDir(deployment), installed.Package))
}

func applyStagedCustomSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	entry config.InstalledCustomSLC,
	verbose bool,
) (SLCApplyOutcome, error) {
	slog.Info(
		"installing custom script language container",
		"alias", entry.Alias, "may_take_minutes", true,
	)

	outcome, err := applySLCChange(ctx, deployment, verbose, true)
	if err != nil {
		return 0, fmt.Errorf(
			"failed to activate custom SLC %s (it is recorded and will be retried on the "+
				"next start): %w", entry.Alias, err,
		)
	}

	if err := verifyCustomSLCApplied(deployment, entry); err != nil {
		return 0, err
	}

	return outcome, nil
}

// The restart runs outside the deployment lock, so this container may have changed meanwhile.
func verifyCustomSLCApplied(
	deployment config.DeploymentDir,
	entry config.InstalledCustomSLC,
) error {
	recorded, err := recordedCustomSLC(deployment, entry.Alias)
	if err != nil {
		return err
	}
	if recorded == nil {
		return fmt.Errorf(
			"custom SLC %s was removed by another operation before it could be activated",
			entry.Alias,
		)
	}
	if recorded.Sha256 != entry.Sha256 || recorded.Language != entry.Language {
		return fmt.Errorf(
			"custom SLC %s was replaced by another operation before it could be activated",
			entry.Alias,
		)
	}
	if !recorded.Activated {
		return customSLCUnavailableError(deployment, entry)
	}

	return nil
}

func recordedCustomSLC(
	deployment config.DeploymentDir,
	alias string,
) (*config.InstalledCustomSLC, error) {
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return nil, err
	}
	idx := findInstalledCustomSLC(state.InstalledCustomSLCs, alias)
	if idx < 0 {
		return nil, nil //nolint:nilnil // absence is the answer, not an error
	}

	return &state.InstalledCustomSLCs[idx], nil
}

func customSLCUnavailableReason(state string) string {
	switch state {
	case "package-missing":
		return "its staged container package is missing"
	case "import-failed":
		return "its container package could not be imported"
	case "":
		return "the deployment has not started with it yet"
	default:
		return state
	}
}

func customSLCUnavailableError(
	deployment config.DeploymentDir,
	entry config.InstalledCustomSLC,
) error {
	detail := "the container could not be activated"
	states, err := readSLCImageStates(deployment)
	if err == nil && !customSLCImageAvailable(states, entry.Image) {
		detail = customSLCUnavailableReason(states[entry.Image])
	}

	return fmt.Errorf(
		"custom SLC %s is recorded but not active: %s; the database is running without it",
		entry.Alias, detail,
	)
}

func RemoveCustomSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	alias string,
) (*CustomSLCRemoveResult, error) {
	var result *CustomSLCRemoveResult
	err := withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			var err error
			result, err = removeCustomSLCLocked(ctx, deployment, alias)

			return err
		})

	return result, err
}

func removeCustomSLCLocked(
	ctx context.Context,
	deployment config.DeploymentDir,
	alias string,
) (*CustomSLCRemoveResult, error) {
	normalized, err := validateCustomAlias(alias)
	if err != nil {
		return nil, err
	}
	if err := requireCustomSLCPreconditions(deployment); err != nil {
		return nil, err
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return nil, err
	}
	idx := findInstalledCustomSLC(state.InstalledCustomSLCs, normalized)
	if idx < 0 {
		return &CustomSLCRemoveResult{Operation: SLCOperationRemove, Alias: normalized}, nil
	}
	removed := state.InstalledCustomSLCs[idx]

	if removed.Activated {
		if !isLocalDeploymentRunning(ctx, deployment) {
			return nil, ErrCustomSLCDatabaseNotRunning
		}
		slog.Info("deactivating custom script language container", "alias", removed.Alias)
		if err := deactivateCustomSLC(
			ctx, deployment, removed.Alias, removed.DisplacedURI,
		); err != nil {
			return nil, err
		}
	}

	state.InstalledCustomSLCs = append(
		state.InstalledCustomSLCs[:idx:idx],
		state.InstalledCustomSLCs[idx+1:]...,
	)
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		return nil, err
	}

	if err := removeCustomSLCPackage(deployment, removed.Package); err != nil {
		slog.Warn(
			"failed to remove the custom SLC package; delete it manually if needed",
			"package", removed.Package, "error", err,
		)
	}

	slog.Info(
		"custom script language container removed",
		"alias", removed.Alias,
		"language", removed.Language,
	)

	return &CustomSLCRemoveResult{
		Operation: SLCOperationRemove,
		Alias:     removed.Alias,
		Found:     true,
		Changed:   true,
	}, nil
}

func reconcileCustomSLCActivation(ctx context.Context, deployment config.DeploymentDir) error {
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return err
	}

	states, err := readSLCImageStates(deployment)
	if err != nil {
		return err
	}

	pending := make([]int, 0, len(state.InstalledCustomSLCs))
	for idx, custom := range state.InstalledCustomSLCs {
		// Reported even when already activated: losing a package is what a user needs told about.
		if !customSLCImageAvailable(states, custom.Image) {
			slog.Warn(
				"custom script language container is unavailable, so its language cannot be "+
					"used; reinstall it with `exasol slc custom install`",
				"alias", custom.Alias,
				"language", custom.Language,
				"reason", customSLCUnavailableReason(states[custom.Image]),
			)

			continue
		}
		if custom.Activated {
			continue
		}
		pending = append(pending, idx)
	}
	if len(pending) == 0 {
		return nil
	}

	if err := activatePendingCustomSLCs(ctx, deployment, state, pending); err != nil {
		return err
	}

	return config.WriteExasolPersonalState(state, deployment)
}

func activatePendingCustomSLCs(
	ctx context.Context,
	deployment config.DeploymentDir,
	state *config.ExasolPersonalState,
	pending []int,
) error {
	return withLocalDatabase(ctx, deployment, func(database connecttypes.Databaser) error {
		entries, err := currentScriptLanguages(ctx, database)
		if err != nil {
			return err
		}

		for _, idx := range pending {
			custom := &state.InstalledCustomSLCs[idx]
			if existing, ok := customslc.FindAlias(entries, custom.Alias); ok &&
				customslc.IsBuiltinURI(existing.URI) && custom.DisplacedURI == "" {
				custom.DisplacedURI = existing.URI
			}
			entries = customslc.SetAlias(
				entries,
				custom.Alias,
				customslc.BuildActivationURI(
					customSLCDir(custom.Alias), customslc.Language(custom.Language),
				),
			)
			custom.Activated = true
		}

		return applyScriptLanguages(ctx, database, entries, state, pending)
	})
}

// A silently-ignored ALTER SYSTEM must fail here, not at the user's first UDF call.
func applyScriptLanguages(
	ctx context.Context,
	database connecttypes.Databaser,
	entries []customslc.AliasEntry,
	state *config.ExasolPersonalState,
	pending []int,
) error {
	if err := setScriptLanguages(ctx, database, entries); err != nil {
		return err
	}
	updated, err := currentScriptLanguages(ctx, database)
	if err != nil {
		return err
	}
	for _, idx := range pending {
		custom := state.InstalledCustomSLCs[idx]
		if !custom.Activated {
			continue
		}
		uri := customslc.BuildActivationURI(
			customSLCDir(custom.Alias), customslc.Language(custom.Language),
		)
		if !aliasResolvesTo(updated, custom.Alias, uri) {
			return fmt.Errorf(
				"activation did not take effect: alias %q does not resolve to the expected "+
					"container in SCRIPT_LANGUAGES after ALTER SYSTEM", custom.Alias,
			)
		}
	}

	return nil
}

// CustomSLCStatuses tolerates a missing state file before deployment initialization.
func CustomSLCStatuses(deployment config.DeploymentDir) ([]CustomSLCStatus, error) {
	has, err := config.HasExasolPersonalStateFile(deployment)
	if err != nil {
		return nil, err
	}
	if !has {
		return []CustomSLCStatus{}, nil
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return nil, err
	}

	states, err := readSLCImageStates(deployment)
	if err != nil {
		return nil, err
	}

	statuses := make([]CustomSLCStatus, 0, len(state.InstalledCustomSLCs))
	for _, inst := range state.InstalledCustomSLCs {
		statuses = append(statuses, CustomSLCStatus{
			Alias:     inst.Alias,
			Language:  inst.Language,
			Source:    inst.Source,
			Available: inst.Activated && customSLCImageAvailable(states, inst.Image),
		})
	}

	return statuses, nil
}

func IsCustomSLCAlias(deployment config.DeploymentDir, alias string) (bool, error) {
	has, err := config.HasExasolPersonalStateFile(deployment)
	if err != nil {
		return false, err
	}
	if !has {
		return false, nil
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return false, err
	}

	return findInstalledCustomSLC(state.InstalledCustomSLCs, alias) >= 0, nil
}

type customSLCRequest struct {
	alias    string
	language customslc.Language
	source   string
}

func validateCustomSLCOpts(opts CustomSLCInstallOpts) (customSLCRequest, error) {
	alias, err := validateCustomAlias(opts.Alias)
	if err != nil {
		return customSLCRequest{}, err
	}
	language, err := customslc.NormalizeLanguage(opts.Language)
	if err != nil {
		return customSLCRequest{}, err
	}
	source, err := validateCustomSource(opts.Source)
	if err != nil {
		return customSLCRequest{}, err
	}

	return customSLCRequest{alias: alias, language: language, source: source}, nil
}

func requireCustomSLCPreconditions(deployment config.DeploymentDir) error {
	if !isLocalDeployment(deployment) {
		return ErrSLCNotSupported
	}

	return requireDeploymentPresent(deployment)
}

func confirmCustomSLCInstall(
	ctx context.Context,
	deployment config.DeploymentDir,
	state *config.ExasolPersonalState,
	request customSLCRequest,
	confirm CustomSLCConfirm,
) (bool, error) {
	if findInstalledCustomSLC(state.InstalledCustomSLCs, request.alias) >= 0 {
		prompt := fmt.Sprintf(
			"a custom SLC is already installed under alias %q; installing replaces it",
			request.alias,
		)

		return true, confirmCustom(confirm, prompt)
	}

	var entries []customslc.AliasEntry
	if isLocalDeploymentRunning(ctx, deployment) {
		read, err := readScriptLanguages(ctx, deployment)
		if err != nil {
			return false, err
		}
		entries = read
	}

	return false, confirmOfficialAliasReuse(state, entries, request.alias, confirm)
}

func confirmOfficialAliasReuse(
	state *config.ExasolPersonalState,
	entries []customslc.AliasEntry,
	alias string,
	confirm CustomSLCConfirm,
) error {
	if flavor := officialOwner(state.InstalledSLCs, alias); flavor != "" {
		return fmt.Errorf(
			"alias %q is provided by the installed official SLC %q; remove it with "+
				"`exasol slc remove %s` or choose a different --alias",
			alias, flavor, flavor,
		)
	}

	officialNames := officialAliasNamespace(entries)
	if !officialNames[alias] {
		return nil
	}

	prompt := fmt.Sprintf(
		"%q is an official alias (%s); installing a custom SLC under it overrides the "+
			"built-in for new sessions",
		alias, strings.Join(sortedKeys(officialNames), ", "),
	)

	return confirmCustom(confirm, prompt)
}

func customSLCDir(alias string) string {
	return customSLCDirPrefix + strings.ToLower(alias)
}

func customSLCTarget(alias string) string {
	return slc.SLCMountRoot + "/" + customSLCDir(alias)
}

// The digest is what makes changed content a reference the runtime has not imported yet.
func customSLCImage(alias, digestHex string) string {
	return customSLCImageRepo + ":" + strings.ToLower(alias) + "-" + shortDigest(digestHex)
}

func customSLCPackageName(alias, digestHex string) string {
	return customSLCDir(alias) + "-" + shortDigest(digestHex) + ".tar.gz"
}

func shortDigest(digestHex string) string {
	if len(digestHex) > customSLCDigestLen {
		return digestHex[:customSLCDigestLen]
	}

	return digestHex
}

func carriedDisplacedURI(installed []config.InstalledCustomSLC, alias string) string {
	if idx := findInstalledCustomSLC(installed, alias); idx >= 0 {
		return installed[idx].DisplacedURI
	}

	return ""
}

func supersededCustomSLCPackage(
	installed []config.InstalledCustomSLC,
	entry config.InstalledCustomSLC,
) string {
	idx := findInstalledCustomSLC(installed, entry.Alias)
	if idx < 0 || installed[idx].Package == entry.Package {
		return ""
	}

	return installed[idx].Package
}

// Requires Activated so a recorded-but-failed install is retried, not reported as a no-op.
func customSLCUnchanged(
	recorded config.InstalledCustomSLC,
	digest string,
	language customslc.Language,
) bool {
	return recorded.Activated &&
		recorded.Sha256 == digest &&
		recorded.Language == string(language)
}

func aliasResolvesTo(entries []customslc.AliasEntry, alias, uri string) bool {
	entry, ok := customslc.FindAlias(entries, alias)

	return ok && entry.URI == uri
}

func deactivateCustomSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	alias, restoreURI string,
) error {
	return withLocalDatabase(ctx, deployment, func(database connecttypes.Databaser) error {
		entries, err := currentScriptLanguages(ctx, database)
		if err != nil {
			return err
		}

		var updated []customslc.AliasEntry
		if restoreURI != "" {
			updated = customslc.SetAlias(entries, alias, restoreURI)
		} else {
			updated = customslc.RemoveAlias(entries, alias)
		}
		if err := setScriptLanguages(ctx, database, updated); err != nil {
			return err
		}

		confirmed, err := currentScriptLanguages(ctx, database)
		if err != nil {
			return err
		}

		return confirmDeactivated(confirmed, alias, restoreURI)
	})
}

func confirmDeactivated(entries []customslc.AliasEntry, alias, restoreURI string) error {
	if restoreURI != "" {
		if !aliasResolvesTo(entries, alias, restoreURI) {
			return fmt.Errorf(
				"deactivation did not take effect: alias %q was not restored to the "+
					"built-in in SCRIPT_LANGUAGES after ALTER SYSTEM", alias,
			)
		}

		return nil
	}
	if _, ok := customslc.FindAlias(entries, alias); ok {
		return fmt.Errorf(
			"deactivation did not take effect: alias %q is still present in "+
				"SCRIPT_LANGUAGES after ALTER SYSTEM", alias,
		)
	}

	return nil
}

func readScriptLanguages(
	ctx context.Context,
	deployment config.DeploymentDir,
) ([]customslc.AliasEntry, error) {
	var entries []customslc.AliasEntry
	err := withLocalDatabase(ctx, deployment, func(database connecttypes.Databaser) error {
		parsed, err := currentScriptLanguages(ctx, database)
		entries = parsed

		return err
	})

	return entries, err
}

func currentScriptLanguages(
	ctx context.Context,
	database connecttypes.Databaser,
) ([]customslc.AliasEntry, error) {
	result, err := database.Exec(ctx, scriptLanguagesQuery, 0)
	if err != nil {
		return nil, err
	}
	rows := result.Rows()
	if len(rows) == 0 || len(rows[0]) == 0 {
		return nil, nil
	}

	return customslc.ParseScriptLanguages(rows[0][0]), nil
}

func setScriptLanguages(
	ctx context.Context,
	database connecttypes.Databaser,
	entries []customslc.AliasEntry,
) error {
	value := customslc.SerializeScriptLanguages(entries)
	if strings.Contains(value, "'") {
		return fmt.Errorf(
			"refusing to write SCRIPT_LANGUAGES: value contains a single quote (%q)",
			value,
		)
	}
	_, err := database.Exec(ctx, "ALTER SYSTEM SET SCRIPT_LANGUAGES='"+value+"'", 0)

	return err
}

func withLocalDatabase(
	ctx context.Context,
	deployment config.DeploymentDir,
	callback func(connecttypes.Databaser) error,
) error {
	connectionInfo, err := config.ResolveConnectionInfo(deployment)
	if err != nil {
		return err
	}

	database, err := connect.NewExasolConnection(
		deployment,
		connectionInfo,
		connectionInfo.Username,
		"",
		connectionInfo.InsecureSkipCertValidation,
	)
	if err != nil {
		return err
	}
	if err := database.Connect(ctx); err != nil {
		return err
	}
	defer database.Close()

	return callback(database)
}

type acquiredTarball struct {
	path    string
	sha256  string
	staged  bool
	cleanup func()
}

func acquireCustomTarball(
	ctx context.Context,
	deployment config.DeploymentDir,
	source string,
) (acquiredTarball, error) {
	if isURLSource(source) {
		return downloadCustomTarball(ctx, deployment, source)
	}

	slog.Info("reading the custom script language container", "file", source)
	tmp, err := newCustomSLCStagingFile(deployment)
	if err != nil {
		return acquiredTarball{}, err
	}
	tempPath := tmp.Name()
	remove := func() { _ = os.Remove(tempPath) }
	removeOnError := true
	defer func() {
		if removeOnError {
			remove()
		}
	}()

	input, err := os.Open(source) //nolint:gosec // path is user-supplied by design (--source)
	if err != nil {
		_ = tmp.Close()

		return acquiredTarball{}, err
	}
	hasher := sha256.New()
	if _, err := io.Copy(tmp, io.TeeReader(input, hasher)); err != nil {
		_ = input.Close()
		_ = tmp.Close()

		return acquiredTarball{}, err
	}
	if err := input.Close(); err != nil {
		_ = tmp.Close()

		return acquiredTarball{}, err
	}
	if err := tmp.Close(); err != nil {
		return acquiredTarball{}, err
	}
	removeOnError = false

	return acquiredTarball{
		path:    tempPath,
		sha256:  hex.EncodeToString(hasher.Sum(nil)),
		staged:  true,
		cleanup: remove,
	}, nil
}

func downloadCustomTarball(
	ctx context.Context,
	deployment config.DeploymentDir,
	url string,
) (acquiredTarball, error) {
	tmp, err := newCustomSLCStagingFile(deployment)
	if err != nil {
		return acquiredTarball{}, err
	}
	defer tmp.Close()

	remove := func() { _ = os.Remove(tmp.Name()) }

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		remove()

		return acquiredTarball{}, err
	}
	client := &http.Client{CheckRedirect: rejectNonHTTPSRedirect}
	resp, err := client.Do(req)
	if err != nil {
		remove()

		return acquiredTarball{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		remove()

		return acquiredTarball{}, fmt.Errorf(
			"failed to download custom SLC from %s: %s", url, resp.Status,
		)
	}

	slog.Info(
		"downloading the custom script language container (this may take a few minutes)",
		"size", resp.ContentLength,
	)

	hasher := sha256.New()
	if _, err := io.Copy(tmp, io.TeeReader(resp.Body, hasher)); err != nil {
		remove()

		return acquiredTarball{}, err
	}

	return acquiredTarball{
		path:    tmp.Name(),
		sha256:  hex.EncodeToString(hasher.Sum(nil)),
		staged:  true,
		cleanup: remove,
	}, nil
}

func rejectNonHTTPSRedirect(req *http.Request, _ []*http.Request) error {
	if !strings.EqualFold(req.URL.Scheme, "https") {
		return fmt.Errorf("refusing redirect to non-https URL %s", req.URL.String())
	}

	return nil
}

func hashFile(filePath string) (string, error) {
	file, err := os.Open(filePath) //nolint:gosec // path is user-supplied by design (--source)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func validateCustomAlias(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("a custom SLC requires an --alias")
	}
	if !customAliasPattern.MatchString(trimmed) {
		return "", fmt.Errorf(
			"invalid alias %q: must start with a letter and use only letters, digits, and "+
				"underscores (max 128 characters)", trimmed,
		)
	}

	return customslc.NormalizeAlias(trimmed), nil
}

func validateCustomSource(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New(
			"a custom SLC requires a --source (a container tarball or an https URL)",
		)
	}

	scheme := urlScheme(trimmed)
	if scheme != "" && scheme != "https" {
		return "", fmt.Errorf(
			"invalid --source %q: a URL source must use https; pass a path for a local file",
			trimmed,
		)
	}

	return trimmed, nil
}

func isURLSource(source string) bool {
	return urlScheme(source) != ""
}

func urlScheme(source string) string {
	parsed, err := neturl.ParseRequestURI(source)
	if err != nil || parsed.Host == "" {
		return ""
	}

	return strings.ToLower(parsed.Scheme)
}

func customRestartConfirm(confirm CustomSLCConfirm, prompt string) ConfirmFunc {
	if confirm == nil {
		return nil
	}

	return func() (bool, error) { return confirm(prompt) }
}

func confirmCustom(confirm CustomSLCConfirm, prompt string) error {
	if confirm == nil {
		return nil
	}
	ok, err := confirm(prompt)
	if err != nil {
		return err
	}
	if !ok {
		return ErrSLCOperationCancelled
	}

	return nil
}

func officialAliasNamespace(entries []customslc.AliasEntry) map[string]bool {
	names := make(map[string]bool)
	for _, alias := range customslc.BuiltinAliases(entries) {
		names[alias] = true
	}

	catalog, err := slc.Load(resources.SLCCatalogYAML)
	if err != nil {
		return names
	}
	catalogEntries, err := catalog.List(runtime.GOARCH)
	if err != nil {
		return names
	}
	for _, entry := range catalogEntries {
		for _, alias := range entry.Aliases {
			names[customslc.NormalizeAlias(alias)] = true
		}
	}

	return names
}

func officialOwner(installed []config.InstalledSLC, alias string) string {
	needle := customslc.NormalizeAlias(alias)
	for _, inst := range installed {
		for _, declared := range inst.Aliases {
			if customslc.NormalizeAlias(declared) == needle {
				return inst.Flavor
			}
		}
	}

	return ""
}

func findInstalledCustomSLC(installed []config.InstalledCustomSLC, alias string) int {
	needle := customslc.NormalizeAlias(alias)
	for idx, inst := range installed {
		if customslc.NormalizeAlias(inst.Alias) == needle {
			return idx
		}
	}

	return -1
}

func upsertInstalledCustomSLC(
	existing []config.InstalledCustomSLC,
	entry config.InstalledCustomSLC,
) []config.InstalledCustomSLC {
	updated := make([]config.InstalledCustomSLC, 0, len(existing)+1)
	for _, inst := range existing {
		if customslc.NormalizeAlias(inst.Alias) == customslc.NormalizeAlias(entry.Alias) {
			continue
		}
		updated = append(updated, inst)
	}
	updated = append(updated, entry)
	slices.SortFunc(updated, func(left, right config.InstalledCustomSLC) int {
		return strings.Compare(left.Alias, right.Alias)
	})

	return updated
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	return keys
}

// A downloaded container already sits in the staging directory.
func placeCustomSLCPackage(
	deployment config.DeploymentDir,
	name string,
	tarball acquiredTarball,
	content io.Reader,
) error {
	if tarball.staged {
		return promoteCustomSLCPackage(deployment, tarball.path, name)
	}

	slog.Info("staging the custom script language container (this may take a few minutes)")

	return stageCustomSLCPackage(deployment, name, content)
}
