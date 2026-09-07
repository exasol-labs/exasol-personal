# AI Agent Guidelines

Always read everything that is linked here at least once.

## Development Guidelines

**General development** - Follow the guide in [doc/development.md](doc/development.md) to understand how to develop and verify code changes.

**Testing** - Follow testing principles and patterns documented in [tests/README.md](tests/README.md). Apply those principles by inspecting existing test organization before adding new tests.

## Repository Workflow

At the start of any task that may modify the repository, read the [Code and Writing Guidelines](CONTRIBUTING.md#code-and-writing-guidelines) before planning or editing. If the task may produce commits, also read the [Commit Guidelines](CONTRIBUTING.md#commit-guidelines) before planning task or commit boundaries. When working with OpenSpec, read the [OpenSpec Guidelines](CONTRIBUTING.md#openspec-guidelines). Follow the rest of the contribution process in [CONTRIBUTING.md](CONTRIBUTING.md) before creating a branch, committing, or opening a pull request.

When preparing a branch or pull request, ask the user for a related Jira or GitHub issue ID if they have not already provided one; when provided, include it in the branch name and pull request description.

For non-trivial changes, propose using OpenSpec even if the user has not asked for it; for OpenSpec-backed work, propose archiving the completed change before committing and opening a pull request.

## Documentation Guidelines

Guidelines for AI agents working with documentation in this repository.

This repository has two documentation trees. `user-docs/` holds the published user documentation, built and validated as described in [doc/development.md](doc/development.md#user-documentation); `doc/` holds contributor documentation that stays in the repository. Route a change to the tree whose audience it serves, and document user-visible behavior, errors, and workflows in `user-docs/`.

**Summary:** Link don't duplicate, keep it high-level, separate concerns clearly, be concise, avoid specifics that change, respect ownership boundaries.

### Avoid Duplication
- Don't repeat content across documents
- Link to authoritative sources instead of copying and replicating info
- Each piece of information has one primary home
- Reference docs and tool documentation inline when mentioned, not in appendix sections

### Keep It High-Level
- Focus on "why" and "what", not "how" in architecture docs
- Avoid implementation details (such as code file paths, ports, package names, specific versions of dependencies)
- Don't include diagrams (folder structures, code organization) because maintaining them is hard

### Be Concise
- Single sentences over bullet lists where possible
- Remove redundant explanations
- Get to the point quickly

## Document Responsibilities

**[README.md](README.md)** - Project entry point: what Exasol Personal is, how to install the launcher, where to find the user documentation

**[user-docs/](user-docs/)** - Published user documentation: getting started, deployment, database usage, troubleshooting, system requirements

**[CONTRIBUTING.md](CONTRIBUTING.md)** - Contribution process, behavioral guidelines (link to details)

**[doc/development.md](doc/development.md)** - Development workflow, Task commands, tool purposes

**[doc/architecture.md](doc/architecture.md)** - Design philosophy, technical decisions, high-level workflows

**[doc/glossary.md](doc/glossary.md)** - Term definitions (link to architecture for depth)

**[doc/best_practices.md](doc/best_practices.md)** - Project-specific coding conventions and guidelines

**[doc/ci.md](doc/ci.md)** - CI/CD workflows, when they run, how to trigger them

**[doc/release.md](doc/release.md)** - Release process, versioning, automation details
