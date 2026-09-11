// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/latere-ai/origo-web/internal/config"
	"github.com/latere-ai/origo-web/internal/origo"
)

// req is what one request carries into a screen: the reader's token, which
// is empty when there is none, and the frame every page renders.
type req struct {
	tok string
	v   view
}

// begin reads the session once per request. A refreshed session is written
// back to the cookie here, before any call to Origo, so a call never races
// the expiry boundary. The token and the name of whoever holds it come from
// that one read, so the page and the call it makes cannot disagree about who
// is signed in.
func (s *Server) begin(w http.ResponseWriter, r *http.Request, section string) req {
	reader := s.sessions.Read(w, r)
	return req{
		tok: reader.Token,
		v: view{
			Section:     section,
			SignedIn:    reader.Token != "",
			Who:         reader.Who,
			KeysEnabled: s.keys != nil,
			CanCreate:   s.cfg.RegistryURL() != nil,
			CSRF:        s.sessions.CSRFToken(w, r),
			Product:     s.cfg.Name(),
			ProjectName: config.ProjectName,
			ProjectURL:  s.cfg.Project(),
			Mark:        s.cfg.Mark,
		},
	}
}

// signInData is the front door, and the one page written for a stranger.
type signInData struct {
	View        view
	ButtonLabel string
	CloneHTTPS  string
	CloneSSH    string
	ReturnTo    string
	Refused     bool

	// Hosted says this installation carries a name of its own, so the page
	// says which project it is an instance of rather than claiming to be
	// the project.
	Hosted bool
}

// signInPage is the whole page, built the same way for the visitor who asked
// for the front door and the one whose credential was refused on a repository.
//
// The notice is the one claim this page can get wrong, so it is not the call
// site's to make: it follows the credential the request actually carried. A
// session that presented nothing was not refused, it was never asked.
//
// Nobody is signed in on this page, so it names no account: the session
// behind a refusal has just been cleared, and there was never one behind the
// front door.
func (s *Server) signInPage(rq req, returnTo string) signInData {
	v := rq.v
	v.Title = "Sign in"
	v.SignedIn = false
	v.Who = ""
	label := "Sign in"
	if s.cfg.IssuerName != "" {
		label = "Continue with " + s.cfg.IssuerName
	}
	return signInData{
		View:        v,
		ButtonLabel: label,
		CloneHTTPS:  s.cfg.CloneHTTPS("<owner>", "<name>"),
		CloneSSH:    s.cfg.CloneSSH("<owner>", "<name>"),
		ReturnTo:    returnTo,
		Refused:     rq.tok != "",
		Hosted:      s.cfg.Hosted(),
	}
}

// signIn renders the sign-in screen. It is what a signed-out visitor sees on
// every repository URL today, because Origo requires a credential on every
// read (spec 007) and has no anonymous path yet. The page keeps the address
// the person asked for, so signing in lands there and not on the home page.
//
// The status is the caller's, because it belongs to the address and not to
// the page: a repository is served to a credential and to nothing else, so a
// request without one is a 401 there, while the front door asks for nothing
// and answers 200.
func (s *Server) signIn(w http.ResponseWriter, r *http.Request, rq req, status int) {
	s.render(w, r, status, "signin", s.signInPage(rq, returnTo(r)))
}

// frontDoorStatus is what the front door answers when it cannot list.
//
// A visitor who has presented nothing is not being refused anything: they
// have landed on a page that asks them to sign in, and it is a document, so
// it answers 200. A session that did present a credential and was turned away
// is a refusal, and that is a 401. A browser renders the body either way; a
// link checker, a crawler and an agent read the code.
func frontDoorStatus(rq req) int {
	if rq.tok != "" {
		return http.StatusUnauthorized
	}
	return http.StatusOK
}

// returnTo is the path to come back to after signing in: this request's own
// path, and never a path from another origin.
func returnTo(r *http.Request) string {
	p := r.URL.Path
	if r.URL.RawQuery != "" {
		p += "?" + r.URL.RawQuery
	}
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return "/"
	}
	return p
}

func (s *Server) handleSignIn(w http.ResponseWriter, r *http.Request) {
	rq := s.begin(w, r, "")
	if rq.tok != "" {
		http.Redirect(w, r, safeReturn(r.URL.Query().Get("return_to")), http.StatusFound)
		return
	}
	s.render(w, r, http.StatusOK, "signin",
		s.signInPage(rq, safeReturn(r.URL.Query().Get("return_to"))))
}

// handleAuthStart hands the browser to the issuer. It is a GET because it
// changes nothing here: the session is written by the callback.
func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	s.sessions.SignIn(w, r, safeReturn(r.URL.Query().Get("return_to")))
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	s.sessions.Callback(w, r)
}

func (s *Server) handleSignOut(w http.ResponseWriter, r *http.Request) {
	if !s.sessions.CSRFValid(r) {
		http.Error(w, "This form has expired. Go back and try again.", http.StatusForbidden)
		return
	}
	s.sessions.Clear(w)
	http.Redirect(w, r, "/sign-in", http.StatusSeeOther)
}

func safeReturn(p string) string {
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return "/"
	}
	return p
}

// homeData is the home screen: the directory when the installation can serve
// one, and the way in without it when it cannot.
type homeData struct {
	View      view
	Owners    []ownerGroup
	Count     int
	Next      string
	Directory bool
	Recent    []listRow
	Query     string
	Stale     string

	// CanFilter says the whole directory arrived in this one answer, so a
	// filter over it covers everything the reader may see. Origo has no
	// filter parameter and no way to ask a question across pages, so a
	// filter offered beside a cursor would search one page and look like
	// it had searched the directory. It is offered when it is complete
	// and withheld when it is not.
	CanFilter bool
}

// ownerGroup is one owner's repositories. The directory is grouped because
// a name is unique under an owner and not across the server, so a flat list
// repeats the owner on every row to say which `origo` a row means.
type ownerGroup struct {
	Owner string
	Repos []listRow
}

type listRow struct {
	Name          string
	Owner         string
	URL           string
	DefaultBranch string
	Size          string
	Pushed        string
	PushedExact   string
	ID            string
}

// handleHome lists the repositories the reader may see.
//
// Origo cannot answer that question today: it has no collection route, and
// its authorizer contract is a yes/no oracle over one named repository with
// no enumerate verb (spec 023). So the screen asks, and on an installation
// that cannot answer it degrades to the way in that works without a
// directory: the address of a repository, and the ones this session has
// already opened. It does not guess the list from a token claim, which would
// be a second access-control model and would diverge from the authorizer's
// answer the first time the two disagreed.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	rq := s.begin(w, r, "")
	rq.v.Title = "Repositories"

	data := homeData{View: rq.v, Query: strings.TrimSpace(r.URL.Query().Get("q"))}
	page, err := s.api.List(r.Context(), rq.tok, r.URL.Query().Get("cursor"), 50)
	switch {
	case err == nil:
		data.Directory = true
		data.Next = page.Next
		data.Stale = staleSentence(page.Meta)
		data.CanFilter = page.Next == "" && r.URL.Query().Get("cursor") == ""
		rows := make([]listRow, 0, len(page.Items))
		for _, repo := range page.Items {
			row := s.listRow(repo)
			if data.CanFilter && !matches(row, data.Query) {
				continue
			}
			rows = append(rows, row)
		}
		data.Count = len(rows)
		data.Owners = groupByOwner(rows)
	case origo.Unauthenticated(err):
		// A credential the installation turned away is a dead one, so
		// it goes. A visitor who presented none has nothing to clear
		// and is told nothing about a credential.
		if rq.tok != "" {
			s.sessions.Clear(w)
		}
		s.signIn(w, r, rq, frontDoorStatus(rq))
		return
	default:
		// ErrNoDirectory, and every other refusal of a route this
		// installation does not serve, are the same screen: the one
		// that works without a directory.
		data.Directory = false
	}

	for _, id := range s.sessions.Recent(r) {
		data.Recent = append(data.Recent, listRow{ID: id, Name: id, URL: "/r/" + url.PathEscape(id)})
	}
	s.render(w, r, http.StatusOK, "home", data)
}

// matches reports whether a row answers the filter a reader typed. The filter
// is one string against the owner and the name, folded for case, because that
// is the whole of what a reader can see to type.
func matches(row listRow, q string) bool {
	if q == "" {
		return true
	}
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(row.Name), q) ||
		strings.Contains(strings.ToLower(row.Owner), q)
}

// groupByOwner gathers rows under their owner, keeping the order Origo sent
// and the order each owner first appeared in it.
func groupByOwner(rows []listRow) []ownerGroup {
	var out []ownerGroup
	at := map[string]int{}
	for _, row := range rows {
		i, seen := at[row.Owner]
		if !seen {
			at[row.Owner] = len(out)
			out = append(out, ownerGroup{Owner: row.Owner})
			i = len(out) - 1
		}
		out[i].Repos = append(out[i].Repos, row)
	}
	return out
}

func (s *Server) listRow(repo origo.Repo) listRow {
	row := listRow{
		ID:            repo.ID,
		Name:          repo.Slug,
		Owner:         repo.Owner,
		URL:           "/r/" + url.PathEscape(repo.ID),
		DefaultBranch: repo.DefaultBranch,
		Size:          humanSize(repo.SizeBytes),
	}
	if repo.PushedAt != nil && !repo.PushedAt.IsZero() {
		row.Pushed = relTime(*repo.PushedAt)
		row.PushedExact = absTime(*repo.PushedAt)
	}
	return row
}

// handleOpen takes the identifier a person typed on the home screen and
// sends them to it. It is a GET form and a redirect, so the address bar
// carries the repository and the page can be bookmarked.
func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/r/"+url.PathEscape(id), http.StatusSeeOther)
}
