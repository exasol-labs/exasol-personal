# runtime-artifact-cache Specification

## Purpose
TBD - created by archiving change add-runtime-artifact-cache. Update Purpose after archive.
## Requirements
### Requirement: Runtime artifacts SHALL be cached per user
The launcher SHALL maintain a per-user cache for runtime artifacts that can be reused across launcher operations.

#### Scenario: Runtime artifact is materialized on demand
- **WHEN** a launcher operation requires a runtime artifact that is not present in the user's cache
- **THEN** the launcher materializes the artifact before using it
- **AND** the artifact is recorded in the user's cache metadata

#### Scenario: Cached runtime artifact is reused
- **WHEN** a launcher operation requires a runtime artifact that is already present and valid in the user's cache
- **THEN** the launcher reuses the cached artifact
- **AND** the artifact is not downloaded again

### Requirement: Runtime artifact cache entries SHALL track last use
The launcher SHALL record when each cached runtime artifact was last used.

#### Scenario: New artifact records last use
- **WHEN** a runtime artifact is added to the user's cache
- **THEN** the cache metadata records a last-use timestamp for that artifact

#### Scenario: Cache hit updates last use
- **WHEN** a launcher operation reuses a cached runtime artifact
- **THEN** the cache metadata updates that artifact's last-use timestamp

### Requirement: Runtime artifact changes SHALL refresh cached artifacts
The launcher SHALL treat a cached runtime artifact as invalid when the artifact requested by the launcher no longer matches the artifact recorded in the cache.

#### Scenario: Changed artifact is refreshed
- **WHEN** a launcher operation requires a runtime artifact
- **AND** the user's cache contains an older or different artifact for the same runtime need
- **THEN** the launcher materializes the requested artifact before using it
- **AND** the previous cached artifact remains eligible for cleanup

### Requirement: Runtime artifact downloads SHALL be verified before use
The launcher SHALL verify downloaded runtime artifacts before making them available for use.

#### Scenario: Verification succeeds
- **WHEN** a runtime artifact is downloaded
- **AND** the downloaded artifact matches the expected integrity metadata
- **THEN** the launcher may record and use the artifact

#### Scenario: Verification fails
- **WHEN** a runtime artifact is downloaded
- **AND** the downloaded artifact does not match the expected integrity metadata
- **THEN** the launcher rejects the artifact
- **AND** the rejected artifact is not recorded as usable in the cache

### Requirement: Cache cleanup SHALL use configured retention
The launcher SHALL use per-user cache configuration to determine when cached runtime artifacts are old enough to be cleaned. This configuration SHALL be read from a `resources.yaml` file at the platform's conventional configuration location.

#### Scenario: Configured retention identifies stale artifacts
- **WHEN** cache cleanup evaluates cached runtime artifacts
- **THEN** artifacts whose last-use timestamp is older than the configured retention are treated as stale
- **AND** artifacts whose last-use timestamp is within the configured retention are preserved

#### Scenario: Missing retention configuration uses a default
- **WHEN** cache cleanup runs without an existing user retention configuration
- **THEN** the launcher uses a default retention value

### Requirement: Cache cleanup SHALL remove stale artifacts
The launcher SHALL remove stale runtime artifacts and their cache metadata during cleanup.

#### Scenario: Manual cleanup removes stale artifacts
- **WHEN** a user invokes cache cleanup
- **THEN** the launcher removes cached runtime artifacts that are stale
- **AND** the launcher reports the cleanup result

#### Scenario: Automatic cleanup is attempted during cache use
- **WHEN** the launcher successfully uses the runtime artifact cache
- **AND** automatic cleanup is due
- **THEN** the launcher attempts to remove stale cached runtime artifacts

### Requirement: Cache cleanup SHALL support corrupted artifact removal
The launcher SHALL provide a way to remove cached runtime artifacts that fail integrity verification.

#### Scenario: User cleans corrupted artifacts
- **WHEN** a user invokes cache cleanup for corrupted runtime artifacts
- **THEN** the launcher removes cached runtime artifacts that fail integrity verification
- **AND** the launcher reports the cleanup result

### Requirement: Cache cleanup SHALL support full cache removal
The launcher SHALL provide a way to remove all cached runtime artifacts.

#### Scenario: User removes all cached artifacts
- **WHEN** a user invokes cleanup for all cached runtime artifacts
- **THEN** the launcher removes all cached runtime artifacts
- **AND** the launcher reports the cleanup result

#### Scenario: Full cleanup removes unindexed cache contents
- **WHEN** a user invokes cleanup for all cached runtime artifacts
- **AND** the runtime artifact cache contains files or directories not referenced by cache metadata
- **THEN** the launcher removes those unreferenced cache contents
- **AND** the launcher resets runtime artifact cache metadata

### Requirement: Cache cleanup SHALL support partial download removal
The launcher SHALL provide a way to remove partial runtime artifact downloads that were not committed as usable cached artifacts.

#### Scenario: User cleans partial downloads
- **WHEN** a user invokes cache cleanup for partial downloads
- **THEN** the launcher removes partial runtime artifact downloads
- **AND** indexed cached runtime artifacts remain available
- **AND** the launcher reports the cleanup result

### Requirement: Cache cleanup SHALL support previewing cleanup
The launcher SHALL provide a way to preview selected cleanup work without changing cache contents or cache metadata.

#### Scenario: User previews indexed cleanup
- **WHEN** a user invokes cache cleanup in preview mode for cleanup that selects cached runtime artifacts
- **THEN** the launcher reports which indexed cached runtime artifacts would be removed
- **AND** cached runtime artifacts are not removed
- **AND** cache metadata is not changed

#### Scenario: User previews partial download cleanup
- **WHEN** a user invokes cache cleanup for partial downloads in preview mode
- **THEN** the launcher reports which partial runtime artifact downloads would be removed
- **AND** partial runtime artifact downloads are not removed
- **AND** cache metadata is not changed

### Requirement: Cache listing SHALL report cached artifacts
The launcher SHALL provide a way to list cached runtime artifacts.

#### Scenario: User lists cached artifacts
- **WHEN** a user requests the runtime artifact cache contents
- **THEN** the launcher reports each cached runtime artifact
- **AND** the report includes each artifact's last-use timestamp

#### Scenario: Empty cache is listed
- **WHEN** a user requests the runtime artifact cache contents
- **AND** no runtime artifacts are cached
- **THEN** the launcher reports that the cache is empty

### Requirement: Cache unlocking SHALL support stale-lock recovery

The launcher SHALL provide a way to clear a stale runtime artifact cache lock.

#### Scenario: User clears stale cache lock

- **WHEN** a user requests cache unlocking
- **THEN** the launcher clears the cache lock if one exists
- **AND** the launcher reports the unlock result as an operational notice on standard error

### Requirement: Cache diagnostics SHALL report cache state without mutation
The launcher SHALL provide runtime artifact cache diagnostics that inspect cache state without changing cache contents or cache locks.

#### Scenario: User inspects cache diagnostics
- **WHEN** a user requests runtime artifact cache diagnostics
- **THEN** the launcher reports cache state information
- **AND** cached runtime artifacts are not removed
- **AND** cache locks are not cleared

#### Scenario: Diagnostics report corrupted artifacts
- **WHEN** a user requests runtime artifact cache diagnostics
- **AND** one or more cached runtime artifacts fail integrity verification
- **THEN** cache diagnostics report those artifacts as corrupted
- **AND** cached runtime artifacts are not removed

#### Scenario: Diagnostics report cache problems
- **WHEN** runtime artifact cache metadata, configuration, integrity, or lock state cannot be interpreted normally
- **THEN** cache diagnostics report the observed problem
- **AND** diagnostics continue reporting any remaining cache state that can be inspected

### Requirement: Runtime artifact cache operations SHALL coordinate concurrent access
The launcher SHALL coordinate operations that read or mutate runtime artifact cache state so concurrent launcher processes do not corrupt cached artifacts or cache metadata.

#### Scenario: Concurrent cache mutation is serialized
- **WHEN** multiple launcher processes attempt to mutate runtime artifact cache state concurrently
- **THEN** only one mutation proceeds at a time

#### Scenario: Cache operation reports lock contention
- **WHEN** a cache operation cannot proceed because another process holds the cache
- **THEN** the launcher reports that the cache is currently locked

### Requirement: Runtime artifact cache SHALL support fetching from git repository sources
The runtime artifact cache SHALL accept git repository URLs (`git@`, `git://`, `https://*.git`, `http://*.git`) as artifact sources, cloning or updating the repository into the cache.

#### Scenario: Git repository source is cloned on first use
- **WHEN** a git repository source is requested and no cache entry exists for it
- **THEN** the cache SHALL clone the repository content into a new cache entry

#### Scenario: Git repository source is updated on subsequent use
- **WHEN** a git repository source is requested and a cache entry already exists
- **AND** the remote ref has advanced to a new commit
- **THEN** the cache SHALL update the cached clone to the new commit

#### Scenario: Git repository source is pinned by named ref
- **WHEN** a git repository source is requested with a branch or tag ref
- **THEN** the cache SHALL check out the content at that named ref

#### Scenario: Git repository source is pinned by commit SHA
- **WHEN** a git repository source is requested with a full 40-character commit SHA
- **THEN** the cache SHALL check out the content at that exact commit

#### Scenario: Git repository cache entry is reused on same commit
- **WHEN** a git repository source is requested
- **AND** the resolved commit hash matches an existing cache entry
- **THEN** the cache SHALL return the cached path without re-cloning

### Requirement: Runtime artifact cache SHALL identify git artifacts by resolved commit hash
The runtime artifact cache SHALL use the commit hash resolved at fetch time as the content identity for git sources, not a user-supplied checksum.

#### Scenario: Git artifact cache key uses commit hash
- **WHEN** a git artifact is stored in the cache
- **THEN** the cache entry records the resolved commit hash as the content identity

#### Scenario: Git artifact definitions SHALL NOT specify a checksum
- **WHEN** a statically defined git artifact specifies a checksum
- **THEN** the cache SHALL reject the definition with a configuration error

### Requirement: Runtime artifact cache SHALL support fetching from local filesystem sources
The runtime artifact cache SHALL accept `file://` URIs pointing to local directories or archive files as artifact sources.

#### Scenario: file:// directory source is returned directly
- **WHEN** a `file://` URI points to an existing local directory
- **THEN** the cache SHALL return the resolved absolute path directly without copying, extracting, or creating a symlink

#### Scenario: file:// archive source is extracted into the cache
- **WHEN** a `file://` URI points to a local archive file in a supported format
- **THEN** the cache SHALL extract the archive into a cache entry

#### Scenario: file:// path that does not exist returns an error
- **WHEN** a `file://` URI points to a path that does not exist
- **THEN** the cache SHALL return an error that includes the path

### Requirement: Runtime artifact cache SHALL support ZIP archive extraction
The runtime artifact cache SHALL extract `.zip` archives in addition to `.tar.gz`/`.tgz` archives when materialising archive-type artifacts.

#### Scenario: .zip archive artifact is extracted
- **WHEN** an artifact source resolves to a `.zip` archive
- **THEN** the cache SHALL extract the archive contents into the cache entry

#### Scenario: Unsupported archive format returns an error
- **WHEN** an artifact source resolves to a file with an unrecognised archive format
- **THEN** the cache SHALL return an error identifying the unsupported format

### Requirement: Runtime artifact cache SHALL extract directory entries within a tar.gz archive
The runtime artifact cache SHALL extract an explicit directory entry within a
tar.gz archive as a directory, in addition to the regular file and symlink
entries it already extracts.

#### Scenario: Directory entry is created
- **WHEN** a tar.gz archive being extracted contains an explicit directory
  entry
- **THEN** the cache SHALL create that directory in the extraction target
- **AND** extraction of the remaining entries in the archive SHALL continue

#### Scenario: Entries nested under an extracted directory entry are extracted
- **WHEN** a tar.gz archive contains a directory entry followed by regular
  file or symlink entries nested under that directory
- **THEN** the cache SHALL extract all of those nested entries into the
  created directory

### Requirement: Runtime artifact cache SHALL support platform-independent artifact definitions
The runtime artifact cache SHALL accept artifact definitions that use an `"any"` platform key, resolving that definition for any platform when no platform-specific variant is present.

#### Scenario: Platform-independent artifact is resolved for any platform
- **WHEN** an artifact definition specifies only an `"any"` platform key
- **THEN** the cache SHALL resolve that definition regardless of the current platform

#### Scenario: Platform-specific artifact takes precedence over platform-independent
- **WHEN** an artifact definition contains both a platform-specific key and an `"any"` key
- **THEN** the cache SHALL resolve the platform-specific variant for a matching platform

### Requirement: Runtime artifact cache SHALL always re-fetch archive artifacts without a checksum
An archive artifact definition that does not specify a checksum cannot be reliably cached by content identity. The runtime artifact cache SHALL re-fetch such artifacts on every request and SHALL replace any existing cache entry.

#### Scenario: No-checksum archive is re-fetched on every request
- **WHEN** an archive artifact with no checksum is requested
- **THEN** the cache SHALL re-fetch the artifact regardless of any existing cache entry

#### Scenario: No-checksum re-fetch is logged
- **WHEN** an archive artifact with no checksum is re-fetched
- **THEN** the cache SHALL emit a log message indicating the source is being re-fetched because no checksum was specified

### Requirement: Runtime artifact cache SHALL support runtime-constructed artifact definitions
The runtime artifact cache SHALL resolve artifact definitions that are constructed at runtime by callers, without requiring those definitions to be registered in the static resource catalog.

#### Scenario: Runtime-constructed definition is resolved
- **WHEN** a caller supplies an artifact definition directly at resolution time
- **THEN** the cache SHALL resolve and cache the artifact using that definition

#### Scenario: Runtime-constructed definition may omit a checksum
- **WHEN** a runtime-constructed archive artifact definition does not specify a checksum
- **THEN** the cache SHALL resolve the artifact applying the no-checksum re-fetch policy

### Requirement: A statically defined local-path artifact may omit a checksum
A statically defined `file://` or bare local-path artifact SHALL NOT be required to declare a
checksum, since its integrity comes from being part of the same versioned repository commit as
the launcher itself, not from a hand-authored value.

#### Scenario: Local-path artifact without a checksum is accepted
- **WHEN** a statically defined resource declares a `file://` or bare local-path artifact with
  no checksum
- **THEN** the specification is accepted

#### Scenario: Non-local artifact still requires a checksum
- **WHEN** a statically defined resource declares an artifact whose source is neither a git
  repository nor a local path, and specifies no checksum
- **THEN** the specification is rejected as invalid, exactly as before this capability existed

### Requirement: An embedded resource source materializes resources from data compiled into the binary
For resources marked `embed: true`, the resource manager SHALL resolve exclusively from embedded data compiled into the binary, never from network-based sources. For resources not marked `embed: true`, the resource manager SHALL resolve exclusively from its network-based sources, exactly as before this capability existed.

#### Scenario: Embedded data resolves a resource without network access
- **WHEN** a resource is marked `embed: true` and matching data is present in the binary's embedded registry
- **THEN** the resource manager materializes the resource from that embedded data without contacting any network source

#### Scenario: Missing embedded data is a hard failure, not a fallback
- **WHEN** a resource is marked `embed: true` but no matching data is present in the binary's embedded registry
- **THEN** the resource manager fails to resolve that resource, and does not attempt to resolve it from any network-based source

#### Scenario: A resource without embed: true never consults embedded data
- **WHEN** a resource is not marked `embed: true`
- **THEN** the resource manager resolves it only from its network-based sources, regardless of whether the binary's embedded registry contains data under the same resource identifier

#### Scenario: Embedded resource extraction reuses existing extraction
- **WHEN** an `embed: true` resource with archive extraction enabled is materialized from embedded data
- **THEN** the resource manager extracts it using the same extraction mechanism used for a network-fetched archive of that format

#### Scenario: Embedded resource data is not re-verified against a checksum at resolution time
- **WHEN** a resource is materialized from embedded data
- **THEN** the resource manager does not re-verify that data against the resource's configured checksum

### Requirement: The resource cache SHALL live under a platform-specific cache root
The cache root SHALL be a platform-conventional, launcher-owned cache directory, using the `resources` leaf name.

#### Scenario: Platform-specific cache root
- **WHEN** the launcher resolves the resource cache root
- **THEN** it uses `${XDG_CACHE_HOME:-~/.cache}/exasol/launcher/resources` on Linux, `~/Library/Caches/exasol/launcher/resources` on macOS, and `%LOCALAPPDATA%\exasol\launcher\resources` on Windows

### Requirement: The cache configuration file SHALL live at a platform-specific configuration location
The resource cache's configuration SHALL be one `resources.yaml` file per user, located at the platform's conventional configuration directory.

#### Scenario: Platform-specific cache config file location
- **WHEN** the launcher resolves the cache configuration file path
- **THEN** it uses `${XDG_CONFIG_HOME:-~/.config}/exasol/launcher/resources.yaml` on Linux, `~/Library/Application Support/exasol/launcher/resources.yaml` on macOS, and `%APPDATA%\exasol\launcher\resources.yaml` on Windows

### Requirement: An existing legacy cache config file SHALL be migrated to resources.yaml and then removed
On CLI startup, before any command records an operation in progress, an existing `runtime-artifacts.yaml` cache config file SHALL have its retention setting written to `resources.yaml`, and SHALL then be removed.

#### Scenario: Legacy cache config is migrated once
- **WHEN** a `runtime-artifacts.yaml` file exists at its legacy location
- **THEN** its `retention_days` value is written to the `retention_days` field of `resources.yaml`
- **AND** the legacy `runtime-artifacts.yaml` file is removed

### Requirement: The legacy resource cache directory SHALL be deleted once the new cache location is in use
The legacy resource cache directory SHALL be deleted outright; the new cache starts empty and repopulates on demand.

#### Scenario: Legacy cache is deleted
- **WHEN** CLI startup migration runs and a legacy resource cache directory exists
- **THEN** the legacy directory is deleted
- **AND** the new cache location contains only entries created after migration
