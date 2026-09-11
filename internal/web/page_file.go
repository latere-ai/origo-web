// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"bytes"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"latere.ai/x/pkg/md"

	"github.com/latere-ai/origo-web/internal/config"
	"github.com/latere-ai/origo-web/internal/origo"
)

// ---- tree ----

type treeData struct {
	View    view
	Repo    *repoView
	Path    string
	Crumbs  []crumb
	Parent  string
	Entries []entryView
	NextURL string
	History string
	// Count says how many entries this page of the directory holds. Origo
	// pages a tree by cursor and returns no total, so it counts the rows on
	// the screen and never claims to count the directory.
	Count string
	Stale string
	Empty bool

	// NoCommits says the repository has nothing to list at any path: nobody
	// has pushed to it. The screen keeps its sections and says what to do
	// next, in place of a listing.
	NoCommits bool
}

// handleTree lists one directory at one revision.
//
// One call, never recursive: a repository with a hundred thousand files
// costs one page of five thousand entries. The last commit that touched each
// entry is deliberately absent, because that is one call per row and Origo
// offers no batch for it; the directory's history is one link away instead.
func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.openRepo(w, r, "tree")
	if !ok {
		return
	}
	path := cleanPath(r.PathValue("path"))
	data := treeData{
		View:    rc.v,
		Repo:    rc.rv,
		Path:    path,
		Crumbs:  pathCrumbs(rc.rv.URL(), rc.rv.RefQueryValue(), path),
		Stale:   rc.stale,
		History: rc.rv.URL() + "/log?" + logQuery(rc.rv, path),
	}

	// A repository with no commit has no tree to list. Origo answers its
	// tree with a missing reference, which is also what it answers for a
	// branch that was never pushed, so the question is put to the
	// references first, the way the overview puts it, and the screen says
	// what to do next in place of a listing. Rendering the missing
	// reference as "this repository does not exist" under that
	// repository's own name was a dead end: the reader had just seen it.
	if rc.repo.Head == "" {
		branches, _, _, err := s.refsFor(r, rc.tok, rc.repo.ID)
		if err != nil {
			s.readFailedIn(w, r, rc, err)
			return
		}
		if len(branches) == 0 {
			data.NoCommits = true
			rc.v.Title = rc.repo.Slug + " at " + rc.ref
			data.View = rc.v
			s.render(w, r, http.StatusOK, "tree", data)
			return
		}
	}

	page, err := s.api.Tree(r.Context(), rc.tok, rc.repo.ID, rc.ref, origo.TreeOptions{
		Path: path, Cursor: r.URL.Query().Get("cursor"),
	})
	if err != nil {
		s.readFailedIn(w, r, rc, err)
		return
	}
	data.Entries = s.entryViews(rc.rv, page.Items)
	data.Count = entryCount(len(page.Items))
	data.Stale = firstNonEmpty(rc.stale, staleSentence(page.Meta))
	data.Empty = len(page.Items) == 0
	if path != "" {
		data.Parent = rc.rv.URL() + "/tree/" + escapePath(parentPath(path)) + rc.rv.RefQuery()
	}
	if page.Next != "" {
		q := url.Values{}
		setIfNotEmpty(q, "ref", rc.rv.RefQueryValue())
		q.Set("cursor", page.Next)
		data.NextURL = rc.rv.URL() + "/tree/" + escapePath(path) + "?" + q.Encode()
	}
	rc.v.Title = firstNonEmpty(path, rc.repo.Slug) + " at " + rc.ref
	data.View = rc.v
	s.render(w, r, http.StatusOK, "tree", data)
}

// entryCount says how many entries one page of a tree holds. Origo pages a
// tree by cursor and returns no total, so this counts the rows on the screen
// and never claims to count the directory.
func entryCount(n int) string {
	if n == 1 {
		return "1 entry"
	}
	return itoa(n) + " entries"
}

func logQuery(rv *repoView, path string) string {
	q := url.Values{}
	setIfNotEmpty(q, "ref", rv.RefQueryValue())
	setIfNotEmpty(q, "path", path)
	return q.Encode()
}

func (s *Server) entryViews(rv *repoView, entries []origo.Entry) []entryView {
	out := make([]entryView, 0, len(entries))
	q := rv.RefQuery()
	for _, e := range entries {
		v := entryView{Name: e.Name(), Type: entryType(e), Size: humanSize(e.Size)}
		if e.IsDir() {
			v.Name += "/"
			v.Size = ""
			v.URL = rv.URL() + "/tree/" + escapePath(e.Path) + q
		} else {
			v.URL = rv.URL() + "/blob/" + escapePath(e.Path) + q
		}
		out = append(out, v)
	}
	return out
}

// entryType names what a tree entry is, in words a person reads: a symbolic
// link and an executable are the two a listing must distinguish.
func entryType(e origo.Entry) string {
	switch {
	case e.Type == "tree":
		return "directory"
	case e.Type == "commit":
		return "submodule"
	case e.Mode == "120000":
		return "symlink"
	case e.Mode == "100755":
		return "executable"
	default:
		return "file"
	}
}

// ---- file ----

type blobData struct {
	View       view
	Repo       *repoView
	Path       string
	Crumbs     []crumb
	Name       string
	Size       string
	Type       string
	Mode       string
	Lines      []sourceLine
	LineCount  int
	Truncated  bool
	TooLarge   bool
	Binary     bool
	RawURL     string
	HistoryURL string
	CloneHTTPS string
	// Pin says whether this address moves, and carries the address of the
	// other state. It is nil where Origo named no commit for the read.
	Pin *pinView
	// Copy is what a reader is likely to quote from this page.
	Copy  []copyItem
	Stale string
}

// pinView is the one strip that says whether the address a reader is on
// shows the same bytes tomorrow.
//
// An address naming a branch or a tag shows other bytes after the next push;
// an address naming a commit shows the same bytes forever. The difference is
// between a citation that holds and one that drifts, so the screen says which
// one it is and offers the other. Origo names the object every read resolved
// to in Origo-Commit, so the pinned address costs no second call.
type pinView struct {
	// Ref is what the address names: the reference it follows, or the short
	// id it is pinned to.
	Ref string
	// Pinned reports that the address names one commit.
	Pinned bool
	// Note is the one sentence that says what that means.
	Note string
	// URL and Link are the way to the other state.
	URL  string
	Link string
}

func newPinView(rv *repoView, base, resolved string) *pinView {
	if resolved == "" {
		return nil
	}
	resolved = strings.ToLower(resolved)
	short := origo.Short(resolved)
	if ref := strings.ToLower(rv.Ref); isObjectID(ref) && strings.HasPrefix(resolved, ref) {
		return &pinView{
			Ref: short, Pinned: true,
			Note: "This address always shows these bytes.",
			URL:  base, Link: "View current on " + rv.DefaultBranch,
		}
	}
	return &pinView{
		Ref:  rv.Ref,
		Note: "The next push can change what this address shows.",
		URL:  base + "?ref=" + url.QueryEscape(resolved),
		Link: "Pin to " + short,
	}
}

// permalink is the whole address of this file at the commit the read
// resolved to, which is what a citation of it has to carry. It is empty where
// Origo named no commit, because an address at a reference is not one.
func permalink(cfg config.Config, rv *repoView, path, resolved string) string {
	if resolved == "" {
		return ""
	}
	return cfg.Absolute(rv.URL() + "/blob/" + escapePath(path) + "?ref=" + url.QueryEscape(resolved))
}

// isObjectID reports whether a reference is written as an object id: hex, and
// long enough for git to take it as an abbreviation.
func isObjectID(ref string) bool {
	if len(ref) < 7 || len(ref) > 64 {
		return false
	}
	for _, r := range ref {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

type sourceLine struct {
	N    int
	Text string
}

// renderLimit is how many bytes of a file this interface renders. Past it
// the screen shows the first part and says so: the cap is enforced by the
// request, with a Range, so the rest is never sent.
const renderLimit = 1 << 20

// fetchLimit is where the interface stops asking at all. It is Origo's own
// blob cap (spec 009), above which a read without a Range is refused, so a
// file this large is a download and a clone and not a page.
const fetchLimit = 50 << 20

// handleBlob shows one file.
//
// The tree call comes first and is what makes the size known before any byte
// is fetched: spec 009's blob route takes a sha and no path, so the entry has
// to be found anyway, and finding it also decides whether to fetch at all.
func (s *Server) handleBlob(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.openRepo(w, r, "tree")
	if !ok {
		return
	}
	path := cleanPath(r.PathValue("path"))
	entry, meta, ok := s.findEntry(w, r, rc, path)
	if !ok {
		return
	}

	rawURL := rc.rv.URL() + "/raw/" + escapePath(path) + rc.rv.RefQuery()
	// The file's own bytes, with the type Origo detected. It is this
	// screen's one machine view.
	rc.v.Alternates = []alternate{{Name: "raw", Type: "text/plain", Href: rawURL}}
	data := blobData{
		View:       rc.v,
		Repo:       rc.rv,
		Path:       path,
		Crumbs:     pathCrumbs(rc.rv.URL(), rc.rv.RefQueryValue(), path),
		Name:       entry.Name(),
		Size:       humanSize(entry.Size),
		Mode:       entry.Mode,
		Type:       entryType(entry),
		RawURL:     rawURL,
		HistoryURL: rc.rv.URL() + "/log?" + logQuery(rc.rv, path),
		CloneHTTPS: s.cfg.CloneHTTPS(rc.repo.Owner, rc.repo.Slug),
		Pin:        newPinView(rc.rv, rc.rv.URL()+"/blob/"+escapePath(path), meta.Commit),
		Stale:      rc.stale,

		// The permalink comes first: it is the address a citation of this
		// file should carry, and the one the strip above offers to move to.
		Copy: copyList(
			copyItem{Label: "Permalink", Value: permalink(s.cfg, rc.rv, path, meta.Commit)},
			copyItem{Label: "Commit", Value: meta.Commit},
			copyItem{Label: "Path", Value: path},
			copyItem{Label: "Clone address", Value: s.cfg.CloneHTTPS(rc.repo.Owner, rc.repo.Slug)},
		),
	}
	rc.v.Title = entry.Name()
	data.View = rc.v

	if entry.Size > fetchLimit {
		data.TooLarge = true
		s.render(w, r, http.StatusOK, "blob", data)
		return
	}

	// The cap is enforced by the request. Origo honours Range, so the
	// bytes past it are never sent, never read, and never discarded.
	blob, err := s.api.BlobRange(r.Context(), rc.tok, rc.repo.ID, entry.SHA, renderLimit)
	if err != nil {
		if origo.TooLarge(err) {
			data.TooLarge = true
			s.render(w, r, http.StatusOK, "blob", data)
			return
		}
		s.readFailedIn(w, r, rc, err)
		return
	}
	data.Type = describeType(blob.ContentType, data.Type)

	// A file is rendered as text only when the type Origo detected is
	// textual and the bytes decode as UTF-8. Everything else is a
	// download: name, size, mode and type, and no bytes.
	if !textual(blob.ContentType) || !utf8.Valid(blob.Body) {
		data.Binary = true
		s.render(w, r, http.StatusOK, "blob", data)
		return
	}
	// The notice follows the bytes and not the status: an installation
	// that ignored the Range would send the whole file, and one that
	// answered 200 with part of it would still have cut it.
	data.Truncated = int64(len(blob.Body)) < entry.Size
	data.Lines = sourceLines(string(blob.Body))
	data.LineCount = len(data.Lines)
	s.render(w, r, http.StatusOK, "blob", data)
}

// findEntry reads one directory to find one file's object id and size. It
// returns the read's own meta as well, because Origo names the commit it
// resolved to there and that is what a pinned address is built from.
func (s *Server) findEntry(w http.ResponseWriter, r *http.Request, rc repoContext, path string) (origo.Entry, origo.Meta, bool) {
	if path == "" {
		s.notFound(w, r, rc.v)
		return origo.Entry{}, origo.Meta{}, false
	}
	page, err := s.api.Tree(r.Context(), rc.tok, rc.repo.ID, rc.ref, origo.TreeOptions{Path: parentPath(path)})
	if err != nil {
		s.readFailedIn(w, r, rc, err)
		return origo.Entry{}, origo.Meta{}, false
	}
	for _, e := range page.Items {
		if e.Path == path && !e.IsDir() {
			return e, page.Meta, true
		}
	}
	s.notFound(w, r, rc.v)
	return origo.Entry{}, origo.Meta{}, false
}

// maxLineRunes bounds one rendered line. A minified file is one line of two
// megabytes, and a browser asked to lay that out stops being a browser.
const maxLineRunes = 4000

func sourceLines(text string) []sourceLine {
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	parts := strings.Split(text, "\n")
	out := make([]sourceLine, 0, len(parts))
	for i, p := range parts {
		p = strings.TrimSuffix(p, "\r")
		if utf8.RuneCountInString(p) > maxLineRunes {
			p = string([]rune(p)[:maxLineRunes]) + " …"
		}
		out = append(out, sourceLine{N: i + 1, Text: p})
	}
	return out
}

// textual reports whether the type Origo detected is one this interface
// renders as text.
func textual(ct string) bool {
	ct, _, _ = strings.Cut(ct, ";")
	ct = strings.TrimSpace(strings.ToLower(ct))
	switch {
	case ct == "":
		return false
	case strings.HasPrefix(ct, "text/"):
		return true
	case strings.HasSuffix(ct, "+json"), strings.HasSuffix(ct, "+xml"):
		return true
	}
	switch ct {
	case "application/json", "application/xml", "application/javascript",
		"application/x-sh", "application/x-yaml", "application/yaml":
		return true
	}
	return false
}

// describeType names the content type in the register of a person, falling
// back to what the tree said the entry is.
func describeType(ct, fallback string) string {
	ct, _, _ = strings.Cut(ct, ";")
	ct = strings.TrimSpace(ct)
	if ct == "" {
		return fallback
	}
	return ct
}

// ---- raw ----

// handleRaw copies the bytes through, as an attachment and with the type
// Origo detected. The body is streamed and never buffered, so a large file
// costs this service no memory.
func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.openRepo(w, r, "tree")
	if !ok {
		return
	}
	path := cleanPath(r.PathValue("path"))
	entry, _, ok := s.findEntry(w, r, rc, path)
	if !ok {
		return
	}
	resp, err := s.api.BlobStream(r.Context(), rc.tok, rc.repo.ID, entry.SHA)
	if err != nil {
		if origo.TooLarge(err) {
			rc.v.Title = "Too large"
			s.render(w, r, http.StatusOK, "message", messageData{
				View:    rc.v,
				Repo:    rc.rv,
				Heading: "File too large",
				Body:    "This file exceeds the size the server sends in one response. Clone the repository to read it.",
				Links:   []crumb{{Name: "Back", URL: rc.rv.URL() + "/blob/" + escapePath(path) + rc.rv.RefQuery()}},
			})
			return
		}
		s.readFailedIn(w, r, rc, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	h := w.Header()
	h.Set("Content-Type", firstNonEmpty(resp.Header.Get("Content-Type"), "application/octet-stream"))
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		h.Set("Content-Length", cl)
	}
	// An attachment, always. A repository holds bytes nobody vetted, and a
	// browser must never be invited to run them on this origin.
	h.Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(entry.Name()))
	_, _ = io.Copy(w, resp.Body)
}

// renderReadme turns a readme into HTML, or into text when it is not
// Markdown.
//
// The Markdown renderer is goldmark through latere.ai/x/pkg/md, configured
// without its unsafe option, so raw HTML inside a repository's readme is
// dropped rather than rendered. The page's Content-Security-Policy is the
// second line of the same defence.
func renderReadme(name string, body []byte) (html template.HTML, text string) {
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, ".md") && !strings.HasSuffix(lower, ".markdown") {
		return "", string(body)
	}
	out, err := md.Render(body)
	if err != nil {
		return "", string(body)
	}
	// goldmark runs without its unsafe option here, so raw HTML in the
	// source is dropped rather than rendered and what remains is the
	// renderer's own markup.
	return template.HTML(demoteHeadings(out)), ""
}

// demoteHeadings pushes every heading of a rendered readme down one level,
// so the page keeps exactly one h1 of its own and the readme's own headings
// nest under it. A document with two h1 elements has no outline, and a screen
// reader reads the outline.
func demoteHeadings(fragment []byte) []byte {
	nodes, err := html.ParseFragment(bytes.NewReader(fragment), &html.Node{
		Type: html.ElementNode, Data: "div", DataAtom: atom.Div,
	})
	if err != nil {
		return fragment
	}
	demote := map[string]string{"h1": "h2", "h2": "h3", "h3": "h4", "h4": "h5", "h5": "h6"}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if to, ok := demote[n.Data]; ok {
				n.Data, n.DataAtom = to, atom.Lookup([]byte(to))
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	var out bytes.Buffer
	for _, n := range nodes {
		walk(n)
		if err := html.Render(&out, n); err != nil {
			return fragment
		}
	}
	return out.Bytes()
}
