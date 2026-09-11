// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package keys

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// The client of the key store: the three calls, the credential each carries,
// and the refusals a screen renders a sentence from.

// store is a recording key store a test drives the client against.
type store struct {
	t *testing.T

	status int
	body   string

	// seen is what the last call carried. raw is the request line itself,
	// because Path is already decoded and an escaped segment would read
	// there exactly as an unescaped one.
	method string
	path   string
	raw    string
	bearer string
	sent   map[string]any
}

func (s *store) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.method, s.path, s.raw = r.Method, r.URL.Path, r.RequestURI
	s.bearer = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if r.Body != nil {
		raw, _ := io.ReadAll(r.Body)
		s.sent = map[string]any{}
		_ = json.Unmarshal(raw, &s.sent)
	}
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, s.body)
}

func newStore(t *testing.T) (*store, *Client) {
	t.Helper()
	s := &store{t: t}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return s, New(base, srv.Client())
}

// TestListReadsTheKeys asserts the read: the path, the reader's own bearer,
// and every field a screen shows, including the null a key that has never
// authenticated carries.
func TestListReadsTheKeys(t *testing.T) {
	s, c := newStore(t)
	s.body = `{"keys":[
		{"id":"k1","comment":"aki@thinkpad","key_type":"ssh-ed25519","bits":256,
		 "fingerprint":"SHA256:one","created_at":"2026-02-04T09:12:00Z",
		 "last_used_at":"2026-09-11T08:00:00Z"},
		{"id":"k2","comment":"","key_type":"ssh-rsa","bits":4096,
		 "fingerprint":"SHA256:two","created_at":"2024-08-02T00:00:00Z","last_used_at":null}]}`

	got, err := c.List(context.Background(), "the-readers-token")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if s.method != http.MethodGet || s.path != "/me/ssh-keys" {
		t.Errorf("List called %s %s", s.method, s.path)
	}
	if s.bearer != "the-readers-token" {
		t.Errorf("List carried %q, want the reader's own token", s.bearer)
	}
	if len(got) != 2 {
		t.Fatalf("List returned %d keys", len(got))
	}
	if got[0].Comment != "aki@thinkpad" || got[0].Bits != 256 || got[0].Fingerprint != "SHA256:one" {
		t.Errorf("the first key reads as %+v", got[0])
	}
	if got[0].LastUsed == nil || !got[0].LastUsed.Equal(time.Date(2026, 9, 11, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("last used reads as %v", got[0].LastUsed)
	}
	// A key that has never authenticated carries no moment at all, which
	// is a different fact from a moment long ago.
	if got[1].LastUsed != nil {
		t.Errorf("a never-used key reports %v", got[1].LastUsed)
	}
}

// TestParseStoresNothingAndAddStores asserts the two steps differ by one
// field and by nothing else, so the store runs the same parse both times.
func TestParseStoresNothingAndAddStores(t *testing.T) {
	s, c := newStore(t)
	s.body = `{"key":{"key_type":"ssh-ed25519","bits":256,"comment":"aki@framework",
	           "fingerprint":"SHA256:Tz6Y"},"stored":false}`

	got, err := c.Parse(context.Background(), "tok", "ssh-ed25519 AAAA aki@framework")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.method != http.MethodPost || s.path != "/me/ssh-keys" {
		t.Errorf("Parse called %s %s", s.method, s.path)
	}
	if s.sent["confirm"] != false {
		t.Errorf("Parse sent confirm %v, want false so nothing is stored", s.sent["confirm"])
	}
	if s.sent["public_key"] != "ssh-ed25519 AAAA aki@framework" {
		t.Errorf("Parse sent %q, want the line as typed", s.sent["public_key"])
	}
	if got.Fingerprint != "SHA256:Tz6Y" || got.Comment != "aki@framework" {
		t.Errorf("Parse returned %+v", got)
	}

	s.status = http.StatusCreated
	s.body = `{"key":{"id":"k9","key_type":"ssh-ed25519","bits":256,
	           "fingerprint":"SHA256:Tz6Y"},"stored":true}`
	stored, err := c.Add(context.Background(), "tok", "ssh-ed25519 AAAA aki@framework")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if s.sent["confirm"] != true {
		t.Errorf("Add sent confirm %v, want true", s.sent["confirm"])
	}
	if stored.ID != "k9" {
		t.Errorf("Add returned %+v, want the id the screen removes by", stored)
	}
}

// TestRemoveDeletesByID asserts the delete, and that an id is escaped rather
// than pasted into a path.
func TestRemoveDeletesByID(t *testing.T) {
	s, c := newStore(t)
	s.status = http.StatusNoContent

	if err := c.Remove(context.Background(), "tok", "k1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if s.method != http.MethodDelete || s.path != "/me/ssh-keys/k1" {
		t.Errorf("Remove called %s %s", s.method, s.path)
	}
	// An id carrying a slash or a .. is one segment on the wire and never
	// a path. url.URL.JoinPath would have resolved both and sent this call
	// to the key "b", which is a different key on the same account.
	if err := c.Remove(context.Background(), "tok", "a/../b"); err != nil {
		t.Fatalf("Remove with an odd id: %v", err)
	}
	if s.raw != "/me/ssh-keys/a%2F..%2Fb" {
		t.Errorf("the request line is %q, want the id escaped into one segment", s.raw)
	}
	if err := c.Remove(context.Background(), "tok", ".."); err != nil {
		t.Fatalf("Remove with a traversal id: %v", err)
	}
	if s.raw != "/me/ssh-keys/.." {
		t.Errorf("the request line is %q, want the id left where it was addressed", s.raw)
	}
	// A base carrying a path of its own keeps it.
	base, err := url.Parse("https://keys.example/auth/")
	if err != nil {
		t.Fatal(err)
	}
	if got := New(base, nil).one("k1").String(); got != "https://keys.example/auth/me/ssh-keys/k1" {
		t.Errorf("a store under a path addressed %q", got)
	}
}

// TestRefusalsCarryTheirCode asserts each refusal reaches the screen as the
// code it switches on, and that the store's own message never does: the
// store writes for whoever reads its API, and every sentence a person meets
// is written on the screen.
func TestRefusalsCarryTheirCode(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		code   string
		is     func(error) bool
	}{
		{"invalid key", http.StatusBadRequest,
			`{"error":"invalid_public_key","message":"for an API","detail":"ssh-dss"}`,
			CodeInvalidKey, nil},
		{"already yours", http.StatusConflict,
			`{"error":"key_already_added","message":"for an API"}`, CodeAlreadyYours, nil},
		{"already taken", http.StatusConflict,
			`{"error":"key_already_registered"}`, CodeAlreadyTaken, nil},
		{"unauthenticated", http.StatusUnauthorized, `{"error":"unauthorized"}`,
			"unauthorized", Unauthenticated},
		{"forbidden", http.StatusForbidden, `{"error":"forbidden"}`, "forbidden", Forbidden},
		{"absent", http.StatusNotFound, `{"error":"not_found"}`, "not_found", Absent},
		{"failed", http.StatusInternalServerError, `{"error":"internal_error"}`,
			"internal_error", Unavailable},
		// A refusal with no document still reaches the screen as a code,
		// so nothing has to switch on a status by hand.
		{"no document", http.StatusBadGateway, `not json at all`, "http_502", Unavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, client := newStore(t)
			s.status, s.body = c.status, c.body

			_, err := client.Add(context.Background(), "tok", "ssh-ed25519 AAAA")
			if err == nil {
				t.Fatal("the refusal was not reported")
			}
			if got := Code(err); got != c.code {
				t.Errorf("Code = %q, want %q", got, c.code)
			}
			if c.is != nil && !c.is(err) {
				t.Errorf("the predicate did not recognise %v", err)
			}
			if strings.Contains(err.Error(), "for an API") {
				t.Errorf("the store's own message reached the caller: %v", err)
			}
			// Every call reports the same refusal the same way.
			if _, err := client.List(context.Background(), "tok"); Code(err) != c.code {
				t.Errorf("List reported %q", Code(err))
			}
			if err := client.Remove(context.Background(), "tok", "k1"); Code(err) != c.code {
				t.Errorf("Remove reported %q", Code(err))
			}
		})
	}
}

// TestAStoreThatCannotBeReachedIsAnOutage asserts the shape of the worst
// case: no status at all is an outage and never a refusal of anything the
// person did.
func TestAStoreThatCannotBeReachedIsAnOutage(t *testing.T) {
	base, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	c := New(base, &http.Client{Timeout: time.Second})

	_, listErr := c.List(context.Background(), "tok")
	if listErr == nil {
		t.Fatal("a store that is not there answered")
	}
	if !Unavailable(listErr) {
		t.Errorf("an unreachable store did not read as an outage: %v", listErr)
	}
	for _, predicate := range []func(error) bool{Unauthenticated, Forbidden, Absent} {
		if predicate(listErr) {
			t.Error("an outage read as a refusal")
		}
	}
	if _, err := c.Add(context.Background(), "tok", "ssh-ed25519 AAAA"); !Unavailable(err) {
		t.Errorf("Add against an unreachable store: %v", err)
	}
	if err := c.Remove(context.Background(), "tok", "k1"); !Unavailable(err) {
		t.Errorf("Remove against an unreachable store: %v", err)
	}
}

// TestAnUnreadableAnswerIsAnError asserts a 200 carrying something that is
// not the document is a failure, and never an account read as having no
// keys.
func TestAnUnreadableAnswerIsAnError(t *testing.T) {
	s, c := newStore(t)
	s.body = `{"keys": "not a list"}`

	if _, err := c.List(context.Background(), "tok"); err == nil {
		t.Fatal("an unreadable answer read as an empty account")
	}
}

// TestNewTakesADefaultClient asserts a caller that passes none gets the
// traced client rather than an unusable nil.
func TestNewTakesADefaultClient(t *testing.T) {
	base, err := url.Parse("https://keys.example")
	if err != nil {
		t.Fatal(err)
	}
	if c := New(base, nil); c == nil || c.hc == nil {
		t.Fatal("New returned a client that cannot send")
	}
}

// TestCodeIgnoresAnErrorThatIsNotTheStores asserts the predicates say
// nothing about an error from somewhere else.
func TestCodeIgnoresAnErrorThatIsNotTheStores(t *testing.T) {
	other := errors.New("something else entirely")
	if Code(other) != "" {
		t.Errorf("Code = %q for an unrelated error", Code(other))
	}
	for _, predicate := range []func(error) bool{Unauthenticated, Forbidden, Absent, Unavailable} {
		if predicate(other) {
			t.Error("a predicate claimed an unrelated error")
		}
	}
}

// TestErrorReadsForADeveloper asserts the three shapes an Error prints in,
// which is what reaches a log line.
func TestErrorReadsForADeveloper(t *testing.T) {
	cases := []struct {
		err  *Error
		want string
	}{
		{&Error{Status: 409, Code: "key_already_added"}, "409 key_already_added"},
		{&Error{Status: 400, Code: "invalid_public_key", Detail: "ssh-dss"}, "ssh-dss"},
		{&Error{cause: errors.New("dial refused")}, "dial refused"},
	}
	for _, c := range cases {
		if !strings.Contains(c.err.Error(), c.want) {
			t.Errorf("Error() = %q, want it to carry %q", c.err.Error(), c.want)
		}
	}
	cause := errors.New("dial refused")
	if !errors.Is(&Error{cause: cause}, cause) {
		t.Error("an Error does not unwrap to what caused it")
	}
}
