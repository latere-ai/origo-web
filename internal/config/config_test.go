// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"
)

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for _, k := range []string{
		"ORIGOWEB_ADDR", "ORIGOWEB_ORIGO_URL", "ORIGOWEB_PUBLIC_URL", "ORIGOWEB_CLONE_HOST",
		"ORIGOWEB_SSH_CLONE_HOST", "ORIGOWEB_KEYS_URL", "ORIGOWEB_ISSUER_NAME",
		"ORIGOWEB_PRODUCT_NAME", "ORIGOWEB_PROJECT_URL", "ORIGOWEB_BRAND_MARK",
		"ORIGOWEB_AUTH_CLIENT_ID", "ORIGOWEB_AUTH_REDIRECT_URL", "ORIGOWEB_AUTH_URL",
		"AUTH_CLIENT_ID", "AUTH_URL", "AUTH_REDIRECT_URL",
	} {
		t.Setenv(k, "")
	}
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func TestLoadTakesItsDefaults(t *testing.T) {
	setEnv(t, map[string]string{
		"ORIGOWEB_ORIGO_URL":      "https://git.example/",
		"ORIGOWEB_PUBLIC_URL":     "https://code.example",
		"ORIGOWEB_AUTH_CLIENT_ID": "origoweb",
		"ORIGOWEB_AUTH_URL":       "https://issuer.example",
	})
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" {
		t.Errorf("the listener defaults to %q", c.Addr)
	}
	// The clone host defaults to the installation's own address, for the
	// common deployment where the git surface and the API are one.
	if c.CloneHost.String() != "https://git.example" {
		t.Errorf("the clone host defaults to %q", c.CloneHost)
	}
	if c.SSHCloneHost != "" || c.KeysURL != "" {
		t.Error("an SSH surface or a key surface was assumed")
	}
	if c.OIDC.RedirectURL != "https://code.example/auth/callback" {
		t.Errorf("the redirect URI is %q", c.OIDC.RedirectURL)
	}
	if c.OIDC.AuthURL != "https://issuer.example" || c.OIDC.ClientID != "origoweb" {
		t.Errorf("the identity provider reads as %+v", c.OIDC)
	}

	// An installation nobody named is the project itself, under the
	// project's own name, with nobody else's mark on it.
	if c.Name() != ProjectName || c.Hosted() {
		t.Errorf("an unnamed installation calls itself %q, hosted=%v", c.Name(), c.Hosted())
	}
	if c.Mark != "" {
		t.Errorf("an unnamed installation drew the mark %q", c.Mark)
	}
	if c.Project() != DefaultProjectURL {
		t.Errorf("the project link defaults to %q", c.Project())
	}
}

// TestTheProductNameAndTheMarkAreSettings asserts the rule that keeps one
// operator's branding off another's installation: the name the interface
// calls itself, the project it links to, and the mark it draws are all
// settings, and the defaults name the open-source project and nobody's
// product.
func TestTheProductNameAndTheMarkAreSettings(t *testing.T) {
	setEnv(t, map[string]string{
		"ORIGOWEB_ORIGO_URL":      "https://git.example",
		"ORIGOWEB_PUBLIC_URL":     "https://code.example",
		"ORIGOWEB_AUTH_CLIENT_ID": "origoweb",
		"ORIGOWEB_PRODUCT_NAME":   " Latere Code ",
		"ORIGOWEB_PROJECT_URL":    " https://example.org/origo ",
		"ORIGOWEB_BRAND_MARK":     MarkLatere,
	})
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Name() != "Latere Code" || !c.Hosted() {
		t.Errorf("a named installation calls itself %q, hosted=%v", c.Name(), c.Hosted())
	}
	if c.Project() != "https://example.org/origo" {
		t.Errorf("the project link is %q", c.Project())
	}
	if c.Mark != MarkLatere {
		t.Errorf("the mark is %q", c.Mark)
	}

	// An operator who names their installation but asks for a mark this
	// build does not carry is told so, rather than served a blank space.
	setEnv(t, map[string]string{
		"ORIGOWEB_ORIGO_URL":      "https://git.example",
		"ORIGOWEB_PUBLIC_URL":     "https://code.example",
		"ORIGOWEB_AUTH_CLIENT_ID": "origoweb",
		"ORIGOWEB_BRAND_MARK":     "acme",
	})
	if _, err := Load(); err == nil {
		t.Error("a mark this build does not carry started anyway")
	}
}

func TestLoadRefusesWhatItCannotServe(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"no installation address": {"ORIGOWEB_PUBLIC_URL": "https://code.example"},
		"no public address":       {"ORIGOWEB_ORIGO_URL": "https://git.example"},
		"an address with no scheme": {
			"ORIGOWEB_ORIGO_URL": "git.example", "ORIGOWEB_PUBLIC_URL": "https://code.example",
		},
		"an address with no host": {
			"ORIGOWEB_ORIGO_URL": "https://", "ORIGOWEB_PUBLIC_URL": "https://code.example",
		},
		"a clone host that is not a URL": {
			"ORIGOWEB_ORIGO_URL": "https://git.example", "ORIGOWEB_PUBLIC_URL": "https://code.example",
			"ORIGOWEB_CLONE_HOST": "ssh://git.example",
		},
	} {
		setEnv(t, env)
		if _, err := Load(); err == nil {
			t.Errorf("%s: the service started anyway", name)
		}
	}
}

func TestCloneAndArchiveComeFromConfiguration(t *testing.T) {
	setEnv(t, map[string]string{
		"ORIGOWEB_ORIGO_URL":      "https://api.example",
		"ORIGOWEB_PUBLIC_URL":     "https://code.example",
		"ORIGOWEB_CLONE_HOST":     "https://git.example",
		"ORIGOWEB_SSH_CLONE_HOST": "git.example",
		"ORIGOWEB_AUTH_CLIENT_ID": "origoweb",
	})
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := c.CloneHTTPS("infra", "origo"); got != "https://git.example/infra/origo.git" {
		t.Errorf("the HTTPS clone address is %q", got)
	}
	if got := c.CloneSSH("infra", "origo"); got != "git@git.example:infra/origo.git" {
		t.Errorf("the SSH clone address is %q", got)
	}
	// The archive link points at the installation, so a large tarball
	// never passes through this service.
	got := c.ArchiveURL("r1", "main")
	if !strings.HasPrefix(got, "https://api.example/v1/repos/r1/archive/") {
		t.Errorf("the archive link is %q", got)
	}

	c.SSHCloneHost = ""
	if got := c.CloneSSH("infra", "origo"); got != "" {
		t.Errorf("an installation with no SSH surface offered %q", got)
	}
}

func TestKeysAndIssuerNameAreRead(t *testing.T) {
	setEnv(t, map[string]string{
		"ORIGOWEB_ORIGO_URL":      "https://git.example",
		"ORIGOWEB_PUBLIC_URL":     "https://code.example",
		"ORIGOWEB_KEYS_URL":       " https://keys.example ",
		"ORIGOWEB_ISSUER_NAME":    "Okta",
		"ORIGOWEB_AUTH_CLIENT_ID": "origoweb",
	})
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.KeysURL != "https://keys.example" || c.IssuerName != "Okta" {
		t.Errorf("the key surface and the issuer name read as %q and %q", c.KeysURL, c.IssuerName)
	}
}
