# Changelog

Every tag has a section here, and the section is the body of the GitHub
release. The pre-push hook and the release workflow refuse a tag without
one. Add entries under `Unreleased` as work lands. `lateregate release
vX.Y.Z` turns that into the tag's section, then commits, tags and pushes.

A section says what changed for people using the release. The commit log
holds what was committed.

## Unreleased

- The identity gate now also reads the frontend for the retired admin flag,
  so a page that still branches on it is caught beside the Go, and it refuses
  a second copy of the authorizer envelope, the question and the decision
  that `latere.ai/x/pkg` declares once (ci-gate v0.42.0). Nothing changes for
  a user of the console.

- Signing out now ends the session at the identity provider as well as here.
  It used to clear the cookie alone, which left the single sign-on session
  alive: the next visit was answered without a word and the person who signed
  out was signed straight back in. The interface also answers the identity
  provider's front-channel logout at `/logout/notify`, so signing out
  somewhere else ends a session here rather than leaving it to expire. An
  operator registers `<ORIGOWEB_PUBLIC_URL>/logout/notify` as the client's
  front-channel logout URI; the README says what else it needs.

## v0.10.2 - 2026-09-16

- Creating a repository no longer retries the installation. A repository is
  usable as soon as the control plane registers it, on every replica, so
  the interface makes the one call. A repository the installation does not
  recognise is reported at once instead of twelve seconds later.

## v0.10.1 - 2026-09-16

- The interface no longer asks the identity provider for the
  `origo:ssh-keys` scope. Keys are managed at the platform control plane,
  which authorizes the actor token minted for `api.latere.ai` and grants
  nothing on a login scope. **An operator upgrading removes
  `origo:ssh-keys` from `ORIGOWEB_AUTH_SCOPES`**, leaving
  `openid,email,profile,offline_access`. Leaving it in breaks sign-in once
  the provider drops the scope from this client: an authorization request
  naming a scope the client does not hold is refused outright, so no page
  is reachable, not just the key screen.

## v0.10.0 - 2026-09-16

- The repository registry and the SSH key store are the platform control
  plane's, not the identity provider's. **An operator must set two
  addresses before this release runs**: `ORIGOWEB_REGISTRY_URL` and
  `ORIGOWEB_KEYS_URL`, both the control plane's public base
  (`https://platform.latere.ai` on Latere's installation). Each client
  appends its own path, so neither takes a suffix.

  The interface now presents those two a token minted for the control
  plane's audience, `api.latere.ai`, instead of the session token, which is
  the identity provider's and opens nothing there. The identity provider
  must grant this interface that audience first, or the creation screen and
  the key screen say the provider did not answer and offer no form. Every
  other screen is unaffected: reading a repository uses the token minted
  for Origo, as before.

  The *claim a name* link of the creation screen still points at the
  identity provider's account screen. It was derived from the registry's
  address, which is no longer the same place.

- A tag now rolls Latere's installation instead of stopping at the image.
  The release applies `deploy/prod`, sets the new image, waits for the
  rollout and reads the Deployment's tag back, then fetches the sign-in
  page. Applying the overlay is the part that matters: a setting the
  overlay has dropped used to outlive the release that removed it, because
  setting the image alone leaves the live Deployment's own fields in place.
  The job runs only where `ORIGOWEB_RELEASE_DEPLOY` is set, and waits on the
  `production` environment's reviewer, so another installation publishes the
  image and deploys on its own terms.

## v0.9.0 - 2026-09-13

- Every call to Origo carries a token the identity provider mints for
  Origo alone, for the signed-in person and for minutes; the session token
  stays with the identity provider, where the repository registry and the
  key store are. `ORIGOWEB_AUTH_AUDIENCE` is gone: the client asks for no
  audience at sign-in and is registered at the identity provider to mint
  for `origo`. When the identity provider does not mint, the page says so
  instead of reading as a stranger.

- The sign-in page says what each clone address needs: a key on your
  account over SSH, an agent token over HTTPS, and nothing for a public
  repository.
- The README opens with what the interface is for and how a person uses
  it. Operator detail moved to the deployment notes.

## v0.8.0 - 2026-09-11

- The tree tab of a repository nobody has pushed to says the repository has
  no commits yet, under the repository's own tabs. It answered "Not found"
  with nowhere to go.
- A branch, path or commit that is not there, inside a repository you can
  see, is said in those words. The repository's tabs stay on the page with
  a link to its references, instead of a bare "this repository does not
  exist". The page for a repository you cannot open links back to the list.
- The "Opened in this session" list names each repository by owner and
  name instead of its identifier, and is not shown when the page already
  lists every repository.
- The new repository form is one column: owner, name, then the button.
- The clone box says what git asks for at each address: an agent token as
  the password over HTTPS, with any username, and a key on your account
  over SSH. Each links to the page where you get one.
- The agent documentation and the token page give the installation's
  public address. They printed the address this service talks to, which on
  a cluster is one only the cluster can reach.
- The agent documentation describes the `origo` command and its skill, which
  ship with Origo, in place of a planned MCP server that was never built.
- Repositories are addressed by owner and name: `/{owner}/{name}` and the
  screens under it. The old `/r/{id}` addresses redirect there for good, a
  clone address pasted into the browser lands on the repository, and the
  box on an installation without a directory takes `owner/name` or an
  identifier.
- An administrator can delete a repository from its overview. The page
  says what happens and asks for the name to be typed back. The server
  keeps the content for seven days, within which an administrator can
  restore it through the API.

## v0.7.0 - 2026-09-11

- The file page says whether its address moves. A file opened at a branch
  warns that the next push can change it and offers the address pinned to
  the commit. A file opened at a commit says the address always shows those
  bytes and offers the way back to the branch.
- Every file and commit page ends with a `machine` line linking the same
  page in a format a program reads: `raw` on a file, `patch` on a commit.
  The same addresses are in the page head as `link rel="alternate"`.
- File and commit pages carry a `copy` section that a click opens. It lists
  the permalink, the commit, the path, the patch address and the clone
  address as fields you can select and copy.
- A commit page names the committer when it is not the author, and offers
  one link that opens every file the page rendered shut.
- A directory listing counts the entries it shows and links that
  directory's own history.
- The commit log pager names how many commits the next link fetches.

## v0.6.1 - 2026-09-11

- Every page is denser. Corners are 4px or less, buttons are rectangles,
  headings are one step smaller, and panels, table rows and controls carry
  less padding.
- An address the interface does not serve answers with a 404 page that
  carries the masthead and a link back to the repositories page, instead
  of the server's plain text.
- The wording on every page is shorter and plainer. Notices say what
  happened and what to do. The agent documentation is restructured as
  reference material. The README and this changelog are rewritten in the
  same style.

## v0.6.0 - 2026-09-11

- The palette follows Latere Design System v2: a neutral background and
  surface, with one deep iris accent on links, focus rings and primary
  buttons.
- Secondary text such as line numbers, column headers, breadcrumbs and
  timestamps is darker and meets the contrast floor. The lightest grey is
  used only for separators.
- An administrator can make a repository public or private from the
  overview page. A public repository shows a `public` badge. The visibility
  page says what changes before the button is pressed. When the registry
  cannot be reached, the badge says visibility is unavailable.

## v0.5.3 - 2026-09-11

- The repository page puts the name, the counts, the branch picker and the
  clone address in one block at the top. The branch picker opens in place.
  HTTPS and SSH clone addresses are two tabs. The last commit sits above
  the file list.
- SSH key management. The keys page lists your keys with the comment from
  the pasted line, the algorithm, the fingerprint, the date added and the
  last use. A key that has never been used is marked in red.
- Adding a key takes two steps: paste the key, review the fingerprint the
  key store computed, then confirm. Check it against `ssh-keygen -lf` on
  the machine that owns the key.
- Removing a key asks for confirmation on a page that names the key. Clone
  and push with that key stop working within a minute.
- A key already on your account is named back to you. A key on another
  account is reported as taken, without naming the holder.
- The keys page appears only when the installation runs a key store. See
  "SSH keys" in the README for the API it expects.

## v0.5.2 - 2026-09-11

- The repositories page groups repositories by owner and shows counts.
  When the whole list fits on one page, a filter box narrows it by name or
  owner.
- Repository creation. **New repository** on the repositories page asks
  for an owner and a name and opens the empty repository with push
  instructions. Owners are your account and the organisations you
  administer. If you have not claimed a name, the page links to your
  account. Nothing is imported. The button appears only on an installation
  with a repository registry.
- Shorter wording on the sign-in page, the repositories page and the agent
  documentation.

## v0.5.1 - 2026-09-11

- The sign-in page is one column: the sign-in button first, then the clone
  address and the project link, separated by rules. The page fills the
  window height.
- The clone address hint says cloning uses the same sign-in.

## v0.5.0 - 2026-09-11

- New look: a warm paper background, panels and tables with visible
  borders, and one rust accent on links, focus rings and primary buttons.
- Text is set in IBM Plex Sans. Hashes, paths, refs, filenames and code are
  set in IBM Plex Mono.
- Secondary grey text in the light theme is darker and meets the contrast
  floor.

## v0.4.2 - 2026-09-11

- The section list on the documentation page sits beside the text instead
  of at the far right of the window. Tables and samples still use the full
  width.

## v0.4.1 - 2026-09-11

- A long repository identifier wraps on the agent tokens page instead of
  making the choices scroll sideways.

## v0.4.0 - 2026-09-11

- The bar at the top of every page names the signed-in account. A page
  with nothing on it says so in words.
- A first visit shows the sign-in page without claiming a credential was
  rejected. That message is kept for a session that held a credential and
  was refused.
- Every page uses the full window width. The documentation has its section
  list down the side, the sign-in page has three panels across, and the
  token form fills its panel.
- No native select menus. Choices are radio lists you can move through
  with the arrow keys. Long lists scroll in place.
- Text that does the same job looks the same everywhere: choices, hints
  and headings are consistent across pages.
- The agent tokens page says each thing once. What a token cannot do, and
  why, is on the documentation page.

## v0.3.0 - 2026-09-10

- Pages use the whole window. A diff, a file tree, a commit log and a file
  get the full width. Running text keeps a readable line length.
- The signed-out page opens with what you can do here instead of repeating
  the name and logo from the bar above it.

## v0.2.0 - 2026-09-10

- An installation can set its own name with `ORIGOWEB_PRODUCT_NAME`, a
  logo with `ORIGOWEB_BRAND_MARK`, and a project link with
  `ORIGOWEB_PROJECT_URL`. Unset, the interface calls itself Origo, draws no
  logo and links to the Origo repository.
- The signed-out page says what this is: a place to read git repositories,
  running open-source software, with a link to the project and the name of
  the installation. The sign-in button and the clone address come first.
- Notices, callouts and quotations have a surface and a border instead of
  a rule down the left edge.

## v0.1.1

- The image sets its user by numeric id, so a pod that requires
  `runAsNonRoot` starts. Before this the kubelet refused the container
  because it cannot resolve a user name.

## v0.1.0

- First release. Browses an Origo installation: repositories, branches and
  tags, the commit log, commit diffs, files with line links, and raw files.
  Signs you in through the identity provider the installation trusts and
  shows only what your credential can fetch. Plain HTML, no JavaScript,
  follows the system light or dark theme.
