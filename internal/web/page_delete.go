// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/latere-ai/origo-web/internal/registry"
)

// The deletion screen.
//
// Deleting is the one thing this interface does to a repository that
// exists, and spec 023 says on what terms: asked about on a page that names
// the repository, with the name typed back, by the person's own credential,
// and kept by Origo as a hold and not a purge. The screen says what happens
// before it offers the button, because this interface runs no script and
// there is no dialog to say it in.
//
// A deletion is one call to the registry, which decides whether this person
// may delete the repository, then removes its row and deletes it at Origo as
// one operation. The screen carries that answer back.

// deleteData is the screen.
type deleteData struct {
	View view
	Repo *repoView
	// Name is what has to be typed back: the owner and the slug.
	Name string
	// Typed is what was typed, on a refused submission, so nothing is
	// retyped.
	Typed string
	Error string
	// Hold is how long the server keeps the content, in words.
	Hold string
}

// deleteHold is how long Origo keeps the content of a deleted repository
// before purging it, its spec 020's DeleteHold, as the sentence the screen
// says. The person reads it before they press the button.
const deleteHold = "seven days"

// The sentences this screen refuses with.
const (
	// deleteSentence is what a submission whose name does not match says.
	deleteSentence        = "Type the repository's name exactly as shown to confirm."
	deleteBusySentence    = "The repository was not deleted. Another change to it is still in progress. Try again in a moment."
	deleteManagedSentence = "The repository was not deleted. It belongs to another service on this installation, and is deleted through that service."
	deleteRefusedSentence = "The repository was not deleted. The installation refused it. Ask your administrator."
	deleteNoneSentence    = "The repository was not deleted. Deleting repositories is not available on this installation."
)

// handleDelete draws the screen for one repository.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.openRepo(w, r, "overview")
	if !ok {
		return
	}
	if !s.administers(w, r, rc) {
		return
	}
	s.renderDelete(w, r, rc, http.StatusOK, "", "")
}

// handleDeletePost deletes the repository and lands on the list.
func (s *Server) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	if !s.sessions.CSRFValid(r) {
		http.Error(w, "This form has expired. Go back and try again.", http.StatusForbidden)
		return
	}
	rc, ok := s.openRepo(w, r, "overview")
	if !ok {
		return
	}
	if !s.administers(w, r, rc) {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderDelete(w, r, rc, http.StatusBadRequest, "", deleteSentence)
		return
	}
	name := rc.repo.Owner + "/" + rc.repo.Slug
	typed := strings.TrimSpace(r.PostFormValue("name"))
	if typed != name {
		s.renderDelete(w, r, rc, http.StatusBadRequest, typed, deleteSentence)
		return
	}

	if err := s.registry.Forget(r.Context(), rc.platform, rc.repo.ID); err != nil {
		s.deleteRefused(w, r, rc, typed, err)
		return
	}
	s.sessions.Forget(w, r, rc.repo.ID)
	if key, keyed := visibilityKey(rc.sub, rc.repo.ID); keyed {
		s.visibility.Invalidate(key)
	}
	http.Redirect(w, r, "/?deleted="+url.QueryEscape(name), http.StatusSeeOther)
}

// administers reports that the signed-in person may delete this repository,
// and has answered the request when they may not.
//
// The registry decides, because it holds the record of who owns what, and
// the question is the one it already answers for the visibility screen:
// who may change this repository is who administers it. The registry
// decides again on the deletion itself. A reader who is not an
// administrator gets the one refusal every refusal gets, so nobody learns
// which of absent and withheld it was; a signed-out visitor is sent to sign
// in.
func (s *Server) administers(w http.ResponseWriter, r *http.Request, rc repoContext) bool {
	if !s.requirePlatform(w, r, rc.req) {
		return false
	}
	current, err := s.registry.ReadVisibility(r.Context(), rc.platform, rc.repo.ID)
	if err != nil {
		s.visibilityFailed(w, r, rc, err)
		return false
	}
	if !current.CanChange {
		s.notFound(w, r, rc.v)
		return false
	}
	return true
}

// deleteRefused answers a deletion the registry refused. Nothing was deleted
// in any of these.
//
// A refusal about the repository's own state keeps the person on the
// screen with the name they typed. The rest are the answers the screen's
// first question gets: a refused credential signs in again, a repository
// this person may not delete is the one refusal, and an installation that
// did not answer is unavailable. Origo's refusal is read by its code,
// because the registry answers it with the 502 it also gives an Origo it
// could not reach.
func (s *Server) deleteRefused(w http.ResponseWriter, r *http.Request, rc repoContext, typed string, err error) {
	switch {
	case registry.CodeOf(err) == registry.CodeOrigoRefused:
		s.renderDelete(w, r, rc, http.StatusConflict, typed, deleteRefusedSentence)
	case registry.CodeOf(err) == registry.CodeRegistered:
		s.renderDelete(w, r, rc, http.StatusConflict, typed, deleteManagedSentence)
	case registry.CodeOf(err) == registry.CodeOrigoUnavailable:
		s.renderDelete(w, r, rc, http.StatusConflict, typed, deleteNoneSentence)
	case registry.Conflict(err):
		s.renderDelete(w, r, rc, http.StatusConflict, typed, deleteBusySentence)
	default:
		s.visibilityFailed(w, r, rc, err)
	}
}

// renderDelete draws the screen.
func (s *Server) renderDelete(w http.ResponseWriter, r *http.Request, rc repoContext, status int, typed, refusal string) {
	name := rc.repo.Owner + "/" + rc.repo.Slug
	rc.v.Title = "Delete " + name
	s.render(w, r, status, "delete", deleteData{
		View: rc.v, Repo: rc.rv, Name: name, Typed: typed, Error: refusal, Hold: deleteHold,
	})
}

// repoName is the shape of an owner and a slug, each in the characters a
// name may carry. The list repeats a deleted name from its own address,
// and repeats nothing that is not shaped like one.
var repoName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}/[A-Za-z0-9._-]{1,64}$`)

// deletedName is the name the list was sent to after a deletion, empty when
// the address carries none or carries something that is not a name.
func deletedName(q string) string {
	if repoName.MatchString(q) {
		return q
	}
	return ""
}
