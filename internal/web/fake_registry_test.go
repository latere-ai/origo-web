// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/latere-ai/origo-web/internal/registry"
)

// fakeRegistry is the component that records who owns a repository: the
// half of a creation Origo does not hold.
//
// It answers the two questions the creation screen asks, records every call
// with the credential it carried, and can be made to refuse each one the way
// a real registry refuses, so a test drives the screen's refusals without
// standing up an identity provider.
type fakeRegistry struct {
	mu    sync.Mutex
	calls []string
	// tokens is the Authorization header of every call, which is what
	// proves this service sends the person's own token and no credential
	// of its own.
	tokens []string

	spaces registry.Namespaces
	// written is every row the screen asked for, in order, and forgotten
	// every row it withdrew.
	written   []registry.Repository
	forgotten []string

	// createStatus and createBody override the answer of a write, so a
	// test can drive each refusal the screen renders.
	createStatus int
	createCode   string
	// namespacesStatus overrides the answer of the namespaces question.
	namespacesStatus int
	// absent makes every route answer 404, which is an installation whose
	// authorizer keeps no registry at all.
	absent bool

	// visibility is what the registry says about a repository, and
	// canChange whether this person may write it. visibilityStatus
	// overrides the answer, so a test can drive each refusal.
	visibility       string
	canChange        bool
	visibilityStatus int
	// visibilityWrites is every value the screen wrote, in order.
	visibilityWrites []string

	// perToken answers the visibility question differently for a named
	// credential, so a test can put two readers of one repository through
	// the interface and see which answer each is given.
	perToken map[string]registry.Visibility
}

// newFakeRegistry returns a registry holding one namespace, which is the
// ordinary case: a person with a name of their own.
func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{
		spaces: registry.Namespaces{
			Handle: "alice",
			Namespaces: []registry.Namespace{
				{OwnerType: "principal", OwnerID: "p-alice", Label: "alice", Name: "alice", Remaining: 97},
				{OwnerType: "org", OwnerID: "o-infra", Label: "infra", Name: "Infrastructure", Remaining: 12},
			},
		},
	}
}

func (f *fakeRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	f.tokens = append(f.tokens, r.Header.Get("Authorization"))
	absent, nsStatus := f.absent, f.namespacesStatus
	createStatus, createCode := f.createStatus, f.createCode
	f.mu.Unlock()

	if absent {
		http.NotFound(w, r)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repositories/namespaces":
		if nsStatus != 0 {
			writeRegistryError(w, nsStatus, "refused")
			return
		}
		f.mu.Lock()
		spaces := f.spaces
		f.mu.Unlock()
		writeJSON(w, spaces)
	case r.Method == http.MethodPost && r.URL.Path == "/repositories":
		if createStatus != 0 {
			writeRegistryError(w, createStatus, createCode)
			return
		}
		var body struct {
			ID         string `json:"id"`
			OwnerLabel string `json:"owner_label"`
			Slug       string `json:"slug"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeRegistryError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		row := registry.Repository{
			ID: body.ID, OwnerType: "principal", OwnerID: "p-alice",
			OwnerLabel: body.OwnerLabel, Slug: body.Slug,
		}
		f.mu.Lock()
		f.written = append(f.written, row)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, row)
	case strings.HasSuffix(r.URL.Path, "/visibility") &&
		(r.Method == http.MethodGet || r.Method == http.MethodPut):
		f.mu.Lock()
		status, current, can := f.visibilityStatus, f.visibility, f.canChange
		own, particular := f.perToken[r.Header.Get("Authorization")]
		f.mu.Unlock()
		if status != 0 {
			writeRegistryError(w, status, "refused")
			return
		}
		if r.Method == http.MethodGet {
			if particular {
				writeJSON(w, own)
				return
			}
			if current == "" {
				current = registry.Private
			}
			writeJSON(w, registry.Visibility{Visibility: current, CanChange: can})
			return
		}
		var body struct {
			Visibility string `json:"visibility"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeRegistryError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		f.mu.Lock()
		f.visibility = body.Visibility
		f.visibilityWrites = append(f.visibilityWrites, body.Visibility)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/repositories/"):
		f.mu.Lock()
		f.forgotten = append(f.forgotten, strings.TrimPrefix(r.URL.Path, "/repositories/"))
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// writeRegistryError writes the error document a registry refuses with.
func writeRegistryError(w http.ResponseWriter, status int, code string) {
	if code == "" {
		code = http.StatusText(status)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": code, "message": "a sentence for a developer", "detail": "the developer half",
	})
}

// Calls is what the screen asked, in order.
func (f *fakeRegistry) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// VisibilityWrites is every visibility the screen wrote, in order.
func (f *fakeRegistry) VisibilityWrites() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.visibilityWrites...)
}

// Tokens is the credential every call carried.
func (f *fakeRegistry) Tokens() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.tokens...)
}

// Written is every row the screen asked for.
func (f *fakeRegistry) Written() []registry.Repository {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]registry.Repository(nil), f.written...)
}

// Forgotten is every row the screen withdrew.
func (f *fakeRegistry) Forgotten() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.forgotten...)
}

// answerFor makes the visibility question answer this credential its own
// way, whatever the registry says to everybody else.
func (f *fakeRegistry) answerFor(bearer string, v registry.Visibility) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.perToken == nil {
		f.perToken = map[string]registry.Visibility{}
	}
	f.perToken[bearer] = v
}

// refuseCreate makes the next write answer this status and code.
func (f *fakeRegistry) refuseCreate(status int, code string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createStatus, f.createCode = status, code
}

// setNamespaces replaces what the screen is told it may create under.
func (f *fakeRegistry) setNamespaces(n registry.Namespaces) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spaces = n
}
