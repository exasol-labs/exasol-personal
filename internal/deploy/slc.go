// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"strings"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/exasol/exasol-personal/internal/slc"
)

// ErrSLCNotSupported is returned when SLC management is attempted on a backend that does
// not support it (only local deployments do, via Podman image mount).
var ErrSLCNotSupported = errors.New(
	"script language container management is only supported for local deployments",
)

// ErrSLCOperationCancelled is returned when the user declines the database-restart prompt.
var ErrSLCOperationCancelled = errors.New("operation cancelled")

// ErrDeploymentNotPresent is returned when an SLC change operation (install/update/remove)
// is attempted on a deployment that has only been initialized, never deployed. SLC
// management operates on a deployed database, so there is nothing to change — and, crucially,
// no launcher state is recorded in this case.
var ErrDeploymentNotPresent = errors.New(
	"deployment is not present; run `exasol deploy` first",
)

// requireDeploymentPresent rejects an SLC change operation when the deployment has been
// initialized but not deployed yet. It is called before any state is read for modification
// so a not-yet-deployed deployment fails cleanly without recording anything.
func requireDeploymentPresent(deployment config.DeploymentDir) error {
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return err
	}
	workflowState, err := state.GetWorkflowState()
	if err != nil {
		return err
	}
	if _, notDeployed := workflowState.(*config.WorkflowStateInitialized); notDeployed {
		return ErrDeploymentNotPresent
	}

	return nil
}

// ConfirmFunc asks the user to confirm a database restart. It is invoked only when an
// operation would restart a *running* database, after validation and before any state is
// written. Returning false aborts with ErrSLCOperationCancelled. A nil ConfirmFunc means
// "already confirmed" (e.g. the caller passed --auto-approve).
type ConfirmFunc func() (bool, error)

func confirmOrCancel(confirm ConfirmFunc) error {
	if confirm == nil {
		return nil
	}
	ok, err := confirm()
	if err != nil {
		return err
	}
	if !ok {
		return ErrSLCOperationCancelled
	}

	return nil
}

//nolint:revive // restart is a user-controlled flag (--no-restart), not internal control coupling.
func confirmRestartIfRunning(
	ctx context.Context,
	deployment config.DeploymentDir,
	restart bool,
	confirm ConfirmFunc,
) error {
	if restart && isLocalDeploymentRunning(ctx, deployment) {
		return confirmOrCancel(confirm)
	}

	return nil
}

// SLCApplyOutcome describes how an install, update, or remove was applied to the running state.
type SLCApplyOutcome int

const slcApplyOutcomeUnknown = "unknown"

const (
	SLCOperationInstall = "install"
	SLCOperationUpdate  = "update"
	SLCOperationRemove  = "remove"
)

const (
	// SLCApplyNone means no apply action was needed.
	SLCApplyNone SLCApplyOutcome = iota
	// SLCApplyRestarted means the running database was stopped and started again.
	SLCApplyRestarted
	// SLCApplyStarted means a stopped database was started to apply the change.
	SLCApplyStarted
	// SLCApplyDeferred means the change was persisted but the (stopped) database was not
	// started; it will take effect on the next start.
	SLCApplyDeferred
)

func (outcome SLCApplyOutcome) String() string {
	switch outcome {
	case SLCApplyNone:
		return "none"
	case SLCApplyRestarted:
		return "restarted"
	case SLCApplyStarted:
		return "started"
	case SLCApplyDeferred:
		return "deferred"
	default:
		return slcApplyOutcomeUnknown
	}
}

func (outcome SLCApplyOutcome) MarshalText() ([]byte, error) {
	return []byte(outcome.String()), nil
}

// SLCInstallResult reports the outcome of an install.
type SLCInstallResult struct {
	Operation        string              `json:"operation"`
	Entry            config.InstalledSLC `json:"entry"`
	AlreadyInstalled bool                `json:"alreadyInstalled"`
	Replaced         bool                `json:"replaced"`
	Changed          bool                `json:"changed"`
	Outcome          SLCApplyOutcome     `json:"outcome"`
}

// SLCRemoveResult reports the outcome of a remove.
type SLCRemoveResult struct {
	Operation string               `json:"operation"`
	Found     bool                 `json:"found"`
	Changed   bool                 `json:"changed"`
	Entry     *config.InstalledSLC `json:"entry,omitempty"`
	Outcome   SLCApplyOutcome      `json:"outcome"`
}

// SLCUpdateResult reports the outcome of an update.
type SLCUpdateResult struct {
	Operation   string               `json:"operation"`
	Found       bool                 `json:"found"`
	Unchanged   bool                 `json:"unchanged"`
	Changed     bool                 `json:"changed"`
	FromFlavor  string               `json:"fromFlavor,omitempty"`
	FromVersion string               `json:"fromVersion,omitempty"`
	Entry       *config.InstalledSLC `json:"entry,omitempty"`
	Outcome     SLCApplyOutcome      `json:"outcome"`
}

// SLCStatus describes one catalog SLC and whether it is installed in this deployment.
type SLCStatus struct {
	Language  string   `json:"language"`
	Flavor    string   `json:"flavor"`
	Version   string   `json:"version"`
	Aliases   []string `json:"aliases"`
	Installed bool     `json:"installed"`
}

// InstallSLC resolves an alias against the official SLC catalog, records the SLC in
// launcher state, and applies it by (re)starting the local database. It returns a
// clear error (leaving the SLC configured for a later start) if the database cannot be
// brought up with the mount, rather than reporting success.
//
//nolint:revive // restart is a user-controlled flag (--no-restart), not internal control coupling.
func InstallSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	alias string,
	verbose bool,
	restart bool,
	confirm ConfirmFunc,
) (*SLCInstallResult, error) {
	entry, state, err := resolveOfficialSLCForChange(deployment, alias)
	if err != nil {
		return nil, err
	}

	// An identical already-installed image is a no-op: no state change, no restart.
	if idx := findInstalledByImage(state.InstalledSLCs, entry.Image); idx >= 0 {
		return &SLCInstallResult{
			Operation:        SLCOperationInstall,
			Entry:            state.InstalledSLCs[idx],
			AlreadyInstalled: true,
			Outcome:          SLCApplyNone,
		}, nil
	}

	replaces, err := slc.CheckInstallable(installedEntries(state), entry)
	if err != nil {
		return nil, err
	}

	// Confirm before disrupting a running database. Only prompt when a restart will
	// actually happen; validation above has already succeeded, so we never prompt for an
	// install that would fail anyway.
	if err := confirmRestartIfRunning(ctx, deployment, restart, confirm); err != nil {
		return nil, err
	}

	state.InstalledSLCs = upsertInstalledSLC(state.InstalledSLCs, toInstalledSLC(entry))
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		return nil, err
	}

	if !restart {
		slog.Info(
			"script language container install recorded",
			"alias", alias,
			"flavor", entry.Flavor,
			"version", entry.Version,
			"replaced", replaces,
			"activation", "next_start",
		)

		return &SLCInstallResult{
			Operation: SLCOperationInstall,
			Entry:     toInstalledSLC(entry),
			Replaced:  replaces,
			Changed:   true,
			Outcome:   SLCApplyDeferred,
		}, nil
	}

	slog.Info(
		"installing script language container",
		"alias", alias,
		"flavor", entry.Flavor,
		"version", entry.Version,
		"may_take_minutes", true,
	)
	outcome, err := applySLCChange(ctx, deployment, verbose, true)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to activate SLC %s (it is recorded and will be retried on the next start): %w",
			entry.Flavor, err,
		)
	}

	slog.Info(
		"script language container installed",
		"alias", alias,
		"flavor", entry.Flavor,
		"version", entry.Version,
		"outcome", outcome.String(),
	)

	return &SLCInstallResult{
		Operation: SLCOperationInstall,
		Entry:     toInstalledSLC(entry),
		Replaced:  replaces,
		Changed:   true,
		Outcome:   outcome,
	}, nil
}

// RemoveSLC removes an installed SLC (matched by alias, language, or flavor) and, if
// the database is running, restarts it so the change takes effect.
//
//nolint:revive // restart is a user-controlled flag (--no-restart), not internal control coupling.
func RemoveSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	alias string,
	verbose bool,
	restart bool,
	confirm ConfirmFunc,
) (*SLCRemoveResult, error) {
	if !isLocalDeployment(deployment) {
		return nil, ErrSLCNotSupported
	}
	if err := requireDeploymentPresent(deployment); err != nil {
		return nil, err
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return nil, err
	}

	index := findInstalledSLC(state.InstalledSLCs, alias)
	if index < 0 {
		return &SLCRemoveResult{Operation: SLCOperationRemove, Found: false}, nil
	}

	// Confirm before disrupting a running database (only when a restart will happen).
	if err := confirmRestartIfRunning(ctx, deployment, restart, confirm); err != nil {
		return nil, err
	}

	removed := state.InstalledSLCs[index]
	state.InstalledSLCs = append(
		state.InstalledSLCs[:index:index],
		state.InstalledSLCs[index+1:]...,
	)
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		return nil, err
	}

	if !restart {
		slog.Info(
			"script language container removal recorded",
			"alias", alias,
			"flavor", removed.Flavor,
			"version", removed.Version,
			"activation", "next_start",
		)

		return &SLCRemoveResult{
			Operation: SLCOperationRemove,
			Found:     true,
			Changed:   true,
			Entry:     &removed,
			Outcome:   SLCApplyDeferred,
		}, nil
	}

	if isLocalDeploymentRunning(ctx, deployment) {
		slog.Info(
			"removing script language container",
			"alias", alias,
			"flavor", removed.Flavor,
			"version", removed.Version,
			"may_take_minutes", true,
		)
	}
	outcome, err := applySLCChange(ctx, deployment, verbose, false)
	if err != nil {
		return nil, err
	}

	slog.Info(
		"script language container removed",
		"alias", alias,
		"flavor", removed.Flavor,
		"version", removed.Version,
		"outcome", outcome.String(),
	)

	return &SLCRemoveResult{
		Operation: SLCOperationRemove,
		Found:     true,
		Changed:   true,
		Entry:     &removed,
		Outcome:   outcome,
	}, nil
}

// UpdateSLC re-resolves an installed SLC against the catalog and, if the resolved image
// has changed, replaces the installed one and restarts the database to apply it.
//
//nolint:revive // restart is a user-controlled flag (--no-restart), not internal control coupling.
func UpdateSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	alias string,
	verbose bool,
	restart bool,
	confirm ConfirmFunc,
) (*SLCUpdateResult, error) {
	entry, state, err := resolveOfficialSLCForChange(deployment, alias)
	if err != nil {
		return nil, err
	}

	index := findInstalledSLC(state.InstalledSLCs, entry.Language)
	if index < 0 {
		return &SLCUpdateResult{Operation: SLCOperationUpdate, Found: false}, nil
	}
	installed := state.InstalledSLCs[index]

	// Shared no-op test with install: an unchanged (content-addressed) image is nothing to
	// update.
	if findInstalledByImage(state.InstalledSLCs, entry.Image) >= 0 {
		return &SLCUpdateResult{
			Operation: SLCOperationUpdate,
			Found:     true,
			Unchanged: true,
			Entry:     &installed,
			Outcome:   SLCApplyNone,
		}, nil
	}

	// An update can move to a new flavor (e.g. the unversioned alias shifting from
	// python-3.12 to python-3.13). Check alias-disjointness against the *other* installed
	// SLCs so replacing this one never falsely collides with itself.
	remaining := make([]config.InstalledSLC, 0, len(state.InstalledSLCs)-1)
	remaining = append(remaining, state.InstalledSLCs[:index]...)
	remaining = append(remaining, state.InstalledSLCs[index+1:]...)
	if _, err := slc.CheckInstallable(entriesFromInstalled(remaining), entry); err != nil {
		return nil, err
	}

	if err := confirmRestartIfRunning(ctx, deployment, restart, confirm); err != nil {
		return nil, err
	}

	result := &SLCUpdateResult{
		Operation:   SLCOperationUpdate,
		Found:       true,
		Changed:     true,
		FromFlavor:  installed.Flavor,
		FromVersion: installed.Version,
	}
	resultEntry := toInstalledSLC(entry)
	result.Entry = &resultEntry

	state.InstalledSLCs = upsertInstalledSLC(remaining, resultEntry)
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		return nil, err
	}

	if !restart {
		slog.Info(
			"script language container update recorded",
			"alias", alias,
			"from_flavor", installed.Flavor,
			"from_version", installed.Version,
			"flavor", entry.Flavor,
			"version", entry.Version,
			"activation", "next_start",
		)
		result.Outcome = SLCApplyDeferred

		return result, nil
	}

	slog.Info(
		"updating script language container",
		"alias", alias,
		"from_flavor", installed.Flavor,
		"from_version", installed.Version,
		"flavor", entry.Flavor,
		"version", entry.Version,
		"may_take_minutes", true,
	)
	outcome, err := applySLCChange(ctx, deployment, verbose, true)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to activate updated SLC %s (it is recorded and will be "+
				"retried on the next start): %w",
			entry.Flavor,
			err,
		)
	}
	result.Outcome = outcome

	slog.Info(
		"script language container updated",
		"alias", alias,
		"from_flavor", installed.Flavor,
		"from_version", installed.Version,
		"flavor", entry.Flavor,
		"version", entry.Version,
		"outcome", outcome.String(),
	)

	return result, nil
}

// SLCStatuses returns every SLC in the catalog (for the current architecture) with its
// installation status in this deployment.
func SLCStatuses(deployment config.DeploymentDir) ([]SLCStatus, error) {
	catalog, err := slc.Load(resources.SLCCatalogYAML)
	if err != nil {
		return nil, err
	}
	entries, err := catalog.List(runtime.GOARCH)
	if err != nil {
		return nil, err
	}

	installed, err := installedFlavors(deployment)
	if err != nil {
		return nil, err
	}

	statuses := make([]SLCStatus, 0, len(entries))
	for _, entry := range entries {
		statuses = append(statuses, SLCStatus{
			Language:  entry.Language,
			Flavor:    entry.Flavor,
			Version:   entry.Version,
			Aliases:   entry.Aliases,
			Installed: installed[strings.ToLower(entry.Flavor)],
		})
	}

	return statuses, nil
}

// installedFlavors returns the set of installed SLC flavors (lower-cased) for a deployment.
// A missing state file is tolerated as "nothing installed" so `slc list` works before the
// deployment is initialized; a present-but-unreadable state file surfaces as an error rather
// than silently reporting everything as not installed.
func installedFlavors(deployment config.DeploymentDir) (map[string]bool, error) {
	installed := make(map[string]bool)

	has, err := config.HasExasolPersonalStateFile(deployment)
	if err != nil {
		return nil, err
	}
	if !has {
		return installed, nil
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return nil, err
	}
	for _, inst := range state.InstalledSLCs {
		installed[strings.ToLower(inst.Flavor)] = true
	}

	return installed, nil
}

// applySLCChange (re)starts the local database so a changed SLC set takes effect. Success
// is verified through the readiness wait inside Start: startup fails, and the database never
// becomes ready, if an image cannot be pulled or two SLCs collide, so a successful start
// confirms the change applied.
//
//nolint:revive // ensureRunning selects apply semantics (start vs. defer), not internal control coupling.
func applySLCChange(
	ctx context.Context,
	deployment config.DeploymentDir,
	verbose bool,
	ensureRunning bool,
) (SLCApplyOutcome, error) {
	if isLocalDeploymentRunning(ctx, deployment) {
		if err := Stop(ctx, deployment, verbose); err != nil {
			return 0, err
		}
		// SLC restarts happen after the initial Prepare's approval moment.
		// Podman and its machine are already set up, so preparation
		// short-circuits. The zero PrepareOptions carries no approver,
		// which denies any host change instead of prompting mid-restart.
		if err := Start(ctx, deployment, verbose, StartOptions{
			WaitTimeoutSeconds: StartedDefaultTimeoutSeconds,
		}); err != nil {
			return 0, err
		}

		return SLCApplyRestarted, nil
	}

	if !ensureRunning {
		return SLCApplyDeferred, nil
	}

	if err := Start(ctx, deployment, verbose, StartOptions{
		WaitTimeoutSeconds: StartedDefaultTimeoutSeconds,
	}); err != nil {
		return 0, err
	}

	return SLCApplyStarted, nil
}

func isLocalDeploymentRunning(ctx context.Context, deployment config.DeploymentDir) bool {
	manager := runtimeartifacts.FromContext(ctx)

	selectedRuntime, err := newLocalRuntime(deployment, manager)
	if err != nil {
		return false
	}
	status, err := selectedRuntime.Status(ctx)
	if err != nil {
		return false
	}

	return status.Running
}

func resolveOfficialSLCForChange(
	deployment config.DeploymentDir,
	alias string,
) (slc.Entry, *config.ExasolPersonalState, error) {
	if !isLocalDeployment(deployment) {
		return slc.Entry{}, nil, ErrSLCNotSupported
	}
	if err := requireDeploymentPresent(deployment); err != nil {
		return slc.Entry{}, nil, err
	}

	catalog, err := slc.Load(resources.SLCCatalogYAML)
	if err != nil {
		return slc.Entry{}, nil, err
	}
	entry, err := catalog.Resolve(alias, runtime.GOARCH)
	if err != nil {
		return slc.Entry{}, nil, err
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return slc.Entry{}, nil, err
	}
	if err := checkOfficialAliasNotHeldByCustom(
		state.InstalledCustomSLCs, entry.Aliases,
	); err != nil {
		return slc.Entry{}, nil, err
	}

	return entry, state, nil
}

func installedEntries(state *config.ExasolPersonalState) []slc.Entry {
	return entriesFromInstalled(state.InstalledSLCs)
}

func entriesFromInstalled(installed []config.InstalledSLC) []slc.Entry {
	entries := make([]slc.Entry, 0, len(installed))
	for _, inst := range installed {
		entries = append(entries, slc.Entry{
			Language: inst.Language,
			Flavor:   inst.Flavor,
			Version:  inst.Version,
			Image:    inst.Image,
			Target:   inst.Target,
			Aliases:  inst.Aliases,
		})
	}

	return entries
}

func toInstalledSLC(entry slc.Entry) config.InstalledSLC {
	return config.InstalledSLC{
		Language: entry.Language,
		Flavor:   entry.Flavor,
		Version:  entry.Version,
		Image:    entry.Image,
		Target:   entry.Target,
		Aliases:  entry.Aliases,
	}
}

func upsertInstalledSLC(
	existing []config.InstalledSLC,
	entry config.InstalledSLC,
) []config.InstalledSLC {
	updated := make([]config.InstalledSLC, 0, len(existing)+1)
	for _, s := range existing {
		if strings.EqualFold(s.Flavor, entry.Flavor) {
			continue
		}
		updated = append(updated, s)
	}
	updated = append(updated, entry)
	slices.SortFunc(updated, func(a, b config.InstalledSLC) int {
		return cmp.Compare(a.Flavor, b.Flavor)
	})

	return updated
}

// findInstalledByImage returns the index of an installed SLC whose image reference exactly
// matches, or -1. SLC images are content-addressed (the tag embeds a content hash), so an
// identical reference means identical content — this is the shared no-op test used by both
// install and update.
func findInstalledByImage(installed []config.InstalledSLC, image string) int {
	for idx, inst := range installed {
		if inst.Image == image {
			return idx
		}
	}

	return -1
}

// The official builtin scan does not overwrite SCRIPT_LANGUAGES, so without this the alias
// would keep resolving to the custom SLC.
func checkOfficialAliasNotHeldByCustom(
	customs []config.InstalledCustomSLC,
	officialAliases []string,
) error {
	for _, custom := range customs {
		for _, official := range officialAliases {
			if strings.EqualFold(custom.Alias, official) {
				return fmt.Errorf(
					"alias %q is currently provided by a custom SLC; remove it with "+
						"`exasol slc remove %s` before installing this official SLC",
					strings.ToUpper(official), custom.Alias,
				)
			}
		}
	}

	return nil
}

func findInstalledSLC(installed []config.InstalledSLC, alias string) int {
	needle := strings.TrimSpace(alias)
	for idx, inst := range installed {
		if strings.EqualFold(inst.Language, needle) || strings.EqualFold(inst.Flavor, needle) {
			return idx
		}
		for _, declared := range inst.Aliases {
			if strings.EqualFold(declared, needle) {
				return idx
			}
		}
	}

	return -1
}
