// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"errors"
	"net/http"
	"time"

	"latere.ai/x/pkg/cache"

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

// How long an answer about one repository's visibility is reused, and how
// many are kept.
//
// The overview needs the answer on every render and Origo cannot supply
// it: Origo holds no visibility, by the design of its spec 027, so the
// record the overview already fetches cannot carry the field and there is
// nothing to fold the call into. The call is irreducible, so it is
// cached instead.
//
// 30 seconds is chosen against the one change a reader notices, which is
// their own. A flip made here is written through to the cache by
// handleVisibilityPost below, so the person who made the change sees it
// at once and the lifetime never applies to them. What the lifetime does
// bound is a change made somewhere else: another session, or the
// registration API. Half a minute of a stale badge is a stated bound
// rather than an accidental one, and no decision is made from the cached
// value. The registry decides every read and every write, whatever the
// badge says.
//
// The key carries the account as well as the repository, because CanChange
// is an answer about a person. Two readers of one repository are two
// entries, and one reader's right to change it is never served to
// another.
const (
	visibilityTTL      = 30 * time.Second
	visibilityCacheMax = 4096
)

// newVisibilityCache builds the cache the server holds.
func newVisibilityCache() *cache.TTLCache[string, registry.Visibility] {
	return cache.New[string, registry.Visibility](visibilityTTL,
		cache.WithMaxSize[string, registry.Visibility](visibilityCacheMax))
}

// visibilityKey is one account's answer about one repository, and reports
// whether there is a key at all.
//
// The account is the subject claim, which names one principal. A display
// name does not: two people may call themselves the same thing, and an
// entry keyed by that is served to whichever of them asks second, which
// hands one person's private repository and one person's right to change
// it to another. A session whose issuer minted no subject has no key, and
// nothing about it is stored or reused.
func visibilityKey(sub, id string) (string, bool) {
	if sub == "" {
		return "", false
	}
	return sub + "\x00" + id, true
}

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
	// Written through rather than dropped, so the overview this redirect
	// lands on shows what was just chosen. The lifetime above bounds a
	// change made elsewhere and never a change made here.
	if key, keyed := visibilityKey(rc.sub, rc.repo.ID); keyed {
		s.visibility.Set(key, registry.Visibility{Visibility: target, CanChange: true})
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
