# origo-web

A read-only web interface for an [Origo](https://github.com/latere-ai/origo) installation:
repositories, branches and tags, the commit log, a commit's diff, the file
tree, a file. In the spirit of cgit and sourcehut.

No comments, no reviews, no stars, no forks, no issues. It is a window onto a
git repository for people who already know which repository they want.

It has one screen that is not a read: a form that asks the installation for
a token bound to one repository, which is how an agent gets a credential.
The token is shown once and kept nowhere.

## What it is

One binary. It serves HTML, and it is a client of Origo's read API and
nothing else: it never runs git, never reads object storage, and never
decides who may see what. It sends your own token to the installation and
shows you what comes back, so what you can see here is exactly what you can
clone.

Every page works with JavaScript disabled, because there is no JavaScript.
Every control is a link, a form, or a native `<details>`. Light and dark
follow your system setting.

## Screens

| Screen | Address |
|---|---|
| sign in | `/sign-in` |
| repositories | `/` |
| overview: clone lines, the reference selector, the root tree, the readme | `/r/{id}` |
| branches and tags | `/r/{id}/refs` |
| commit log, filterable by path | `/r/{id}/log` |
| a commit, its message, its parents and its diff | `/r/{id}/commit/{sha}` |
| any two revisions compared | `/r/{id}/compare?base=&head=` |
| the file tree | `/r/{id}/tree/{path}` |
| a file, with line anchors | `/r/{id}/blob/{path}` |
| the bytes of a file | `/r/{id}/raw/{path}` |
| agent tokens: mint one, and see it once | `/tokens` |
| how to drive the installation from an agent | `/docs/agents` |

A branch or a tag is chosen with `?ref=`, which is what the selector on the
overview submits, so any screen at any revision is a URL you can send to
someone.

**Repositories are addressed by identifier, not by name.** Origo's JSON API
answers questions about a repository you already name by its identifier and
has no way to resolve `owner/name` or to list what you may see. Until it
does, the home page is a box you paste an identifier into and a list of what
you have opened in this session, and the names are shown on every page even
though the address is the identifier. See [Limits](#limits).

## Run it

You need an Origo installation, a second OIDC client registered for the
browser flow whose tokens carry Origo's audience, and a cookie key.

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

Then point a hostname at it and open it. Nothing about your Origo
installation changes, and an operator who skips all of this has an
installation that works exactly as before.

**Give it its own hostname.** Origo's git surface claims
`/{owner}/{name}/...` for clone and push, which is the shape a browsing
interface wants; one hostname cannot serve both. `code.example.com` in front
of this and `git.example.com` in front of Origo.

### Settings

| Setting | Default | What it does |
|---|---|---|
| `ORIGOWEB_ADDR` | `:8080` | the address it listens on |
| `ORIGOWEB_ORIGO_URL` | required | the Origo installation, the one address it talks to |
| `ORIGOWEB_PUBLIC_URL` | required | its own address, used to build the sign-in redirect |
| `ORIGOWEB_CLONE_HOST` | `ORIGOWEB_ORIGO_URL` | what the clone lines say, when your git host differs from your API address |
| `ORIGOWEB_SSH_CLONE_HOST` | unset | the host in the SSH clone line; unset shows the HTTPS line alone |
| `ORIGOWEB_ISSUER_NAME` | unset | what the sign-in button calls your identity provider |
| `ORIGOWEB_PRODUCT_NAME` | `Origo` | what your installation calls itself, if you have named it |
| `ORIGOWEB_PROJECT_URL` | the Origo repository | where you link people to the open-source project |
| `ORIGOWEB_BRAND_MARK` | unset | a logo beside the name; the only one this build carries is `latere`, and it is Latere's |
| `ORIGOWEB_KEYS_URL` | unset | a key management surface, if you have one; see [Limits](#limits) |
| `ORIGOWEB_AUTH_URL` | `https://auth.latere.ai` | your OIDC issuer, which must be one of Origo's `ORIGO_OIDC_ISSUERS`; it is also where repositories are created, see [Creating a repository](#creating-a-repository) |
| `ORIGOWEB_AUTH_CLIENT_ID` | required | the client registered for the browser flow |
| `ORIGOWEB_AUTH_CLIENT_SECRET` | unset | a confidential client's secret; a public client uses PKCE alone |
| `ORIGOWEB_AUTH_AUDIENCE` | the issuer | must be the audience Origo verifies, normally `origo` |
| `ORIGOWEB_AUTH_COOKIE_KEY` | required | 32 bytes of hex; rotating it signs everyone out |

`deploy/` holds a Kubernetes base: a Deployment of two stateless replicas, a
Service, an Ingress, and the three secrets above.

## Naming your installation

Origo is the software. An installation of it can carry your own name: set
`ORIGOWEB_PRODUCT_NAME` and every page says that instead, while the
signed-out page still says it runs Origo and links to the project. Leave it
unset and your installation calls itself Origo, which is what it is.

The logo is separate, because a logo belongs to whoever owns it. Nothing is
drawn unless `ORIGOWEB_BRAND_MARK` names one, and the only mark this build
carries is Latere's. Your name is yours to set; Latere's mark is not.

## Signing in

You sign in with the account your organisation already gave you. There is no
password here and no sign-up. Your access token lives in one encrypted
cookie, is sent to Origo unchanged, and is in no page, no address bar and no
log line. A session lasts twelve hours from sign-in; refreshing your token
does not extend it.

What you may read is Origo's decision and not this interface's. A repository
you cannot see and a repository that does not exist read the same, because
that is a difference Origo deliberately refuses to make.

## Creating a repository

**New repository** on the repositories page. Choose a name to put it under
and a name for it, and you land on the empty repository with the commands to
push to it. Nothing is imported and no first commit is made.

What you may put it under is your own name, and any organisation you
administer. If you have not claimed a name yet, the page says so and links
you to your account, because a repository lives under a name and this
interface will not invent one for you.

Two things decide the rest, and neither of them is this interface. Your
identity provider says which names are yours and how many repositories each
may hold; Origo makes the repository and asks the same question again before
it writes anything. A refusal from either is shown as it came, with your
form as you left it, and nothing is created.

The button is there only when your installation has somewhere to record who
owns a repository, which is what `ORIGOWEB_AUTH_URL` names. An installation
without one has repositories made some other way, and shows no button.

Renaming, transferring and deleting are not here, and will not be. They
change a repository that already exists, and they belong to whatever holds
your audit trail.

## Limits

Four things are missing because no component can answer them yet, not
because they were left out.

- **A list of your repositories.** Origo answers questions about one
  repository at a time, and the service that decides who may see what
  answers one yes or no at a time, with no way to enumerate. The home page
  degrades to a box and a per-session history until both grow the question.
- **Public repositories.** Origo requires a credential on every read, so a
  signed-out visitor sees the sign-in page and nothing else. Every read here
  is already issued without a credential when there is none, so the day
  Origo answers one, the same page shows the repository.
- **SSH keys.** Public keys are held outside Origo, behind a resolver Origo
  only reads, and no component owns a place to add or remove one. The screen
  and its navigation entry appear only when `ORIGOWEB_KEYS_URL` names a
  surface, which is never by default.

- **A list of your agent tokens, and a way to revoke one.** Origo signs a
  token rather than recording it, so nothing is written when one is minted:
  there is nothing to list and nothing to withdraw, and a short lifetime is
  what bounds a leak instead. The token screen says so in the reader's own
  words and offers the form that works. The table is written against a
  capability, so an installation that grows a record of its tokens shows it
  here with no change to this interface.

Two more, on purpose: there is no syntax highlighting, and no blame. Neither
is needed to read a diff, and both are a large dependency over file content
nobody vetted.

## Where it lives

This directory is a Go module of its own, `github.com/latere-ai/origo-web`,
sitting inside the Origo repository until its own exists. Nothing here can
import Origo's internals, which is the point: it speaks the same published
API a third-party client would.

MIT, like Origo. The typeface is Inter under the SIL Open Font License,
served from the binary; the monospace stack is your system's.
