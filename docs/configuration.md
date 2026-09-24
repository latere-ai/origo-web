# Configuration

Every environment variable `origoweb` reads, its default, and what it
changes. The process reads its configuration once at start-up. A required
variable that is missing, an address that does not parse, or an unknown
brand mark stops it with a message saying which.

Addresses are absolute `http` or `https` URLs, written without a trailing
slash.

## The service

| Variable | Required | Default | What it is |
|---|---|---|---|
| `ORIGOWEB_ADDR` | no | `:8080` | the address the listener binds. It serves the pages, the stylesheet and fonts, and the probes `/livez`, `/readyz`, and `/version`. |
| `ORIGOWEB_ORIGO_URL` | yes | none | the Origo installation this interface calls. Every read, the creation of a repository, its deletion, and the minting of an agent token go here. In a cluster that also runs Origo, name Origo's in-cluster Service. |
| `ORIGOWEB_PUBLIC_URL` | yes | none | this interface's own address, as browsers reach it. The sign-in redirect, the address the identity provider returns to after a sign-out, and every absolute link a page shows are built from it and never from a request header. |
| `ORIGOWEB_CLONE_HOST` | no | `ORIGOWEB_ORIGO_URL` | the base of the HTTPS clone address on the overview, and of the addresses the agent pages hand out. Set it whenever `ORIGOWEB_ORIGO_URL` is an address people cannot reach, such as an in-cluster Service. |
| `ORIGOWEB_SSH_CLONE_HOST` | no | unset | the host of the SSH clone address, `git@<host>:owner/name.git`. Unset, the overview shows the HTTPS address alone. |

## Naming the installation

| Variable | Required | Default | What it is |
|---|---|---|---|
| `ORIGOWEB_PRODUCT_NAME` | no | `Origo` | the name the installation goes by, on every page. When it is set, the signed-out page also says that the installation runs Origo and links to the project. |
| `ORIGOWEB_PROJECT_URL` | no | `https://github.com/latere-ai/origo` | where that link points. |
| `ORIGOWEB_ISSUER_NAME` | no | unset | the identity provider's name on the sign-in button, which then reads "Continue with" and the name. |
| `ORIGOWEB_BRAND_MARK` | no | unset | a logo drawn beside the name. The only mark this build carries is `latere`, which is Latere's; any other value stops start-up. Unset draws nothing. |

## Sign-in

The browser client registered at the identity provider.
[`integrations.md`](integrations.md) says what the provider must serve.

Each of these is also read without the `ORIGOWEB_` prefix, as `AUTH_URL`,
`AUTH_CLIENT_ID`, and so on, when the prefixed variable is unset. The
prefixed name wins.

| Variable | Required | Default | What it is |
|---|---|---|---|
| `ORIGOWEB_AUTH_URL` | no | `https://auth.latere.ai` | the identity provider's base address. Every endpoint the interface calls there is built from it. It must be one of Origo's `ORIGO_OIDC_ISSUERS`. The default is Latere's own provider, so every other installation sets it. |
| `ORIGOWEB_AUTH_CLIENT_ID` | yes | none | the client id registered for the browser flow. Without it the process refuses to start. |
| `ORIGOWEB_AUTH_CLIENT_SECRET` | no | unset | the secret of a confidential client. Unset, the client is public and PKCE alone secures the code exchange. |
| `ORIGOWEB_AUTH_COOKIE_KEY` | yes, for a public client | derived from the client secret | the key the session cookie is encrypted with: 32 bytes as hex, which `openssl rand -hex 32` prints. A public client without it refuses to start. A confidential client without it derives one from the secret and logs a warning. Rotating it signs everyone out. |
| `ORIGOWEB_AUTH_REDIRECT_URL` | no | `ORIGOWEB_PUBLIC_URL` followed by `/auth/callback` | the redirect URI of the sign-in flow. Register the same value at the provider. |
| `ORIGOWEB_AUTH_SCOPES` | no | `openid,email,profile` | the scopes asked for at sign-in, separated by commas or spaces. Add `offline_access` so the provider returns a refresh token: without one, a session ends when its first access token expires instead of after twelve hours. The manifests set `openid,email,profile,offline_access`. A scope the provider has not granted this client fails every sign-in. |
| `ORIGOWEB_AUTH_AUDIENCE` | no | unset | an audience to ask for at sign-in. Leave it unset: the session token is addressed to the provider, and every call to another service carries a token minted for that service. |
| `ORIGOWEB_AUTH_INSECURE_COOKIES` | no | unset | `1`, `true`, `yes`, or `on` drops the `Secure` attribute and the `__Host-` prefix from the cookies, so a browser keeps them over plain HTTP. For a local run only. |

## Account services

Both are optional. Each turns on screens that need a service this interface
does not provide. [`integrations.md`](integrations.md) is the contract each
must answer.

| Variable | Required | Default | What it is |
|---|---|---|---|
| `ORIGOWEB_REGISTRY_URL` | no | `ORIGOWEB_AUTH_URL` | the base address of the repository registry, which records who owns which repository and whether it is public. Creating a repository, changing its visibility, and deleting it call it. An address that answers 404 to the registry's routes is an installation without one: **New repository** says creation is not available. |
| `ORIGOWEB_KEYS_URL` | no | unset | the base address of the SSH key store. Set, the **SSH keys** page and its navigation entry appear. Unset, neither exists. |
