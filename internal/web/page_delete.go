// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// The deletion screen.
//
// Deleting is the one thing this interface does to a repository that
// exists, and spec 023 says on what terms: asked about on a page that names
// the repository, with the name typed back, by the person's own credential,
// and answered by Origo as a hold and not a purge. The screen says what
// happens before it offers the button, because this interface runs no
// script and there is no dialog to say it in.

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
}

// deleteHold is how long Origo keeps the content of a deleted repository
// before purging it, its spec 020's DeleteHold, as the sentence the screen
// says. Origo reports the exact moment on the deletion itself, but the
// person reads this before they press the button.
const deleteHold = "seven days"

// deleteSentence is what a submission whose name does not match says.
const deleteSentence = "Type the repository's name exactly as shown to confirm."

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

	// Origo first, the ownership row second, which is the reverse of a
	// creation for the same reason. Origo asks the authorizer whether this
	// person may administer the repository, and the authorizer answers
	// from the row: a row withdrawn first would turn the deletion into a
	// refusal and leave the repository standing under no name. A row that
	// outlives the deletion holds the name until an operator removes it,
	// which is the lesser failure, and the log line below is what they
	// remove it from.
	if _, err := s.api.Delete(r.Context(), rc.tok, rc.repo.ID); err != nil {
		s.readFailed(w, r, rc.req, err)
		return
	}
	s.withdrawRow(r.Context(), rc.tok, rc.repo.ID)
	s.sessions.Forget(w, r, rc.repo.ID)
	s.visibility.Invalidate(visibilityKey(rc.v.Who, rc.repo.ID))
	http.Redirect(w, r, "/?deleted="+url.QueryEscape(name), http.StatusSeeOther)
}

// administers reports that the signed-in person may delete this repository,
// and has answered the request when they may not.
//
// The registry decides, because it holds the record of who owns what, and
// the question is the one it already answers for the visibility screen:
// who may change this repository is who administers it. Origo asks its
// authorizer the same question again on the deletion itself. A reader who
// is not an administrator gets the one refusal every refusal gets, so
// nobody learns which of absent and withheld it was; a signed-out visitor
// is sent to sign in.
func (s *Server) administers(w http.ResponseWriter, r *http.Request, rc repoContext) bool {
	if rc.tok == "" {
		s.signIn(w, r, rc.req, http.StatusOK)
		return false
	}
	current, err := s.registry.ReadVisibility(r.Context(), rc.tok, rc.repo.ID)
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

// withdrawRow removes the ownership row of a repository Origo has deleted,
// on a context detached from the request for the reason the creation
// screen gives: the person may have closed the tab, and a call on the
// request's own context would fail the moment the browser went.
func (s *Server) withdrawRow(ctx context.Context, tok, id string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), forgetTimeout)
	defer cancel()
	if err := s.registry.Forget(ctx, tok, id); err != nil {
		slog.ErrorContext(ctx, "origoweb: a registry row outlived the repository it named",
			"repository_id", id, "error", err)
	}
}

// renderDelete draws the screen.
func (s *Server) renderDelete(w http.ResponseWriter, r *http.Request, rc repoContext, status int, typed, refusal string) {
	name := rc.repo.Owner + "/" + rc.repo.Slug
	rc.v.Title = "Delete " + name
	s.render(w, r, status, "delete", deleteData{
		View: rc.v, Repo: rc.rv, Name: name, Typed: typed, Error: refusal,
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
