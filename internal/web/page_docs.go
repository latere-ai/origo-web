// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
)

// docsData is the documentation page. Every address on it comes from
// configuration, so the page a reader copies from is their installation's.
type docsData struct {
	View       view
	APIBase    string
	CloneHost  string
	CloneHTTPS string
	CloneSSH   string
	RepoRead   string
	RateLimit  int
	MaxTTL     string
}

// handleAgentDocs renders the one page written for whoever is configuring an
// agent this afternoon. It needs no session: a person who has not signed in
// still has to read how to get a token.
func (s *Server) handleAgentDocs(w http.ResponseWriter, r *http.Request) {
	rq := s.begin(w, r, "docs")
	rq.v.Title = "Agent access"
	s.render(w, r, http.StatusOK, "agents", docsData{
		View:       rq.v,
		APIBase:    s.cfg.APIURL(),
		CloneHost:  hostOf(s.cfg.CloneHTTPS("<owner>", "<name>")),
		CloneHTTPS: s.cfg.CloneHTTPS("<owner>", "<name>"),
		CloneSSH:   s.cfg.CloneSSH("<owner>", "<name>"),
		RepoRead:   s.cfg.RepoAPIURL("<id>"),
		MaxTTL:     "one hour",
	})
}
