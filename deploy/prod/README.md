# Production deployment

The interface runs in the `latere` namespace of Latere's cluster and shares
the hostname `code.latere.ai` with the Origo installation it reads.

```
kubectl apply -k deploy/prod/
```

That command does not create the Secret the Deployment reads. Create it
once from `deploy/bootstrap/secrets.example.yaml`.

## One hostname, two Ingress objects

`code.latere.ai` serves both services. Git smart HTTP is always exactly two
path segments followed by one of three fixed names, in both of Origo's URL
forms (`/{owner}/{slug}/…` and `/r/{id}/…`, with or without `.git`), so
the split is exact.

| Request | Backend | Rule |
|---|---|---|
| `/…/…/info/refs`, `/…/…/git-upload-pack`, `/…/…/git-receive-pack` | `origod` | regex `/[^/]+/[^/]+/(info/refs\|git-upload-pack\|git-receive-pack)$` |
| `/…/…/info/lfs/…` | `origod` | regex `/[^/]+/[^/]+/info/lfs(/.*)?$` |
| `/v1/…` | `origod` | prefix |
| `/readyz`, `/version`, `/favicon.ico`, `/.well-known/jwks.json` | `origod` | exact |
| everything else, including `/` | `origoweb` | prefix `/` |

Two consequences:

- `/r/` belongs to the interface. Origo serves only the three git endpoints
  and LFS under `/r/{id}`. All of the interface's browsing pages live under
  `/r/{id}` as well. The two regular expressions route git without claiming
  the prefix.
- `/` is the interface's home page, replacing Origo's landing page.

The `origod` rules live in `latere-ai/origo` under
`deploy/prod/ingress.yaml`. That repository re-applies its `deploy/prod`
on every release and would otherwise restore the `/` prefix.

## Matching order

nginx tries regular-expression locations before the longest matching
prefix, so the two git rules win over `/`, and `/v1/` wins over `/` on
length. `/.well-known/jwks.json` is an exact match rather than a
`/.well-known/` prefix: a prefix would also capture the ACME challenge path
and break certificate renewal.

## Owner names the interface shadows

The interface's own first path segments win over a repository owner of
the same name, because a literal segment is the more specific route:
`assets` and `r` entirely, and `auth` and `docs` for the repository names
`start`, `callback` and `agents`. Origo refuses `r` and `v1` as owners
itself. The authorizer that hands out owner names is where the rest
belong.
