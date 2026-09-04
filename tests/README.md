# Testing Guide

This directory contains integration and deployment tests for the Exasol Personal launcher.

## Testing Principles

When adding or changing tests or test infrastructure, follow these principles:

**Match Existing Patterns** - Inspect the codebase before adding new tests. Follow established file organization, naming conventions, and test structure patterns - or if they don't exist, establish these patterns.

**Test Behavioral Space** - Cover the full range of behaviors: success cases, failure cases, edge cases, and repeated operations. One test per acceptance criterion is often insufficient.

**Test Contracts, Not Artifacts** - Focus on stable contracts and guarantees (what MUST always be true), not incidental implementation details (what happens to be true now).

**Separate Concerns** - Organize tests by the component or command they test. Related functionality belongs together; unrelated functionality should be separated.

**Use Clear Names** - Test names and docstrings should explain what behavior is being tested.

**Keep Tests Independent** - Each test should be runnable in isolation without depending on other tests.

### Given / When / Then Structure

Use the "Given / When / Then" pattern to make test intent explicit and consistent across the repo. Express these phases with short comments inside the test body:

- Given: setup and preconditions (fixtures, stubs, initial state)
- When: the action under test (a function call, a command execution, or a signal emitted)
- Then: assertions about observable behavior and outcomes (results, side effects, logs)

Guidelines:
- Keep the phases small and focused; avoid mixing setup and assertions.
- Prefer clear comments over lengthy names when the flow needs emphasis.
- For Go tests, add inline `// Given`, `// When`, `// Then` comments.
- For Python pytest tests, use `# Given`, `# When`, `# Then` comments.
- If a test has multiple actions and assertions, repeat `When/Then` pairs as needed.

## Test Strategy

The project uses multiple layers of testing to ensure quality at different levels:

### 1. Go Unit Tests

- **Location:** `*_test.go` files throughout the codebase
- **Purpose:** Test individual Go functions and packages in isolation
- **Run with:** `task tests-unit` (from project root)
- **Speed:** Fast (seconds)
- **Resources:** No external dependencies
- **When to use:** Test core logic, data structures, utility functions

### 2. Python Integration Tests

- **Location:** `tests/integration/`
- **Purpose:** Test CLI behavior and command interactions without cloud resources
- **Run with:** `task tests-integration` (from project root)
- **Speed:** Fast (seconds to minutes)
- **Resources:** No cloud resources provisioned
- **When to use:**
  - Verify CLI argument parsing and validation
  - Test command help output and error messages
  - Validate idempotency and state management
  - Check file creation and configuration handling
  
### 3. Python Deployment Tests (cloud and local)

Cloud tests provision real infrastructure and share one session-scoped deployment
(the `reusable_deployment` fixture). They are split by kind across three directories,
and each test is stamped with a kind marker matching its directory (`-m deployment`,
`-m e2e`, `-m chaos`):

- **`tests/deployment/`** — provisioning and lifecycle (stop/start, custom sizing, local VM ports, remote archive).
- **`tests/e2e/`** — read-only connect / query / output workflows against the running deployment.
- **`tests/chaos/`** — fault injection and recovery (interrupting in-flight lifecycle operations).

- **Run with:** `task tests-deployment` (runs all three), or the marker-scoped tasks below.
- **Speed:** Slow (10-30+ minutes)
- **Resources:** **Creates real cloud resources (incurs costs!)**
- **When to use:**
  - Validate complete deployment workflows
  - Test actual cloud provisioning
  - Test local deployment behavior on supported hosts
  - Verify deployed database functionality
  - Pre-release validation

**Warning:** Cloud tests provision actual infrastructure and will incur charges. Use sparingly and clean up resources after testing.

## Running Tests

### Prerequisites

```bash
# First time setup - install Python dependencies
task tests-setup
```

This synchronizes the locked uv environment needed for integration and deployment tests.
All integration and deployment tests are written using [pytest](https://pytest.org/).

### Running Tests

#### From Project Root (Recommended)

```bash
# Run all Go unit tests
task tests-unit

# Run Python integration tests
task tests-integration

# Run Python deployment tests (default: AWS)
task tests-deployment

# Run Python deployment tests with Azure preset
task tests-deployment INFRA=azure

# Run infrastructure-focused deployment tests
task tests-deployment-infrastructure INFRA=aws

# Run installation-focused end-to-end tests
task tests-deployment-installation INFRA=aws

# Run local deployment tests on a supported Linux or macOS host
task tests-deployment-local
```

#### From tests/ Directory

If you need more control over pytest execution:

```bash
cd tests

# Run all integration tests
uv run --locked --no-build pytest --exasol-path=../bin/exasol tests/integration

# Run specific test file
uv run --locked --no-build pytest --exasol-path=../bin/exasol tests/integration/test_cli.py

# Run specific test
uv run --locked --no-build pytest --exasol-path=../bin/exasol tests/integration/test_cli.py::test_init_idempotent

# Run with verbose output
uv run --locked --no-build pytest --exasol-path=../bin/exasol tests/integration -v

# Run all cloud tests (default: AWS preset)
export AWS_PROFILE=your-profile
uv run --locked --no-build pytest --exasol-path=../bin/exasol tests/deployment tests/e2e tests/chaos

# Run all cloud tests with Azure preset
uv run --locked --no-build pytest --exasol-path=../bin/exasol --infra=azure tests/deployment tests/e2e tests/chaos

# Run a single kind (directory or marker both work)
uv run --locked --no-build pytest --exasol-path=../bin/exasol tests/e2e
uv run --locked --no-build pytest --exasol-path=../bin/exasol -m "chaos" tests/deployment tests/e2e tests/chaos

# Run only infrastructure-focused tests
uv run --locked --no-build pytest --exasol-path=../bin/exasol --infra=aws -m "infrastructure_e2e" tests/deployment tests/e2e tests/chaos

# Run only installation-focused end-to-end tests
uv run --locked --no-build pytest --exasol-path=../bin/exasol --infra=aws -m "installation_e2e" tests/deployment tests/e2e tests/chaos
```

### Seeing the commands a test runs

Live logging is on at `INFO`, which does not include the individual launcher
invocations. Raise it to `DEBUG` and every command the suite executes is printed
before it starts, with its working directory when one is set:

```bash
uv run --locked --no-build pytest --exasol-path=../bin/exasol tests/integration --log-cli-level=DEBUG
```

```
DEBUG executing command: ../bin/exasol presets list --json
DEBUG executing command (cwd: /tmp/.../work): /.../exasol status --json --deployment staging
```

This covers every command regardless of how a test invokes it — the shared
`run_command` helpers, the `framework` launcher, and tests that reach for
`subprocess` directly — because the root `conftest.py` logs at the single point
they all funnel through. Nothing needs to be passed to the helpers to opt in.

## Continuous Integration

GitHub Actions CI coverage:
- Unit tests and integration tests run on every push
- Deployment tests run manually via workflow dispatch; the local suite has Linux AMD64 and macOS ARM64 lanes
- See [CI Documentation](../doc/ci.md) for details
