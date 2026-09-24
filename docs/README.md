# Documentation

For people who use origo-web, run it, or connect it to their own identity
provider and account services.

## Using it

| Page | |
|---|---|
| [Using it](using.md) | signing in, every screen and its address, cloning, SSH keys, agent tokens, creating a repository, visibility, deleting, and what each refusal means |

## Running it

Read these in order the first time.

| Page | |
|---|---|
| [Install](install.md) | what it needs, the identity provider client, running the binary, applying the manifests, and serving it beside Origo on one hostname or two |
| [Configuration](configuration.md) | every environment variable the binary reads, its default, and what it changes |

## Integrating it

| Page | |
|---|---|
| [Integrations](integrations.md) | the calls origo-web makes to the identity provider, the repository registry, the SSH key store, and Origo, and what each must answer |

## Changing it

[`internals/`](internals/README.md) is how the code is organized, how a
request flows from the browser to Origo, and how the suite tests it.
[`CONTRIBUTING.md`](../CONTRIBUTING.md) is how to build and send a change.
The design, and the reasoning behind each decision, is in
[`specs/`](../specs/).
