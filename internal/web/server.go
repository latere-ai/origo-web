// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

// Package web is the interface itself: the routes, the screens, and the
// templates that render them.
//
// Every screen is a document a browser can render with scripting off. There
// is no script anywhere in this package, and the Content-Security-Policy
// below says so, so a page that grew one would stop working rather than
// quietly become required.
package web

import (
	"net/http"
	"strings"

	"latere.ai/x/pkg/health"

	"github.com/latere-ai/origo-web/internal/config"
	"github.com/latere-ai/origo-web/internal/keys"
	"github.com/latere-ai/origo-web/internal/origo"
	"github.com/latere-ai/origo-web/internal/registry"
	"github.com/latere-ai/origo-web/internal/session"
)

// Options is what a server needs to run.
type Options struct {
	Config   config.Config
	Sessions *session.Manager
	API      *origo.Client
	// Registry writes the row that says who owns a repository, which is
	// the half of a creation Origo does not hold. Nil leaves the creation
	// screen absent, which is what an installation whose authorizer keeps
	// no registry gets.
	Registry *registry.Client

	// Keys is the installation's public key store, nil where the operator
	// runs none. With no store the key routes are not mounted and the
	// masthead carries no entry for them.
	Keys *keys.Client

	Version   string
	Commit    string
	BuildTime string
}

// Server serves the interface.
type Server struct {
	cfg      config.Config
	sessions *session.Manager
	api      *origo.Client
	registry *registry.Client
	keys     *keys.Client
	mux      *http.ServeMux
}

// Route is one entry of the route table.
type Route struct {
	Method  string
	Pattern string
}

// Routes is the whole route table, in one place so it can be asserted
// against. Everything here is a GET except five forms: sign-out, the mint
// form, the one that creates a repository, and the two on the key screen.
//
// None of them touches repository content. This interface has no write path
// to a repository of any kind: nothing here edits a file, moves a reference,
// renames, transfers, freezes or deletes anything. Minting writes nothing,
// because the token it asks for is signed and kept nowhere. Creating brings
// an empty repository into being, which is the one thing a person cannot
// obtain from any other screen and which changes no repository that exists.
// The key forms write to the installation's key store, which holds an
// account's credentials and no repository at all.
func Routes(keys bool) []Route {
	rs := []Route{
		{"GET", "/{$}"},
		{"GET", "/sign-in"},
		{"GET", "/open"},
		{"GET", "/auth/start"},
		{"GET", "/auth/callback"},
		{"POST", "/sign-out"},
		{"GET", "/assets/{file}"},
		{"GET", "/tokens"},
		{"POST", "/tokens"},
		{"GET", "/new"},
		{"POST", "/new"},
		{"GET", "/docs/agents"},
		{"GET", "/r/{id}"},
		{"GET", "/r/{id}/refs"},
		{"GET", "/r/{id}/log"},
		{"GET", "/r/{id}/commit/{sha}"},
		{"GET", "/r/{id}/patch/{sha}"},
		{"GET", "/r/{id}/compare"},
		{"GET", "/r/{id}/tree/{path...}"},
		{"GET", "/r/{id}/blob/{path...}"},
		{"GET", "/r/{id}/raw/{path...}"},
		{"GET", "/r/{id}/visibility"},
		{"POST", "/r/{id}/visibility"},
	}
	if keys {
		rs = append(rs,
			Route{"GET", "/keys"},
			Route{"POST", "/keys"},
			Route{"GET", "/keys/{id}/remove"},
			Route{"POST", "/keys/{id}/remove"},
		)
	}
	return rs
}

// New builds the server.
func New(o Options) *Server {
	s := &Server{
		cfg: o.Config, sessions: o.Sessions, api: o.API,
		registry: o.Registry, keys: o.Keys, mux: http.NewServeMux(),
	}

	handlers := map[string]http.HandlerFunc{
		"GET /{$}":                   s.handleHome,
		"GET /sign-in":               s.handleSignIn,
		"GET /open":                  s.handleOpen,
		"GET /auth/start":            s.handleAuthStart,
		"GET /auth/callback":         s.handleAuthCallback,
		"POST /sign-out":             s.handleSignOut,
		"GET /assets/{file}":         s.handleAsset,
		"GET /tokens":                s.handleTokens,
		"POST /tokens":               s.handleTokensPost,
		"GET /new":                   s.handleNew,
		"POST /new":                  s.handleNewPost,
		"GET /docs/agents":           s.handleAgentDocs,
		"GET /r/{id}":                s.handleOverview,
		"GET /r/{id}/refs":           s.handleRefs,
		"GET /r/{id}/log":            s.handleLog,
		"GET /r/{id}/commit/{sha}":   s.handleCommit,
		"GET /r/{id}/patch/{sha}":    s.handlePatch,
		"GET /r/{id}/compare":        s.handleCompare,
		"GET /r/{id}/tree/{path...}": s.handleTree,
		"GET /r/{id}/blob/{path...}": s.handleBlob,
		"GET /r/{id}/raw/{path...}":  s.handleRaw,
		"GET /r/{id}/visibility":     s.handleVisibility,
		"POST /r/{id}/visibility":    s.handleVisibilityPost,
		"GET /keys":                  s.handleKeys,
		"POST /keys":                 s.handleKeysPost,
		"GET /keys/{id}/remove":      s.handleKeyRemove,
		"POST /keys/{id}/remove":     s.handleKeyRemovePost,
	}
	for _, rt := range Routes(o.Keys != nil) {
		pattern := rt.Method + " " + rt.Pattern
		h, listed := handlers[pattern]
		if !listed {
			panic("origoweb: route " + pattern + " has no handler")
		}
		s.mux.HandleFunc(pattern, h)
	}

	// The probes and the metric registry go on the same listener, because
	// this service has one. They are not screens and carry no session.
	s.mux.Handle("/livez", health.Handler(health.Options{Version: o.Version, Commit: o.Commit, BuildTime: o.BuildTime}))
	s.mux.Handle("/readyz", health.Handler(health.Options{Version: o.Version, Commit: o.Commit, BuildTime: o.BuildTime}))
	s.mux.Handle("/version", health.Handler(health.Options{Version: o.Version, Commit: o.Commit, BuildTime: o.BuildTime}))
	return s
}

// ServeHTTP answers one request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()

	// No script, no frame, no third party. The policy is what makes "every
	// page works without JavaScript" a property of the deployment and not
	// only of the templates.
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'none'; "+
		"style-src 'self'; font-src 'self'; img-src 'self' data:; "+
		"form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "same-origin")

	// A rendered page belongs to the subject it was rendered for and to
	// nobody else. Origo's ETag is the repository's index sequence, the
	// same for every reader, so any shared cache keyed on it would hand
	// one person's private repository to the next visitor. This service
	// keeps no cache and asks every cache in front of it to keep none.
	if !strings.HasPrefix(r.URL.Path, "/assets/") {
		h.Set("Cache-Control", "private, no-store")
	}
	s.mux.ServeHTTP(w, r)
}

// handleAsset serves the stylesheet and the fonts out of the binary. They are
// this service's own bytes, the same for every reader, and nothing else is
// served from here: the list is closed, so an asset directory that grows a
// file does not grow a route.
func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	switch name {
	case "app.css",
		"plex-sans-latin.woff2",
		"plex-mono-400-latin.woff2",
		"plex-mono-500-latin.woff2",
		"plex-mono-600-latin.woff2":
	default:
		http.NotFound(w, r)
		return
	}
	body, err := assetFS.ReadFile("assets/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(name, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "font/woff2")
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(body)
}
