// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/latere-ai/origo-web/internal/registry"
)

// The one screen that brings a repository into being.
//
// Everything else here reads. This does not, and the boundary it keeps is
// the same one: it holds no credential, decides nothing about who may create
// what, and runs no git. A creation is one call to the registry with the
// person's own token. The registry decides whether the name is theirs, then
// writes its row and makes the repository at Origo as one operation, so it
// answers with both or with neither. The screen carries that answer back.
//
// It is not a write path to repository content. Nothing here edits a file,
// moves a reference, or changes a repository that exists; the result is an
// empty repository with a default branch and no commit, which is the one
// thing a person cannot obtain from any other screen and cannot obtain by
// pushing, because a push to a name that resolves to nothing is refused.

// nameFormatRe is the repository name this screen accepts. It is Origo's own
// label shape, checked here so a name is refused before the registry is
// asked.
var nameFormatRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// newRepoData is the creation screen.
type newRepoData struct {
	View view
	// Owners are the names this person may create under. Empty with an
	// empty Handle is a person who has claimed no name.
	Owners []registry.Namespace
	Handle string
	// AccountURL is where a person claims a name, which is the identity
	// provider's own account screen.
	AccountURL string
	// Form is what a refused submission carries back, so nothing is
	// retyped.
	Form  newRepoForm
	Error string
}

// newRepoForm holds a submission between a refusal and the next try.
type newRepoForm struct {
	Owner string
	Name  string
}

// The sentences this screen refuses with. Each is one thing that went wrong,
// in the words of the person it happened to.
const (
	newNameSentence    = "Choose an owner and a name. Names use letters, digits, . _ and -, up to 64 characters."
	newOwnerSentence   = "You cannot create repositories under that owner. Choose one of the names below."
	newTakenSentence   = "A repository with that name already exists under that owner. Choose another name."
	newLimitSentence   = "That owner has reached its repository limit. Remove one first, or ask your administrator to raise it."
	newRefusedSentence = "The repository was not created. The installation refused it. Ask your administrator."
	newNoneSentence    = "Repository creation is not available on this installation."
)

// handleNew renders the creation screen.
func (s *Server) handleNew(w http.ResponseWriter, r *http.Request) {
	rq := s.begin(w, r, "new")
	if !rq.v.CanCreate {
		// An installation that records who owns a repository nowhere has
		// no creation screen, for anybody, signed in or not. It is not a
		// refusal and it is not a missing credential.
		s.noCreation(w, r, rq.v)
		return
	}
	if !s.requirePlatform(w, r, rq) {
		return
	}
	s.renderNew(w, r, rq, http.StatusOK, newRepoForm{Owner: r.URL.Query().Get("owner")}, "")
}

// renderNew draws the screen, whether it was asked for or refused.
func (s *Server) renderNew(w http.ResponseWriter, r *http.Request, rq req, status int, form newRepoForm, refusal string) {
	rq.v.Title = "New repository"
	data := newRepoData{View: rq.v, Form: form, Error: refusal, AccountURL: s.cfg.AccountURL()}

	spaces, err := s.registry.Namespaces(r.Context(), rq.platform)
	switch {
	case err == nil:
		data.Owners, data.Handle = spaces.Namespaces, spaces.Handle
	case registry.Unauthenticated(err):
		// A credential the installation turned away is a dead one, which
		// is how every other screen answers it too.
		s.sessions.Clear(w)
		s.signIn(w, r, rq, http.StatusUnauthorized)
		return
	case errors.Is(err, registry.ErrNoRegistry):
		s.noCreation(w, r, rq.v)
		return
	default:
		s.unavailable(w, r, rq.v)
		return
	}
	// One name preselected, so a person with one namespace fills in a name
	// and nothing else.
	if data.Form.Owner == "" && len(data.Owners) > 0 {
		data.Form.Owner = data.Owners[0].Label
	}
	s.render(w, r, status, "new", data)
}

// handleNewPost creates one repository and sends the person to it.
//
// The registry is asked once. It writes the row and makes the repository at
// Origo inside one operation of its own, so an answer means both exist and a
// refusal means neither does. There is nothing here to take back and nothing
// to try again.
func (s *Server) handleNewPost(w http.ResponseWriter, r *http.Request) {
	if !s.sessions.CSRFValid(r) {
		http.Error(w, "This form has expired. Go back and try again.", http.StatusForbidden)
		return
	}
	rq := s.begin(w, r, "new")
	if !rq.v.CanCreate {
		s.noCreation(w, r, rq.v)
		return
	}
	if !s.requirePlatform(w, r, rq) {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderNew(w, r, rq, http.StatusBadRequest, newRepoForm{}, newNameSentence)
		return
	}
	form := newRepoForm{
		Owner: strings.TrimSpace(r.PostFormValue("owner")),
		Name:  strings.TrimSpace(r.PostFormValue("name")),
	}
	if form.Owner == "" || !nameFormatRe.MatchString(form.Name) {
		s.renderNew(w, r, rq, http.StatusBadRequest, form, newNameSentence)
		return
	}

	// The registry takes the id from its caller, and the same id names the
	// repository at Origo.
	repo, err := s.registry.Create(r.Context(), rq.platform, uuid.NewString(), form.Owner, form.Name)
	if err != nil {
		s.createRefused(w, r, rq, form, err)
		return
	}

	// The address is the registry's answer, which names the owner as the
	// registry holds it rather than in the case it was typed.
	s.sessions.Remember(w, r, repo.ID, repo.OwnerLabel+"/"+repo.Slug)
	http.Redirect(w, r, nameURL(repo.OwnerLabel, repo.Slug), http.StatusSeeOther)
}

// createRefused answers a refusal from the registry, which decides who may
// create under which name and carries back what Origo answered it.
func (s *Server) createRefused(w http.ResponseWriter, r *http.Request, rq req, form newRepoForm, err error) {
	switch {
	case registry.Unauthenticated(err):
		s.sessions.Clear(w)
		s.signIn(w, r, rq, http.StatusUnauthorized)
	case registry.CodeOf(err) == registry.CodeOrigoRefused:
		// Read by its code, because the registry answers it with the 502 it
		// also gives an Origo it could not reach, and the person is told a
		// different thing for each.
		s.renderNew(w, r, rq, http.StatusConflict, form, newRefusedSentence)
	case registry.Refused(err):
		s.renderNew(w, r, rq, http.StatusForbidden, form, newOwnerSentence)
	case registry.CodeOf(err) == registry.CodeAtTheLimit:
		s.renderNew(w, r, rq, http.StatusConflict, form, newLimitSentence)
	case registry.Conflict(err):
		// The name is taken, or the owner name holds another owner's
		// repositories. The registry gives the same code to a second change
		// on an id while the first is in flight, which a fresh id never
		// meets.
		s.renderNew(w, r, rq, http.StatusConflict, form, newTakenSentence)
	case registry.Invalid(err):
		s.renderNew(w, r, rq, http.StatusBadRequest, form, newNameSentence)
	case errors.Is(err, registry.ErrNoRegistry), registry.CodeOf(err) == registry.CodeOrigoUnavailable:
		s.noCreation(w, r, rq.v)
	default:
		s.unavailable(w, r, rq.v)
	}
}

// noCreation is the screen of an installation whose authorizer keeps no
// registry. Creating a repository needs a component that knows who owns a
// name, and an installation without one has repositories made some other
// way. It is a 404 because the address is not served here, not a refusal of
// the person asking.
func (s *Server) noCreation(w http.ResponseWriter, r *http.Request, v view) {
	v.Title = "New repository"
	s.render(w, r, http.StatusNotFound, "message", messageData{
		View:    v,
		Heading: "Not available",
		Body:    newNoneSentence,
	})
}
