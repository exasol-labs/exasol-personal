# CI/CD

The project uses GitHub Actions for continuous integration and delivery. Workflow definitions are in `.github/workflows/`.

Security policy for public-repo CI is defined in [Repository Security and Automation Governance](repository_security_spec.md) and [CI Security Best Practices](ci_security_best_practices.md).

## Automated Workflows

### CI Pipeline (`ci.yml`)

Runs automatically on every push to `main` and on pull requests targeting `main` or a release
line under `release/`:

- **Go Linting** - Runs `golangci-lint` and `tflint`
- **Python Linting** - Runs `ruff` and `mypy` on integration tests and documentation tooling
- **Unit Tests** - Runs Go unit tests with coverage
- **Integration Tests** - Runs Python integration tests on Linux and Windows

Build and integration tests run on both `ubuntu-latest` and `windows-latest`, so Windows-only behavior — including Windows local deployments — is covered on every pull request. Unit tests and linting run on Linux only.

This is the only workflow that runs contributor code in pull request context. It is intentionally non-privileged and does not use deployment/release credentials.
All CI jobs declare explicit minimal permissions.

### Dependabot Auto-Merge (`dependabot-auto-merge.yml`)

Runs for Dependabot pull requests and enables auto-merge for Go and Python patch and minor version updates after required branch protection checks pass.
Eligibility is derived from the pull request's branch name: Go and Python updates that land on their ecosystem's grouped update branch (see `groups` in [`dependabot.yml`](../.github/dependabot.yml)) are routine and are approved immediately, since Dependabot cooldown already delayed their creation.
Any Go or Python Dependabot branch that falls outside its group is treated as a security update — Dependabot never assigns security updates to a group — and waits 7 days after PR creation before the workflow approves it and enables auto-merge, since Dependabot cooldown does not delay security updates.
GitHub Actions updates remain manual review items regardless of grouping.
Repository auto-merge must be enabled for this workflow to queue eligible pull requests.
The workflow uses `pull_request_target` only for pull request metadata (branch name, creation time, review status) and GitHub pull request operations; it does not check out or execute pull request code, and does not depend on Dependabot's commit-message metadata.
Scheduled rechecks run every 6 hours to pick up eligible open Dependabot PRs after the merge delay has elapsed.

Dependabot version updates use built-in cooldown periods before PR creation:

- GitHub Actions updates wait 7 days and remain manual.
- Go and Python patch updates wait 2 days.
- Go and Python minor updates wait 7 days.

The merge delay is separate from Dependabot cooldown: cooldown delays routine PR creation, while the workflow delay defers approval and auto-merge for ungrouped (security) PRs after they exist.
Auto-merge does not itself rebase Dependabot PRs; branch protection and Dependabot's own update behavior determine whether stale branches are refreshed before merging.

### Merge Workflow (`merge.yml`)

Runs automatically on every push to `main`:

- Builds binaries for Windows and macOS platforms
- Uploads build artifacts for verification

This ensures multi-platform compatibility is validated on the main branch.

### Orphaned Deployment Cleanup (`cleanup-orphaned-deployments.yml`)

Runs nightly (and on manual dispatch) to delete cloud resources left behind by failed or interrupted deployment tests, using the [cleanup tool](../tools/cleanup/README.md).
Deployments older than the configured age threshold are removed across AWS, Azure, and Exoscale.

Deployments in the region CI currently deploys to (`AWS_REGION`) are removed once they are a day old, as are Azure and Exoscale deployments.
AWS resources outlive a change of deployment region, so the regions CI deployed to previously are swept as well, but on a longer grace period of three days, since nothing left there belongs to a recent test run.
Each region runs as its own job with its own threshold, so add the outgoing region as a row whenever `AWS_REGION` changes, and drop a row once its region is known to be empty.
Splitting by region also keeps a deployment from matching two search targets at once, which the cleanup tool rejects: AWS reports global resources such as IAM roles in `us-east-1` regardless of which region holds the deployment's instances.

## Manual Workflows

### Release Pipeline (`release.yml`)

Maintainers dispatch this workflow with an existing version tag; pushing a tag does not trigger it.
It checks out the tag it is given and never inspects branches, so releasing from the default branch
and releasing a patch from a release line work the same way. Dispatch it with the default branch
selected, so that documentation publication is accepted by the `github-pages` environment.

Three jobs run in order:

- `validate-docs` builds the tag's user documentation with a strict MkDocs build and rejects a tag
  whose documentation version cannot be derived. Invalid documentation therefore fails the release
  before anything is published.
- `goreleaser` builds and signs binaries for Linux, macOS, and Windows and creates the GitHub
  release with its artifacts. It runs in the protected `release` environment, which gates
  publishing and signing behind approval.
- `publish-docs` calls the documentation workflow for the release tag. It runs only for a stable
  tag, so a pre-release publishes no documentation version. A failure here leaves the release
  published, and dispatching the documentation workflow for the same tag republishes the same
  documentation version.

Tests are not part of this workflow; they run in CI before the commit is tagged. See
[Release Process](release.md) for the maintainer steps.

### Documentation Publication (`docs.yml`)

Versioned user documentation is published by this workflow, either by the
[release pipeline](#release-pipeline-releaseyml) for a stable release or by a manual dispatch.
GitHub Pages must be enabled with **GitHub Actions** as its source, and the existing
`github-pages` environment must allow deployments only from `main`.

The workflow accepts four inputs:

- `operation`: `publish` or `delete`.
- `source_ref`: an existing Git tag, a branch under `release/`, or a full 40-character commit SHA,
  required for publication.
- `version`: the published documentation version, either a release line such as `2.2` or a full
  version such as `2.2.0-rc1`. It is required for deletion and optional for publication when
  `source_ref` is named `v<version>` or ends in `/v<version>`. A stable source version derives its
  release line, so `v2.2.1` publishes `2.2`, and a pre-release derives its full version.
- `make_latest`: whether the published version takes the `latest` alias and the site root.
  `auto` grants it when no higher stable version is published, `yes` and `no` decide explicitly.

An explicit version can map historical documentation content to an older release line without
changing an existing release tag. For example, publish a reviewed snapshot based on a current commit
as `2.1`, or map its full commit SHA by supplying `2.1` explicitly. It is also the way to publish a
full stable version such as `2.2.0`.

Documentation is published per release line. Releasing publishes the line from its release tag, and
a later correction is published from that line's release branch, such as `release/v2.2` published
as version `2.2`. Publishing the branch replaces the line from its current tip, so content and
styling can change after a release without moving any release tag, and a patch can be reviewed
together with the documentation it changes. Branches outside `release/` are rejected, which
keeps publication from being requested against development work. A name that exists as both a branch
and a tag is rejected until `source_ref` is qualified with `refs/heads/` or `refs/tags/`. Protect the
`release/*` branches with a ruleset, as described in
[CI security best practices](ci_security_best_practices.md).

Documentation for a line that is not released yet is published with `make_latest` set to `no`, and
takes the alias on a later publication once the release is out.

Publication shallow-checks out the source once and builds its self-contained `user-docs/` directory,
including its content, configuration, scripts, and locked uv environment, before updating the
version catalog. Because the snapshot supplies the publishing script, a source revision predating a
workflow input rejects the publication request before any published version changes.

The highest published stable version carries `latest` and the site root, so publishing
documentation for an older release leaves both untouched. Release candidates never take the alias
and remain independently selectable. Set `make_latest` to `yes` to move the alias onto an older
version, for example after withdrawing a release, or to `no` to publish a new stable version
without fronting it yet. Deletion preserves all other versions and rejects removal of the version
referenced by `latest` until a newer stable version is published.

The version selector marks the version carrying `latest`. It reads the configuration built into
each published version, so a version published before that configuration existed shows the marker
only after it is republished. Readers may see a stale selector for up to ten minutes after a
publication, because GitHub Pages caches the version catalog it fetches.

All operations are serialized in request order, including a publication requested by a release. If
Pages deployment fails after the version catalog is updated, rerun the same operation to deploy the
complete stored catalog. A repeated deletion succeeds when the selected version is already absent
and redeploys that catalog.

### Deployment Tests (`tests-deployment.yml`)

Full end-to-end tests that create real cloud or local deployments. These are expensive and slow, so they run only when needed:

**Trigger manually via:**
- GitHub Actions UI: [tests-deployment.yml](https://github.com/exasol/exasol-personal/actions/workflows/tests-deployment.yml) → "Run workflow"

Security guards:
- Runs only by manual trigger (`workflow_dispatch`)
- Uses OIDC and short-lived AWS credentials
- Should be protected by an environment approval gate and ref restrictions in repository settings

Workflow input:
- `suite`: test-suite selector (`all`, `cloud`, `local`; default `all`)
- `os`: OS selector for both cloud and local matrices (`all`, `ubuntu-latest`, `windows-latest`, `macos-latest`; default `all`)
- The workflow filters declarative cloud and local test plans before matrix expansion, so non-selected jobs are not created.
- Current enabled cloud rows:
  - AWS runs `tests-deployment` (installation + infrastructure lanes)
  - Azure runs `tests-deployment-infrastructure`
  - Exoscale runs `tests-deployment-infrastructure`
- Current enabled local rows:
  - Linux AMD64 runs `tests-deployment-local` on `ubuntu-latest`
  - Windows AMD64 runs `tests-deployment-local` on `windows-latest`
  - macOS ARM64 runs `tests-deployment-local` on the self-hosted virtualization runner
  - Linux ARM64 coverage is deferred.
  - The Windows row runs most of the local suite. Two pseudo-terminal cases remain POSIX-only, and the VM sizing cases remain macOS-only, so Windows covers 22 of the 27 selected tests.
- After pushing a branch, dispatch one local row with `task github:trigger-deployment-tests SUITE=local OS=ubuntu-latest` (or `OS=windows-latest`, `OS=macos-latest`), then verify deployment, tests, cleanup, and the final commit status.
- Credential bootstrap:
  - AWS via OIDC role assumption
  - Azure via OIDC (`azure/login`)
  - Exoscale via `EXOSCALE_API_KEY` / `EXOSCALE_API_SECRET` secrets
  - Azure identifiers are sourced from GitHub secrets: `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`

**Warning:** Cloud rows create real infrastructure and incur costs; local rows create a real database deployment on the selected runner.

### Integration Tests (`tests-integration.yml`)

The same integration suite the CI pipeline runs, dispatchable on its own when a targeted run is wanted without pushing a new commit or opening a pull request.

**Trigger manually via:**
- GitHub Actions UI: [tests-integration.yml](https://github.com/exasol/exasol-personal/actions/workflows/tests-integration.yml) → "Run workflow"
- After pushing a branch: `task github:trigger-integration-tests OS=windows-latest`

Workflow input:
- `os`: OS selector (`all`, `ubuntu-latest`, `windows-latest`; default `all`)

On Windows, a failing run also uploads Podman machine state, container inspection and logs, listening ports, and the test deployment directory as a diagnostics artifact.

## AWS Identity Provider and IAM Role for Deployment Tests

Deployment tests authenticate to the "exa-aws-dev-platform" AWS account using GitHub Actions’ OpenID Connect (OIDC). This avoids long‑lived AWS secrets and issues short‑lived credentials per workflow run.

What’s set up in AWS:
- An IAM OIDC identity provider `token.actions.githubusercontent.com` for GitHub with provider URL `https://token.actions.githubusercontent.com` and audience `sts.amazonaws.com`.
- An IAM role `PlatformGithubWorkflows` trusted by that OIDC provider. The role’s trust policy limits which repositories/branches/environments can assume it using conditions on `token.actions.githubusercontent.com:sub` and `token.actions.githubusercontent.com:aud`.

Where it’s used in CI:
- The workflow `tests-deployment.yml` configures AWS via a shared action that consumes two repository variables:
	- `AWS_CI_ROLE_PLATFORM` — ARN of the IAM role to assume in the platform account
	- `AWS_REGION` — target region for deployments
- The job permissions include `id-token: write` to allow OIDC token issuance.

Maintenance tips:
- Prefer least privilege: attach only the permissions required for deployment tests to the IAM role.
- Scope trust policies narrowly to this repository/branch/environment using the `sub` claim; adjust as the workflow structure evolves.
- When rotating roles or changing account setup, update the `AWS_CI_ROLE_PLATFORM` variable with the new role ARN; for region changes, update `AWS_REGION` and add the outgoing region to the cleanup workflow's matrix.
- Audit and monitor with AWS CloudTrail; review trust and permission policies regularly.

Authoritative references:
- GitHub Docs — Configuring OIDC in AWS: https://docs.github.com/en/actions/deployment/security-hardening-your-deployments/configuring-openid-connect-in-amazon-web-services
- AWS IAM — Creating OIDC identity providers: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_create_oidc.html
- AWS IAM — Configuring a role for GitHub OIDC: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_create_for-idp_oidc.html#idp_oidc_Create_GitHub
- Action — aws-actions/configure-aws-credentials: https://github.com/aws-actions/configure-aws-credentials

## Governance Controls

Changes to workflow definitions, shared GitHub Actions, and CI security documents are protected by [CODEOWNERS](../.github/CODEOWNERS).
GitHub Actions dependencies are updated automatically via [Dependabot](../.github/dependabot.yml).
