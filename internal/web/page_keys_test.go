// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/latere-ai/origo-web/internal/config"
	"github.com/latere-ai/origo-web/internal/keys"
)

// The key screen: what a person does on it, and the one property the whole
// design rests on, which is that this service reads no key itself.

// keyHarness is a server with the key screen on.
func keyHarness(t *testing.T) *harness {
	t.Helper()
	return newHarness(t, func(cfg *config.Config) { cfg.KeysURL = mustURL(t, "https://keys.example") })
}

const samplePaste = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample aki@framework"

// TestAddingAKeyIsTwoSteps asserts the flow the design draws: a paste comes
// back as a fingerprint to check, storing nothing, and a confirmation stores
// it and lands on the list. Two plain form submissions, no dialog.
func TestAddingAKeyIsTwoSteps(t *testing.T) {
	h := keyHarness(t)
	c := h.signedIn("alice")

	first := h.post("/keys", url.Values{"key": {samplePaste}}, c)
	if first.Code != http.StatusOK {
		t.Fatalf("the paste answered %d", first.Code)
	}
	body := first.Body.String()
	if !strings.Contains(body, "C3NzaC1lZDI1NTE5AAAAIExample") {
		t.Errorf("the fingerprint is not shown for checking:\n%s", body)
	}
	if !strings.Contains(body, "ssh-keygen -lf") {
		t.Error("the confirmation does not say how to check the fingerprint")
	}
	if !strings.Contains(body, "Add this key") {
		t.Error("the confirmation offers no way to confirm")
	}
	if held := h.keys.held(); len(held) != 0 {
		t.Fatalf("the first step stored %d keys; it must store none", len(held))
	}

	second := h.post("/keys", url.Values{"key": {samplePaste}, "confirm": {"yes"}}, c)
	if second.Code != http.StatusSeeOther {
		t.Fatalf("the confirmation answered %d, want a redirect so a reload repeats nothing", second.Code)
	}
	if got := second.Header().Get("Location"); got != "/keys" {
		t.Errorf("the confirmation lands on %q", got)
	}
	held := h.keys.held()
	if len(held) != 1 {
		t.Fatalf("the confirmation stored %d keys", len(held))
	}
	if held[0].Comment != "aki@framework" {
		t.Errorf("the stored key's label is %q, want the comment from the pasted line", held[0].Comment)
	}
	if !strings.Contains(h.get("/keys", c).Body.String(), "aki@framework") {
		t.Error("the list does not show the key that was just added")
	}
}

// TestTheScreenReadsNoKeyItself is the property the two-step add exists for.
//
// The interface never parses a key, never computes a fingerprint and never
// judges an algorithm: it sends the paste on and renders what came back. The
// proof is a paste this build could not have read and a fingerprint it could
// not have computed: both reach the screen unchanged, which they could not do
// if anything here were reading them.
func TestTheScreenReadsNoKeyItself(t *testing.T) {
	h := keyHarness(t)
	c := h.signedIn("alice")
	h.keys.parse = func(pasted string) (keys.Key, int, string) {
		return keys.Key{
			Type:        "ssh-ed448-from-the-future@example.com",
			Bits:        456,
			Comment:     "read by the store",
			Fingerprint: "SHA256:AFingerprintNothingHereCouldHaveComputed",
		}, 0, ""
	}

	body := h.post("/keys", url.Values{"key": {"anything at all, not a key"}}, c).Body.String()
	for _, want := range []string{
		"SHA256:AFingerprintNothingHereCouldHaveComputed",
		"ssh-ed448-from-the-future@example.com",
		"read by the store",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the screen did not render %q from the store:\n%s", want, body)
		}
	}

	// And the paste crosses the wire exactly as typed, so the second step
	// stores what the first parsed.
	h.post("/keys", url.Values{"key": {"anything at all, not a key"}, "confirm": {"yes"}}, c)
	held := h.keys.held()
	if len(held) != 1 {
		t.Fatalf("the confirmation stored %d keys", len(held))
	}
	if got := h.keys.lastPaste(); got != "anything at all, not a key" {
		t.Errorf("the store received %q, want the line as typed", got)
	}
}

// TestARefusedPasteKeepsWhatWasTyped asserts that a refusal is a page a
// person can act on: the sentence says what is wrong, and the paste is still
// in the box.
func TestARefusedPasteKeepsWhatWasTyped(t *testing.T) {
	h := keyHarness(t)
	c := h.signedIn("alice")
	const typed = "this is not a public key"

	rec := h.post("/keys", url.Values{"key": {typed}}, c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a refused paste answered %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "not a public key this installation accepts") {
		t.Errorf("the refusal does not say what was wrong:\n%s", body)
	}
	if !strings.Contains(body, typed) {
		t.Error("the paste was thrown away, so it has to be retyped")
	}
	// The store's own message is written for whoever reads its API and is
	// never the sentence a person meets.
	if strings.Contains(body, "the store said so") || strings.Contains(body, "for a developer") {
		t.Error("the screen showed the store's own words to a person")
	}
	// An empty submission is refused before anything is sent.
	if code := h.post("/keys", url.Values{"key": {"   "}}, c).Code; code != http.StatusBadRequest {
		t.Errorf("an empty paste answered %d", code)
	}
}

// TestADuplicateSaysWhichKindItIs holds the one refusal that has two
// meanings. A key the person already has is a different fact from a key
// somebody else has, and the store tells them apart, so the screen does too.
func TestADuplicateSaysWhichKindItIs(t *testing.T) {
	for _, c := range []struct {
		name, code, want string
	}{
		{"already yours", keys.CodeAlreadyYours, "already added this key"},
		{"someone else's", keys.CodeAlreadyTaken, "registered to another account"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := keyHarness(t)
			cookie := h.signedIn("alice")
			h.keys.parse = func(string) (keys.Key, int, string) {
				return keys.Key{}, http.StatusConflict, c.code
			}
			rec := h.post("/keys", url.Values{"key": {samplePaste}}, cookie)
			if rec.Code != http.StatusConflict {
				t.Fatalf("answered %d, want 409", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), c.want) {
				t.Errorf("the sentence does not say %q:\n%s", c.want, rec.Body.String())
			}
		})
	}
}

// TestRemovingAKeyAsksFirst asserts a removal is a decision taken on a page
// that names the key, and that the removal itself is a form and never a link.
func TestRemovingAKeyAsksFirst(t *testing.T) {
	h := keyHarness(t)
	c := h.signedIn("alice")
	h.keys.add(keys.Key{
		ID: "k1", Comment: "aki@thinkpad", Type: "ssh-ed25519", Bits: 256,
		Fingerprint: "SHA256:TheKeyBeingRemoved",
	})

	ask := h.get("/keys/k1/remove", c)
	if ask.Code != http.StatusOK {
		t.Fatalf("the removal page answered %d", ask.Code)
	}
	body := ask.Body.String()
	if !strings.Contains(body, "SHA256:TheKeyBeingRemoved") || !strings.Contains(body, "aki@thinkpad") {
		t.Errorf("the page does not name the key it is about:\n%s", body)
	}
	if !strings.Contains(body, "Remove this key") {
		t.Error("the page offers no way to go ahead")
	}
	if len(h.keys.held()) != 1 {
		t.Fatal("opening the page removed the key")
	}

	done := h.post("/keys/k1/remove", nil, c)
	if done.Code != http.StatusSeeOther {
		t.Fatalf("the removal answered %d", done.Code)
	}
	if len(h.keys.held()) != 0 {
		t.Error("the key is still in the store")
	}

	// A key that is not the reader's and one that never existed are one
	// answer, so guessing an id discloses nothing.
	if code := h.get("/keys/k9/remove", c).Code; code != http.StatusNotFound {
		t.Errorf("an unknown id answered %d, want 404", code)
	}
	if code := h.post("/keys/k9/remove", nil, c).Code; code != http.StatusNotFound {
		t.Errorf("removing an unknown id answered %d, want 404", code)
	}
}

// TestAnOutageSaysSoAndOffersNoForm asserts the screen does not offer an
// action it knows will fail, and says the keys themselves are unaffected.
func TestAnOutageSaysSoAndOffersNoForm(t *testing.T) {
	h := keyHarness(t)
	c := h.signedIn("alice")
	h.keys.down = true

	rec := h.get("/keys", c)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("the screen answered %d with the store down", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "key store is not answering") {
		t.Errorf("the page does not say the store is down:\n%s", body)
	}
	if !strings.Contains(body, "Your keys keep working") {
		t.Error("the page does not say that access is unaffected")
	}
	if strings.Contains(body, "Add a key") {
		t.Error("the page offers a form that cannot work")
	}
}

// TestTheStoreRefusingThisClientIsAnOperatorProblem asserts the one refusal
// the person cannot act on is not written as though they could.
func TestTheStoreRefusingThisClientIsAnOperatorProblem(t *testing.T) {
	h := keyHarness(t)
	c := h.signedIn("alice")
	h.keys.forbid = true

	body := h.post("/keys", url.Values{"key": {samplePaste}}, c).Body.String()
	if !strings.Contains(body, "not allowed to manage keys on this installation") {
		t.Errorf("the sentence does not name the installation's own problem:\n%s", body)
	}
	if strings.Contains(body, "Try a different") || strings.Contains(body, "Paste one line") {
		t.Error("the sentence asks the person to fix a thing they cannot fix")
	}
}

// TestAnExpiredCredentialSendsThePersonToSignIn asserts the session is
// cleared once and the person is asked to sign in, rather than meeting a
// refusal they cannot read.
func TestAnExpiredCredentialSendsThePersonToSignIn(t *testing.T) {
	h := keyHarness(t)
	c := h.signedIn("alice")
	h.keys.unauthenticated = true

	rec := h.get("/keys", c)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("the screen answered %d with a refused credential", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "/auth/start") {
		t.Error("the page does not offer a way to sign in again")
	}
}

// TestTheTableShowsWhatTellsTwoKeysApart asserts the row carries the four
// facts a person needs: the label, the algorithm beside the fingerprint, the
// day it was added, and whether it has ever been used.
func TestTheTableShowsWhatTellsTwoKeysApart(t *testing.T) {
	h := keyHarness(t)
	c := h.signedIn("alice")
	added := time.Date(2026, 2, 4, 9, 12, 0, 0, time.UTC)
	used := time.Now().Add(-6 * 24 * time.Hour)
	h.keys.add(keys.Key{
		ID: "k1", Comment: "aki@thinkpad", Type: "ssh-ed25519", Bits: 256,
		Fingerprint: "SHA256:First", Created: &added, LastUsed: &used,
	})
	h.keys.add(keys.Key{
		ID: "k2", Type: "ssh-rsa", Bits: 4096,
		Fingerprint: "SHA256:Second", Created: &added,
	})

	body := h.get("/keys", c).Body.String()
	for _, want := range []string{
		"aki@thinkpad", "SHA256:First", "4 Feb 2026", "6 days ago",
		"ed25519", "rsa-4096", "SHA256:Second",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the table does not show %q:\n%s", want, body)
		}
	}
	// A key with no comment is said to have none rather than given a name
	// nobody chose.
	if !strings.Contains(body, "no comment") {
		t.Error("a key with no label is not described as having none")
	}
	// The one red in the product, and it is beside the words rather than
	// instead of them, so it survives greyscale.
	if !strings.Contains(body, `<span class="unused">never used</span>`) {
		t.Errorf("a never-used key is not marked as one:\n%s", body)
	}
}

// TestTheScreenSendsTheReadersOwnToken asserts this interface holds no
// credential of its own: every key call carries the token of the person at
// the keyboard, so it can do exactly what they could do.
func TestTheScreenSendsTheReadersOwnToken(t *testing.T) {
	h := keyHarness(t)
	c := h.signedIn("alice")
	h.get("/keys", c)

	got := h.keys.bearers()
	if len(got) == 0 {
		t.Fatal("the screen made no call to the store")
	}
	for _, b := range got {
		if b == "" {
			t.Error("a key call carried no credential")
		}
	}
}

// TestKeyCallsAreExact holds the screen to the calls it should make, and to
// their number: a rendered list is one read, and no screen's call count grows
// with the rows it draws.
func TestKeyCallsAreExact(t *testing.T) {
	h := keyHarness(t)
	c := h.signedIn("alice")
	for i := range 5 {
		h.keys.add(keys.Key{
			ID: "k" + strconv.Itoa(i), Type: "ssh-ed25519", Bits: 256,
			Fingerprint: "SHA256:row" + strconv.Itoa(i),
		})
	}
	h.keys.reset()
	h.get("/keys", c)
	if got := h.keys.made(); len(got) != 1 || got[0] != "GET /me/ssh-keys" {
		t.Errorf("rendering five rows made %v, want one read", got)
	}

	// A confirmed add stores once and parses nothing a second time: the
	// screen already holds the line the person confirmed.
	h.keys.reset()
	h.post("/keys", url.Values{"key": {samplePaste}, "confirm": {"yes"}}, c)
	var posts int
	for _, call := range h.keys.made() {
		if call == "POST /me/ssh-keys" {
			posts++
		}
	}
	if posts != 1 {
		t.Errorf("a confirmed add made %d writes: %v", posts, h.keys.made())
	}

	// A removal is one delete.
	h.keys.reset()
	h.post("/keys/k1/remove", nil, c)
	var deletes int
	for _, call := range h.keys.made() {
		if strings.HasPrefix(call, "DELETE ") {
			deletes++
		}
	}
	if deletes != 1 {
		t.Errorf("a removal made %d deletes: %v", deletes, h.keys.made())
	}
}

// TestAlgorithmName pins the two words a person recognises a key by. The
// family alone is enough where the family has one strength, and the strength
// matters where it has several, because a 2048 bit RSA key is the weakest
// thing an installation accepts.
func TestAlgorithmName(t *testing.T) {
	for _, c := range []struct {
		keyType string
		bits    int
		want    string
	}{
		{"ssh-ed25519", 256, "ed25519"},
		{"ssh-rsa", 4096, "rsa-4096"},
		{"ssh-rsa", 2048, "rsa-2048"},
		{"ssh-rsa", 0, "rsa"},
		{"ecdsa-sha2-nistp256", 256, "ecdsa-nistp256"},
		{"ecdsa-sha2-nistp521", 521, "ecdsa-nistp521"},
		{"ssh-dss", 1024, "dsa"},
		// An algorithm this build has no short name for is shown as the
		// store named it, which is true and never a guess.
		{"sk-ssh-ed25519@openssh.com", 256, "sk-ssh-ed25519@openssh.com"},
	} {
		if got := algorithmName(c.keyType, c.bits); got != c.want {
			t.Errorf("algorithmName(%q, %d) = %q, want %q", c.keyType, c.bits, got, c.want)
		}
	}
}

// TestAKeyFormNeedsItsToken asserts both key forms refuse a submission that
// carries no token of this session, which is what stands between the screen
// and a page on another origin posting to it.
func TestAKeyFormNeedsItsToken(t *testing.T) {
	h := keyHarness(t)
	c := h.signedIn("alice")
	h.keys.add(keys.Key{ID: "k1", Fingerprint: "SHA256:x", Type: "ssh-ed25519", Bits: 256})

	for _, path := range []string{"/keys", "/keys/k1/remove"} {
		req := postWithoutToken(t, path)
		req.AddCookie(c)
		rec := record(h, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("POST %s with no token answered %d, want 403", path, rec.Code)
		}
	}
	if len(h.keys.held()) != 1 {
		t.Error("a submission with no token changed the store")
	}
}

// postWithoutToken builds a form submission carrying no CSRF token, which is
// what a page on another origin could send.
func postWithoutToken(t *testing.T, path string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

// record drives one request through the server.
func record(h *harness, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)
	return rec
}
