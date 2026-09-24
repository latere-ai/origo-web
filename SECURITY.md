# Security

Report a vulnerability to security@latere.ai. Do not open a public issue
for one. You will hear back within three business days, and a fix for a
high severity issue ships within thirty days. Credit in the release notes
on request.

Fixes go to the newest release. The releases page lists them.

## What the interface holds

origo-web holds no credential of its own and decides nothing about access.
A person's session lives in one encrypted, `__Host-` prefixed cookie and in
no server-side store. Each call to Origo, to the repository registry, and
to the SSH key store carries a token the identity provider minted for that
person and that one service, valid for minutes, so the interface can do
nothing the person could not do with the same token. The session token and
those tokens never appear in a page, an address, or a log line.

## What the pages allow

- **No script.** Every page carries a Content-Security-Policy that allows
  no script, no third-party resource, and no framing. The one exception is
  the front-channel logout address, which only the identity provider's
  origin may frame.
- **No cache.** Every page is `Cache-Control: private, no-store`, because a
  page belongs to the person it was rendered for.
- **Forms.** Every form that changes something carries a token bound to a
  cookie, and is refused without it.
- **Addresses come from configuration.** Clone addresses, the sign-in
  redirect, and every absolute link are built from configuration, never
  from a request's `Host` header.
- **Deleting** needs the repository's name typed back, the person's own
  administrative access, and ends in Origo's seven-day hold rather than an
  immediate purge.

## The container

The image is distroless and runs as a non-root user. The manifests run it
with a read-only root file system, every capability dropped, no service
account token, and the `RuntimeDefault` seccomp profile.

Dependencies are checked for known vulnerabilities on every push.
