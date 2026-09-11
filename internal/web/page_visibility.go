// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"errors"
	"net/http"

	"github.com/latere-ai/origo-web/internal/registry"
)

// The visibility screen.
//
// A repository is private or public. A public one can be read and cloned by
// anyone, with no account. Origo does not hold that fact: the registry does,
// and the person's own token reads and writes it there.
//
// The change is its own screen and not a control on the overview, because
// the person has to read one sentence before they press the button, and
// this interface runs no script, so there is no dialog to read it in. The
// screen says what changes, then offers the button.

// visibilityData is the screen.
type visibilityData struct {
	View   view
	Repo   *repoView
	Public bool
	// Target is the visibility the button writes: the other one.
	Target string
	Error  string
}

// handleVisibility draws the screen for one repository.
func (s *Server) handleVisibility(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.openRepo(w, r, "overview")
	if !ok {
		return
	}
	if rc.tok == "" {
		s.signIn(w, r, rc.req, http.StatusOK)
		return
	}
	current, err := s.registry.ReadVisibility(r.Context(), rc.tok, rc.repo.ID)
	if err != nil {
		s.visibilityFailed(w, r, rc, err)
		return
	}
	if !current.CanChange {
		// A reader who is not an administrator gets the answer this
		// interface gives to every refusal: the same one an absent
		// repository gets, so nobody learns which it was.
		s.notFound(w, r, rc.v)
		return
	}
	s.renderVisibility(w, r, rc, current.Public(), http.StatusOK, "")
}

// handleVisibilityPost applies the change and returns to the repository.
func (s *Server) handleVisibilityPost(w http.ResponseWriter, r *http.Request) {
	if !s.sessions.CSRFValid(r) {
		http.Error(w, "This form has expired. Go back and try again.", http.StatusForbidden)
		return
	}
	rc, ok := s.openRepo(w, r, "overview")
	if !ok {
		return
	}
	if rc.tok == "" {
		s.signIn(w, r, rc.req, http.StatusOK)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderVisibility(w, r, rc, false, http.StatusBadRequest, visibilitySentence)
		return
	}
	target := r.PostFormValue("visibility")
	if target != registry.Public && target != registry.Private {
		s.renderVisibility(w, r, rc, target == registry.Private, http.StatusBadRequest, visibilitySentence)
		return
	}
	if err := s.registry.SetVisibility(r.Context(), rc.tok, rc.repo.ID, target); err != nil {
		s.visibilityFailed(w, r, rc, err)
		return
	}
	http.Redirect(w, r, rc.rv.URL(), http.StatusSeeOther)
}

// visibilitySentence is what a form the screen cannot read says.
const visibilitySentence = "Choose public or private and try again."

// renderVisibility draws the screen for the state the repository is in.
func (s *Server) renderVisibility(w http.ResponseWriter, r *http.Request, rc repoContext, public bool, status int, refusal string) {
	rc.v.Title = rc.repo.Owner + "/" + rc.repo.Slug
	target := registry.Public
	if public {
		target = registry.Private
	}
	s.render(w, r, status, "visibility", visibilityData{
		View: rc.v, Repo: rc.rv, Public: public, Target: target, Error: refusal,
	})
}

// visibilityFailed turns a refusal from the registry into a screen.
func (s *Server) visibilityFailed(w http.ResponseWriter, r *http.Request, rc repoContext, err error) {
	switch {
	case registry.Unauthenticated(err):
		s.sessions.Clear(w)
		s.signIn(w, r, rc.req, http.StatusUnauthorized)
	case errors.Is(err, registry.ErrNoRegistry), registry.NotFound(err), registry.Refused(err):
		s.notFound(w, r, rc.v)
	default:
		s.unavailable(w, r, rc.v)
	}
}
