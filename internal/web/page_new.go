// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/latere-ai/origo-web/internal/origo"
	"github.com/latere-ai/origo-web/internal/registry"
)

// The one screen that brings a repository into being.
//
// Everything else here reads. This does not, and the boundary it keeps is
// the same one: it holds no credential, decides nothing about who may create
// what, and runs no git. A creation is two calls with the person's own
// token. The registry says who owns the name and writes the row; Origo makes
// the repository and asks its authorizer about the row. Refuse either and
// nothing is created.
//
// It is not a write path to repository content. Nothing here edits a file,
// moves a reference, or changes a repository that exists; the result is an
// empty repository with a default branch and no commit, which is the one
// thing a person cannot obtain from any other screen and cannot obtain by
// pushing, because a push to a name that resolves to nothing is refused.

// nameFormatRe is the repository name this screen accepts. It is Origo's own
// label shape, checked here so a name is refused before a registry row is
// written rather than after.
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
	newUnknownSentence = "The repository was not created. The installation does not recognise it, and trying again will not change that. Ask your administrator."
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
// The order is the registry first and Origo second, because the failure that
// leaves a repository unreachable is preferred to the one that leaves it
// unguarded: a row with no repository authorizes an address Origo answers
// 404 for, while a repository with no row would be a repository nobody can
// reach and nobody owns.
//
// Each half is one call. The registry reads a row it has no copy of through
// to its store, so a repository is usable at every replica the moment it is
// registered, and "unknown_repository" from Origo is a repository the
// registry has no row for rather than a replica that has not caught up.
// There is nothing for a second attempt to wait for.
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

	// The id is this service's to choose, which is what lets the row be
	// written before the repository exists and what names the same
	// repository in both halves.
	id := uuid.NewString()
	if _, err := s.registry.Create(r.Context(), rq.platform, id, form.Owner, form.Name); err != nil {
		s.createRefused(w, r, rq, form, err)
		return
	}

	repo, err := s.api.Create(r.Context(), rq.tok,
		origo.CreateRequest{ID: id, Owner: form.Owner, Slug: form.Name})
	if err != nil {
		// Nothing was created, so nothing is kept. A row left behind
		// would hold the name against its own owner.
		s.forget(r.Context(), rq.platform, id)
		s.originRefused(w, r, rq, form, err)
		return
	}

	s.sessions.Remember(w, r, repo.ID, repo.Owner+"/"+repo.Slug)
	http.Redirect(w, r, nameURL(repo.Owner, repo.Slug), http.StatusSeeOther)
}

// forgetTimeout bounds the compensating call, which runs on a context of its
// own and so needs a deadline of its own.
const forgetTimeout = 5 * time.Second

// forget removes the registry row a failed creation left behind.
//
// It runs on a context detached from the request, because the case it exists
// for is the person who closed the tab: the browser goes while the create at
// Origo is in flight, the request context is cancelled with it, and a
// compensating call made on that same context would fail at once and orphan
// the row it was added to remove. The deadline is its own and short, since
// nobody is waiting for it.
//
// A failure here is logged and not shown: the person has already been told
// the repository was not created, and a second sentence about a row they
// never saw would tell them nothing they can act on.
func (s *Server) forget(ctx context.Context, tok, id string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), forgetTimeout)
	defer cancel()
	if err := s.registry.Forget(ctx, tok, id); err != nil {
		slog.ErrorContext(ctx, "origoweb: a registry row outlived the creation that failed",
			"repository_id", id, "error", err)
	}
}

// createRefused answers a refusal from the registry, which is the half that
// decides who may create under which name.
func (s *Server) createRefused(w http.ResponseWriter, r *http.Request, rq req, form newRepoForm, err error) {
	switch {
	case registry.Unauthenticated(err):
		s.sessions.Clear(w)
		s.signIn(w, r, rq, http.StatusUnauthorized)
	case registry.Refused(err):
		s.renderNew(w, r, rq, http.StatusForbidden, form, newOwnerSentence)
	case registry.CodeOf(err) == registry.CodeAtTheLimit:
		s.renderNew(w, r, rq, http.StatusConflict, form, newLimitSentence)
	case registry.Conflict(err):
		s.renderNew(w, r, rq, http.StatusConflict, form, newTakenSentence)
	case registry.Invalid(err):
		s.renderNew(w, r, rq, http.StatusBadRequest, form, newNameSentence)
	case errors.Is(err, registry.ErrNoRegistry):
		s.noCreation(w, r, rq.v)
	default:
		s.unavailable(w, r, rq.v)
	}
}

// originRefused answers a refusal from Origo, which is the half that makes
// the repository. The row is already gone by the time this runs.
func (s *Server) originRefused(w http.ResponseWriter, r *http.Request, rq req, form newRepoForm, err error) {
	switch {
	case origo.Unauthenticated(err):
		s.sessions.Clear(w)
		s.signIn(w, r, rq, http.StatusUnauthorized)
	case origo.DeniedAs(err, origo.ReasonUnknownRepository):
		// The registry wrote the row and Origo's authorizer has no
		// repository for it. Nothing waiting will fix that, so the screen
		// says so rather than asking for another try.
		s.renderNew(w, r, rq, http.StatusConflict, form, newUnknownSentence)
	case origo.Absent(err):
		// Origo answers 403, 404 and 410 to the same question and says
		// which for none of them, so one sentence covers all three.
		s.renderNew(w, r, rq, http.StatusConflict, form, newRefusedSentence)
	case origo.Unavailable(err):
		s.unavailable(w, r, rq.v)
	default:
		// A name Origo will not take, and a conflict on a name Origo
		// already holds that the registry did not know about.
		s.renderNew(w, r, rq, http.StatusConflict, form, newTakenSentence)
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
