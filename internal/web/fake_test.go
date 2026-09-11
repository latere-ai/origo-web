// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"latere.ai/x/pkg/authkit"
	"latere.ai/x/pkg/authkit/oidc"

	"github.com/latere-ai/origo-web/internal/config"
	"github.com/latere-ai/origo-web/internal/keys"
	"github.com/latere-ai/origo-web/internal/origo"
	"github.com/latere-ai/origo-web/internal/registry"
	"github.com/latere-ai/origo-web/internal/session"
)

// fakeOrigo is an installation that records what a screen asked it. It
// answers from fixtures, so a test asserts against the calls the spec's
// table lists and against nothing else.
type fakeOrigo struct {
	mu    sync.Mutex
	calls []string

	repo    origo.Repo
	heads   []origo.Ref
	tags    []origo.Ref
	commits []origo.Commit
	trees   map[string][]origo.Entry
	blobs   map[string]fakeBlob
	diff    string

	// headers and status are per-path overrides, keyed by the path alone.
	headers map[string]http.Header
	status  map[string]int
	// tokens records the Authorization header of every call, and ranges
	// the Range header of every blob call.
	tokens []string
	ranges []string
	// nextCursor is returned by every paging read when set.
	nextCursor string
	// mintStatus overrides the answer of the mint route, and minted counts
	// the tokens it signed.
	mintStatus int
	minted     []mintCall
	// registry is what a GET of the token route answers. Nil is what every
	// installation answers today: the route is not there.
	registry []origo.TokenRecord
	// ignoreRange makes the installation answer 200 to a ranged read, the
	// way a proxy that strips the header would.
	ignoreRange bool

	// created is every repository the create route made, in order.
	created []origo.CreateRequest
	// unknownFor is how many creates the authorizer denies with
	// unknown_repository before it allows one, which is the refusal a
	// registry row that has not reached every replica produces.
	unknownFor int
	// createStatus and createCode override the create route's answer.
	createStatus int
	createCode   string
	// deleted is every repository the delete route marked, in order, and
	// deleteStatus overrides that route's answer.
	deleted      []string
	deleteStatus int
}

// Deleted is every repository the delete route marked.
func (f *fakeOrigo) Deleted() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleted...)
}

// deleteRoute is Origo's delete (spec 020): a hold, not a purge. The
// repository is marked at once and its content kept for a time an
// administrator can undelete it in.
func (f *fakeOrigo) deleteRoute(w http.ResponseWriter, p string) {
	id := strings.TrimPrefix(p, "/v1/repos/")
	f.mu.Lock()
	code := f.deleteStatus
	if code == 0 {
		f.deleted = append(f.deleted, id)
	}
	f.mu.Unlock()
	if code != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "forbidden"}})
		return
	}
	at := time.Now().UTC()
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]any{"id": id, "deleted_at": at, "purge_after": at.Add(7 * 24 * time.Hour)})
}

// createRoute is Origo's create: the caller chooses the id, and the
// authorizer is asked whether this subject may administer it under this
// owner before anything is written.
func (f *fakeOrigo) createRoute(w http.ResponseWriter, r *http.Request) {
	var req origo.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "invalid"}})
		return
	}
	f.mu.Lock()
	if f.unknownFor > 0 {
		f.unknownFor--
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
			"code": "forbidden", "details": map[string]any{"reason": "unknown_repository"},
		}})
		return
	}
	status, code := f.createStatus, f.createCode
	if status == 0 {
		f.created = append(f.created, req)
	}
	f.mu.Unlock()
	if status != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code}})
		return
	}
	branch := req.DefaultBranch
	if branch == "" {
		branch = "main"
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, origo.Repo{ID: req.ID, Owner: req.Owner, Slug: req.Slug, DefaultBranch: branch})
}

// Created is every repository the create route made.
func (f *fakeOrigo) Created() []origo.CreateRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]origo.CreateRequest(nil), f.created...)
}

type fakeBlob struct {
	body        string
	contentType string
}

// mintCall is one request to the mint route, recorded so a test can assert
// that the screen sent what the person chose.
type mintCall struct {
	Repo  string
	Scope string
	TTL   int
}

func newFakeOrigo() *fakeOrigo {
	at := time.Date(2026, 9, 10, 9, 41, 0, 0, time.UTC)
	return &fakeOrigo{
		repo: origo.Repo{
			ID: "1f2e3d", Owner: "infra", Slug: "origo",
			DefaultBranch: "main", Head: "9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d",
			SizeBytes: 4404019, PushedAt: &at, UpdatedAt: &at,
		},
		heads: []origo.Ref{
			{Name: "refs/heads/main", SHA: "9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d"},
			{Name: "refs/heads/next", SHA: "4c02f7e10b4d3a1e8f6b2c9d05a7e3f1b8c4d6e2"},
		},
		tags: []origo.Ref{
			{Name: "refs/tags/v1.4.2", SHA: "aaa1111", Peeled: "b71d998c0a1f2e3d4c5b6a7988990011aabbccdd"},
		},
		commits: []origo.Commit{{
			SHA:       "9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d",
			Parents:   []string{"4c02f7e10b4d3a1e8f6b2c9d05a7e3f1b8c4d6e2"},
			Author:    origo.Person{Name: "a.hoshino", Email: "aki@example.com", At: at},
			Committer: origo.Person{Name: "a.hoshino", Email: "aki@example.com", At: at},
			Message:   "web: collapse diffs over the render budget\n\nThe pathological case, handled.\n",
			Trailers:  []origo.Trailer{{Key: "Reviewed-by", Value: "m.okonkwo"}},
			Stats:     &origo.Stats{Files: 2, Additions: 3, Deletions: 1},
		}},
		trees: map[string][]origo.Entry{
			"": {
				{Path: "internal", Mode: "040000", Type: "tree", SHA: "t1"},
				{Path: "README.md", Mode: "100644", Type: "blob", SHA: "b1", Size: 42},
				{Path: "logo.png", Mode: "100644", Type: "blob", SHA: "b2", Size: 49152},
				{Path: "huge.json", Mode: "100644", Type: "blob", SHA: "b3", Size: 60 << 20},
				{Path: "big.txt", Mode: "100644", Type: "blob", SHA: "b5", Size: 10 << 20},
			},
			"internal": {
				{Path: "internal/diff.go", Mode: "100644", Type: "blob", SHA: "b4", Size: 120},
			},
		},
		blobs: map[string]fakeBlob{
			"b1": {body: "# Origo\n\nA git server.\n", contentType: "text/plain; charset=utf-8"},
			"b2": {body: "\x89PNG\r\n\x1a\n\x00\x00", contentType: "image/png"},
			"b4": {body: "package diff\n\nfunc F() {}\n", contentType: "text/plain; charset=utf-8"},
			"b5": {body: strings.Repeat("a line of text\n", 74898), contentType: "text/plain; charset=utf-8"},
		},
		diff: "diff --git a/internal/diff.go b/internal/diff.go\n" +
			"index 111..222 100644\n--- a/internal/diff.go\n+++ b/internal/diff.go\n" +
			"@@ -1,3 +1,5 @@ func F\n package diff\n-old line\n+new line\n+another\n+third\n",
		headers: map[string]http.Header{},
		status:  map[string]int{},
	}
}

func (f *fakeOrigo) record(r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := r.URL.Path
	if q := r.URL.Query(); len(q) > 0 {
		keys := make([]string, 0, len(q))
		for k := range q {
			keys = append(keys, k)
		}
		call += "?" + strings.Join(sorted(keys), "&")
	}
	f.calls = append(f.calls, call)
	f.tokens = append(f.tokens, r.Header.Get("Authorization"))
	if strings.Contains(r.URL.Path, "/blob/") {
		f.ranges = append(f.ranges, r.Header.Get("Range"))
	}
}

func sorted(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s
}

// Calls returns the paths a screen asked for, with the query keys but not
// their values, which is the shape the spec's table names.
func (f *fakeOrigo) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeOrigo) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls, f.tokens, f.ranges = nil, nil, nil
}

// blobRanges is the Range header of every blob call since the last reset.
func (f *fakeOrigo) blobRanges() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ranges...)
}

func (f *fakeOrigo) Tokens() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.tokens...)
}

func (f *fakeOrigo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.record(r)
	p := r.URL.Path

	f.mu.Lock()
	maps.Copy(w.Header(), f.headers[p])
	code := f.status[p]
	// Every read Origo answers names the object it resolved to (spec 009),
	// which is what a screen builds a pinned address from.
	if w.Header().Get("Origo-Commit") == "" && f.repo.Head != "" {
		w.Header().Set("Origo-Commit", f.repo.Head)
	}
	f.mu.Unlock()
	if code != 0 {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "denied"}})
		return
	}

	switch {
	case r.Method == http.MethodDelete && strings.HasPrefix(p, "/v1/repos/"):
		f.deleteRoute(w, p)
	case strings.HasSuffix(p, "/tokens"):
		f.tokenRoute(w, r, p)
	case p == "/v1/repos" && r.Method == http.MethodPost:
		f.createRoute(w, r)
	case p == "/v1/repos":
		w.WriteHeader(http.StatusNotImplemented)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "no_directory"}})
	case p == "/v1/repos/"+f.repo.ID:
		writeJSON(w, f.repo)
	case strings.HasSuffix(p, "/refs"):
		if strings.HasPrefix(r.URL.Query().Get("prefix"), "refs/tags/") {
			writeJSON(w, f.tags)
			return
		}
		writeJSON(w, f.heads)
	case strings.Contains(p, "/commits/"):
		writeJSON(w, f.commits[0])
	case strings.HasSuffix(p, "/commits"):
		writeJSON(w, map[string]any{"commits": f.commits, "next_cursor": f.nextCursor})
	case strings.Contains(p, "/compare/"):
		w.Header().Set("Content-Type", "text/x-diff")
		_, _ = w.Write([]byte(f.diff))
	case strings.Contains(p, "/tree/"):
		writeJSON(w, map[string]any{"entries": f.trees[r.URL.Query().Get("path")], "next_cursor": f.nextCursor})
	case strings.Contains(p, "/blob/"):
		sha := p[strings.LastIndex(p, "/")+1:]
		b, ok := f.blobs[sha]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "ref_not_found"}})
			return
		}
		w.Header().Set("Content-Type", b.contentType)
		if r.Header.Get("Range") != "" && !f.ignoreRange {
			w.WriteHeader(http.StatusPartialContent)
		}
		_, _ = w.Write([]byte(b.body))
	default:
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "repo_not_found"}})
	}
}

// tokenRoute is the one route of the installation that is not a read: a
// POST mints, and a GET is not served at all, which is what every
// installation answers today.
func (f *fakeOrigo) tokenRoute(w http.ResponseWriter, r *http.Request, p string) {
	if r.Method != http.MethodPost {
		f.mu.Lock()
		records := f.registry
		f.mu.Unlock()
		if records == nil {
			w.WriteHeader(http.StatusNotImplemented)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "not_implemented"}})
			return
		}
		writeJSON(w, map[string]any{"tokens": records})
		return
	}
	var body struct {
		Scope string `json:"scope"`
		TTL   int    `json:"ttl"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	id := strings.TrimSuffix(strings.TrimPrefix(p, "/v1/repos/"), "/tokens")
	f.mu.Lock()
	f.minted = append(f.minted, mintCall{Repo: id, Scope: body.Scope, TTL: body.TTL})
	code := f.mintStatus
	f.mu.Unlock()
	if code != 0 {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "forbidden"}})
		return
	}
	writeJSON(w, map[string]any{
		"token":      "orig.a.signed.token." + body.Scope,
		"expires_at": time.Now().Add(time.Duration(body.TTL) * time.Second).UTC(),
	})
}

// Minted is every mint request the installation received.
func (f *fakeOrigo) Minted() []mintCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]mintCall(nil), f.minted...)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// harness is a server wired to a fake installation.
type harness struct {
	t       *testing.T
	server  *Server
	fake    *fakeOrigo
	oidc    *oidc.Client
	cfg     config.Config
	backend *httptest.Server

	// registry is the component that records who owns a repository, and
	// registryServer is where it answers. Both are here for every harness,
	// because the creation screen is one of the screens a page-wide
	// property has to hold on.
	registry       *fakeRegistry
	registryServer *httptest.Server

	// keys is the fake key store, wired to the server only when the
	// configuration turns the key screen on.
	keys     *fakeKeys
	keysHTTP *httptest.Server

	// csrfCookies is what the last rendered form left behind.
	csrfCookies []*http.Cookie
}

const testCookieKey = "0123456789abcdef0123456789abcdef"

func newHarness(t *testing.T, opts ...func(*config.Config)) *harness {
	t.Helper()
	fake := newFakeOrigo()
	backend := httptest.NewServer(fake)
	t.Cleanup(backend.Close)

	base, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	reg := newFakeRegistry()
	regServer := httptest.NewServer(reg)
	t.Cleanup(regServer.Close)

	// The registry is the identity provider: the component that issues the
	// token is the one that holds the names and the record of who owns
	// what, which is why there is one address here and not two.
	oc := oidc.Config{
		AuthURL: regServer.URL, ClientID: "origoweb",
		RedirectURL: "https://code.example/auth/callback", CookieKey: testCookieKey,
		Audience: "origo",
	}
	cfg := config.Config{
		Addr: ":0", OrigoURL: base, PublicURL: mustURL(t, "https://code.example"),
		CloneHost: mustURL(t, "https://git.example"), OIDC: oc,
	}
	for _, o := range opts {
		o(&cfg)
	}
	sessions, err := session.New(cfg.OIDC)
	if err != nil {
		t.Fatal(err)
	}
	// The key store exists in the harness either way; the server is given
	// a client for it only when the configuration names one, which is the
	// switch an operator has.
	fakeStore := newFakeKeys()
	storeHTTP := httptest.NewServer(fakeStore)
	t.Cleanup(storeHTTP.Close)
	var keyClient *keys.Client
	if cfg.KeysURL != nil {
		keyClient = keys.New(mustURL(t, storeHTTP.URL), storeHTTP.Client())
	}
	sealer := oc
	sealer.CookieName = session.CookieName
	sealer.SessionTTL = session.Lifetime
	return &harness{
		t: t, fake: fake, backend: backend, cfg: cfg, oidc: oidc.New(sealer),
		registry: reg, registryServer: regServer,
		keys: fakeStore, keysHTTP: storeHTTP,
		server: New(Options{
			Config: cfg, Sessions: sessions,
			API: origo.New(base, backend.Client()),
			// Built from the configuration the way the binary builds it,
			// so a test that unsets the provider gets an interface with
			// no registry rather than one wired past its own settings.
			Registry: registry.New(cfg.RegistryURL(), regServer.Client()),
			Keys:     keyClient,
		}),
	}
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// signedIn returns the cookie a request carries to be somebody. The session
// holds a subject and nothing else, which is the least an issuer can say.
func (h *harness) signedIn(subject string) *http.Cookie {
	h.t.Helper()
	return h.signedInAs(oidc.User{Sub: subject})
}

// signedInAs is the same cookie for a session whose claims a test chooses,
// so what one issuer says about a person and what another says are both
// renderable.
func (h *harness) signedInAs(u oidc.User) *http.Cookie {
	h.t.Helper()
	rec := httptest.NewRecorder()
	if err := h.oidc.SetSession(rec, &oidc.Session{
		AccessToken: "token-for-" + u.Sub,
		Expiry:      time.Now().Add(time.Hour),
		User:        u,
	}); err != nil {
		h.t.Fatal(err)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == session.CookieName {
			return c
		}
	}
	h.t.Fatalf("no session cookie was written")
	return nil
}

// get issues one request, with the cookies given.
func (h *harness) get(path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)
	return rec
}

// post submits one form, with a valid token for the session given.
func (h *harness) post(path string, form url.Values, c *http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	if form == nil {
		form = url.Values{}
	}
	form.Set(authkit.CSRFFieldName(), h.csrf(path, c))
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(c)
	for _, ck := range h.csrfCookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)
	return rec
}

// csrf reads a token out of a rendered form, together with the cookie that
// half of it lives in, so a submission is the one the browser would send.
func (h *harness) csrf(path string, c *http.Cookie) string {
	h.t.Helper()
	rec := h.get(path, c)
	h.csrfCookies = rec.Result().Cookies()
	m := csrfValue.FindStringSubmatch(rec.Body.String())
	if m == nil {
		h.t.Fatalf("no form token on %s", path)
	}
	return m[1]
}

var csrfValue = regexp.MustCompile(`name="` + regexp.QuoteMeta(authkit.CSRFFieldName()) + `" value="([^"]+)"`)

// answering points the interface at an installation that answers the way the
// handler given answers, which is how a test reaches a state the fixture
// installation does not produce: a directory, an empty one, a refusal.
func (h *harness) answering(handler http.HandlerFunc) {
	h.t.Helper()
	srv := httptest.NewServer(handler)
	h.t.Cleanup(srv.Close)
	h.server = New(Options{
		Config:   h.cfg,
		Sessions: mustSessions(h.t, h.cfg),
		API:      origo.New(mustURL(h.t, srv.URL), srv.Client()),
	})
}

// repoPath is the address of the fixture repository in this interface.
func (h *harness) repoPath(suffix string) string {
	return "/r/" + h.fake.repo.ID + suffix
}
