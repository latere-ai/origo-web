// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/latere-ai/origo-web/internal/keys"
)

// fakeKeys is a key store the screen can be driven against.
//
// It answers the three calls the real store answers and it holds one
// installation's keys in a map. It parses no key material: what a paste
// becomes is whatever the test said it becomes, because the interface must
// not depend on how a key is read and a fake that parsed one would hide a
// screen that had started to.
type fakeKeys struct {
	mu sync.Mutex

	// rows is what the store holds, in the order it answers with.
	rows []keys.Key

	// parse is what a paste is read as. A test replaces it to drive a
	// refusal or a particular fingerprint.
	parse func(pasted string) (keys.Key, int, string)

	// forbid answers every call 403, which is the installation whose
	// client was never granted the scope.
	forbid bool
	// down answers every call 500, and downNext answers that many calls 500
	// and then answers normally, which is the store that fails a read and
	// is up again by the next one.
	down     bool
	downNext int
	// unauthenticated answers every call 401.
	unauthenticated bool

	// calls records the method and path of every call, so a test can
	// assert the screen made the calls it should and no others.
	calls []string

	// bearer records the credential each call carried, and pastes the
	// public key each add sent, so a test can assert the interface sends
	// the reader's own token and the line exactly as typed.
	bearer []string
	pastes []string
}

func newFakeKeys() *fakeKeys {
	f := &fakeKeys{}
	f.parse = func(pasted string) (keys.Key, int, string) {
		fields := strings.Fields(pasted)
		if len(fields) < 2 || !strings.HasPrefix(fields[0], "ssh-") {
			return keys.Key{}, http.StatusBadRequest, keys.CodeInvalidKey
		}
		k := keys.Key{
			Type:        fields[0],
			Bits:        256,
			Fingerprint: "SHA256:" + strings.TrimPrefix(fields[1], "AAAA"),
		}
		if len(fields) > 2 {
			k.Comment = fields[2]
		}
		return k, 0, ""
	}
	return f
}

// add puts a key in the store without going through the screen.
func (f *fakeKeys) add(k keys.Key) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, k)
}

func (f *fakeKeys) held() []keys.Key {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]keys.Key(nil), f.rows...)
}

func (f *fakeKeys) made() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// reset forgets what was called, so a test can count the calls of one step
// without the harness's own reads in the way.
func (f *fakeKeys) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
	f.bearer = nil
	f.pastes = nil
}

func (f *fakeKeys) bearers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.bearer...)
}

// lastPaste is the public key the most recent add carried.
func (f *fakeKeys) lastPaste() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pastes) == 0 {
		return ""
	}
	return f.pastes[len(f.pastes)-1]
}

func (f *fakeKeys) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	f.bearer = append(f.bearer, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	forbid, down, unauth := f.forbid, f.down, f.unauthenticated
	if f.downNext > 0 {
		f.downNext--
		down = true
	}
	f.mu.Unlock()

	switch {
	case unauth:
		writeKeyErr(w, http.StatusUnauthorized, "unauthorized")
		return
	case forbid:
		writeKeyErr(w, http.StatusForbidden, "forbidden")
		return
	case down:
		writeKeyErr(w, http.StatusInternalServerError, "internal_error")
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/me/ssh-keys":
		writeKeyJSON(w, http.StatusOK, map[string]any{"keys": f.held()})
	case r.Method == http.MethodPost && r.URL.Path == "/me/ssh-keys":
		f.serveAdd(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/me/ssh-keys/"):
		f.serveDelete(w, strings.TrimPrefix(r.URL.Path, "/me/ssh-keys/"))
	default:
		writeKeyErr(w, http.StatusNotFound, "not_found")
	}
}

func (f *fakeKeys) serveAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PublicKey string `json:"public_key"`
		Confirm   bool   `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeKeyErr(w, http.StatusBadRequest, "invalid_request")
		return
	}
	f.mu.Lock()
	parse := f.parse
	f.pastes = append(f.pastes, body.PublicKey)
	f.mu.Unlock()

	parsed, status, code := parse(body.PublicKey)
	if status != 0 {
		writeKeyErr(w, status, code)
		return
	}
	f.mu.Lock()
	for _, held := range f.rows {
		if held.Fingerprint == parsed.Fingerprint {
			f.mu.Unlock()
			writeKeyErr(w, http.StatusConflict, keys.CodeAlreadyYours)
			return
		}
	}
	f.mu.Unlock()

	if !body.Confirm {
		// The preview stores nothing and carries no id, exactly as the
		// real store answers.
		writeKeyJSON(w, http.StatusOK, map[string]any{"key": parsed, "stored": false})
		return
	}
	now := time.Now()
	parsed.ID = "key-" + parsed.Fingerprint
	parsed.Created = &now
	f.add(parsed)
	writeKeyJSON(w, http.StatusCreated, map[string]any{"key": parsed, "stored": true})
}

func (f *fakeKeys) serveDelete(w http.ResponseWriter, id string) {
	f.mu.Lock()
	removed := false
	for i, held := range f.rows {
		if held.ID == id {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			removed = true
			break
		}
	}
	f.mu.Unlock()
	if !removed {
		writeKeyErr(w, http.StatusNotFound, "not_found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeKeyJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeKeyErr(w http.ResponseWriter, status int, code string) {
	writeKeyJSON(w, status, map[string]string{
		"error": code, "message": "the store said so", "detail": "for a developer",
	})
}
