// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/latere-ai/origo-web/internal/origo"
	"github.com/latere-ai/origo-web/internal/registry"
)

// repoContext is what every repository screen resolves first: the
// repository, the reference being read, and the frame around them.
type repoContext struct {
	req
	repo  origo.Repo
	rv    *repoView
	ref   string
	stale string
}

// openRepo resolves the repository named in the path.
//
// The read is issued with the reader's token when there is one and with no
// Authorization header when there is not, and the answer is rendered as it
// arrives. There is no branch on "is there a session" here: on an
// installation that later answers a tokenless read, the same code path shows
// the repository.
func (s *Server) openRepo(w http.ResponseWriter, r *http.Request, section string) (repoContext, bool) {
	rq := s.begin(w, r, section)
	id := r.PathValue("id")

	repo, meta, err := s.api.Repo(r.Context(), rq.tok, id)
	if err != nil {
		s.readFailed(w, r, rq, err)
		return repoContext{}, false
	}

	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if ref == "" {
		ref = repo.DefaultBranch
	}
	rv := newRepoView(repo, ref)
	rq.v.Title = repo.Owner + "/" + repo.Slug
	s.sessions.Remember(w, r, repo.ID)
	return repoContext{req: rq, repo: repo, rv: rv, ref: ref, stale: staleSentence(meta)}, true
}

// readFailed turns a refusal from Origo into the one screen it deserves.
func (s *Server) readFailed(w http.ResponseWriter, r *http.Request, rq req, err error) {
	switch {
	case origo.Unauthenticated(err):
		// The token was refused. Clear the session and send the person
		// to sign in once; a repeat is an error page, not a loop.
		if rq.tok != "" {
			s.sessions.Clear(w)
		}
		s.signIn(w, r, rq, http.StatusUnauthorized)
	case origo.Absent(err):
		s.notFound(w, r, rq.v)
	case origo.Unavailable(err):
		s.unavailable(w, r, rq.v)
	default:
		s.broken(w, r, rq.v, err)
	}
}

// readFailedIn is readFailed for a read inside a repository the reader has
// already been shown.
//
// The repository answered, so the sentence for one that does not exist is
// false under that repository's own name, and a page with no sections on
// it is a dead end this interface's own link led to. What is not there, or
// withheld, is the branch, tag, commit or file the address names; Origo
// still does not say which, so neither does this. The status and the shape
// of the sentence stay. The sections stay too, at the default branch, so
// every one of them leads to a screen that renders whatever was missing.
func (s *Server) readFailedIn(w http.ResponseWriter, r *http.Request, rc repoContext, err error) {
	if !origo.Absent(err) {
		s.readFailed(w, r, rc.req, err)
		return
	}
	rv := *rc.rv
	rv.Ref = rv.DefaultBranch
	v := rc.v
	v.Title = "Not found"
	s.render(w, r, http.StatusNotFound, "message", messageData{
		View:    v,
		Repo:    &rv,
		Heading: "Not found",
		Body:    absentInRepoSentence,
		Links: []crumb{
			{Name: "Overview", URL: rv.URL()},
			{Name: "Branches and tags", URL: rv.URL() + "/refs"},
		},
	})
}

// refsFor reads the branches and the tags. Two calls, whatever the
// repository holds, because spec 009 pages references by prefix and not by
// row.
func (s *Server) refsFor(r *http.Request, tok, id string) (branches, tags []origo.Ref, truncated bool, err error) {
	branches, bm, err := s.api.Refs(r.Context(), tok, id, "refs/heads/")
	if err != nil {
		return nil, nil, false, err
	}
	tags, tm, err := s.api.Refs(r.Context(), tok, id, "refs/tags/")
	if err != nil {
		return nil, nil, false, err
	}
	sortRefs(branches)
	sortRefs(tags)
	return branches, tags, bm.Truncated || tm.Truncated, nil
}

func sortRefs(rs []origo.Ref) {
	sort.Slice(rs, func(i, j int) bool { return rs[i].Short() < rs[j].Short() })
}

// ---- overview ----

type overviewData struct {
	View       view
	Repo       *repoView
	Selector   refSelector
	Entries    []entryView
	TreeCursor string
	Last       *commitView
	Readme     *readmeView
	CloneHTTPS string
	CloneSSH   string

	// CloneIsSSH says which of the two addresses the screen is showing, and
	// CloneSSHURL is the address of this screen showing the other one.
	// Choosing between them is a link and a re-render, because the choice
	// is in the address and nothing here runs a script to swap two strings.
	CloneIsSSH  bool
	CloneSSHURL string

	// Visibility is what the screen says about who can read this
	// repository: "public", "private", "unknown", or empty on an
	// installation whose registry has no visibility surface, where the
	// question has no answer and the row is absent.
	//
	// "unknown" is a state and not the absence of one. If a failed
	// registry call rendered as private, a public repository would read
	// exactly like a private one, and it would be wrong in the direction
	// nobody reports: the reader sees a plausible screen and the person
	// who made it public is not told. So the screen says it does not
	// know, in words.
	Visibility string
	// VisibilityURL is where an administrator changes it, empty for
	// everybody else.
	VisibilityURL string

	ArchiveURL string
	Branches   int
	Tags       int
	Stale      string
	Empty      bool
}

type readmeView struct {
	Name string
	HTML template.HTML
	Text string
	URL  string
}

// refAndCloneQuery is this screen's own address asking for the SSH form,
// keeping whatever revision the reader is on.
func refAndCloneQuery(rv *repoView) string {
	if q := rv.RefQuery(); q != "" {
		return q + "&clone=ssh"
	}
	return "?clone=ssh"
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.openRepo(w, r, "overview")
	if !ok {
		return
	}
	data := overviewData{
		View:       rc.v,
		Repo:       rc.rv,
		CloneHTTPS: s.cfg.CloneHTTPS(rc.repo.Owner, rc.repo.Slug),
		CloneSSH:   s.cfg.CloneSSH(rc.repo.Owner, rc.repo.Slug),
		Stale:      rc.stale,
	}
	data.CloneIsSSH = data.CloneSSH != "" && r.URL.Query().Get("clone") == "ssh"
	data.CloneSSHURL = rc.rv.URL() + refAndCloneQuery(rc.rv)
	data.Visibility, data.VisibilityURL = s.visibilityOf(r, rc)

	branches, tags, _, err := s.refsFor(r, rc.tok, rc.repo.ID)
	if err != nil {
		s.readFailedIn(w, r, rc, err)
		return
	}
	data.Branches, data.Tags = len(branches), len(tags)
	data.Selector = refSelector{Action: rc.rv.URL(), Ref: rc.ref, Branches: branches, Tags: tags}

	// A repository with no commit is a state, not a failure. Origo answers
	// the log of an empty repository with an empty page, and its tree with
	// a missing reference, so the screen says what to do next instead.
	if rc.repo.Head == "" && len(branches) == 0 {
		data.Empty = true
		s.render(w, r, http.StatusOK, "overview", data)
		return
	}
	data.ArchiveURL = s.cfg.ArchiveURL(rc.repo.ID, rc.ref)

	tree, err := s.api.Tree(r.Context(), rc.tok, rc.repo.ID, rc.ref, origo.TreeOptions{})
	if err != nil {
		s.readFailedIn(w, r, rc, err)
		return
	}
	data.TreeCursor = tree.Next
	data.Entries = s.entryViews(rc.rv, tree.Items)
	if data.Stale == "" {
		data.Stale = staleSentence(tree.Meta)
	}

	log, err := s.api.Commits(r.Context(), rc.tok, rc.repo.ID, origo.CommitsOptions{Ref: rc.ref, Limit: 1})
	if err != nil {
		s.readFailedIn(w, r, rc, err)
		return
	}
	if len(log.Items) > 0 {
		c := newCommitView(log.Items[0], rc.rv.URL())
		data.Last = &c
	}

	if entry, found := findReadme(tree.Items); found {
		data.Readme = s.readme(r, rc, entry)
	}
	s.render(w, r, http.StatusOK, "overview", data)
}

// The three things the overview can say about who may read a repository,
// plus the empty string for an installation that has no answer to give.
const (
	visibilityPublic  = "public"
	visibilityPrivate = "private"
	visibilityUnknown = "unknown"
)

// visibilityOf answers what the overview says about who can read this
// repository, and where an administrator changes it.
//
// Three outcomes, not two. A registry that answers gives public or
// private. A registry that is reachable but has no visibility surface
// gives the empty string, and the screen says nothing, because on such an
// installation there is nothing to say. A registry that should have
// answered and did not gives "unknown", and the screen says so: rendering
// a failed call as private would make a public repository look private,
// which is a wrong answer rather than a missing one, and the reader would
// have no way to tell.
//
// A signed-out reader asks nothing and is told nothing.
func (s *Server) visibilityOf(r *http.Request, rc repoContext) (string, string) {
	if rc.tok == "" {
		return "", ""
	}
	key := visibilityKey(rc.v.Who, rc.repo.ID)
	current, ok := s.visibility.Get(key)
	if !ok {
		var err error
		current, err = s.registry.ReadVisibility(r.Context(), rc.tok, rc.repo.ID)
		switch {
		case err == nil:
			s.visibility.Set(key, current)
		case errors.Is(err, registry.ErrNoRegistry), registry.NotFound(err):
			// The installation keeps no visibility, or this reader may
			// not ask about this repository. Neither is a failure and
			// neither has anything to report.
			return "", ""
		default:
			return visibilityUnknown, ""
		}
	}
	url := ""
	if current.CanChange {
		url = rc.rv.URL() + "/visibility"
	}
	if current.Public() {
		return visibilityPublic, url
	}
	return visibilityPrivate, url
}

// findReadme picks the root readme, preferring the Markdown form. The tree
// page is already in hand, so this costs no call.
func findReadme(entries []origo.Entry) (origo.Entry, bool) {
	var plain origo.Entry
	var havePlain bool
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(strings.ToLower(e.Name()), "readme") {
			continue
		}
		lower := strings.ToLower(e.Name())
		if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
			return e, true
		}
		if !havePlain {
			plain, havePlain = e, true
		}
	}
	return plain, havePlain
}

// readmeLimit is how much of a readme this service will render. A readme
// larger than this is a file, and the file screen is where a file belongs.
const readmeLimit = 512 << 10

func (s *Server) readme(r *http.Request, rc repoContext, e origo.Entry) *readmeView {
	v := &readmeView{
		Name: e.Name(),
		URL:  rc.rv.URL() + "/blob/" + escapePath(e.Path) + rc.rv.RefQuery(),
	}
	if e.Size > readmeLimit {
		return v
	}
	blob, err := s.api.BlobRange(r.Context(), rc.tok, rc.repo.ID, e.SHA, readmeLimit)
	if err != nil {
		return v
	}
	v.HTML, v.Text = renderReadme(e.Name(), blob.Body)
	return v
}

// ---- references ----

type refsData struct {
	View      view
	Repo      *repoView
	Branches  []refRow
	Tags      []refRow
	Truncated bool
	Stale     string
}

type refRow struct {
	Name   string
	Short  string
	Target string
	Sha    string
	Tree   string
	Log    string
	Commit string
}

func (s *Server) handleRefs(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.openRepo(w, r, "refs")
	if !ok {
		return
	}
	branches, tags, truncated, err := s.refsFor(r, rc.tok, rc.repo.ID)
	if err != nil {
		s.readFailedIn(w, r, rc, err)
		return
	}
	rc.v.Title = rc.repo.Owner + "/" + rc.repo.Slug + " references"
	s.render(w, r, http.StatusOK, "refs", refsData{
		View:      rc.v,
		Repo:      rc.rv,
		Branches:  refRows(rc.rv, branches),
		Tags:      refRows(rc.rv, tags),
		Truncated: truncated,
		Stale:     rc.stale,
	})
}

func refRows(rv *repoView, rs []origo.Ref) []refRow {
	out := make([]refRow, 0, len(rs))
	for _, ref := range rs {
		q := "?ref=" + url.QueryEscape(ref.Short())
		out = append(out, refRow{
			Name:   ref.Name,
			Short:  ref.Short(),
			Target: origo.Short(ref.Target()),
			Sha:    ref.Target(),
			Tree:   rv.URL() + "/tree/" + q,
			Log:    rv.URL() + "/log" + q,
			Commit: rv.URL() + "/commit/" + url.PathEscape(ref.Target()),
		})
	}
	return out
}

// ---- commit log ----

type logData struct {
	View    view
	Repo    *repoView
	Commits []commitView
	Path    string
	NextURL string
	// PageSize is how many commits the older link fetches, so the link
	// names the size of the step it takes.
	PageSize int
	Stale    string
	Empty    bool
}

// logPageSize is one screen of history. Origo's own default is the same.
const logPageSize = 50

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.openRepo(w, r, "log")
	if !ok {
		return
	}
	q := r.URL.Query()
	path := cleanPath(q.Get("path"))

	page, err := s.api.Commits(r.Context(), rc.tok, rc.repo.ID, origo.CommitsOptions{
		Ref: rc.ref, Path: path, Limit: logPageSize, Cursor: q.Get("cursor"),
	})
	if err != nil {
		s.readFailedIn(w, r, rc, err)
		return
	}

	data := logData{
		View:     rc.v,
		Repo:     rc.rv,
		Path:     path,
		PageSize: logPageSize,
		Stale:    firstNonEmpty(rc.stale, staleSentence(page.Meta)),
		Empty:    len(page.Items) == 0,
	}
	for _, c := range page.Items {
		data.Commits = append(data.Commits, newCommitView(c, rc.rv.URL()))
	}
	if page.Next != "" {
		next := url.Values{}
		setIfNotEmpty(next, "ref", rc.rv.RefQueryValue())
		setIfNotEmpty(next, "path", path)
		next.Set("cursor", page.Next)
		data.NextURL = rc.rv.URL() + "/log?" + next.Encode()
	}
	rc.v.Title = "Commits on " + rc.ref
	data.View = rc.v
	s.render(w, r, http.StatusOK, "log", data)
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func setIfNotEmpty(q url.Values, k, v string) {
	if v != "" {
		q.Set(k, v)
	}
}
