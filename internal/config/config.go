// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

// Package config reads the interface's settings from the environment.
//
// Every value is a setting an operator gives the process. Nothing here is
// derived from a request: the clone URLs, the redirect URI, and the address
// of the Origo installation are configuration, so a forged Host header
// cannot move them (spec 023, "TestCloneURLsComeFromConfiguration").
package config

import (
	"cmp"
	"fmt"
	"net/url"
	"os"
	"strings"

	"latere.ai/x/pkg/authkit/oidc"
)

// EnvPrefix is the prefix of every variable this service reads, including
// the ones authkit/oidc reads on its behalf.
const EnvPrefix = "ORIGOWEB"

// The open-source project this binary is a build of. A running installation
// is an instance of it and may carry its operator's own name, but the
// project itself is the same everywhere, so it is a constant and not a
// setting.
const (
	// ProjectName is the software.
	ProjectName = "Origo"

	// DefaultProjectURL is where the software lives.
	DefaultProjectURL = "https://github.com/latere-ai/origo"
)

// MarkLatere is the one brand mark this binary carries. The mark is Latere's
// and belongs to Latere's own installation, so an operator asks for it by
// name and no default hands it to anyone else.
const MarkLatere = "latere"

// Config is the whole configuration of one process.
type Config struct {
	// Addr is the listener address.
	Addr string

	// OrigoURL is the base URL of the Origo installation, the one address
	// this service talks to.
	OrigoURL *url.URL

	// PublicURL is this service's own base URL, used for absolute links.
	PublicURL *url.URL

	// CloneHost is what the HTTPS clone URL on the overview names. It
	// defaults to OrigoURL, for the common installation where the git
	// surface and the API surface are one address.
	CloneHost *url.URL

	// SSHCloneHost is the host of the SSH clone form, empty when the
	// installation has no SSH surface; the overview then shows the HTTPS
	// form alone.
	SSHCloneHost string

	// KeysURL is the base address of the installation's public key store,
	// the component that answers Origo's key resolution endpoint and that
	// holds the keys a person adds. Unset is the default and the neutral
	// one: an installation whose operator runs no such component has no
	// key screen and no navigation entry for it, which is what spec 024
	// leaves to the operator's product surface.
	//
	// It is the store's base URL and not one screen's, because this
	// service makes three calls against it.
	KeysURL *url.URL

	// IssuerName is what the sign-in button names, for an installation
	// whose people know their identity provider by name.
	IssuerName string

	// ProductName is what this installation calls itself. It is empty on
	// an installation that has not been named, and Name() then answers
	// with the project's own name, so a self-hoster's interface says what
	// the software is and never someone else's product.
	ProductName string

	// ProjectURL is where the open-source project lives. Every
	// installation links to it, because every installation is an instance
	// of it.
	ProjectURL string

	// Mark names the brand mark the interface draws beside the product
	// name, empty when there is none. A mark belongs to whoever owns it,
	// so this is opt-in and MarkLatere is the only value the binary knows.
	Mark string

	// OIDC is the relying-party configuration authkit reads.
	OIDC oidc.Config
}

// Load reads the configuration from the environment.
func Load() (Config, error) {
	c := Config{
		Addr:         cmp.Or(os.Getenv(EnvPrefix+"_ADDR"), ":8080"),
		SSHCloneHost: strings.TrimSpace(os.Getenv(EnvPrefix + "_SSH_CLONE_HOST")),
		IssuerName:   strings.TrimSpace(os.Getenv(EnvPrefix + "_ISSUER_NAME")),
		ProductName:  strings.TrimSpace(os.Getenv(EnvPrefix + "_PRODUCT_NAME")),
		ProjectURL:   cmp.Or(strings.TrimSpace(os.Getenv(EnvPrefix+"_PROJECT_URL")), DefaultProjectURL),
		Mark:         strings.TrimSpace(os.Getenv(EnvPrefix + "_BRAND_MARK")),
		OIDC:         oidc.LoadConfigWithPrefix(EnvPrefix),
	}

	if c.Mark != "" && c.Mark != MarkLatere {
		return Config{}, fmt.Errorf("%s_BRAND_MARK: this build carries no mark named %q; the only one is %q",
			EnvPrefix, c.Mark, MarkLatere)
	}

	var err error
	if c.OrigoURL, err = requiredURL(EnvPrefix + "_ORIGO_URL"); err != nil {
		return Config{}, err
	}
	if c.PublicURL, err = requiredURL(EnvPrefix + "_PUBLIC_URL"); err != nil {
		return Config{}, err
	}
	// The key store is optional, so an address that does not parse is a
	// misconfiguration and not a reason to run without the screen: an
	// operator who set the variable meant to turn the screen on.
	if raw := strings.TrimSpace(os.Getenv(EnvPrefix + "_KEYS_URL")); raw != "" {
		if c.KeysURL, err = parseURL(EnvPrefix+"_KEYS_URL", raw); err != nil {
			return Config{}, err
		}
	}
	if raw := strings.TrimSpace(os.Getenv(EnvPrefix + "_CLONE_HOST")); raw != "" {
		if c.CloneHost, err = parseURL(EnvPrefix+"_CLONE_HOST", raw); err != nil {
			return Config{}, err
		}
	} else {
		c.CloneHost = c.OrigoURL
	}

	if c.OIDC.RedirectURL == "" {
		c.OIDC.RedirectURL = strings.TrimRight(c.PublicURL.String(), "/") + "/auth/callback"
	}
	return c, nil
}

// RegistryURL is the address of the repository registry, which is the
// identity provider's own: the component that issues the token also holds
// the handles, the organizations and the record of who owns which
// repository, so there is no second address to configure and no second
// credential to hold. Empty when no provider is configured, which leaves
// the interface with no creation screen.
func (c Config) RegistryURL() *url.URL {
	raw := strings.TrimRight(strings.TrimSpace(c.OIDC.AuthURL), "/")
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil
	}
	return u
}

// AccountURL is where a person claims the name their repositories live
// under. It is the identity provider's own account screen, because the name
// is the provider's to hold.
func (c Config) AccountURL() string {
	u := c.RegistryURL()
	if u == nil {
		return ""
	}
	return strings.TrimRight(u.String(), "/") + "/me"
}

// Name is what the interface calls itself, which is the project's own name
// until an operator names the installation something else.
func (c Config) Name() string { return cmp.Or(c.ProductName, ProjectName) }

// Hosted reports whether this installation carries a name of its own, rather
// than being the software under the software's name. A hosted installation
// says which project it is an instance of; an unnamed one is that project.
func (c Config) Hosted() bool { return c.Name() != ProjectName }

// Project is where the open-source project lives, the default when an
// operator set nothing.
func (c Config) Project() string { return cmp.Or(c.ProjectURL, DefaultProjectURL) }

func requiredURL(name string) (*url.URL, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	return parseURL(name, raw)
}

func parseURL(name, raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%s: want an http or https URL, got %q", name, raw)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%s: want a URL with a host, got %q", name, raw)
	}
	return u, nil
}

// CloneHTTPS is the HTTPS clone URL of one repository, built from
// configuration alone.
func (c Config) CloneHTTPS(owner, slug string) string {
	return fmt.Sprintf("%s/%s/%s.git", strings.TrimRight(c.CloneHost.String(), "/"), owner, slug)
}

// CloneSSH is the SSH clone URL of one repository, empty when the
// installation has no SSH surface.
func (c Config) CloneSSH(owner, slug string) string {
	if c.SSHCloneHost == "" {
		return ""
	}
	return fmt.Sprintf("git@%s:%s/%s.git", c.SSHCloneHost, owner, slug)
}

// RepoAPIURL is the address of one repository on the Origo installation:
// the read API's own entry point, which an agent with a token calls
// directly rather than through this interface.
func (c Config) RepoAPIURL(id string) string {
	return fmt.Sprintf("%s/v1/repos/%s", strings.TrimRight(c.OrigoURL.String(), "/"), id)
}

// ArchiveURL is the address of a repository's archive on the Origo
// installation. The link points at Origo directly, so a large tarball never
// passes through this service.
func (c Config) ArchiveURL(id, sha string) string {
	return fmt.Sprintf("%s/v1/repos/%s/archive/%s.tar.gz", strings.TrimRight(c.OrigoURL.String(), "/"), id, sha)
}
