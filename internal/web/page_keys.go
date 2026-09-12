// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/latere-ai/origo-web/internal/keys"
)

// The key screen: the keys this account has, adding one, and removing one.
//
// A key is the credential a clone over SSH presents, and this service holds
// no key knowledge at all. It does not parse a paste, does not compute a
// fingerprint and does not decide what is acceptable: the paste goes to the
// store, and the fingerprint a person checks before confirming is the one
// the store computed with the code that will hold the key. Two parsers
// could disagree about which key was saved; there is only one.

// keyRow is one key as the table renders it.
type keyRow struct {
	ID string

	// Comment is the label, which is the comment at the end of the line the
	// person pasted. Empty is ordinary: ssh-keygen writes user@host and a
	// key made another way may carry nothing.
	Comment string

	// Algorithm is the short name the screen puts beside a fingerprint,
	// "ed25519" or "rsa-4096": what the key is and how strong, in the two
	// words a person recognises.
	Algorithm string

	Fingerprint string
	Added       string
	AddedExact  string

	// LastUsed is when the key last opened a connection, and Unused says it
	// never has. The words differ because the fact does: a key that has
	// never been used usually means it can go.
	LastUsed string
	Unused   bool
}

// parsedKey is what the store said a paste would become, shown for checking
// before it is stored.
type parsedKey struct {
	Type        string
	Comment     string
	Fingerprint string

	// Pasted is the line itself, carried through the confirmation in a
	// hidden field so the second request stores exactly what the first
	// parsed. It is a public key, so it is not a secret in the page.
	Pasted string
}

// keysData is the key screen.
type keysData struct {
	View view
	Who  string
	Keys []keyRow

	// Confirm is the parsed key awaiting a yes, absent on a first visit.
	Confirm *parsedKey

	// Removing is the key a removal is asking about, absent otherwise. It
	// is picked out of the list renderKeys already read, by removing.
	Removing *keyRow

	// removing is the id of the key a removal is asking about. The screen
	// names the key the decision is about, and the list is the only place
	// that name is, so the id travels with the render rather than the row:
	// a handler that looked the row up itself would read the list twice
	// and answer from the first read while rendering the second.
	removing string

	// Paste survives a refusal so nothing has to be retyped, and Error is
	// the one sentence that says what was wrong with it.
	Paste string
	Error string

	// Unavailable says the store could not be reached at all, which is an
	// outage and not a refusal of anything the person did.
	Unavailable bool
}

// handleKeys renders the screen. It exists only where the installation runs
// a key store: Origo holds no public key of its own, so an operator who runs
// none has no place to add one and this screen and its navigation entry are
// both absent.
func (s *Server) handleKeys(w http.ResponseWriter, r *http.Request) {
	rq := s.begin(w, r, "keys")
	if rq.tok == "" {
		s.signIn(w, r, rq, http.StatusOK)
		return
	}
	s.renderKeys(w, r, rq, http.StatusOK, keysData{})
}

// renderKeys lists the keys and draws the screen. The list is read on every
// render, including after a refusal, so the table beside a refused form is
// never a stale one.
func (s *Server) renderKeys(w http.ResponseWriter, r *http.Request, rq req, status int, data keysData) {
	rq.v.Title = "SSH keys"
	data.View = rq.v
	data.Who = rq.v.Who

	rows, err := s.keys.List(r.Context(), rq.tok)
	switch {
	case err == nil:
		data.Keys = keyRows(rows)
		if data.removing != "" {
			status = pickRemoving(&data, status)
		}
	case keys.Unauthenticated(err):
		s.sessions.Clear(w)
		rq.tok = ""
		s.signIn(w, r, rq, http.StatusUnauthorized)
		return
	default:
		// A store that cannot list is a store that cannot add either, so
		// the page says so once instead of offering a form that fails.
		slog.ErrorContext(r.Context(), "origoweb: listing keys failed", "error", err)
		data.Unavailable = true
		data.Confirm = nil
		if status == http.StatusOK {
			status = http.StatusBadGateway
		}
	}
	s.render(w, r, status, "keys", data)
}

// pickRemoving finds the key a removal is asking about in the list just
// read, and says what the screen answers.
//
// A key that is not the reader's and one that does not exist are the same
// answer, which is what the store already does with the id.
func pickRemoving(data *keysData, status int) int {
	for i := range data.Keys {
		if data.Keys[i].ID == data.removing {
			data.Removing = &data.Keys[i]
			return status
		}
	}
	data.Error = "That key is not on your account."
	return http.StatusNotFound
}

// handleKeysPost is both steps of the add: a paste is parsed and shown, and
// a confirmation stores it.
//
// Two plain form submissions and no dialog. The first answers with the same
// screen carrying the parsed key, the second stores and redirects, so a
// reload after an add repeats nothing.
func (s *Server) handleKeysPost(w http.ResponseWriter, r *http.Request) {
	rq, ok := s.beginKeyWrite(w, r)
	if !ok {
		return
	}
	pasted := strings.TrimSpace(r.PostFormValue("key"))
	if pasted == "" {
		s.renderKeys(w, r, rq, http.StatusBadRequest, keysData{
			Error: "Paste a public key first.",
		})
		return
	}
	if r.PostFormValue("confirm") == "" {
		parsed, err := s.keys.Parse(r.Context(), rq.tok, pasted)
		if err != nil {
			s.refuseKey(w, r, rq, pasted, err)
			return
		}
		s.renderKeys(w, r, rq, http.StatusOK, keysData{Confirm: &parsedKey{
			Type:        parsed.Type,
			Comment:     parsed.Comment,
			Fingerprint: parsed.Fingerprint,
			Pasted:      pasted,
		}})
		return
	}
	if _, err := s.keys.Add(r.Context(), rq.tok, pasted); err != nil {
		s.refuseKey(w, r, rq, pasted, err)
		return
	}
	http.Redirect(w, r, "/keys", http.StatusSeeOther)
}

// handleKeyRemove asks before it removes. The list links here rather than
// posting from the table, so the page that takes the decision names the key
// the decision is about.
func (s *Server) handleKeyRemove(w http.ResponseWriter, r *http.Request) {
	rq := s.begin(w, r, "keys")
	if rq.tok == "" {
		s.signIn(w, r, rq, http.StatusOK)
		return
	}
	s.renderKeys(w, r, rq, http.StatusOK, keysData{removing: r.PathValue("id")})
}

// handleKeyRemovePost removes one key.
func (s *Server) handleKeyRemovePost(w http.ResponseWriter, r *http.Request) {
	rq, ok := s.beginKeyWrite(w, r)
	if !ok {
		return
	}
	if err := s.keys.Remove(r.Context(), rq.tok, r.PathValue("id")); err != nil {
		if keys.Unauthenticated(err) {
			s.sessions.Clear(w)
			rq.tok = ""
			s.signIn(w, r, rq, http.StatusUnauthorized)
			return
		}
		if keys.Absent(err) {
			s.renderKeys(w, r, rq, http.StatusNotFound, keysData{
				Error: "That key is not on your account.",
			})
			return
		}
		slog.ErrorContext(r.Context(), "origoweb: removing a key failed", "error", err)
		s.renderKeys(w, r, rq, http.StatusBadGateway, keysData{
			Error: "The key could not be removed. Try again in a few minutes.",
		})
		return
	}
	http.Redirect(w, r, "/keys", http.StatusSeeOther)
}

// beginKeyWrite reads the session and the CSRF token for a form submission.
// A form that has sat past its token is refused before anything is read.
func (s *Server) beginKeyWrite(w http.ResponseWriter, r *http.Request) (req, bool) {
	if !s.sessions.CSRFValid(r) {
		http.Error(w, "This form has expired. Go back and try again.", http.StatusForbidden)
		return req{}, false
	}
	rq := s.begin(w, r, "keys")
	if rq.tok == "" {
		s.signIn(w, r, rq, http.StatusOK)
		return req{}, false
	}
	return rq, true
}

// refuseKey renders a refused add with the paste preserved.
//
// Each sentence is written here, for the person. The store writes its own
// message for whoever is reading its API, and the two registers are not the
// same: a code is what crosses the wire and a sentence is what a reader
// meets.
func (s *Server) refuseKey(w http.ResponseWriter, r *http.Request, rq req, pasted string, err error) {
	if keys.Unauthenticated(err) {
		s.sessions.Clear(w)
		rq.tok = ""
		s.signIn(w, r, rq, http.StatusUnauthorized)
		return
	}
	status := http.StatusBadRequest
	var sentence string
	switch {
	case keys.Code(err) == keys.CodeAlreadyYours:
		sentence = "This key is already on your account. It is in the table above."
		status = http.StatusConflict
	case keys.Code(err) == keys.CodeAlreadyTaken:
		sentence = "This key is registered to another account. Add a different one."
		status = http.StatusConflict
	case keys.Code(err) == keys.CodeInvalidKey:
		sentence = "Invalid public key. Paste one line from a .pub file."
	case keys.Forbidden(err):
		// The installation's own configuration, not anything the person
		// did, so the sentence does not ask them to fix it.
		slog.ErrorContext(r.Context(), "origoweb: the key store refused this client", "error", err)
		sentence = "Key management is not enabled for this interface. Contact your administrator."
		status = http.StatusBadGateway
	default:
		slog.ErrorContext(r.Context(), "origoweb: adding a key failed", "error", err)
		sentence = "The key could not be added. Try again in a few minutes."
		status = http.StatusBadGateway
	}
	s.renderKeys(w, r, rq, status, keysData{Paste: pasted, Error: sentence})
}

// keyRows turns what the store holds into what the table shows.
func keyRows(in []keys.Key) []keyRow {
	out := make([]keyRow, 0, len(in))
	for _, k := range in {
		row := keyRow{
			ID:          k.ID,
			Comment:     k.Comment,
			Algorithm:   algorithmName(k.Type, k.Bits),
			Fingerprint: k.Fingerprint,
			Unused:      k.LastUsed == nil,
			LastUsed:    "never used",
		}
		if k.Created != nil && !k.Created.IsZero() {
			row.Added = absDate(*k.Created)
			row.AddedExact = absTime(*k.Created)
		}
		if k.LastUsed != nil && !k.LastUsed.IsZero() {
			row.LastUsed = relTime(*k.LastUsed)
			row.Unused = false
		}
		out = append(out, row)
	}
	return out
}

// algorithmName is the two words a person recognises a key by: the family
// and, where the family has more than one strength, the strength.
//
// ed25519 has one size, so naming it would say nothing; the ECDSA curves and
// the RSA moduli have several, and which one a key is matters, because a
// 2048 bit RSA key is the weakest thing an installation accepts.
func algorithmName(keyType string, bits int) string {
	switch keyType {
	case "ssh-ed25519":
		return "ed25519"
	case "ssh-rsa":
		if bits > 0 {
			return "rsa-" + strconv.Itoa(bits)
		}
		return "rsa"
	case "ssh-dss":
		return "dsa"
	}
	if curve, ok := strings.CutPrefix(keyType, "ecdsa-sha2-"); ok {
		return "ecdsa-" + curve
	}
	// An algorithm this build has no short name for is shown as the store
	// named it, which is a true statement and never a guess.
	return keyType
}

// RemoveURL is where the row's own removal is asked about.
func (r keyRow) RemoveURL() string { return "/keys/" + url.PathEscape(r.ID) + "/remove" }

// Label is what the first column shows. A key with no comment is named by
// nothing, so the column says that rather than inventing a name.
func (r keyRow) Label() string {
	if r.Comment == "" {
		return "no comment"
	}
	return r.Comment
}

// Named reports whether the pasted line carried a label.
func (r keyRow) Named() bool { return r.Comment != "" }
