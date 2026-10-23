# Contributing to withsecrets

Thanks for checking out withsecrets!
We're excited to hear and learn from you.

We've put together the following guidelines to
help you figure out where you can best be helpful.

## Table of Contents

0. [Types of contributions we're looking for](#types-of-contributions-were-looking-for)
1. [Ground rules & expectations](#ground-rules--expectations)
2. [How to contribute](#how-to-contribute)
3. [Style guide](#style-guide)
4. [Documentation](#documentation)
5. [Code](#code)
6. [Setting up your environment](#setting-up-your-environment)
7. [Community](#community)

## Types of contributions we're looking for

There are many ways you can directly contribute to withsecrets:

- Feature requests
- Bug reports
- Code contributions
- Writing or editing documentation

## Ground rules & expectations

Before we get started,
here are a few things we expect from you (and that you should expect from others):

- Be kind and thoughtful in your conversations around this project.
  We all come from different backgrounds and projects,
  which means we likely have different perspectives on "how open source is done."
  Try to listen to others rather than convince them that your way is correct.
- withsecrets is released with a [Contributor Code of Conduct](./CODE_OF_CONDUCT.md).
  By participating in this project, you agree to abide by its terms.
- Please ensure that your contribution passes all tests if you open a pull request.
  If there are test failures, you will need to address them before we can merge your contribution.
- When adding content, please consider if it's widely valuable.
  Please don't add references or links to things you or your employer have created,
  as others will do so if they appreciate it.

## How to contribute

If you'd like to contribute,
start by searching through the [pull requests](https://github.com/dont-be-evil-company/withsecrets/pulls) to
see whether someone else has raised a similar idea or question.

If you don't see your idea listed, and you think it fits into the goals of this guide, open a pull request.

## Style guide

### Documentation

If you're writing documentation,
see the [style guide](./docs/.vale/styles) (which uses [vale](https://vale.sh)) to
help your prose match the rest of the documentation.

### Code

When writing code,
please follow these configurations:

- [gofmt](https://pkg.go.dev/cmd/gofmt)
- [Golangci-lint](https://golangci-lint.run/)
- [EditorConfig](./.editorconfig)
- [yaml-lint](./.yamllint.yaml)

Most of them are automatically checked by the CI,
so you don't need to worry about them.

### GUI (`ws gui`)

The web GUI lives in `gui/` (SvelteKit + Melt UI). Shared CRUD logic is in `internal/guiapi/`.

```bash
# Build GUI assets and embed them into the Go binary
./scripts/build-gui.sh

# Develop the frontend (proxy /api to a running ws gui server)
ws gui --no-browser
pnpm --dir gui dev
```

Release builds run `build-gui.sh` automatically via `scripts/build.sh`.

## Community

Discussions about the diffconflict.snvim take place on:

- This repository's [Issues](https://github.com/dont-be-evil-company/withsecrets/issues) and
  [Pull Requests](https://github.com/dont-be-evil-company/withsecrets/pulls) sections
- The [mistwieaverco discord server](https://mistweaverco.com/discord)

Anybody is welcome to join these conversations.

Wherever possible,
don't take these conversations to private channels,
including contacting the maintainers directly.

Keeping communication public means everybody can benefit and learn from the conversation.
