// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

// Package keys is a client of the installation's public key store.
//
// Origo holds no public key: it asks an endpoint the operator runs whose an
// offered key is, and the same component that answers that question is
// where a person adds one. This package talks to that component and holds
// no key knowledge of its own. It never parses a key, never computes a
// fingerprint and never decides what is acceptable: the paste goes to the
// store, and what comes back is what the store would save. That is what
// keeps the fingerprint a person confirms and the key that is stored from
// ever disagreeing.
//
// Every call carries the reader's own token. The interface holds no
// credential of its own, so a key call can do exactly what the person at
// the keyboard could do.
package keys

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
	"time"

	"latere.ai/x/pkg/otel"
)

// maxBody bounds a response. The store answers with a short document; a
// list of keys is a few hundred bytes each.
const maxBody = 1 << 20

// Client talks to one key store.
type Client struct {
	base *url.URL
	hc   *http.Client
}

// New returns a client for the store at base.
func New(base *url.URL, hc *http.Client) *Client {
	if hc == nil {
		// The traced client of latere.ai/x/pkg/otel, so a call to the
		// store shows up as one span of the request that made it.
		hc = otel.HTTPClient()
	}
	return &Client{base: base, hc: hc}
}

// Key is one registered public key, as the store holds it.
//
// Comment is the label, which is the comment at the end of the pasted line.
// Type and Bits are the algorithm and its strength, kept apart so a screen
// can draw them together without this package deciding how. LastUsed is nil
// on a key that has never authenticated.
type Key struct {
	ID          string     `json:"id"`
	Comment     string     `json:"comment"`
	Type        string     `json:"key_type"`
	Bits        int        `json:"bits"`
	Fingerprint string     `json:"fingerprint"`
	Created     *time.Time `json:"created_at"`
	LastUsed    *time.Time `json:"last_used_at"`
}

// List returns the reader's own keys, newest first.
func (c *Client) List(ctx context.Context, token string) ([]Key, error) {
	var out struct {
		Keys []Key `json:"keys"`
	}
	if err := c.call(ctx, http.MethodGet, "/me/ssh-keys", token, nil, &out); err != nil {
		return nil, err
	}
	return out.Keys, nil
}

// addRequest is the body of both steps of an add. confirm false parses and
// answers; confirm true stores.
type addRequest struct {
	PublicKey string `json:"public_key"`
	Confirm   bool   `json:"confirm"`
}

// Parse sends a pasted line to the store and returns what it would save,
// storing nothing. It is the first step of the two-step add, and it exists
// so this service needs no key parser: the fingerprint it shows a person to
// check is the store's own, computed by the code that will hold the key.
func (c *Client) Parse(ctx context.Context, token, pasted string) (Key, error) {
	return c.add(ctx, token, pasted, false)
}

// Add stores a key the person has confirmed.
func (c *Client) Add(ctx context.Context, token, pasted string) (Key, error) {
	return c.add(ctx, token, pasted, true)
}

func (c *Client) add(ctx context.Context, token, pasted string, confirm bool) (Key, error) {
	body, err := json.Marshal(addRequest{PublicKey: pasted, Confirm: confirm})
	if err != nil {
		return Key{}, &Error{cause: err}
	}
	var out struct {
		Key Key `json:"key"`
	}
	if err := c.call(ctx, http.MethodPost, "/me/ssh-keys", token, body, &out); err != nil {
		return Key{}, err
	}
	return out.Key, nil
}

// Remove deletes one of the reader's keys. An id that is not theirs is
// answered as one that does not exist, so nothing is disclosed by guessing.
func (c *Client) Remove(ctx context.Context, token, id string) error {
	return c.call(ctx, http.MethodDelete, "/me/ssh-keys/"+url.PathEscape(id), token, nil, nil)
}

// call sends one request and decodes the answer. A refusal becomes an Error
// carrying the store's code, which is what a screen renders a sentence from.
func (c *Client) call(ctx context.Context, method, path, token string, body []byte, out any) error {
	target := c.base.JoinPath(path)
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), rdr)
	if err != nil {
		return &Error{cause: err}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		// No status at all: the store could not be reached, which the
		// screen reads as an outage rather than as a refusal.
		return &Error{cause: err}
	}
	defer func() { _, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody)); _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return readError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
		return &Error{Status: resp.StatusCode, cause: err}
	}
	return nil
}

// Error is a refusal from the key store.
//
// Code is the store's own machine-readable code, which is what a screen
// switches on. The store's message is not kept: it is one service writing
// for another, and every sentence a person reads here is written here.
// Detail is kept for the log, never for the page.
type Error struct {
	Status int
	Code   string
	Detail string
	cause  error
}

func (e *Error) Error() string {
	switch {
	case e.cause != nil:
		return fmt.Sprintf("keys: %v", e.cause)
	case e.Detail != "":
		return fmt.Sprintf("keys: %d %s: %s", e.Status, e.Code, e.Detail)
	default:
		return fmt.Sprintf("keys: %d %s", e.Status, e.Code)
	}
}

func (e *Error) Unwrap() error { return e.cause }

// The codes the store answers a refused add with. A screen renders one
// sentence per code, so each is named here rather than spelled at the call
// site.
const (
	// CodeInvalidKey means the paste is not one acceptable public key.
	CodeInvalidKey = "invalid_public_key"
	// CodeAlreadyYours means the caller has already added this key.
	CodeAlreadyYours = "key_already_added"
	// CodeAlreadyTaken means another account holds it. Nothing about that
	// account is disclosed, here or in the store's answer.
	CodeAlreadyTaken = "key_already_registered"
)

// readError turns a refusal into an Error.
func readError(resp *http.Response) *Error {
	e := &Error{Status: resp.StatusCode}
	var doc struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Detail  string `json:"detail"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&doc); err == nil {
		e.Code = doc.Error
		e.Detail = strings.TrimSpace(doc.Detail)
	}
	if e.Code == "" {
		e.Code = fmt.Sprintf("http_%d", resp.StatusCode)
	}
	return e
}

// Unauthenticated reports that the store refused the credential. The caller
// clears the session and sends the person to sign in once.
func Unauthenticated(err error) bool { return statusIs(err, http.StatusUnauthorized) }

// Forbidden reports that the token may not manage keys, which on this
// installation means the client was never granted the scope. It is an
// operator's misconfiguration and not something the person can fix.
func Forbidden(err error) bool { return statusIs(err, http.StatusForbidden) }

// Absent reports that the key is not the caller's, or is not there at all.
// The store answers both the same way and so does this.
func Absent(err error) bool { return statusIs(err, http.StatusNotFound) }

// Unavailable reports that the store could not answer: it is unreachable or
// it failed.
func Unavailable(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Status == 0 || e.Status >= 500
}

// Code returns the store's code for a refusal, empty when the error is not
// one of the store's.
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func statusIs(err error, status int) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == status
}
