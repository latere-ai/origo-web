# Testing

`go test ./...` is hermetic: it needs the Go toolchain and nothing else. No
test reaches the network beyond loopback servers the test starts itself. A
second tier runs the interface against a real Origo and is selected with a
build tag.

## The unit tier

### The fakes

Screens are tested against in-process fakes of every service the interface
calls, each an `httptest.Server`, so a test asserts the exact calls a screen
makes and the page it renders from the answers:

| Fake | File | Plays |
|---|---|---|
| `fakeOrigo` | `internal/web/fake_test.go` | an Origo installation answering from fixtures: a repository, branches, tags, commits, trees, blobs, and a diff. It records every call with its `Authorization` and `Range` headers, and can be told to answer any path with a given status or header |
| `fakeRegistry` | `internal/web/fake_registry_test.go` | the identity provider's token exchange (`POST /actor-tokens`) and the repository registry together, recording every call with the token it carried and refusing on demand |
| `fakeKeys` | `internal/web/fakekeys_test.go` | the SSH key store. It parses no key: what a paste becomes is what the test says, so a screen that started parsing keys itself would be caught |

`newHarness` in `fake_test.go` starts all three, builds a `Config` pointing
at them, and returns helpers that sign a request in by sealing a session
cookie directly, so no test drives the browser sign-in flow. Options on
`newHarness` change the configuration, such as turning the key store on.

### What the groups hold

| Tests | Hold |
|---|---|
| `screens_test.go` | the calls each screen makes, exactly; a refusal and an absence rendering the same page; no script on any screen; no shared cache between readers; the stale notice; the route table being read-only apart from its forms; clone addresses built from configuration only; degrading without a directory |
| `smoke_test.go`, `layout_test.go`, `stylesheet_test.go` | every screen renders; the layouts; both themes define every color |
| `actor_test.go` | which token reaches which service, and what a signed-in person sees when one of the two mints fails |
| `new_test.go`, `delete_test.go`, `page_visibility_test.go`, `tokens_test.go`, `page_keys_test.go` | the writes: order, compensation, refusals, and CSRF |
| `limits_test.go`, `page_blob_test.go`, `page_commit_test.go`, `page_log_test.go`, `page_tree_test.go` | the reading screens, the file limits, and the diff controls |
| `plain_test.go`, `copy_test.go`, `format_test.go` | the sentences a person reads, the values offered for copying, and how times and sizes are written |
| `release_pin_test.go` | the install page's image and example overlay name the newest release in the changelog |
| `internal/config/document_test.go` | `docs/configuration.md` names every variable the process reads, and nothing it does not |
| `cmd/origoweb/manifests_test.go` | the image and the pod agree on a numeric user; the manifests ask only for the OpenID Connect minimum scopes; a Secret-backed setting has no fallback in the base; no manifest under `deploy/` names a `latest` tag or an image with no tag |
| `tools/ci/workflow_test.go` | the public verification workflow runs on hosted runners |

The clients in `internal/origo`, `internal/registry`, and `internal/keys`
have their own tests against `httptest` servers, including path escaping,
error documents, and the predicates each screen branches on.
`internal/session` is tested against authkit with a fixed cookie key.

## The end-to-end tier

`make test-e2e` runs `test/e2e` with the `e2e` build tag against a real
`origod` with the stub issuer and stub authorizer Origo publishes. It skips
unless these are set:

| Variable | What it is |
|---|---|
| `ORIGOWEB_TEST_ORIGO_URL` | the installation, such as `http://localhost:8090` |
| `ORIGOWEB_TEST_ISSUER_URL` | the stub issuer, whose `POST /mint` mints a token for any subject |
| `ORIGOWEB_TEST_REPO_ID` | a repository with history, a tree, and a readme |
| `ORIGOWEB_TEST_DENIED_SUB` | a subject the stub authorizer denies that repository |

The `e2e` job in `.github/workflows/verify.yml` is the reference for
standing that up, and it pins the images it uses at the top of the file:
MinIO with a bucket, the `origo-stubs` image serving the issuer, the
authorizer, and an event sink, `origod` configured against all three, and a
repository created and pushed with a hundred commits. The job uses host
networking, so run it the same way on Linux. The tier seals sessions from
minted tokens, as the unit tier does; it exercises the reads, the
refusals, and the token exchange against the real services, not the
browser sign-in flow.

## Running the interface locally

`make dev` builds the binary and runs it on `:8090` against the Origo at
`$ORIGO_URL`, `http://localhost:8080` by default, with insecure cookies so
a browser keeps them over plain HTTP. It sets the addresses and nothing
about sign-in, so supply the identity provider in the environment:

```sh
export ORIGOWEB_AUTH_URL=https://auth.example.com
export ORIGOWEB_AUTH_CLIENT_ID=origoweb-dev
export ORIGOWEB_AUTH_COOKIE_KEY=$(openssl rand -hex 32)
ORIGO_URL=http://localhost:<the port Origo's make dev printed> make dev
```

Without them the process exits with
`session: the identity provider is not configured`. The provider has to
serve the browser flow and the token exchange described in
[`../integrations.md`](../integrations.md). The stub issuer of Origo's
`make dev` stack mints tokens and serves the exchange but has no
`/authorize`, so it cannot complete a browser sign-in; for a screen change,
the unit tier's harness is the quicker loop.

## The gate

`make` runs `go tool lateregate`, which runs every check CI runs.
`go tool lateregate list` names them; the ones a change most often meets:

| Check | Fails when |
|---|---|
| `cover` | a package is under 90% statement coverage |
| `hermetic` | a test needs anything on `PATH` beyond the Go toolchain |
| `tempdir` | a test writes outside its temporary directory |
| `otel-client` | an outbound HTTP client is built without tracing |
| `depcheck` | the binary reaches a module the allow list does not name |
| `identity` | the identity rules shared across Latere's services are broken |
| `spec-lint` | a spec's front matter or sections are malformed |
