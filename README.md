<div align="center">

<picture>
  <source srcset="static/Exasol_Logo_2025_Bright.svg" media="(prefers-color-scheme: dark)">
  <img src="static/Exasol_Logo_2025_Dark.svg" alt="Exasol Logo" width="300">
</picture>

# Exasol Personal

**The Analytics Database for Agentic AI — Free for Personal Use**

*Deploy and explore Exasol Database on your own computer or in your cloud account.*

[![User documentation](https://img.shields.io/badge/user%20docs-latest-blue)](https://exasol.github.io/exasol-personal/latest/)
[![Exasol documentation](https://img.shields.io/badge/database%20docs-exasol.com-blue)](https://docs.exasol.com/db/latest/home.htm)
[![Community](https://img.shields.io/badge/community-exasol-green)](https://community.exasol.com)
[![Downloads](https://img.shields.io/badge/downloads-exasol.com-orange)](https://downloads.exasol.com/exasol-personal)

[Changelog](CHANGELOG.md)

</div>

## 📖 About Exasol Personal

Exasol is an in-memory, massively parallel analytics database with full SQL support, a vast
ecosystem, extensive AI capabilities, and flexible extensibility.

Exasol Personal gives individual users a complete Exasol database for development, exploration,
analytics, and AI-assisted workflows. It runs either locally (on macOS, Linux, or Windows) or on
one of the supported clouds (AWS, Azure, STACKIT, or Exoscale), managed by the Exasol Launcher, an
open-source scriptable command-line application. Exasol Personal is free for personal use and
does not impose an artificial data-size limit.

## 🚀 Get started

On macOS, Linux, and Windows (WSL), install the launcher with:
```bash
curl https://www.exasol.com/install/ | sh
```

For a quick start and a general overview of the available commands, type:

```bash
exasol help
```

If you get a `command not found` error, add `~/.local/bin` to your `PATH`.

For next steps, read the [user documentation](https://exasol.github.io/exasol-personal/latest/).
It also covers the system requirements and supported deployment targets, as well as tutorials,
command guidance, and troubleshooting information.

AI coding agents can use
[agent skills](https://github.com/exasol-labs/exasol-agent-skills) to install Exasol Personal,
connect to it, load data, and write Exasol SQL.

## 🛠️ Development and contributions

This repository contains the Exasol Launcher, its built-in deployment presets, tests, and user
documentation sources. To build or contribute:

- Read the [contribution guidelines](CONTRIBUTING.md).
- Set up the project with the [development guide](doc/development.md).
- Follow the [testing guide](tests/README.md).
- Review the high-level [architecture](doc/architecture.md) and
  [project-specific best practices](doc/best_practices.md).

## ⚖️ License

The Exasol Launcher source code is licensed under the [MIT License](LICENSE). Exasol Database is
proprietary software provided by Exasol AG and is free for personal use. Installing the database
means accepting the
[Exasol Personal End User License Agreement](https://www.exasol.com/terms-and-conditions/#h-exasol-personal-end-user-license-agreement).

For help and discussion, visit the [Exasol Community](https://community.exasol.com) and use the
`exasol-personal` tag.
