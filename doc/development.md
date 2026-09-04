# Development Guide

This guide provides detailed instructions for developers working on the Exasol Personal project.

## Prerequisites

### Required Tools

- **[Go](https://golang.org/doc/install)** - See `go.mod` for required version
- **[Python](https://www.python.org/downloads/)** - Required for integration and deployment tests
- **[Task](https://taskfile.dev/)** - Build automation tool
  ```bash
  # Install Task (recommended method)
  sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b ~/.local/bin
  
  # Or install from Go
  go install github.com/go-task/task/v3/cmd/task@latest
  ```
  
  Ensure the installation directory is in your `PATH`.

### Supported Platforms

Development is supported on:
- **Linux** (primary development platform)
- **macOS** (Intel and Apple Silicon)
- **Windows**

### Development Tools

The project uses the following tools (automatically managed by Go's tool dependencies):

- **[golangci-lint](https://golangci-lint.run/)** - Go linting and static analysis
- **[counterfeiter](https://github.com/maxbrunsfeld/counterfeiter)** - Mock generation for testing
- **[tflint](https://github.com/terraform-linters/tflint)** - Terraform/OpenTofu linting
- **[uv](https://docs.astral.sh/uv/)** - Python dependency management
- **[pytest](https://pytest.org/)** - Python testing framework
- **[ruff](https://docs.astral.sh/ruff/)** - Python linting and formatting
- **[mypy](https://mypy-lang.org/)** - Python type checking

These tools are invoked via Task commands and don't need manual installation.

## Getting Started

```bash
# Clone the repository
git clone https://github.com/exasol/exasol-personal.git
cd exasol-personal

# Generate code and stage embedded assets
task generate

# Build the binary
task build

# Run it
./bin/exasol version
```

## User Documentation

The published user documentation and integration tests are uv workspace members with one shared
locked dependency set and virtual environment at the repository root.

Install [uv](https://docs.astral.sh/uv/getting-started/installation/), then run the strict build. uv
synchronizes the locked workspace dependencies into the root `.venv` automatically:

```bash
task docs-build
```

The generated site is written to `user-docs/site/`. Warnings fail the build, including unresolved
internal documentation links. Run `task docs-check` to lint and test the publishing tooling before
performing the strict build.

Publication-request validation and version-catalog operations are implemented by
`user-docs/scripts/versions.py`. The script updates only a local `gh-pages` branch and never pushes;
use a disposable clone when exercising catalog operations manually.

## Building

### Standard Build

```bash
# Generate code and stage platform-specific embedded assets
task generate

# Build the binary
task build

# Result: bin/exasol (or bin/exasol.exe on Windows)
```

Builds apply the project's raw binary size optimization policy; see [Binary Size Optimization](binary_size.md).
For local debugger sessions, use `DEBUG_BUILD=true` with `task build` or `task build-windows` to preserve debug information.

### Cross-Compilation

Build for different platforms using Go's cross-compilation:

```bash
# Example: Build for Windows from Linux/macOS
GOOS=windows GOARCH=amd64 task build

# Example: Build for macOS Apple Silicon
GOOS=darwin GOARCH=arm64 task build
```

**Note:** The launcher resolves [OpenTofu](https://opentofu.org/) at runtime, so builds no longer need platform-specific OpenTofu downloads. The Exasol Local runner (macOS Apple Silicon only) is a different kind of resource: it's marked `embed: true` in `assets/resources/resources.yaml`, so it's fetched and baked directly into the binary at build time rather than resolved at runtime. `task build` stages it for whatever `GOOS`/`GOARCH` you're targeting automatically (as in the examples above). `task generate` itself always targets the host regardless of `GOOS`/`GOARCH` — pass `TARGET_GOOS`/`TARGET_GOARCH` to target a different platform's embedded resources on their own, without a full build:

```bash
task generate TARGET_GOOS=darwin TARGET_GOARCH=arm64
```

### Building Without Task

If you prefer to use Go commands directly (or Task is unavailable):

```bash
# Generate code, including embedded resources for the host platform
GOFLAGS=-tags=containers_image_openpgp go generate ./...

# Build the binary
go build -tags=containers_image_openpgp -o bin/exasol ./cmd/exasol

# For cross-compilation, stage that target's embedded resource with
# TARGET_GOOS/TARGET_GOARCH (not GOOS/GOARCH, which would cross-compile
# this `go run` itself instead of just picking the tool's target), then build
GOFLAGS=-tags=containers_image_openpgp TARGET_GOOS=windows TARGET_GOARCH=amd64 go generate ./...
GOOS=windows GOARCH=amd64 go build -tags=containers_image_openpgp -o bin/exasol.exe ./cmd/exasol
```

### Embedded resources: generation and local overrides

Resources marked `embed: true` in `assets/resources/resources.yaml` (e.g. `exasol-local-runner`) are fetched, checksum-verified, and baked into the binary at build time by `tools/resourceembedder`, generating build-tag-gated `.go` files under `assets/resources/generated/` (fully gitignored — nothing resource-specific is ever checked in). `task generate` (optionally parameterized by `TARGET_GOOS`/`TARGET_GOARCH`, used by `task build`'s cross-compile staging step and the release pipeline's per-target hook) performs a real, checksum-verified fetch for whatever platform it targets. `tests-unit`/`lint-golang`(-fix) instead depend on `task generate SKIP_EMBED=true`, which always writes an empty placeholder without fetching — they only need the package to compile, never the actual bytes. A platform with no declared artifact for a given resource (e.g. `exasol-local-runner` on anything but `darwin/arm64`) gets the same kind of placeholder automatically, with or without `SKIP_EMBED`. A resource marked `embed: always` instead of `embed: true` is exempt from `SKIP_EMBED` and is always fetched for real — for resources like the built-in presets (below), where code and tests need to resolve real content even when `SKIP_EMBED=true` is set to speed up an unrelated build.

To iterate locally on the embedded resource without waiting on the real network artifact, edit its `url` in `resources.yaml` to a local path (`file://` or a bare path) instead of the real URL, then re-run the generator and rebuild:

```yaml
exasol-local-runner:
  extract: true
  embed: true
  artifact:
    darwin/arm64:
      url: /path/to/local/mac-runner-aarch64.zip
      sha256: <sha256 of that local file, or leave the original value in place>
      resource_path: launcher
```

A few things worth knowing about this override:
- `url` can point at a directory, a supported archive (`.zip`, `.tar.gz`, `.tgz`), or a bare file — the local-path source redirects straight to whatever's there, uniformly. For an `extract: true` resource (like the committed `exasol-local-runner` entry above), it still needs to be something a registered `Extractor` can unpack, since `resource_path` picks the file out of the extracted result.
- To iterate on a locally-built `launcher` binary directly, without zipping it up each time, also set `extract: false` and drop `resource_path` for your local edit:

  ```yaml
  exasol-local-runner:
    extract: false
    embed: true
    artifact:
      darwin/arm64:
        url: /path/to/local/launcher
        sha256: <sha256 of that local file, or leave the original value in place>
  ```

  With `extract: false`, the bare binary is embedded and resolved exactly as given — no extraction, no `resource_path` lookup.
- `sha256` can be omitted for a local-path source entirely (its integrity comes from being part of the same versioned repository commit, not a hand-authored checksum) — or left in place unchecked, since it's not validated against a local file's content.
- This only affects what the *generator* embeds on its next run — the shipped binary always uses what's actually embedded in it over anything in `resources.yaml`, so a `file://` edit requires re-running the generator and rebuilding `exasol` to take effect.

### Glob-based resource groups

A resource entry can declare `glob: true` instead of a single fixed `resource_path`: the entry stays one resource — fetched, embedded, and resolved through the exact same pipeline as any other — but its own `resource_path` is a pattern to match against the resolved artifact's own entries (files or directories, uniformly across a local directory, git checkout, or extracted archive) rather than a literal subpath. Built-in infrastructure and installation presets are declared this way:

```yaml
infrastructure-presets:
  glob: true
  embed: always
  artifact:
    any:
      url: assets/infrastructure
      resource_path: "*"
      download_path: infrastructure-presets.tar.gz
```

`Manager.RequestMember(ctx, resourceID, member)` resolves a single matched entry: it resolves the group `resourceID` normally, then matches `member` against the result. `download_path` matters specifically for a `glob: true` + embedded resource: `tools/resourceembedder` always embeds a glob group as one archive of its matched entries (even when the live source is already a bare directory needing no archiving to read), so a recognizable archive extension is required for the extractor to unpack it again at runtime. `Manager.GroupMembers(resourceID)` lists the member names `tools/resourceembedder` found when it matched the group's `resource_path` at build time — that's how a running binary answers "what presets exist" without extracting the embedded archive just to glob within it.

Adding a new built-in preset directory under `assets/infrastructure/` or `assets/installation/` needs no `resources.yaml` edit — it's picked up automatically the next time resources are generated, since `resourceembedder` re-matches each `glob: true` entry's `resource_path` against its current directory tree. For this reason, adding a category of built-in resources this way (rather than one hand-written catalog entry per item) is the model to follow for any future built-in resource category shaped like "one entry per subdirectory."

## Development Workflow

### Essential Task Commands

View all available tasks:
```bash
task --list
```

### Typical Development Cycle

1. **Make code changes**

2. **Generate code** (if needed):
   ```bash
   task generate
   ```
   Run this after modifying generated code or interfaces.

3. **Format code**:
   ```bash
   task fmt
   ```

4. **Run linters**:
   ```bash
   task lint
   
   # Or auto-fix some issues
   task lint-golang-fix
   ```

5. **Run tests**:
   ```bash
   # Go unit tests
   task tests-unit
   
   # Python integration tests (requires test setup)
   task tests-integration
   ```

6. **Build**:
   ```bash
   task build
   ```

7. **Test manually**:
   ```bash
   ./bin/exasol <command>
   ```

### All-in-One

Run the full pipeline:
```bash
task all    # Runs lint, test, and build
```

## Testing

The project uses a combination of Go unit tests and Python integration/deployment tests. For detailed information about test types, strategy, and usage, see the [Testing README](../tests/README.md).

### Quick Reference

```bash
# Go unit tests
task tests-unit

# Python integration tests (no cloud resources)
task tests-integration

# Full deployment tests (requires AWS credentials, incurs costs)
task tests-deployment
```

## Code Quality

### Formatting

```bash
# Format all code (Go and Python)
task fmt
```

The project uses standard [Go](https://go.dev/) formatting (`go fmt`, `goimports`) and [Ruff](https://docs.astral.sh/ruff/) for Python.

### Linting

```bash
# Run all linters (Go, Python, Terraform)
task lint

# Auto-fix some Go linting issues
task lint-golang-fix
```

Configuration files:
- `.golangci.yml` - Go linting configuration
- `.tflint.hcl` - Terraform/OpenTofu linting
- `tests/pyproject.toml` and `user-docs/pyproject.toml` - Python linting and type checking

### Best Practices

See [Best Practices](best_practices.md) for project-specific coding guidelines and conventions.

## Common Issues

**OpenTofu binary not found:**
- OpenTofu is resolved at runtime through the per-user runtime artifact cache.
- Use `exasol diag cache` to inspect cache state, `exasol cache clean --invalid` to remove artifacts that fail integrity checks, and `exasol cache clean --partial-downloads` to remove interrupted downloads.
- For direct tofu invocations in development workflows, use `task fmt-terraform` or `go run ./tools/tofu/main.go ...`.

**Windows fails with `tofu init -lockfile=readonly` due to missing hashes in `.terraform.lock.hcl`:**
- Regenerate the lockfiles with hashes for all supported platforms: run `task tofu-lock-update`.
- This updates the committed lockfile(s) under `assets/infrastructure/` without leaving temporary `.terraform/` directories behind.
- Presets that don't use OpenTofu (no `tofu:` section in `infrastructure.yaml` or no `.tf` files) are skipped.

**Tests fail with AWS errors:**
- Verify AWS credentials are configured (`AWS_PROFILE` or credential files)
- Check AWS permissions

## Dependency Management

```bash
# Update all dependencies (Go and Python)
task deps-update
```

Standard [Go module](https://go.dev/ref/mod) commands also work for managing Go dependencies directly.

## CI/CD and Releases

The project uses GitHub Actions for continuous integration and automated releases.

**CI Pipeline** - See [CI Documentation](ci.md) for details on:
- Automated linting and testing on every push
- Manual deployment tests

**Releases** - See [Release Process](release.md) for how to create and publish releases.
