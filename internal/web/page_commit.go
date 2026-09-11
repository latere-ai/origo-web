// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/latere-ai/origo-web/internal/diff"
	"github.com/latere-ai/origo-web/internal/origo"
)

type commitData struct {
	View     view
	Repo     *repoView
	Commit   commitView
	Trailers []origo.Trailer
	Parents  []parentLink
	Stats    *origo.Stats
	Patch    diff.Patch
	Files    []fileView
	Root     bool
	Single   string
	PatchURL string
	TreeURL  string
	// ExpandURL opens every file the budget rendered shut, and is empty
	// when nothing on the screen is shut. CollapseURL is the way back, and
	// is set only while the screen is expanded.
	ExpandURL   string
	CollapseURL string
	// Copy is what a reader is likely to quote from this page.
	Copy      []copyItem
	Truncated bool
	Stale     string
}

type parentLink struct {
	Short string
	URL   string
}

// fileView is one file of a diff with the links its state needs.
type fileView struct {
	diff.File
	ID      string
	OnlyURL string
	BlobURL string
	LogURL  string
	// PatchURL is the whole commit as the bytes git wrote, which is the
	// answer for a file this page will not render. It is empty on a screen
	// that has no patch of its own, such as a comparison.
	PatchURL string
}

func (s *Server) handleCommit(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.openRepo(w, r, "log")
	if !ok {
		return
	}
	sha := r.PathValue("sha")

	commit, meta, err := s.api.Commit(r.Context(), rc.tok, rc.repo.ID, sha)
	if err != nil {
		s.readFailed(w, r, rc.req, err)
		return
	}
	rc.v.Title = commit.Subject()
	patchURL := rc.rv.URL() + "/patch/" + url.PathEscape(commit.SHA)
	// The whole commit as the bytes git wrote. It is the one machine view
	// this screen has: Origo serves the diff and this service passes it
	// through, so the link is a format of this page and not a second API.
	rc.v.Alternates = []alternate{{Name: "patch", Type: "text/x-patch", Href: patchURL}}
	data := commitData{
		View:     rc.v,
		Repo:     rc.rv,
		Commit:   newCommitView(commit, rc.rv.URL()),
		Trailers: commit.Trailers,
		Stats:    commit.Stats,
		PatchURL: patchURL,
		TreeURL:  rc.rv.URL() + "/tree/?ref=" + url.QueryEscape(commit.SHA),
		Stale:    firstNonEmpty(rc.stale, staleSentence(meta)),
		Single:   r.URL.Query().Get("path"),
	}
	data.Copy = copyList(
		copyItem{Label: "Commit", Value: commit.SHA},
		copyItem{Label: "Patch address", Value: s.cfg.Absolute(patchURL)},
		copyItem{Label: "Tree at this commit", Value: s.cfg.Absolute(data.TreeURL)},
		copyItem{Label: "Clone address", Value: s.cfg.CloneHTTPS(rc.repo.Owner, rc.repo.Slug)},
	)
	for _, p := range commit.Parents {
		data.Parents = append(data.Parents, parentLink{Short: origo.Short(p), URL: rc.rv.URL() + "/commit/" + url.PathEscape(p)})
	}

	// A commit with no parent has nothing to be compared against. It is a
	// state, not a failure: the screen says so and offers the tree.
	if len(commit.Parents) == 0 {
		data.Root = true
		s.render(w, r, http.StatusOK, "commit", data)
		return
	}

	base := commit.Parents[0]
	text, dm, err := s.api.Compare(r.Context(), rc.tok, rc.repo.ID, base, commit.SHA, data.Single)
	if err != nil {
		s.readFailed(w, r, rc.req, err)
		return
	}
	budget := diff.DefaultBudget
	if data.Single != "" {
		budget.Force = data.Single
	}
	expanded := r.URL.Query().Get("expand") == "1"
	if expanded {
		budget = openEvery(budget)
	}
	data.Patch = diff.Parse(text, budget)
	data.Patch.Truncated = dm.Truncated
	data.Truncated = dm.Truncated
	selfURL := rc.rv.URL() + "/commit/" + url.PathEscape(commit.SHA)
	data.Files = fileViews(rc.rv, data.Patch.Files, selfURL, commit.SHA, data.PatchURL)
	data.ExpandURL, data.CollapseURL = expandLinks(selfURL, data.Single, expanded, data.Patch)
	s.render(w, r, http.StatusOK, "commit", data)
}

// openEvery is the budget with the two shut-file rules off. A file whose body
// is over the per-file rule stays over it: that body was never rendered, and
// opening every file is not the same question as rendering one of them.
func openEvery(b diff.Budget) diff.Budget {
	b.Collapse, b.PerPatch = 0, 0
	return b
}

// expandLinks are the two addresses of the shut-file state: the one that
// opens every file, and the one back. Each is empty where it would do
// nothing, so a screen with nothing shut carries no control.
func expandLinks(selfURL, single string, expanded bool, p diff.Patch) (expand, collapse string) {
	q := url.Values{}
	setIfNotEmpty(q, "path", single)
	if expanded {
		back := selfURL
		if len(q) > 0 {
			back += "?" + q.Encode()
		}
		return "", back
	}
	var shut bool
	for _, f := range p.Files {
		if f.Collapsed {
			shut = true
			break
		}
	}
	if !shut {
		return "", ""
	}
	q.Set("expand", "1")
	return selfURL + "?" + q.Encode(), ""
}

func fileViews(rv *repoView, files []diff.File, selfURL, rev, patchURL string) []fileView {
	out := make([]fileView, 0, len(files))
	for i, f := range files {
		v := fileView{File: f, ID: "f" + itoa(i+1), PatchURL: patchURL}
		sep := "?"
		if strings.Contains(selfURL, "?") {
			sep = "&"
		}
		v.OnlyURL = selfURL + sep + "path=" + url.QueryEscape(f.Path)
		if f.Status != "deleted" {
			v.BlobURL = rv.URL() + "/blob/" + escapePath(f.Path) + "?ref=" + url.QueryEscape(rev)
		}
		v.LogURL = rv.URL() + "/log?ref=" + url.QueryEscape(rev) + "&path=" + url.QueryEscape(f.Path)
		out = append(out, v)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// handlePatch serves the diff as the bytes Origo produced, so a reader can
// pipe it into git apply. It is the same call the commit screen makes.
func (s *Server) handlePatch(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.openRepo(w, r, "log")
	if !ok {
		return
	}
	sha := r.PathValue("sha")
	commit, _, err := s.api.Commit(r.Context(), rc.tok, rc.repo.ID, sha)
	if err != nil {
		s.readFailed(w, r, rc.req, err)
		return
	}
	if len(commit.Parents) == 0 {
		s.notFound(w, r, rc.v)
		return
	}
	text, _, err := s.api.Compare(r.Context(), rc.tok, rc.repo.ID, commit.Parents[0], commit.SHA, "")
	if err != nil {
		s.readFailed(w, r, rc.req, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+origo.Short(commit.SHA)+".patch\"")
	_, _ = w.Write(text)
}

type compareData struct {
	View      view
	Repo      *repoView
	Base      string
	Head      string
	Patch     diff.Patch
	Files     []fileView
	Truncated bool
	Single    string
	Stale     string
	Ready     bool
}

// handleCompare is the same diff view between any two revisions, and the one
// screen that starts as a form: with no revisions named it asks for two.
func (s *Server) handleCompare(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.openRepo(w, r, "log")
	if !ok {
		return
	}
	q := r.URL.Query()
	base, head := strings.TrimSpace(q.Get("base")), strings.TrimSpace(q.Get("head"))
	rc.v.Title = "Compare"
	data := compareData{View: rc.v, Repo: rc.rv, Base: base, Head: head, Single: q.Get("path"), Stale: rc.stale}
	if base == "" || head == "" {
		s.render(w, r, http.StatusOK, "compare", data)
		return
	}

	text, meta, err := s.api.Compare(r.Context(), rc.tok, rc.repo.ID, base, head, data.Single)
	if err != nil {
		s.readFailed(w, r, rc.req, err)
		return
	}
	budget := diff.DefaultBudget
	if data.Single != "" {
		budget.Force = data.Single
	}
	data.Ready = true
	data.Patch = diff.Parse(text, budget)
	data.Patch.Truncated = meta.Truncated
	data.Truncated = meta.Truncated
	selfURL := rc.rv.URL() + "/compare?base=" + url.QueryEscape(base) + "&head=" + url.QueryEscape(head)
	data.Files = fileViews(rc.rv, data.Patch.Files, selfURL, head, "")
	data.Stale = firstNonEmpty(rc.stale, staleSentence(meta))
	s.render(w, r, http.StatusOK, "compare", data)
}
