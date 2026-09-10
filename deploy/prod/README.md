# The production target

The interface runs in the namespace `latere` of Latere's cluster and shares
the hostname `code.latere.ai` with the Origo installation it reads.

```
kubectl apply -k deploy/prod/
```

The Secret the Deployment reads is not applied by that command. Create it
once from `deploy/bootstrap/secrets.example.yaml`.

## The hostname is split between two Ingress objects

`code.latere.ai` answers for both services. The split is exact rather than a
guess, because Origo's browser-facing surface is a closed list and git's
smart HTTP is always **exactly two path segments** followed by one of three
fixed names, in both of Origo's URL forms (`/{owner}/{slug}/…` and
`/r/{id}/…`, with or without `.git`).

| Request | Backend | Rule |
|---|---|---|
| `/…/…/info/refs`, `/…/…/git-upload-pack`, `/…/…/git-receive-pack` | `origod` | regex `/[^/]+/[^/]+/(info/refs\|git-upload-pack\|git-receive-pack)$` |
| `/…/…/info/lfs/…` | `origod` | regex `/[^/]+/[^/]+/info/lfs(/.*)?$` |
| `/v1/…` | `origod` | prefix |
| `/readyz`, `/version`, `/favicon.ico`, `/.well-known/jwks.json` | `origod` | exact |
| everything else, including `/` | `origoweb` | prefix `/` |

Two consequences worth stating.

**`/r/` is the interface's, not Origo's.** Origo serves only the three git
endpoints and the LFS paths under `/r/{id}`; the interface's whole browsing
surface is `/r/{id}`, `/r/{id}/refs`, `/log`, `/commit/{sha}`, `/patch/{sha}`,
`/compare`, `/tree/…`, `/blob/…` and `/raw/…`. Sending the `/r/` prefix to
Origo would swallow all of it. The two regular expressions above keep git
whole without claiming the prefix.

**`/` is now the interface's home screen**, in place of Origo's landing page.

The rules for `origod` live with `origod`, in `latere-ai/origo` under
`deploy/prod/ingress.yaml`, because that repository's release pipeline
re-applies its own `deploy/prod` on every release and would otherwise put
the `/` prefix back.

## Matching order

nginx tries regular expression locations before it falls back to the longest
matching prefix, so the two git rules beat `/` however short it is, and
`/v1/` beats `/` on length. `/.well-known/jwks.json` is exact rather than a
`/.well-known/` prefix on purpose: a regular expression there would also
match the ACME challenge path and break certificate renewal.
