# Changelog

Every tag has a section here, and the section is the body of the GitHub
release. A tag without one is refused at the pre-push and fails the release
workflow. Write under `Unreleased` as work lands; `lateregate release vX.Y.Z`
turns that into the tag's section, commits, tags and pushes.

A section says what changed for whoever uses the release, not what was
committed: the commit log already holds that.

## Unreleased

- First release of the interface. It reads an Origo installation and
  renders repositories, branches and tags, the commit log, a commit's
  diff, a file with line links, and a raw file. It signs you in through
  the same identity provider the installation trusts, and shows only what
  your own credential can already fetch. Every page is plain HTML: no
  JavaScript is required, and it follows your light or dark preference.
