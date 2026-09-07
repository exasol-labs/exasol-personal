# Contributing to Exasol Personal

Thank you for your interest in contributing! We welcome contributions from the community.

## How to Contribute

1. **Fork the repository** and clone your fork
2. **Set up your development environment** - see [Development Guide](doc/development.md)
3. **Create a branch** for your changes: `git checkout -b feature/your-feature-name`
4. **Make your changes** following our [coding guidelines](doc/best_practices.md)
5. **Test your changes** - see [Development Guide](doc/development.md#testing)
6. **Submit a pull request**

## Code and Writing Guidelines

- **Keep code comments rare and concise** - add comments only for non-obvious current invariants, constraints, or workarounds. Do not explain obvious behavior, repeat names, describe historical changes or rejected alternatives, or refer to tools, tickets, tasks, or OpenSpec artifacts.
- **Keep task, issue, and pull request descriptions concise and outcome-focused** - put implementation mechanisms in supporting details or acceptance criteria. Avoid unrelated scope, empty sections, and em dashes.

## OpenSpec Guidelines

- **Use positive, observable requirements** - specify what users can verify at a stable boundary, never negative requirements, internal mechanisms, or what must remain unchanged.
- **Keep requirements and scenarios precise** - state the relevant condition and its observable outcome, using measurable limits where needed.
- **Avoid overlap** - give each behavior one canonical specification; unchanged behavior is implied by its existing, unmodified specification.

## Commit Guidelines

- **Use [Conventional Commits](https://www.conventionalcommits.org/)** - capitalize the subject after the type and omit the commit body unless the contribution process or a maintainer specifically requests one. For example, `docs: Clarify contribution guidelines`.
- **Keep cleanup with the related change** - amend the relevant commit instead of adding fixup commits for linting, formatting, typos, or later work that belongs to an earlier task.
- **Keep OpenSpec tasks independently verifiable** - map each task to exactly one commit and ensure that commit passes the full relevant check suite.
- **Archive OpenSpec changes before committing their artifacts** - do not commit OpenSpec artifacts while the change is active, and do not mix them with non-OpenSpec files in the same commit. Follow any more specific timing instructions defined by the change.

## Pull Request Guidelines

- **Keep PRs focused** - one feature or fix per PR
- **Write clear descriptions** - explain what and why, not just how
- **Explain CLI changes with examples** - when changing CLI commands or behavior, describe the user-visible behavior in the PR description and include example invocations or output
- **Update the changelog for user-facing changes** - add an entry to [CHANGELOG.md](CHANGELOG.md) under `Unreleased` when a change affects CLI behavior, deployment behavior, supported platforms/providers, user-visible errors, documentation that changes how users operate the tool, or compatibility/breaking behavior. Use the existing `Added`, `Changed`, `Fixed`, and `Breaking Changes` sections, and include a short command example for new or changed CLI behavior when useful.
- **Use the [pull request template](.github/pull_request_template.md)** when submitting a pull request
- **Add tests** for new features or bug fixes
- **Update documentation** if you're changing functionality - user-facing changes belong in `user-docs/`, contributor-facing changes in `doc/`
- **Run `task fmt` and `task lint`** before committing
- **Ensure all tests pass** - run `task all` to check
- **Reference related issues** using `#issue-number`

## Code Review Process

- All submissions require review before merging
- Be responsive to feedback and questions
- Address review comments promptly
- Maintainers will merge approved PRs

## Reporting Issues

- Use GitHub Issues for bugs and feature requests
- Search existing issues first to avoid duplicates
- For bugs: provide clear reproduction steps and environment details
- For features: explain the use case and expected behavior

## Community Guidelines

- Be respectful and professional
- Welcome newcomers and help others
- Provide constructive feedback

## Licensing

This repository is licensed under the [MIT License](./LICENSE). By submitting a pull request, you agree that your contributions are licensed under the same terms.

Note that the Exasol database deployed by this tool is proprietary software under its own [EULA](https://www.exasol.com/terms-and-conditions/#h-exasol-personal-end-user-license-agreement). Contributions to this repository do not affect the database license.

## Questions?

- Check the [documentation](doc/)
- Ask in GitHub Discussions
- Use tag `exasol-personal` on [Exasol Community](https://community.exasol.com)
