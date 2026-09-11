// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

// Package registry is a client of the repository registry the installation's
// authorizer keeps.
//
// Origo stores no user and no permission. Before every repository operation
// it asks an operator-run endpoint whether a subject may read, write or
// administer one repository, and that endpoint is the only component that
// knows who owns what. Creating a repository therefore has two halves: a row
// in that registry saying who owns it, and the repository itself at Origo.
// This package is the first half.
//
// It holds the same rule the rest of this service holds. There is no
// credential of its own here: every call carries the signed-in person's own
// token, so the registry decides on that person's authority and this service
// can do nothing a person could not do at the same address with curl. An
// installation whose authorizer serves no such surface simply has no
// creation screen, which is what ErrNoRegistry is for.
package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"latere.ai/x/pkg/otel"
)

// Client talks to one registry.
type Client struct {
	base *url.URL
	hc   *http.Client
}

// New returns a client for the registry at base, which is the installation's
// identity provider: the component that holds handles, organizations and the
// repository registry is the same one that issues the token. A nil base
// means no registry is configured and every call answers ErrNoRegistry.
func New(base *url.URL, hc *http.Client) *Client {
	if hc == nil {
		hc = otel.HTTPClient()
	}
	return &Client{base: base, hc: hc}
}

// Namespace is one name a person may create repositories under: their own
// handle, or an organization they administer.
type Namespace struct {
	OwnerType string `json:"owner_type"`
	OwnerID   string `json:"owner_id"`
	// Label is the owner segment of the repository's address.
	Label string `json:"label"`
	// Name is what to call it on a screen.
	Name string `json:"name"`
	// Remaining is how many more repositories this owner may hold. Zero
	// means a namespace that is shown and refused.
	Remaining int `json:"remaining"`
}

// Namespaces is the whole answer: where a person may create, and the handle
// they hold. An empty list with an empty handle is a person who has claimed
// no name, which is the one case the screen answers with what to do next
// rather than with a form.
type Namespaces struct {
	Namespaces []Namespace `json:"namespaces"`
	Handle     string      `json:"handle"`
}

// ErrNoRegistry is what every call answers on an installation whose
// authorizer serves no registry surface: none is configured, or the address
// answers no such route. It is not a failure, and the interface has no
// creation screen on such an installation.
var ErrNoRegistry = errors.New("registry: this installation registers no repositories")

// Error is a refusal, carrying the status and the code the registry named.
// The sentence the registry sends is not kept: it is written for a developer
// reading an API, and every sentence a person reads here is written for a
// person.
type Error struct {
	Status int
	Code   string
	// Detail is the developer half, kept for a log line and never rendered.
	Detail string
	cause  error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("registry: %s: %v", e.Code, e.cause)
	}
	return fmt.Sprintf("registry: %d %s", e.Status, e.Code)
}

func (e *Error) Unwrap() error { return e.cause }

// As is [errors.As], named here so a caller needs no second import.
func As(err error, target any) bool { return errors.As(err, target) }

// Unauthenticated reports that the registry refused the credential, which
// ends the same way every other refused credential does: the session is
// cleared and the person signs in once.
func Unauthenticated(err error) bool { return statusIs(err, http.StatusUnauthorized) }

// Refused reports that this person may not do this: the name is not theirs,
// or their account may not create.
func Refused(err error) bool { return statusIs(err, http.StatusForbidden) }

// NotFound reports a 404. The registry answers it both for a repository
// that is not there and for one this person holds no role on, so a caller
// learns nothing about which of the two it was.
func NotFound(err error) bool { return statusIs(err, http.StatusNotFound) }

// Conflict reports that the name is taken, the owner is at its limit, or the
// name belongs to a different owner. CodeOf says which.
func Conflict(err error) bool { return statusIs(err, http.StatusConflict) }

// Invalid reports that the registry would not read the request.
func Invalid(err error) bool { return statusIs(err, http.StatusBadRequest) }

// Unavailable reports that the registry could not answer at all.
func Unavailable(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Status == 0 || e.Status >= 500
}

// CodeAtTheLimit is the conflict code that means this owner already holds as
// many repositories as it may.
const CodeAtTheLimit = "repository_limit"

// CodeOf is the code the registry named, empty when the error is not one of
// its refusals.
func CodeOf(err error) string {
	var e *Error
	if !errors.As(err, &e) {
		return ""
	}
	return e.Code
}

func statusIs(err error, status int) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == status
}

// Namespaces asks where this person may create.
func (c *Client) Namespaces(ctx context.Context, tok string) (Namespaces, error) {
	if c == nil || c.base == nil {
		return Namespaces{}, ErrNoRegistry
	}
	resp, err := c.do(ctx, http.MethodGet, tok, nil, "repositories", "namespaces")
	if err != nil {
		return Namespaces{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out Namespaces
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&out); err != nil {
		return Namespaces{}, fmt.Errorf("decode namespaces: %w", err)
	}
	return out, nil
}

// Repository is the row the registry wrote.
type Repository struct {
	ID         string `json:"id"`
	OwnerType  string `json:"owner_type"`
	OwnerID    string `json:"owner_id"`
	OwnerLabel string `json:"owner_label"`
	Slug       string `json:"slug"`
}

// Create writes one registry row: this id, under this name, owned by
// whoever holds that name.
//
// The row comes first and the repository at Origo second, which is the order
// the registry's own contract fixes: the failure that leaves a repository
// unreachable is preferred to the one that leaves it unguarded. It is
// idempotent by id, so a caller may repeat it.
func (c *Client) Create(ctx context.Context, tok, id, label, slug string) (Repository, error) {
	if c == nil || c.base == nil {
		return Repository{}, ErrNoRegistry
	}
	body, err := json.Marshal(map[string]string{"id": id, "owner_label": label, "slug": slug})
	if err != nil {
		return Repository{}, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.do(ctx, http.MethodPost, tok, body, "repositories")
	if err != nil {
		return Repository{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out Repository
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&out); err != nil {
		return Repository{}, fmt.Errorf("decode repository: %w", err)
	}
	return out, nil
}

// Forget removes a row this person owns.
//
// It is the compensating half of a creation whose second step failed. If
// the repository was never made, both sides are empty again and the name is
// free; if only the answer was lost, the row is gone and the repository is
// unreachable, which is the preferred failure of the two and which writing
// the row again by the same id repairs.
func (c *Client) Forget(ctx context.Context, tok, id string) error {
	if c == nil || c.base == nil {
		return ErrNoRegistry
	}
	resp, err := c.do(ctx, http.MethodDelete, tok, nil, "repositories", id)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// maxBody bounds an answer. The largest is a namespace list, which is one
// entry per organization a person administers.
const maxBody = 1 << 20

// do makes one call. The path is given as segments, and each is escaped into
// exactly one segment of the address, so an id carrying a slash cannot add a
// segment of its own. The escaped form is written to RawPath beside the
// decoded Path, because url.URL escapes Path on its own terms and leaves a
// slash inside it alone.
func (c *Client) do(ctx context.Context, method, tok string, body []byte, segments ...string) (*http.Response, error) {
	u := *c.base
	decoded := []string{strings.TrimRight(u.Path, "/")}
	escaped := []string{strings.TrimRight(u.EscapedPath(), "/")}
	for _, segment := range segments {
		decoded = append(decoded, segment)
		escaped = append(escaped, url.PathEscape(segment))
	}
	u.Path, u.RawPath = strings.Join(decoded, "/"), strings.Join(escaped, "/")
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rdr)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// The person's own token, and nothing else. This service holds no
	// credential, so a registry call can reach exactly what its caller
	// could reach.
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, &Error{Status: 0, Code: "unreachable", cause: err}
	}
	if resp.StatusCode == http.StatusNotFound && method != http.MethodDelete {
		// A route this installation's authorizer does not serve. It is
		// not a refusal and not an outage: the interface has no creation
		// screen there.
		_ = resp.Body.Close()
		return nil, ErrNoRegistry
	}
	if resp.StatusCode >= 400 {
		defer func() { _ = resp.Body.Close() }()
		return nil, readError(resp)
	}
	return resp, nil
}

// readError turns a refusal into an Error, reading the code from the
// registry's error document. The document is {"error", "message", "detail"}:
// the code is the machine half, the detail is for a log line, and the
// message is the registry's own sentence, which this service never renders.
func readError(resp *http.Response) *Error {
	e := &Error{Status: resp.StatusCode}
	var doc struct {
		Error  string `json:"error"`
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&doc); err == nil {
		e.Code, e.Detail = doc.Error, doc.Detail
	}
	if e.Code == "" {
		e.Code = http.StatusText(resp.StatusCode)
	}
	return e
}

// The two visibilities a repository can have.
const (
	Private = "private"
	Public  = "public"
)

// Visibility is what the registry says about one repository: whether it is
// public, and whether this person may change it.
type Visibility struct {
	Visibility string `json:"visibility"`
	CanChange  bool   `json:"can_change"`
}

// Public reports whether the repository is readable without a credential.
func (v Visibility) Public() bool { return v.Visibility == Public }

// ReadVisibility asks whether a repository is public. A person with no
// role on it gets the same answer as an id the registry never heard of, so
// the error says nothing about whether the repository is there.
func (c *Client) ReadVisibility(ctx context.Context, tok, id string) (Visibility, error) {
	if c == nil || c.base == nil {
		return Visibility{}, ErrNoRegistry
	}
	resp, err := c.do(ctx, http.MethodGet, tok, nil, "repositories", id, "visibility")
	if err != nil {
		return Visibility{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out Visibility
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&out); err != nil {
		return Visibility{}, fmt.Errorf("decode visibility: %w", err)
	}
	return out, nil
}

// SetVisibility makes a repository public or private. The registry refuses
// a caller without admin on it.
func (c *Client) SetVisibility(ctx context.Context, tok, id, visibility string) error {
	if c == nil || c.base == nil {
		return ErrNoRegistry
	}
	body, err := json.Marshal(map[string]string{"visibility": visibility})
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := c.do(ctx, http.MethodPut, tok, body, "repositories", id, "visibility")
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
