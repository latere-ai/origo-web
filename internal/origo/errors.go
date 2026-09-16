// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package origo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Error is a refusal from Origo, carrying the status and the code Origo's
// error document names. The message Origo sends is deliberately not kept:
// it is written for a developer reading an API, and every sentence a person
// reads on a screen of this service is written for a person.
type Error struct {
	Status int
	Code   string
	// Details is the developer half of Origo's error document. One key
	// matters to this service: on a 403 from the authorizer it carries
	// "reason", the token the operator's endpoint denied with.
	Details map[string]any
	cause   error
}

// Reason is the authorizer's own deny token, empty when the refusal did not
// come from one. It is what lets a screen tell one refusal from another: a
// creation refused with "unknown_repository" is a repository the registry
// has no row for, and not the same thing as a subject the authorizer will
// not let administer this owner.
func (e *Error) Reason() string {
	reason, _ := e.Details["reason"].(string)
	return reason
}

// DeniedAs reports that err is a refusal the authorizer gave with this
// reason.
func DeniedAs(err error, reason string) bool {
	var e *Error
	return errors.As(err, &e) && e.Reason() == reason
}

// ReasonUnknownRepository is what an authorizer answers for a repository it
// has no row for. The platform control plane reads a miss through to its
// store, so a row it has just written is an allow at every replica at once:
// this reason means the repository is registered nowhere.
const ReasonUnknownRepository = "unknown_repository"

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("origo: %s: %v", e.Code, e.cause)
	}
	return fmt.Sprintf("origo: %d %s", e.Status, e.Code)
}

func (e *Error) Unwrap() error { return e.cause }

// As is [errors.As], named here so a caller of this package needs no second
// import to read a refusal.
func As(err error, target any) bool { return errors.As(err, target) }

// Unauthenticated reports that Origo refused the credential. The caller
// clears the session and sends the person to sign in once.
func Unauthenticated(err error) bool { return statusIs(err, http.StatusUnauthorized) }

// Absent reports that the repository, reference, or object is not there or
// that the reader may not see it. Origo answers 403 and 404 to exactly the
// same question and refuses to say which, so this predicate refuses too, and
// one sentence covers both.
func Absent(err error) bool {
	return statusIs(err, http.StatusForbidden) ||
		statusIs(err, http.StatusNotFound) ||
		statusIs(err, http.StatusGone)
}

// TooLarge reports that Origo would not serve the object because of its size.
func TooLarge(err error) bool { return statusIs(err, http.StatusRequestEntityTooLarge) }

// Unavailable reports that the installation could not answer: it is
// unreachable, its storage is degraded, or its authorizer is not answering.
func Unavailable(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Status == 0 || e.Status >= 500
}

func statusIs(err error, status int) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == status
}

// readError turns a refusal into an Error, reading the code from Origo's
// error document when there is one.
func readError(resp *http.Response) *Error {
	e := &Error{Status: resp.StatusCode}
	var doc struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&doc); err == nil {
		e.Code = doc.Error.Code
		e.Details = doc.Error.Details
		if e.Code == "" {
			e.Code = doc.Code
		}
		if e.Details == nil {
			e.Details = doc.Details
		}
	}
	if e.Code == "" {
		e.Code = http.StatusText(resp.StatusCode)
	}
	return e
}
