# Contributing

Thanks for looking. This file is for people and agents changing origo-web.
Users read the [README](README.md) and [`docs/`](docs/README.md); how the
code is organized and tested is in [`docs/internals/`](docs/internals/README.md);
the design and the reasoning behind it live in [`specs/`](specs/).

## Getting set up

You need Go 1.27 or newer and `git`. Then:

```sh
make          # the quality gate
make build    # out/origoweb
```

`make` needs only the Go toolchain and git. Everything it pins comes from
public modules, so it runs the same on your machine as in CI.

Install the hooks once with `git config core.hooksPath .githooks`. The
pre-commit hook checks formatting, import grouping, the license notice, and
that every outbound HTTP client is traced. The pre-push hook refuses a
release tag without a changelog section and runs the linter over the
packages the push changes, so you see a finding before CI does.

Running the interface against a real Origo, and the end-to-end tier, need
an Origo installation and an identity provider;
[`docs/internals/testing.md`](docs/internals/testing.md) says how to stand
them up.

## Sending a change

Fork the repository, work on a branch, and open a pull request. Keep one
logical change per commit, stage the files explicitly, and write the
subject as `scope: what changed`, for whoever reads the log. Maintainers
push to `main` directly; the pipeline runs the gate on every push and pull
request, and a `v*` tag cuts a release.

If you are planning something large, open an issue first. A design that
lands without a spec is harder to review than one that arrives with the
reasoning attached.

## The bar

`make` runs the whole gate, `go tool lateregate`: formatting,
modernization, a CGO-free build, traced HTTP clients, the license notice,
the spec tree, the dependency allow list, the identity rules, the linter,
known vulnerabilities, the suite with and without the race detector, the
suite with only the Go toolchain on `PATH`, the suite against an empty
temporary directory, and per-package coverage at 90% or more.
`go tool lateregate list` names every check and `go tool lateregate <name>`
runs one.

A bug fix carries a test that fails without it. A change that lowers a
threshold or adds a waiver records the reason in `.lateregate.yaml`, so the
exception is reviewable rather than invisible. A new dependency is a
decision recorded in the `depcheck` section of that file: the binary
reaches the shared library, the OpenTelemetry SDK, a Markdown renderer, and
the standard library, and no git, cloud SDK, or Kubernetes client.

A change a user or an operator would notice gets an entry under
`Unreleased` in [`CHANGELOG.md`](CHANGELOG.md), written for them. A new or
changed environment variable is a row in
[`docs/configuration.md`](docs/configuration.md); the suite fails when the
two disagree.

## Specs first

A feature starts as a spec in `specs/` with acceptance criteria that are
testable sentences. The implementation follows the spec, and a divergence
is recorded in the spec's Outcome section rather than left in the code.
Small fixes do not need a spec. A new screen, a new call to another
service, or a change to what a page may do does.

## Where a package belongs

`internal/` holds what is specific to this interface. A generic package
with a plausible second consumer belongs in
[`latere.ai/x/pkg`](https://github.com/latere-ai/pkg), the shared library
this module already depends on: the sign-in flow, the session cookie, the
health probes, and the traced HTTP client live there. Nothing here imports
Origo's own packages; origo-web speaks only Origo's published HTTP API, as
any other client would.

## Three registers

Every sentence is written for one reader, and the register follows the
reader: the person using the interface on every page, the contributor in
specs, this file, package documentation, and commit messages, the developer
in logs and error details. A page never shows a sentence another service
wrote. The rule and the review checklist are in
[`docs/writing/registers.md`](https://github.com/latere-ai/pkg/blob/main/docs/writing/registers.md).

## Releasing

`go tool lateregate release vX.Y.Z` turns the `Unreleased` section of the
changelog into the release's section, rewrites the version the install page
names, commits, tags, and pushes. The release workflow builds the image for
`linux/amd64` and `linux/arm64`, publishes it as
`ghcr.io/latere-ai/origoweb:vX.Y.Z`, and creates the GitHub release from
that section. A release deploys nothing: each installation pins the tag in
its own overlay.

## Reporting a vulnerability

Do not open an issue. [`SECURITY.md`](SECURITY.md) says where to send it.
