# Installing origo-web

For whoever runs origo-web in front of an Origo installation. It covers
what to prepare, running the binary, applying the Kubernetes manifests, and
the two ways to share hostnames with Origo.

origo-web is one stateless binary. It keeps no database, no volume, and no
repository: everything it knows is in the request and the session cookie,
so any number of replicas serve any request and a rollout needs no order.

## What you need

| | What | Notes |
|---|---|---|
| Origo | an installation, and an address this service can reach it at | the in-cluster Service, when both run in one cluster |
| An identity provider | one Origo lists in `ORIGO_OIDC_ISSUERS` | it must serve the endpoints and the token exchange in [`integrations.md`](integrations.md), which is more than plain OpenID Connect |
| A client at the provider | registered for the browser flow | the next section |
| A cookie key | 32 random bytes as hex | `openssl rand -hex 32` |
| A hostname and a certificate | for this interface | it can share Origo's hostname; see the last section |
| A repository registry | optional | turns on creating a repository, changing its visibility, and deleting it |
| An SSH key store | optional | turns on the **SSH keys** page |

## 1. Register the client

At the identity provider, register one client for this interface. Replace
`https://code.example.com` with the address people will open:

| Setting | Value |
|---|---|
| Redirect URI | `https://code.example.com/auth/callback` |
| Post-logout redirect URI | `https://code.example.com/sign-in` |
| Front-channel logout URI | `https://code.example.com/logout/notify` |
| Scopes | `openid`, `email`, `profile`, `offline_access` |
| Actor-token audiences | `origo`, and `api.latere.ai` when a registry or a key store is in use |

A client with no secret is a public client and uses PKCE alone; that is
enough, and it is what the manifests assume. Without `offline_access` the
provider returns no refresh token, and a session ends when its first access
token expires instead of after twelve hours. Without the front-channel
logout URI, signing out at the provider leaves the session here alive; that
also needs the provider on the same site as this interface, because the
session cookie is `SameSite=Lax`.

## 2. Run it

The release image is `ghcr.io/latere-ai/origoweb:<version>`, built for
`linux/amd64` and `linux/arm64`; the releases page lists the versions. It
runs as an unprivileged user and listens on port 8080.

```sh
docker run --rm -p 8080:8080 \
	-e ORIGOWEB_ORIGO_URL=https://git.example.com \
	-e ORIGOWEB_PUBLIC_URL=https://code.example.com \
	-e ORIGOWEB_AUTH_URL=https://auth.example.com \
	-e ORIGOWEB_AUTH_CLIENT_ID=origoweb \
	-e ORIGOWEB_AUTH_COOKIE_KEY="$(openssl rand -hex 32)" \
	-e ORIGOWEB_AUTH_SCOPES=openid,email,profile,offline_access \
	ghcr.io/latere-ai/origoweb:v0.10.3
```

Or build it from a checkout with Go 1.27 or newer:

```sh
make build
out/origoweb
```

Set `ORIGOWEB_AUTH_URL` even though it has a default: the default is
Latere's own identity provider. Keep the cookie key: every replica needs the
same one, and a new key signs everyone out. Add the optional addresses as
you need them:

```sh
export ORIGOWEB_CLONE_HOST=https://git.example.com     # when ORIGOWEB_ORIGO_URL is internal
export ORIGOWEB_SSH_CLONE_HOST=git.example.com         # when Origo serves SSH
export ORIGOWEB_REGISTRY_URL=https://platform.example.com
export ORIGOWEB_KEYS_URL=https://platform.example.com
```

Every variable is in [`configuration.md`](configuration.md).

**Check it.** `GET /readyz` answers once the process is serving. Open the
public address: a signed-out browser gets the sign-in page. Sign in, and the
repository list, or the box to open one by name, is what Origo lets that
account see. When the sign-in comes back with an error, the provider's
reason is in the address bar as `auth_error` and in this service's log.

## 3. On Kubernetes

`deploy/` holds the manifests:

```
deploy/base/        Deployment (two replicas), Service, Ingress, ServiceAccount
deploy/bootstrap/   secrets.example.yaml, the one Secret the Deployment reads
```

Apply the base through an overlay of your own, never directly. The
Deployment runs as user 65532 with a read-only root file system, no
capabilities, no service account token, and the `RuntimeDefault` seccomp
profile, so it admits under Pod Security `restricted`.

**The Secret.** Copy `deploy/bootstrap/secrets.example.yaml`, fill it in, and
apply it into the namespace. Keep the filled copy out of version control. It
has four keys, which the Deployment reads by name:

| Key | Becomes |
|---|---|
| `auth-url` | `ORIGOWEB_AUTH_URL` |
| `auth-client-id` | `ORIGOWEB_AUTH_CLIENT_ID` |
| `auth-client-secret` | `ORIGOWEB_AUTH_CLIENT_SECRET`; empty for a public client |
| `cookie-key` | `ORIGOWEB_AUTH_COOKIE_KEY` |

**The overlay.** Name `deploy/base` and set, at least:

- **the release**, in a kustomize `images:` entry naming
  `ghcr.io/latere-ai/origoweb` and the tag. The base carries the
  placeholder `unreleased`, which is never published, so an overlay that
  pins nothing fails to pull rather than running an unknown build.
- **the namespace.**
- **the hostname**, in the Ingress and in `ORIGOWEB_PUBLIC_URL`. They must
  agree: the sign-in flow returns to `ORIGOWEB_PUBLIC_URL`.
- **`ORIGOWEB_ORIGO_URL`**, whose base value
  `http://origod.origo.svc.cluster.local` is Origo's Service in the
  namespace `origo`, and **`ORIGOWEB_CLONE_HOST`**, the address people clone
  from.
- **the ingress class and its settings.** The base Ingress carries an
  ingress-nginx annotation limiting a request body to 1 MiB, which is ample:
  no form here uploads more than a public key.

`ORIGOWEB_AUTH_SCOPES` is already `openid,email,profile,offline_access` in
the base. The rest of [`configuration.md`](configuration.md) is optional.

An overlay that does all of that is two files. Each `example.com` name is
yours to replace, and `resources` names a checkout of this repository at the
release the overlay pins:

```yaml
# kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: code

resources:
  - ../origo-web/deploy/base

images:
  - name: ghcr.io/latere-ai/origoweb
    newTag: v0.10.3

patches:
  - path: settings.yaml
  - target: {kind: Ingress, name: origoweb}
    patch: |-
      - op: replace
        path: /spec/rules/0/host
        value: code.example.com
      - op: replace
        path: /spec/tls/0/hosts/0
        value: code.example.com
      - op: add
        path: /spec/ingressClassName
        value: nginx
```

```yaml
# settings.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: origoweb
spec:
  template:
    spec:
      containers:
        - name: origoweb
          env:
            - name: ORIGOWEB_ORIGO_URL
              value: http://origod.origo.svc.cluster.local
            - name: ORIGOWEB_PUBLIC_URL
              value: https://code.example.com
            - name: ORIGOWEB_CLONE_HOST
              value: https://git.example.com
```

kustomize merges `env` by name, so the patch replaces the base's value of
each variable it names and keeps every other one.

```sh
kubectl apply -k path/to/your/overlay
kubectl -n <namespace> rollout status deployment/origoweb
```

The readiness and liveness probes read `/readyz` and `/livez`. Neither
reaches Origo or the identity provider, so a ready pod says the process is
serving and nothing more; step 2's check is what proves the integrations.

## 4. Sharing hostnames with Origo

Origo serves git over HTTPS at `/{owner}/{name}.git/...`, and this interface
shows the repository at `/{owner}/{name}`. Two layouts work.

**Two hostnames.** This interface at `code.example.com`, Origo at
`git.example.com`. Nothing routes by path and no rule can be wrong. Set
`ORIGOWEB_CLONE_HOST=https://git.example.com`. This is what `deploy/base`
does, and where to start.

**One hostname.** Both at `code.example.com`, with the ingress splitting by
path. Origo's browser-facing surface is a closed list, so the split is exact
rather than a guess about repository names:

| Request | Goes to | Rule |
|---|---|---|
| `/{a}/{b}/info/refs`, `/{a}/{b}/git-upload-pack`, `/{a}/{b}/git-receive-pack` | Origo | regular expression `^/[^/]+/[^/]+/(info/refs\|git-upload-pack\|git-receive-pack)$` |
| `/{a}/{b}/info/lfs/...` | Origo | regular expression `^/[^/]+/[^/]+/info/lfs(/.*)?$` |
| `/v1/...` | Origo | prefix |
| `/openapi.yaml`, `/.well-known/jwks.json`, `/favicon.ico` | Origo | exact |
| everything else, including `/` | origo-web | prefix `/` |

The git rules cover both of Origo's address forms, `/{owner}/{name}.git` and
`/r/{id}.git`, because each is exactly two segments before the service name.
Match `/.well-known/jwks.json` exactly rather than by the `/.well-known/`
prefix, which would also capture the path an ACME client answers
certificate challenges on. With one hostname, `ORIGOWEB_CLONE_HOST` is that
hostname, and this interface's home page replaces Origo's landing page.

With ingress-nginx, regular-expression locations are tried before the
longest prefix, so the git rules win over `/`, and `/v1/` wins over `/` on
length.

**Owner names the interface takes.** An address the interface serves itself
wins over a repository of the same shape, so a few owner names cannot be
browsed here:

| Owner | Repository names affected |
|---|---|
| `assets`, `r` | every name |
| `auth` | `start`, `callback` |
| `docs` | `agents` |
| `logout` | `notify` |

Origo refuses `r` and `v1` as owners itself. Reserve the rest wherever owner
names are handed out, usually the identity provider or the registry.

## When something fails

| What you see | What it means | What to do |
|---|---|---|
| the process exits with `session: the identity provider is not configured` | `ORIGOWEB_AUTH_CLIENT_ID` is unset, or a public client has no cookie key | set both |
| the process exits naming `ORIGOWEB_ORIGO_URL` or `ORIGOWEB_PUBLIC_URL` | a required address is missing or does not parse | set it to an absolute `http` or `https` URL |
| sign-in lands on the provider's error page | the redirect URI the provider holds differs from `ORIGOWEB_PUBLIC_URL/auth/callback`, or a requested scope is not granted to the client | register the exact URI; drop the scope or grant it |
| sign-in returns with `auth_error` in the address | the provider refused, or the code could not be exchanged, or the token did not verify | the provider's reason is in this service's log |
| signed in, and repository pages show the sign-in page saying the identity provider did not answer | the provider will not mint a token for the audience `origo` | let this client mint actor tokens for `origo` |
| **New repository** and **SSH keys** say the provider did not answer, and reading works | the provider will not mint for `api.latere.ai` | let this client mint for that audience too |
| signed in, and every repository page is the sign-in page again | Origo refuses the token: the provider is not in `ORIGO_OIDC_ISSUERS`, or `origo` is not in `ORIGO_OIDC_AUDIENCE` | fix Origo's configuration |
| **New repository** says creation is not available | the registry address answers 404 to the registry's routes | set `ORIGOWEB_REGISTRY_URL` to the registry's base address |
| a session ends after a few minutes instead of twelve hours | there is no refresh token | add `offline_access` to the scopes and to the client |
| signing out elsewhere leaves this session alive | the front-channel logout URI is not registered, or the provider is on another site | step 1 |
| a pod cannot pull `ghcr.io/latere-ai/origoweb:unreleased` | the overlay pins no release, and the base's placeholder tag is never published | pin a release tag in your overlay |
