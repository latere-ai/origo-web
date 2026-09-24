// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"latere.ai/x/pkg/authkit/oidc"
)

// The reference an operator reads, docs/configuration.md, is written by
// hand. These tests hold it to the code in both directions: every variable
// the process reads has a row, and every variable a row names is read.

// reference is the page, relative to this package.
const reference = "../../docs/configuration.md"

// authVariables are the sign-in variables authkit/oidc reads on this
// service's behalf under EnvPrefix, each with a value that differs from its
// default so a probe can see it was read.
var authVariables = map[string]string{
	"AUTH_URL":              "https://issuer.example",
	"AUTH_CLIENT_ID":        "origoweb",
	"AUTH_CLIENT_SECRET":    "secret",
	"AUTH_REDIRECT_URL":     "https://code.example/cb",
	"AUTH_COOKIE_KEY":       "0123456789abcdef0123456789abcdef",
	"AUTH_AUDIENCE":         "audience",
	"AUTH_SCOPES":           "openid",
	"AUTH_INSECURE_COOKIES": "1",
}

// setInCode are the fields of oidc.Config this service sets itself rather
// than reading from a variable: the session package names the cookie and
// fixes the session window.
var setInCode = []string{"CookieName", "SessionTTL"}

// clearAuth unsets every sign-in variable under both the prefixed and the
// plain name, so a probe sees only what it set and nothing from the shell
// the suite runs in.
func clearAuth(t *testing.T) {
	t.Helper()
	for name := range authVariables {
		t.Setenv(EnvPrefix+"_"+name, "")
		t.Setenv(name, "")
	}
}

// TestEverySignInVariableIsRead proves each documented sign-in variable
// changes what authkit loads, so a row on the page cannot outlive the
// variable it names.
func TestEverySignInVariableIsRead(t *testing.T) {
	for name, value := range authVariables {
		t.Run(name, func(t *testing.T) {
			clearAuth(t)
			before := oidc.LoadConfigWithPrefix(EnvPrefix)
			t.Setenv(EnvPrefix+"_"+name, value)
			if after := oidc.LoadConfigWithPrefix(EnvPrefix); reflect.DeepEqual(before, after) {
				t.Errorf("%s_%s is documented and changes nothing authkit loads", EnvPrefix, name)
			}
		})
	}
}

// TestNoSignInVariableIsMissing catches a variable authkit starts to read
// that the page does not name: with every documented variable set, a field
// of oidc.Config left at its zero value is filled from somewhere this
// package does not know about.
func TestNoSignInVariableIsMissing(t *testing.T) {
	clearAuth(t)
	for name, value := range authVariables {
		t.Setenv(EnvPrefix+"_"+name, value)
	}
	cfg := reflect.ValueOf(oidc.LoadConfigWithPrefix(EnvPrefix))
	for i := range cfg.NumField() {
		field := cfg.Type().Field(i).Name
		if slices.Contains(setInCode, field) {
			continue
		}
		if cfg.Field(i).IsZero() {
			t.Errorf("oidc.Config.%s is set by no documented variable: name it in %s and in authVariables", field, reference)
		}
	}
}

// variablesRead is every variable config.go reads itself, found as the
// expression EnvPrefix + "_NAME" that each read is written with.
func variablesRead(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "config.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok || bin.Op != token.ADD {
			return true
		}
		ident, ok := bin.X.(*ast.Ident)
		if !ok || ident.Name != "EnvPrefix" {
			return true
		}
		lit, ok := bin.Y.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		suffix, err := strconv.Unquote(lit.Value)
		if err != nil || !strings.HasPrefix(suffix, "_") {
			return true
		}
		if name := EnvPrefix + suffix; !slices.Contains(out, name) {
			out = append(out, name)
		}
		return true
	})
	// A rewrite of Load that stopped spelling its reads this way would
	// leave the list empty and the test below vacuous.
	if len(out) < 10 {
		t.Fatalf("found %d variables in config.go, want the eleven Load reads: %v", len(out), out)
	}
	return out
}

// TestTheReferenceNamesEveryVariable holds docs/configuration.md to the
// variables the process reads, and the process to the page.
func TestTheReferenceNamesEveryVariable(t *testing.T) {
	body, err := os.ReadFile(filepath.FromSlash(reference))
	if err != nil {
		t.Fatal(err)
	}
	page := string(body)

	read := variablesRead(t)
	for name := range authVariables {
		read = append(read, EnvPrefix+"_"+name)
	}
	for _, name := range read {
		if !strings.Contains(page, "`"+name+"`") {
			t.Errorf("%s is read and %s does not name it", name, reference)
		}
	}

	named := regexp.MustCompile("`("+EnvPrefix+"_[A-Z_]+)`").FindAllStringSubmatch(page, -1)
	for _, m := range named {
		if !slices.Contains(read, m[1]) {
			t.Errorf("%s names %s, which nothing reads", reference, m[1])
		}
	}
}
