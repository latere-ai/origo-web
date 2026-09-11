// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package registry

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// server returns a client pointed at h, and the recorder of what h saw.
func server(t *testing.T, h http.HandlerFunc) (*Client, *[]*http.Request) {
	t.Helper()
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return New(u, srv.Client()), &seen
}

// TestTheCallCarriesTheReadersTokenAndNothingElse: this package holds no
// credential, so every call is made on its caller's authority.
func TestTheCallCarriesTheReadersTokenAndNothingElse(t *testing.T) {
	c, seen := server(t, func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, Namespaces{Handle: "alice"})
	})
	if _, err := c.Namespaces(context.Background(), "a-token"); err != nil {
		t.Fatalf("namespaces: %v", err)
	}
	if _, err := c.Create(context.Background(), "a-token", "an-id", "alice", "notes"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := c.Forget(context.Background(), "a-token", "an-id"); err != nil {
		t.Fatalf("forget: %v", err)
	}
	if len(*seen) != 3 {
		t.Fatalf("%d calls, want three", len(*seen))
	}
	for _, r := range *seen {
		if got := r.Header.Get("Authorization"); got != "Bearer a-token" {
			t.Errorf("%s %s carried %q", r.Method, r.URL.Path, got)
		}
	}
	// A call with no token carries no header at all, rather than an empty
	// one, so the registry sees a request with no credential.
	c2, seen2 := server(t, func(w http.ResponseWriter, r *http.Request) { writeOK(w, Namespaces{}) })
	if _, err := c2.Namespaces(context.Background(), ""); err != nil {
		t.Fatalf("namespaces: %v", err)
	}
	if _, ok := (*seen2)[0].Header["Authorization"]; ok {
		t.Error("a call with no token sent an Authorization header")
	}
}

// TestTheRequestIsWhatTheRegistryExpects pins the paths and the body, since
// the registry is another repository's surface and a drift here is a
// creation that silently stops working.
func TestTheRequestIsWhatTheRegistryExpects(t *testing.T) {
	var body map[string]string
	c, seen := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		writeOK(w, Repository{ID: "an-id"})
	})
	if _, err := c.Namespaces(context.Background(), "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Create(context.Background(), "t", "an-id", "alice", "notes"); err != nil {
		t.Fatal(err)
	}
	// An id is escaped into the path, so a value that would otherwise add
	// a segment cannot.
	if err := c.Forget(context.Background(), "t", "an id/with slashes"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /repositories/namespaces",
		"POST /repositories",
		"DELETE /repositories/an%20id%2Fwith%20slashes",
	}
	for i, r := range *seen {
		if got := r.Method + " " + r.URL.EscapedPath(); got != want[i] {
			t.Errorf("call %d is %q, want %q", i, got, want[i])
		}
	}
	if body["id"] != "an-id" || body["owner_label"] != "alice" || body["slug"] != "notes" {
		t.Errorf("the create body is %v", body)
	}
}

// TestAnInstallationWithNoRegistryIsNotAFailure: an unconfigured provider
// and an address that serves no such route are the same answer, and it is
// not an outage.
func TestAnInstallationWithNoRegistryIsNotAFailure(t *testing.T) {
	none := New(nil, nil)
	if _, err := none.Namespaces(context.Background(), "t"); !errors.Is(err, ErrNoRegistry) {
		t.Errorf("namespaces with no provider: %v", err)
	}
	if _, err := none.Create(context.Background(), "t", "i", "o", "s"); !errors.Is(err, ErrNoRegistry) {
		t.Errorf("create with no provider: %v", err)
	}
	if err := none.Forget(context.Background(), "t", "i"); !errors.Is(err, ErrNoRegistry) {
		t.Errorf("forget with no provider: %v", err)
	}

	c, _ := server(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	if _, err := c.Namespaces(context.Background(), "t"); !errors.Is(err, ErrNoRegistry) {
		t.Errorf("an address that serves no such route: %v", err)
	}
	// A delete is the exception: a row that is not there is already the
	// outcome the caller wanted, so a 404 there is a refusal to read and
	// not a missing surface.
	if err := c.Forget(context.Background(), "t", "i"); errors.Is(err, ErrNoRegistry) {
		t.Error("a missing row read as a missing registry")
	}
}

// TestEachRefusalIsItsOwn: the registry says which rule stopped a call, and
// each predicate answers for exactly one of them.
func TestEachRefusalIsItsOwn(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		code   string
		check  func(error) bool
	}{
		{"a credential the registry refused", http.StatusUnauthorized, "unauthorized", Unauthenticated},
		{"a name that is not yours", http.StatusForbidden, "forbidden", Refused},
		{"a name already taken", http.StatusConflict, "conflict", Conflict},
		{"a body it will not read", http.StatusBadRequest, "invalid_request", Invalid},
		{"a registry that broke", http.StatusInternalServerError, "internal_error", Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := server(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": tc.code, "message": "for a developer", "detail": "the developer half",
				})
			})
			_, err := c.Create(context.Background(), "t", "i", "o", "s")
			if !tc.check(err) {
				t.Fatalf("the predicate did not recognise %v", err)
			}
			if got := CodeOf(err); got != tc.code {
				t.Errorf("code = %q, want %q", got, tc.code)
			}
			var e *Error
			if !As(err, &e) || e.Detail != "the developer half" {
				t.Errorf("the developer half was dropped: %+v", e)
			}
			if e.Error() == "" || errors.Unwrap(err) != nil {
				t.Errorf("the error reads %q and unwraps to %v", e.Error(), errors.Unwrap(err))
			}
		})
	}

	// An owner at its limit is a conflict with a code of its own, which is
	// what lets a screen say the one thing that is actually wrong.
	c, _ := server(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": CodeAtTheLimit})
	})
	_, err := c.Create(context.Background(), "t", "i", "o", "s")
	if !Conflict(err) || CodeOf(err) != CodeAtTheLimit {
		t.Errorf("an owner at its limit reads as %v, code %q", err, CodeOf(err))
	}

	// A refusal with no document still has a code, and a body that is not
	// a document does too.
	bare, _ := server(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("not a document"))
	})
	_, err = bare.Create(context.Background(), "t", "i", "o", "s")
	if CodeOf(err) != http.StatusText(http.StatusTeapot) {
		t.Errorf("a refusal with no document reads as %q", CodeOf(err))
	}
	if CodeOf(errors.New("not a refusal")) != "" {
		t.Error("CodeOf named a code on an error that is not a refusal")
	}
}

// TestAnUnreachableRegistryIsUnavailableAndNotARefusal: an address that does
// not answer is an outage, which a screen renders differently from every
// refusal.
func TestAnUnreachableRegistryIsUnavailableAndNotARefusal(t *testing.T) {
	u, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	c := New(u, &http.Client{})
	_, err = c.Namespaces(context.Background(), "t")
	if !Unavailable(err) || Refused(err) || Conflict(err) || Unauthenticated(err) {
		t.Fatalf("an unreachable registry reads as %v", err)
	}
	if !Unavailable(err) || errors.Unwrap(err) == nil {
		t.Errorf("the cause was dropped: %v", err)
	}
	if Unavailable(errors.New("not a refusal")) {
		t.Error("an error that is not a refusal read as an outage")
	}
}

// TestTheAnswerIsDecoded: a namespaces answer and a written row come back
// whole, because a screen renders both.
func TestTheAnswerIsDecoded(t *testing.T) {
	want := Namespaces{
		Handle: "alice",
		Namespaces: []Namespace{
			{OwnerType: "principal", OwnerID: "p1", Label: "alice", Name: "alice", Remaining: 7},
		},
	}
	c, _ := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writeOK(w, Repository{ID: "an-id", OwnerType: "principal", OwnerLabel: "alice", Slug: "notes"})
			return
		}
		writeOK(w, want)
	})
	got, err := c.Namespaces(context.Background(), "t")
	if err != nil {
		t.Fatal(err)
	}
	if got.Handle != want.Handle || len(got.Namespaces) != 1 || got.Namespaces[0].Remaining != 7 {
		t.Fatalf("namespaces = %+v, want %+v", got, want)
	}
	row, err := c.Create(context.Background(), "t", "an-id", "alice", "notes")
	if err != nil {
		t.Fatal(err)
	}
	if row.ID != "an-id" || row.OwnerLabel != "alice" || row.Slug != "notes" {
		t.Fatalf("the row is %+v", row)
	}

	// A body that is not the answer is an error and not a zero value, so a
	// screen never renders a namespace list it did not receive.
	bad, _ := server(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{"))
	})
	if _, err := bad.Namespaces(context.Background(), "t"); err == nil {
		t.Error("a body that is not an answer decoded without error")
	}
	if _, err := bad.Create(context.Background(), "t", "i", "o", "s"); err == nil {
		t.Error("a body that is not a row decoded without error")
	}
}

func writeOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
