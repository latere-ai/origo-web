# Changelog

Every tag has a section here, and the section is the body of the GitHub
release. A tag without one is refused at the pre-push and fails the release
workflow. Write under `Unreleased` as work lands; `lateregate release vX.Y.Z`
turns that into the tag's section, commits, tags and pushes.

A section says what changed for whoever uses the release, not what was
committed: the commit log already holds that.

## Unreleased

- The interface uses the whole window. Screens were laid out in a narrow
  column with empty space either side of it; now a diff, a file tree, a
  commit log and a file get the width the window has. Running text is
  still set to a readable line length, so the signed-out page and the
  documentation read the same as before.
- The signed-out page opens with what you can do here instead of
  repeating the name and the logo from the bar above it.

## v0.2.0 - 2026-09-10

- An installation can carry a name of its own. `ORIGOWEB_PRODUCT_NAME` is
  what the interface calls itself in the masthead, the tab title and the
  sign-in heading; `ORIGOWEB_BRAND_MARK` draws a logo beside it, and
  `ORIGOWEB_PROJECT_URL` is where it links people to the project. Leave
  all three unset and your installation calls itself Origo, draws no
  logo, and links to the Origo repository.
- The signed-out page now says what this is: a place to read git
  repositories, running open-source software, with a link to the project
  and the name of the installation you have landed on. It is still a
  door: the way in and the clone address come first.
- Notices, callouts and quotations are set apart by their surface and
  their border. The rule down their left edge is gone.

## v0.1.1

- The image names its user by id, so a pod that asks to run as non-root
  starts. Before this the kubelet refused the container: it cannot resolve
  a user name, and the image carried one.

## v0.1.0

- First release of the interface. It reads an Origo installation and
  renders repositories, branches and tags, the commit log, a commit's
  diff, a file with line links, and a raw file. It signs you in through
  the same identity provider the installation trusts, and shows only what
  your own credential can already fetch. Every page is plain HTML: no
  JavaScript is required, and it follows your light or dark preference.
