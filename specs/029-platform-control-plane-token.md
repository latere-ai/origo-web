---
title: "The registry and the key store move to the control plane, and the token moves with them"
status: in-progress
track: infra
depends_on:
  - specs/023-web-interface.md
affects: [deploy/prod/settings.yaml]
effort: medium
created: 2026-09-16
updated: 2026-09-16
author: changkun
---

# The platform control plane takes a token of its own

## Overview

The repository registry and the public key store are leaving the identity
provider for the platform control plane (`platformd`), which is step 3 of
the family's identity epic, `specs/infrastructure/identity/id-06-code-control-plane.md`
in `latere-ai/specs`, design C. This spec is this repository's half of it.

Two things move together, and they have to, because either alone is a
broken interface:

1. **The address.** `ORIGOWEB_REGISTRY_URL` and `ORIGOWEB_KEYS_URL` point
   at the control plane's public base instead of the issuer's. Both knobs
   already exist; this is an operator's setting and no new code.
2. **The credential.** The control plane verifies `aud = api.latere.ai`
   and refuses the session token, which is addressed to the issuer. So
   the interface mints a second actor token, for that audience, and
   forwards it to the registry and to the key store.

Everything else is a consequence: `AccountURL()` can no longer be derived
from the registry, and a mint that fails for one audience must not be
said on a screen that reads the other.

## Current state

Spec 023 built the interface and `v0.9.0` put the first actor token in it:
every call to Origo carries a token the issuer mints for the `origo`
audience, and the session token stays with the issuer, where the registry
and the key store were.

`ORIGOWEB_REGISTRY_URL` landed on branch `id-06/registry-repoint` and
decouples the registry's address from the issuer's. Nothing on that branch
changes the credential, so pointing it at the control plane today would
send a token the control plane refuses.

## Design

### The three tokens

```
                         ┌──────────────── aud = <issuer> ───────▶ issuer
                         │                 sign in, refresh, mint
person ──▶ origo-web ────┼──────────────── aud = origo ──────────▶ origod
   cookie                │
                         └──────────────── aud = api.latere.ai ──▶ platformd
                                           /repositories…, /me/ssh-keys
```

`session.Reader` carries all three. `Token` is the session token and goes
to no service but the issuer. `Origo` is the actor token every call to
Origo carries. `Platform` is the actor token every call to the control
plane carries, minted for `PlatformAudience = "api.latere.ai"` through the
same library call as `Origo`: `authkit/oidc` caches per `{session,
audience}` and re-mints shortly before a token lapses, so a page that
reads Origo and the registry pays for one mint per audience and not one
per call.

The session token is not a downstream credential any more. A screen shows
whether a person is signed in from `Reader.Token`; nothing else reads it.

### What forwards which

| Call site | Client | Token |
|---|---|---|
| `page_new.go` namespaces, create, forget | registry | `Platform` |
| `page_delete.go`, `page_visibility.go`, `page_repo.go` visibility | registry | `Platform` |
| `page_keys.go` list, parse, add, remove | keys | `Platform` |
| every read and write at Origo | origo | `Origo` |
| sign-in, callback, refresh, the mints themselves | authkit | `Token` |

`req.auth` is renamed `req.platform`, because a field named for the
issuer that holds a token for another service is how the wrong credential
gets forwarded next time.

### The paths are the control plane's public ones

The registry client builds `repositories/namespaces`, `repositories`,
`repositories/{id}` and `repositories/{id}/visibility` from the
configured base; the keys client builds `/me/ssh-keys` and
`/me/ssh-keys/{id}`. Those are the routes `platformd` serves to a person,
and the identity provider served the same shapes, so no path changes.

`/internal/origo/*` is the registrar-and-Origo listener at the control
plane, gated on `PLATFORM_REGISTRARS`. This interface is neither and must
never address it.

### The account screen is the issuer's

`AccountURL()` returned `RegistryURL() + "/me"`, which was one address
until this change splits it in two. Repointing the registry would then
send the *claim a name* link of `new.gohtml` — the one sentence a person
without a handle is given — to a control plane that holds no account
screen.

A handle is the identity provider's to hold, so `AccountURL()` reads the
issuer's URL directly. `RegistryURL()`'s fallback reads the same value, so
both call one helper rather than repeating the parse.

### Two mints, two faults

The mints fail apart. An issuer that has not yet granted this client the
`api.latere.ai` audience refuses that mint with `invalid_target` and mints
for Origo as usual — which is exactly the state between the auth release
and this one.

So `Reader` carries `Fault` and `PlatformFault` separately, and the
interface says each where it is felt:

| State | Home and repository screens | Create and keys screens |
|---|---|---|
| both minted | the screens | the screens |
| Origo refused | the issuer-fault sentence | the issuer-fault sentence |
| platform refused | unaffected | sign-in page with the issuer-fault sentence |

One combined fault would put a sentence about the identity provider on
screens that are reading Origo perfectly well; a fault the create screen
could not see would leave a person at a sign-in page with nothing said.
The wording does not change: it is audience-neutral already.

### Deployment

`deploy/prod` sets both addresses to the control plane's public base:

```yaml
ORIGOWEB_REGISTRY_URL: https://platform.latere.ai
ORIGOWEB_KEYS_URL:     https://platform.latere.ai
```

Each client appends its own path to that base, so neither takes a suffix.
Both are addresses and neither is a credential, so both are in the
manifest and no Secret changes.

Staleness is the actor token's, not this interface's: the owner labels a
create screen draws come from claims re-read at mint time, so a handle
renamed at the issuer is stale for at most one token life.

```
staleness ≤ actor TTL = 300 s
```

## What must land first

The issuer must grant this client the audience before this release runs.
`origoweb`'s client row at auth gains `api.latere.ai` in `actor_audiences`;
until it does, every mint for that audience is refused with
`invalid_target` and the create and keys screens fail closed. That row
ships with auth, and the control plane must already answer the routes.
Rollback is the two addresses unset and this release rolled back, which
works while the identity provider still serves the registry.

## Acceptance criteria

| Criterion | Held by |
|---|---|
| the session token never reaches the registry or the key store | `internal/web`: the fake registry's `Tokens()` carries `actor-api.latere.ai-for-…` on every call and never the session value; the key store's bearers likewise |
| every registry call and every keys call carries the platform actor token | the same assertions, across the create, delete, visibility and keys screens |
| Origo still takes its own actor token | the existing assertion on Origo's fake, unchanged |
| the two mints fail apart | `internal/session`: the issuer refuses `api.latere.ai` alone, `Origo` is minted, `PlatformFault` is set and `Fault` is not |
| a person whose platform mint failed is told why | a create-screen render carrying the issuer-fault sentence |
| the *claim a name* link points at the issuer with the registry repointed | `internal/config`: `ORIGOWEB_REGISTRY_URL` at a platform host and `AccountURL()` still under the issuer's host |
| the addresses are the control plane's, in the manifest | `deploy/prod/settings.yaml` sets both; no Secret changes |
