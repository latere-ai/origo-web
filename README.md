# origo-web

**A browser interface for an [Origo](https://github.com/latere-ai/origo) git
host.** It shows repositories, branches and tags, the commit log, commit
diffs, the file tree, and file contents. It also covers the few account
tasks that sit around a git remote: creating a repository, registering an
SSH key, and minting a short-lived token for an agent. Every page is
rendered on the server and works with JavaScript turned off.

[![CI](https://github.com/latere-ai/origo-web/actions/workflows/verify.yml/badge.svg)](https://github.com/latere-ai/origo-web/actions/workflows/verify.yml)
[![Release](https://img.shields.io/github/v/release/latere-ai/origo-web)](https://github.com/latere-ai/origo-web/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/latere-ai/origo-web)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Origo serves git and a JSON API and has no web pages of its own. origo-web
is a separate, optional program that reads that API with the signed-in
person's own credential and renders the answer. Origo neither needs it nor
knows about it.

## What it is not

It is a browser for git, not a forge. There are no pull requests, reviews,
comments, issues, stars, or forks, and it will not grow them. It does not
edit files, move references, rename, transfer, or freeze repositories; those
are calls to Origo's API. There is no search, no blame, and no syntax
highlighting.

It decides nothing about access. Origo answers every read with the
permissions of the person signed in, so a page shows exactly what that
person could clone, and a repository they may not see looks the same as one
that does not exist.

## Using it

Sign in with the account your identity provider holds. Then:

- **Browse.** The repository list shows what you may read, grouped by
  owner. On an installation that does not list repositories, open one by
  typing `owner/name`. Every screen has its own address, so a branch, a
  commit, a file, a line, and a comparison each travel as a link.
- **Clone.** A repository's overview shows its clone address:

  ```sh
  git clone git@code.example.com:owner/name.git
  ```

  SSH uses a key you registered on the **SSH keys** page. Over HTTPS, a
  private repository asks for a password: use an agent token, with any
  username.
- **Create a repository.** **New repository** asks for an owner and a name
  and opens the empty repository with push instructions.
- **Give an agent access.** **Agent tokens** mints a token for one
  repository, read or write, for between five minutes and an hour. It is
  shown once, and it cannot be listed or revoked afterwards: it expires.
- **Change visibility, or delete.** An administrator of a repository can
  make it public or private, and delete it, from its overview.

[`docs/using.md`](docs/using.md) is the full guide: every screen, what each
action does, and what you see when something is refused.

## Running it

origo-web is one stateless binary, published as the image
`ghcr.io/latere-ai/origoweb`. It needs:

- an Origo installation to read;
- an identity provider that Origo trusts, with a browser client registered
  for this interface. The provider also has to mint short-lived tokens
  addressed to other services on the person's behalf, which is more than
  plain OpenID Connect asks for;
  [`docs/integrations.md`](docs/integrations.md) says exactly what;
- optionally, a repository registry, which turns on creating a repository,
  changing its visibility, and deleting it, and an SSH key store, which
  turns on the SSH keys page.

[`docs/install.md`](docs/install.md) goes from those to a running
installation, on Kubernetes or as a plain process.

## Documentation

| | |
|---|---|
| [Using it](docs/using.md) | every screen, cloning, SSH keys, agent tokens, creating, visibility, and deleting |
| [Install](docs/install.md) | from an Origo installation and an identity provider to a signed-in page |
| [Configuration](docs/configuration.md) | every environment variable, with its default |
| [Integrations](docs/integrations.md) | what the identity provider, the repository registry, and the SSH key store must answer |

[`docs/README.md`](docs/README.md) is the index. How the code is organized
and tested is in [`docs/internals/`](docs/internals/README.md), for people
changing it.

## Contributing

Issues and pull requests are welcome. [`CONTRIBUTING.md`](CONTRIBUTING.md)
covers the quality gate, how to run it locally, and how a change is
reviewed.

## Security

[`SECURITY.md`](SECURITY.md) is how to report a vulnerability. Please do not
open a public issue for one.

## License

MIT. See [`LICENSE`](LICENSE). The interface serves IBM Plex Sans and IBM
Plex Mono from the binary under the SIL Open Font License, whose text is
[`internal/web/assets/IBMPlex-OFL.txt`](internal/web/assets/IBMPlex-OFL.txt).
