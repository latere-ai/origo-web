# origo-web

A web interface for an [Origo](https://github.com/latere-ai/origo)
installation. It shows repositories, branches and tags, the commit log,
commit diffs, the file tree and file contents. It also creates
repositories, changes a repository's visibility, manages SSH keys and
issues agent tokens.

There are no comments, reviews, stars, forks or issues.

## How it works

One binary that serves HTML. It is a client of Origo's HTTP API: it never
runs git, never reads object storage and never decides access. It sends
your own token to Origo and renders the response, so you see exactly what
you can clone.

There is no JavaScript. Every control is a link, a form or a native
`<details>` element. Light and dark themes follow the system setting.

## Screens

| Screen | Address |
|---|---|
| Sign in | `/sign-in` |
| Repositories | `/` |
| New repository | `/new` |
| Repository overview: clone address, branch picker, root tree, readme | `/{owner}/{name}` |
| Branches and tags | `/{owner}/{name}/refs` |
| Commit log, filterable by path | `/{owner}/{name}/log` |
| Commit: message, parents, diff | `/{owner}/{name}/commit/{sha}` |
| Patch download | `/{owner}/{name}/patch/{sha}` |
| Compare two revisions | `/{owner}/{name}/compare?base=&head=` |
| File tree | `/{owner}/{name}/tree/{path}` |
| File with line anchors | `/{owner}/{name}/blob/{path}` |
| Raw file | `/{owner}/{name}/raw/{path}` |
| Visibility | `/{owner}/{name}/visibility` |
| Delete | `/{owner}/{name}/delete` |
| Agent tokens | `/tokens` |
| SSH keys | `/keys` |
| Agent documentation | `/docs/agents` |

`?ref=` selects a branch or tag on any repository screen. The branch picker
on the overview sets it, so every screen at every revision has a URL.

Repositories are addressed by owner and name, the way git addresses
them. Origo resolves the pair, so a rename moves the address. The
identifier address every screen once had, `/r/{id}`, redirects to the
name for good, and a clone address pasted into the browser lands on the
repository. The identifier itself is what the API and agent tokens use;
it is shown on the agent tokens page.

An owner name that is one of this interface's own first path segments is
shadowed by the interface: `assets` and `r` entirely, and `auth` and
`docs` for the names `start`, `callback` and `agents`. Origo reserves `r`
and `v1` itself. An authorizer that hands out owner names should reserve
the others.

The repositories page lists what you can see, grouped by owner, when the
installation can list repositories. Otherwise it offers a box to enter
`owner/name` or an identifier, and lists the repositories opened in this
session.

## Run it

You need an Origo installation, an OIDC client registered for the browser
flow whose tokens carry Origo's audience, and a cookie key.

```sh
export ORIGOWEB_ORIGO_URL=https://git.example.com
export ORIGOWEB_PUBLIC_URL=https://code.example.com
export ORIGOWEB_AUTH_URL=https://auth.example.com
export ORIGOWEB_AUTH_CLIENT_ID=origoweb
export ORIGOWEB_AUTH_CLIENT_SECRET=...
export ORIGOWEB_AUTH_AUDIENCE=origo
export ORIGOWEB_AUTH_COOKIE_KEY=$(openssl rand -hex 32)
make build && out/origoweb
```

Point a hostname at it. Origo itself is unchanged.

Use a separate hostname. Origo serves clone and push under
`/{owner}/{name}/...`, which collides with a browsing interface. For
example, `code.example.com` for this and `git.example.com` for Origo.
`deploy/prod/README.md` shows how to share one hostname with an Ingress
split.

### Settings

| Setting | Default | Meaning |
|---|---|---|
| `ORIGOWEB_ADDR` | `:8080` | Listen address |
| `ORIGOWEB_ORIGO_URL` | required | Base URL of the Origo installation |
| `ORIGOWEB_PUBLIC_URL` | required | This service's own base URL, used for the sign-in redirect |
| `ORIGOWEB_CLONE_HOST` | `ORIGOWEB_ORIGO_URL` | Base URL in HTTPS clone addresses, when it differs from the API address |
| `ORIGOWEB_SSH_CLONE_HOST` | unset | Host in SSH clone addresses. Unset shows HTTPS only |
| `ORIGOWEB_ISSUER_NAME` | unset | Name of the identity provider on the sign-in button |
| `ORIGOWEB_PRODUCT_NAME` | `Origo` | Name of this installation |
| `ORIGOWEB_PROJECT_URL` | the Origo repository | Link to the open-source project |
| `ORIGOWEB_BRAND_MARK` | unset | Logo beside the name. The only value this build accepts is `latere` |
| `ORIGOWEB_KEYS_URL` | unset | Base URL of the SSH key store. Enables the SSH keys page. See [SSH keys](#ssh-keys) |
| `ORIGOWEB_AUTH_URL` | `https://auth.latere.ai` | OIDC issuer. Must be one of Origo's `ORIGO_OIDC_ISSUERS`. Also the repository registry, see [Creating a repository](#creating-a-repository) |
| `ORIGOWEB_AUTH_CLIENT_ID` | required | OIDC client id for the browser flow |
| `ORIGOWEB_AUTH_CLIENT_SECRET` | unset | Client secret. A public client uses PKCE only |
| `ORIGOWEB_AUTH_AUDIENCE` | the issuer | Audience Origo verifies, normally `origo` |
| `ORIGOWEB_AUTH_COOKIE_KEY` | required | 32 bytes as hex. Rotating it signs everyone out |

`deploy/` holds a Kubernetes base: a Deployment with two stateless
replicas, a Service, an Ingress and the three secrets above.

## SSH keys

Origo serves git over SSH and stores no public keys. It resolves an offered
key through a key store you run, and that store is where a person adds
keys. Set `ORIGOWEB_KEYS_URL` to its base address and the SSH keys page
appears. Unset, the page and its navigation entry are absent.

The page makes three calls to that address, each with the signed-in
person's own token:

| Call | Response |
|---|---|
| `GET /me/ssh-keys` | `{"keys": [...]}`, each with `id`, `comment`, `key_type`, `bits`, `fingerprint`, `created_at`, `last_used_at` |
| `POST /me/ssh-keys` with `{"public_key", "confirm": false}` | the parsed key, not stored |
| `POST /me/ssh-keys` with `"confirm": true` | `201` and the stored key |
| `DELETE /me/ssh-keys/{id}` | `204` |

Adding a key takes two steps. The first request sends the pasted key with
`confirm: false` and the page shows the fingerprint the store computed. The
second request stores it. This service never parses a key, computes a
fingerprint or restricts algorithms. The store does all three, so the
fingerprint you reviewed is the key that was saved.

Check the fingerprint against `ssh-keygen -lf ~/.ssh/id_ed25519.pub` on
the machine that owns the key before confirming.

The page understands three error codes: `invalid_public_key`,
`key_already_added` for a key on your account, and
`key_already_registered` for a key on another account. A `403` from the
store means this client is not permitted to manage keys, which is a
setting on the store.

Latere's auth service implements this API. Point `ORIGOWEB_KEYS_URL` at it
and grant this client the scope it requests.

## Naming the installation

Set `ORIGOWEB_PRODUCT_NAME` to give your installation its own name. Every
page uses it. The signed-out page still says the installation runs Origo
and links to the project. Unset, the interface calls itself Origo.

`ORIGOWEB_BRAND_MARK` draws a logo beside the name. The only mark in this
build is `latere`, which belongs to Latere. Nothing is drawn unless it is
set.

## Signing in

Sign in with the account your organisation provides. There is no password
and no sign-up here. Your access token lives in one encrypted cookie and is
sent to Origo unchanged. It appears in no page, URL or log line. A session
lasts twelve hours from sign-in.

Origo decides what you may read. A repository you cannot see and a
repository that does not exist look the same, because Origo does not
distinguish them.

## Creating a repository

**New repository** on the repositories page asks for an owner and a name
and opens the empty repository with push instructions. Nothing is imported
and no first commit is made.

Owners are your own account name and the organisations you administer. If
you have not claimed a name, the page links to your account.

The identity provider decides which names are yours and how many
repositories each may hold. Origo creates the repository and checks again
before writing. A refusal from either is shown with your form intact.

The button appears only when `ORIGOWEB_AUTH_URL` names a registry that
records repository ownership.

Renaming, transferring and freezing are not offered here. An
administrator can delete a repository from its overview. The server keeps
the content for seven days, within which an administrator can restore it
through the API.

## Visibility

An administrator of a repository can make it public or private from the
overview page. A public repository can be read and cloned without an
account. Making a repository private does not affect existing clones.

The overview shows a `public` badge. When the registry cannot be reached,
the badge says visibility is unavailable, so a failed read never looks
like a private repository.

## Not included

- **Token listing and revocation.** Origo signs tokens and does not record
  them, so there is nothing to list or revoke. The tokens page says so. On
  an installation that records tokens, the page lists them.
- **Syntax highlighting and blame.** Both are large dependencies over
  unvetted file content.
- **Search.** There is no file or repository search.

## Where it lives

This is its own Go module, `github.com/latere-ai/origo-web`. It imports
nothing from Origo's internals and speaks the same published API a
third-party client would.

MIT, like Origo. IBM Plex Sans and IBM Plex Mono are served from the
binary under the SIL Open Font License.
