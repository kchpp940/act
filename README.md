![act-logo](https://raw.githubusercontent.com/wiki/nektos/act/img/logo-150.png)

# Overview [![push](https://github.com/nektos/act/workflows/push/badge.svg?branch=master&event=push)](https://github.com/nektos/act/actions) [![Go Report Card](https://goreportcard.com/badge/github.com/nektos/act)](https://goreportcard.com/report/github.com/nektos/act) [![awesome-runners](https://img.shields.io/badge/listed%20on-awesome--runners-blue.svg)](https://github.com/jonico/awesome-runners)

> "Think globally, `act` locally"

Run your [GitHub Actions](https://developer.github.com/actions/) locally! Why would you want to do this? Two reasons:

- **Fast Feedback** - Rather than having to commit/push every time you want to test out the changes you are making to your `.github/workflows/` files (or for any changes to embedded GitHub actions), you can use `act` to run the actions locally. The [environment variables](https://help.github.com/en/actions/configuring-and-managing-workflows/using-environment-variables#default-environment-variables) and [filesystem](https://help.github.com/en/actions/reference/virtual-environments-for-github-hosted-runners#filesystems-on-github-hosted-runners) are all configured to match what GitHub provides.
- **Local Task Runner** - I love [make](<https://en.wikipedia.org/wiki/Make_(software)>). However, I also hate repeating myself. With `act`, you can use the GitHub Actions defined in your `.github/workflows/` to replace your `Makefile`!

> [!TIP]
> **Now Manage and Run Act Directly From VS Code!**<br/>
> Check out the [GitHub Local Actions](https://sanjulaganepola.github.io/github-local-actions-docs/) Visual Studio Code extension which allows you to leverage the power of `act` to run and test workflows locally without leaving your editor.

# How Does It Work?

When you run `act` it reads in your GitHub Actions from `.github/workflows/` and determines the set of actions that need to be run. It uses the Docker API to either pull or build the necessary images, as defined in your workflow files and finally determines the execution path based on the dependencies that were defined. Once it has the execution path, it then uses the Docker API to run containers for each action based on the images prepared earlier. The [environment variables](https://help.github.com/en/actions/configuring-and-managing-workflows/using-environment-variables#default-environment-variables) and [filesystem](https://docs.github.com/en/actions/using-github-hosted-runners/about-github-hosted-runners#file-systems) are all configured to match what GitHub provides.

Let's see it in action with a [sample repo](https://github.com/cplee/github-actions-demo)!

![Demo](https://raw.githubusercontent.com/wiki/nektos/act/quickstart/act-quickstart-2.gif)

# Act User Guide

Please look at the [act user guide](https://nektosact.com) for more documentation.

<!-- AUTO-GENERATED from release.json by scripts/release_gen.sh - DO NOT EDIT MANUALLY -->
## Installation

```bash
curl -sL https://raw.githubusercontent.com/nektos/act/master/install.sh | bash
```

Or via Homebrew:

```bash
brew install act
```

Or via Chocolatey (Windows):

```powershell
choco install act-cli
```

### Supported Platforms

| OS | Architecture | Archive |
|---|---|---|
| darwin | amd64 | `act_Darwin_x86_64.tar.gz` |
| darwin | arm64 | `act_Darwin_arm64.tar.gz` |
| linux | 386 | `act_Linux_i386.tar.gz` |
| linux | amd64 | `act_Linux_x86_64.tar.gz` |
| linux | arm64 | `act_Linux_arm64.tar.gz` |
| linux | arm v6 | `act_Linux_armv6.tar.gz` |
| linux | arm v7 | `act_Linux_armv7.tar.gz` |
| linux | riscv64 | `act_Linux_riscv64.tar.gz` |
| windows | 386 | `act_Windows_i386.zip` |
| windows | amd64 | `act_Windows_x86_64.zip` |
| windows | arm64 | `act_Windows_arm64.zip` |
| windows | arm v7 | `act_Windows_armv7.zip` |

### Docker

```bash
docker pull nektos/act:latest
```

Available tags: `latest`, `<major>.<minor>`, `<version>`
<!-- END AUTO-GENERATED -->
<!-- END AUTO-GENERATED -->
<!-- END AUTO-GENERATED -->
<!-- END AUTO-GENERATED -->

# Support

Need help? Ask in [discussions](https://github.com/nektos/act/discussions)!

# Contributing

Want to contribute to act? Awesome! Check out the [contributing guidelines](CONTRIBUTING.md) to get involved.

## Manually building from source

- Install Go tools 1.20+ - (<https://golang.org/doc/install>)
- Clone this repo `git clone git@github.com:nektos/act.git`
- Run unit tests with `make test`
- Build and install: `make install`
