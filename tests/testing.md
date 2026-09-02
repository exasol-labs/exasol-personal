

# Deployment test suites

Cloud tests share one session-scoped deployment (the `reusable_deployment` fixture in
`tests/tests/conftest.py`) and are split by kind across three directories. Each test is
also stamped with a kind marker (`e2e`, `deployment`, `chaos`) matching its directory, so
`-m e2e` / `-m deployment` / `-m chaos` select a whole kind. They can be triggered using
`task github:trigger-deployment-tests`.

## E2E Testing

Read-only connect / query / output workflows against the running deployment.

`tests/tests/e2e/test_connect_query.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_connectable` | cloud | Confirms the deployed database is reachable |
| `test_single_query` | local, cloud | Runs a basic `SELECT * FROM Dual` query to confirm the DB is reachable and functional |
| `test_exit_command` | local, cloud | Confirms the `exit` command properly terminates the interactive shell session |
| `test_multiple_queries` | local, cloud | Runs a sequence of DDL + DML operations (create schema, create table, insert rows, select) |
| `test_file_import` | local, cloud | Tests CSV file import into a table |
| `test_connect_table_width` | local, cloud | Validates query output formatting with varying column widths and long content |
| `test_connect_interactive_shows_version_and_exit_hint` | local, cloud | Verifies that interactive mode displays the Exasol version string and an `exit` hint on startup |
| `test_diag_cos_runs_confd_client` | cloud | Runs COS diagnostic commands (skipped for local infra, which uses a VM shell fallback) |
| `test_license_session_limit` | local, cloud | Confirms the Exasol Personal license enforces a 20 concurrent session cap |
| `test_password_marker_not_leaked_to_logs` | local, cloud | Verifies a failing connection never echoes the DB password into output or logs |
| `test_connect_shows_exit_hint` | local, cloud | Verifies an interactive connection prints the "how to exit" banner |
| `test_invalid_sql_does_not_crash_shell` | local, cloud | Verifies an invalid statement reports an error but keeps the shell alive |
| `test_many_statements_remain_stable` | local, cloud | Runs 50 small statements in one session to confirm no crash or hang |

`tests/tests/e2e/test_import_local_data.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_import_csv_missing_file_reports_client_side` | local, cloud | Verifies a missing CSV path fails client-side with a clear "file not found" error |
| `test_import_csv_uses_local_filesystem` | local, cloud | Verifies `IMPORT FROM LOCAL CSV` reads from the client filesystem, not the database node |
| `test_import_parquet_uses_local_filesystem` | local, cloud | Verifies `IMPORT FROM LOCAL PARQUET` reads a Parquet file from the client filesystem and imports its rows |
| `test_import_parquet_missing_file_reports_client_side` | local, cloud | Verifies a missing local Parquet path fails client-side with a clear "file not found" error |
| `test_import_large_csv_completes_or_fails_actionably` | local, cloud | Verifies a large CSV import either completes or fails with actionable guidance; size and timeout are tunable via `EXASOL_STRESS_CSV_MB` and `EXASOL_STRESS_TIMEOUT_S` |

## Deployment Testing

Provisioning and lifecycle behavior.

`tests/tests/deployment/test_virtual_schema.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_postgresql_virtual_schema_create_query_and_refresh` | local | Verifies a JDBC adapter can be staged, registered, queried, and refreshed against PostgreSQL |

See the [Virtual schemas on local deployments](../doc/virtual_schemas.md) guide for the setup
and execution details covered by this smoke test.

`tests/tests/deployment/test_deploy_lifecycle.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_stop_and_start` | cloud | Verifies the stop → start lifecycle and that `info` reports the correct cluster state at each step |
| `test_remote_archive_registered` | cloud | Verifies a remote archive volume is registered and exposed via the Admin UI backup options |

`tests/tests/deployment/test_deploy_ops.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_deploy_is_idempotent` | cloud | Verifies re-running deploy on a healthy cluster is a clean no-op or fails with actionable guidance |
| `test_info_includes_connection_details` | cloud | Verifies `info` surfaces the SQL host, port and Admin UI URL of a deployed cluster |
| `test_destroy_removes_deployment` | cloud | Verifies destroy removes the cloud resources and clears deployment state |

`tests/tests/deployment/test_object_storage.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_minimal_iam_policy_includes_bootstrap_bucket_actions` | cloud (AWS) | Verifies the documented minimal IAM policy covers the object-storage bootstrap actions |
| `test_first_run_downloads_then_reuses_opentofu` | cloud | Verifies OpenTofu is downloaded on first use and reused from the cache afterwards |

The manually dispatched local suite runs its portable cases on Linux AMD64 and
macOS ARM64. VM-specific and historical-update cases remain guarded for macOS.

`tests/tests/deployment/test_local_deployment.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_ports_override_sets_db_port` | local | Verifies that `--ports db:<port>` correctly routes the DB port through the selected local runtime; confirms the DB is reachable on the custom port |
| `test_ports_override_stable_across_restarts` | local | Verifies that a custom DB port assigned at `exasol init` is preserved in `deployment.json` and remains reachable after a stop/start cycle |
| `test_static_local_port_selection_reconfiguration_and_recovery` | local | Verifies deterministic automatic selection, stable restart ports, stopped reconfiguration, occupied-port failure, and recovery with a new automatic port |

`tests/tests/deployment/test_local_update.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_historical_local_update_preserves_committed_data` | local | Verifies updates from supported historical macOS ARM64 launchers preserve committed database rows through runner replacement and Nano-data migration |

`tests/tests/deployment/test_local_vm.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_full_local_deployment_lifecycle` | local | Walks a real local VM through init → deploy → stop → start → destroy |
| `test_memory_default_is_half_host_ram` | local | Verifies the default VM memory is derived from host RAM and honours the supported minimum |
| `test_local_deployment_json_is_endpoint_based` | local | Verifies `deployment.json` describes a local deployment as a single endpoint rather than a node list |
| `test_node_access_key_is_openssh_and_legacy_key_is_regenerated` | local | Verifies the node access key is written in OpenSSH format and a legacy-format key is regenerated |
| `test_allow_unsupported_escape_hatch` | any | Verifies the test-only escape hatch allows `init local` on an unsupported platform |

`tests/tests/deployment/test_slc.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_slc_list_reports_catalog_containers` | local, cloud | Verifies `slc list` reports the catalog in both text and JSON regardless of what is installed |
| `test_slc_install_rejects_unknown_alias` | local, cloud | Verifies an unknown alias fails before any restart and leaves install state unchanged |
| `test_slc_remove_when_not_installed_is_noop` | local, cloud | Verifies removing an SLC that is not installed succeeds without restarting the database |
| `test_official_slc_install_runs_udf` | local, cloud | Verifies installing an official SLC makes its UDFs runnable and that reinstalling is a no-op |
| `test_official_slc_remove_uninstalls_language` | local, cloud | Verifies removing an installed official SLC clears its status and makes its UDFs fail |
| `test_slc_install_no_restart_activates_on_next_start` | local, cloud | Verifies `--no-restart` records the SLC without applying it, and the next start mounts it |
| `test_slc_update_when_current_is_noop` | local, cloud | Verifies updating an SLC already at the catalog version is a no-op and leaves it usable |

`tests/tests/deployment/test_custom.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_custom_deployment_rejects_small_instance_types` | cloud | Verifies custom deployments reject instance types below the minimum sizing (parametrized per provider) |
| `test_custom_deployment_success` | cloud | Verifies a custom-configured deployment provisions successfully and serves queries |

## Chaos Testing

Fault-injection and recovery. Each chaos test restores the deployment to a
database-ready state on exit so the other cloud suites can rely on a running cluster.

`tests/tests/chaos/test_lifecycle_faults.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_start_interrupt_sets_interrupted_state` | cloud | Interrupts in-flight stop and start operations and asserts the deployment reaches the `interrupted` state, then recovers |
| `test_reconcile_vm_state_after_improper_shutdown` | local | Verifies VM state is reconciled after the local VM is shut down outside the launcher |
| `test_deploy_can_be_interrupted_and_recovered` | local, cloud | Kills an in-flight deploy and verifies the next deploy either resumes to database-ready or reports recovery guidance |

`tests/tests/chaos/test_faults.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_deploy_fails_clearly_with_invalid_aws_credentials` | cloud (AWS) | Verifies invalid credentials produce an authentication error and leave no dangling resources |

# Integration Testing

Integration tests can be run using `task tests-integration` or in CI using `task github:trigger-integration-tests`. They run on Ubuntu and Windows in CI.

## CLI

`tests/tests/integration/test_cli.py`

| Test | Description |
|------|-------------|
| `test_help_flag` | Tests that the `--help` flag outputs correct help and usage information |
| `test_help_flag_surfaces_local_preset_and_quick_start` | Verifies root help advertises the local preset and the quick-start path |
| `test_help_output_never_has_more_than_one_blank_line` | Verifies help output never contains consecutive blank lines |
| `test_help_usage_section_is_indented_and_separated` | Verifies the usage section is indented and separated consistently for root, parent and leaf commands |
| `test_version` | Verifies the version command outputs the git tag version correctly |
| `test_version_json` | Tests version command with JSON output format |
| `test_info_command_exists` | Verifies the info command is available and shows proper help |
| `test_info_reports_missing_deployment_without_error` | Verifies info reports state on stdout and next-step guidance on stderr when no deployment exists |
| `test_info_json_reports_missing_deployment_without_error` | Verifies JSON info reports structured state and omits `message`, `actions` and `connection` when the deployment is missing |
| `test_info_command_init_deployment` | Tests info command output after initialization |
| `test_help_uses_inclusive_phrasing` | Verifies `destroy`, `remove` and `install` help avoids non-inclusive wording |
| `test_destroy_help_mentions_deployment_resources` | Verifies destroy help states that it removes deployment resources |
| `test_help_consistent_in_powershell_and_cmd` | Verifies `--help` output and exit code match between PowerShell and CMD on Windows |
| `test_unknown_flag_exits_nonzero_with_usage` | Verifies an unsupported flag exits non-zero with usage and no stack trace |

## Presets

`tests/tests/integration/test_presets.py`

| Test | Description |
|------|-------------|
| `test_presets_help_mentions_subcommands` | Verifies the presets command shows list and export subcommands |
| `test_presets_list_outputs_sections` | Tests that presets list shows infrastructure and installation headers |
| `test_presets_list_json_is_valid` | Tests presets list with JSON output format |
| `test_presets_export_writes_files` | Tests exporting infrastructure and installation presets to a directory |
| `test_presets_export_fails_on_non_empty_dir` | Tests that export fails when the target directory is not empty |

## External Presets

`tests/tests/integration/test_external_presets.py`

| Test | Description |
|------|-------------|
| `test_init_accepts_file_uri_directory_infra_preset` | Verifies init accepts a `file://` infrastructure preset directory |
| `test_init_accepts_file_uri_tar_gz_infra_preset` | Verifies init accepts a `file://` `.tar.gz` infrastructure preset archive |
| `test_init_accepts_file_uri_zip_infra_preset` | Verifies init accepts a `file://` `.zip` infrastructure preset archive |
| `test_init_accepts_file_uri_directory_install_preset` | Verifies init accepts a `file://` installation preset directory as the second argument |
| `test_unknown_preset_name_error_includes_available_names` | Verifies an unknown preset name error lists the available preset names |
| `test_file_uri_nonexistent_path_returns_error` | Verifies a `file://` URI pointing at a missing path returns an error |
| `test_file_uri_plain_file_without_manifest_returns_error` | Verifies a plain file that is neither a preset directory nor an archive returns an error |
| `test_at_ref_on_non_git_url_returns_error` | Verifies an `@ref` git-style suffix on a non-git URL returns an error |
| `test_init_accepts_http_tar_gz_infra_preset` | Verifies init accepts a `.tar.gz` infrastructure preset served over HTTP |
| `test_init_accepts_http_zip_infra_preset` | Verifies init accepts a `.zip` infrastructure preset served over HTTP |

## Backend Compatibility

`tests/tests/integration/test_backend.py`

| Test | Description |
|------|-------------|
| `test_unknown_backend_is_rejected` | Verifies a preset manifest declaring an unknown backend is rejected |
| `test_compatible_pair_succeeds` | Verifies a compatible infrastructure/installation preset pair initializes successfully |
| `test_incompatible_pair_rejected_before_mutation` | Verifies an incompatible preset pair is rejected before the deployment directory is touched |
| `test_compatibility_matrix_rendered` | Verifies the backend compatibility matrix is rendered by `presets list` and `init --help` |
| `test_help_subcommand_matches_help_flag` | Verifies `help <command>` and `<command> --help` produce the same preset help |
| `test_debug_build_is_larger_than_release_build` | Verifies the release build is size-optimized relative to a debug build |

## Cache

`tests/tests/integration/test_cache.py`

| Test | Description |
|------|-------------|
| `test_cache_list_text_output` | Verifies `cache list` renders the cache as text |
| `test_cache_list_json_is_valid` | Verifies `cache list --json` emits parseable JSON |
| `test_cache_clean_dry_run_removes_nothing` | Verifies a dry-run clean reports what it would do without removing anything |
| `test_cache_clean_selectors_are_mutually_exclusive` | Verifies incompatible `cache clean` selectors are rejected |
| `test_cache_clean_reports_mode_summary` | Verifies each real cleanup mode reports a summary of what it removed |
| `test_cache_unlock_reports_cleared` | Verifies `cache unlock` confirms the lock was cleared as a terminal notice on stderr |
| `test_diag_cache_reports_status_fields` | Verifies `diag cache` reports the expected status fields |
| `test_diag_cache_does_not_mutate` | Verifies `diag cache` leaves the cache contents unchanged |
| `test_default_output_is_quiet_but_debug_and_log_are_verbose` | Verifies default output stays quiet while `--log-level debug` and log files are verbose |

## Connect CLI

`tests/tests/integration/test_connect_cli.py`

| Test | Description |
|------|-------------|
| `test_help_describes_invocation_json_contract` | Verifies connect help documents the invocation modes and the JSON output contract |
| `test_invalid_json_format_is_rejected` | Verifies an invalid `--json` format value is rejected |
| `test_command_and_file_are_mutually_exclusive` | Verifies `--command` and `--file` cannot be combined |
| `test_json_and_csv_are_mutually_exclusive` | Verifies `--json` and `--csv` cannot be combined |
| `test_connect_without_deploy_fails_clearly` | Verifies connecting to an initialized-but-not-deployed directory fails with a clear error |
| `test_connect_with_corrupt_deployment_info_fails_clearly` | Verifies a corrupt deployment info file produces a clear error rather than a crash |

## Deployment Directory Resolution

`tests/tests/integration/test_deployment_directory_resolution.py`

| Test | Description |
|------|-------------|
| `test_status_uses_default_deployment_dir_without_corrupting_json` | Tests default deployment directory resolution with JSON output |
| `test_status_uses_named_deployment_dir_without_corrupting_json` | Verifies `--deployment <name>` resolution keeps stdout parseable JSON |
| `test_status_reports_uninitialized_named_deployment_dir` | Verifies status reports `not_initialized` for an unused named deployment |
| `test_named_deployment_dir_wins_over_current_directory` | Verifies `--deployment` takes precedence over a recognized current directory |
| `test_deployment_dir_and_deployment_are_mutually_exclusive_before_any_side_effect` | Verifies the two targeting flags cannot be combined, and no directory or log file is created when they are |
| `test_deployment_shorthand_wins_over_current_directory` | Verifies the `--deployment` shorthand also wins over the current directory |
| `test_deployment_flag_rejects_invalid_characters` | Verifies deployment names with invalid characters are rejected |
| `test_status_reports_uninitialized_explicit_deployment_dir` | Tests explicit deployment directory handling |
| `test_status_debug_logs_current_deployment_dir` | Tests debug logging of the current deployment directory |
| `test_init_creates_default_deployment_dir` | Tests that init automatically creates the default deployment directory |
| `test_init_creates_named_deployment_dir` | Verifies init creates the directory for a named deployment |
| `test_init_refuses_different_preset_in_named_deployment_dir` | Verifies init refuses to switch presets inside an existing named deployment |
| `test_info_reports_uninitialized_resolved_default_dir` | Tests info command output with a resolved default directory |
| `test_status_reports_resolved_default_dir` | Verifies status reports `not_initialized` against the resolved default directory |
| `test_init_without_flag_uses_default_dir` | Verifies init with no targeting flag initializes the default directory |
| `test_status_resolves_current_deployment_dir` | Verifies status resolves the current directory both implicitly and via `--deployment-dir .` |

## Deployments List

`tests/tests/integration/test_deployments_list.py`

| Test | Description |
|------|-------------|
| `test_deployments_list_json_is_empty_array_when_none_exist` | Verifies the JSON listing is an empty array when no deployments exist |
| `test_deployments_list_reports_named_deployments` | Verifies named deployments are listed with their initialization state |
| `test_deployments_list_marks_active_entry_from_current_directory` | Verifies the entry matching the current directory is marked active |
| `test_deployments_list_does_not_accept_deployment_dir_or_deployment` | Verifies the command rejects the deployment-targeting flags |

## Init

`tests/tests/integration/test_init.py`

| Test | Description |
|------|-------------|
| `test_init_defaults_and_help` | Tests that init help displays preset names and references |
| `test_init_requires_infra_preset_arg` | Tests that init requires an infrastructure preset argument |
| `test_init_succeeds` | Tests successful init including EULA display |
| `test_init_creates_deployment_dir` | Tests that init creates the deployment directory |
| `test_init_allows_deployment_dir_flag_before_preset_arg` | Tests that the deployment dir flag can appear before the preset argument |
| `test_init_fails_on_non_empty_directory` | Tests that init rejects a non-empty target directory |
| `test_init_idempotent` | Tests that init can be re-run without modifying an already-initialized deployment |
| `test_init_accepts_infra_preset_path` | Tests init with an exported preset directory path |
| `test_init_accepts_install_preset_path_as_second_arg` | Tests passing an installation preset as a second argument |
| `test_init_performs_version_check` | Tests that init performs a launcher version check |
| `test_init_skips_version_check` | Tests that `--no-launcher-version-check` suppresses the version check |
| `test_init_in_non_writable_dir_fails_with_clear_error` | Verifies init into a non-writable directory fails with an actionable error |

## Install

`tests/tests/integration/test_install.py`

| Test | Description |
|------|-------------|
| `test_install_requires_infra_preset_arg` | Tests that the install command requires a preset argument |
| `test_install_help` | Tests install help documentation |
| `test_install_executes_init_step` | Tests that install executes the init step and surfaces failures correctly |
| `test_init_local_rejects_unsupported_platform_before_writing_files` | Tests platform validation for local deployments |
| `test_init_local_host_help_exposes_only_host_parameters` | Tests that the Linux and Windows host runtimes expose no VM sizing flags |
| `test_init_local_host_configuration_omits_vm_sizing` | Tests that Linux and Windows host runtime configuration contains only host-relevant options |
| `test_init_local_accepts_explicit_minimum_memory` | Tests minimum memory configuration for local deployments |
| `test_init_local_rejects_memory_below_minimum` | Tests that memory below the minimum is rejected before any files are written |
| `test_deploy_local_with_fake_runner_override` | Tests local deployment end-to-end using a fake runner script |
| `test_windows_lifecycle_flags_are_accepted` | Tests that every CLI flag the Windows lifecycle test uses exists, on all platforms |
| `test_install_local_windows_lifecycle` | Tests Windows Podman preparation, SQL readiness, stop/start persistence, recovery after the Podman machine is stopped out of band, and destroy cleanup |

## Reconfiguration

`tests/tests/integration/test_reconfiguration.py`

| Test | Description |
|------|-------------|
| `test_config_set_updates_same_preset_variables` | Tests that config set preserves infrastructure state when updating variables |
| `test_init_updates_same_preset_variables` | Tests re-running init with changed options |
| `test_config_get_outputs_active_configuration` | Tests querying the active configuration as JSON |
| `test_config_reset_restores_selected_defaults` | Tests resetting individual configuration options to their defaults |
| `test_config_reset_all_restores_all_defaults` | Tests resetting all options to their defaults |
| `test_config_set_refuses_running_deployment` | Tests that config set is rejected while a deployment is running |
| `test_config_set_refuses_state_with_possible_resources` | Tests config set rejection for failed/interrupted deployment states |
| `test_install_updates_same_preset_configuration_before_retry` | Tests that install preserves state while applying configuration updates on retry |
| `test_install_refuses_same_preset_configuration_change_for_running_deployment` | Tests that install refuses configuration changes to a running deployment |
| `test_init_refuses_different_preset_without_remove` | Tests that init rejects switching presets without first removing the existing deployment |
| `test_install_refuses_different_preset_without_removing_local_state` | Tests that install validates preset switching and rejects without removal |
| `test_destroy_remove_removes_local_deployment_directory` | Tests destroy with local state cleanup |
| `test_remove_removes_local_deployment_directory_without_destroy` | Tests removing a deployment without destroying cloud resources |
| `test_remove_refuses_non_deployment_directory` | Tests that remove validates the target is a deployment directory |
| `test_install_retries_same_preset_after_failed_state` | Tests install retry with preserved state after a failure |

## Launcher Framework

`tests/tests/integration/test_launcher_framework.py`

| Test | Description |
|------|-------------|
| `test_has_no_deployment_accepts_info_guidance_on_stderr` | Verifies the shared test helper accepts info's split output: state on stdout, next-step guidance on stderr |
