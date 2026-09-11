# Changelog

Every tag has a section here, and the section is the body of the GitHub
release. A tag without one is refused at the pre-push and fails the release
workflow. Write under `Unreleased` as work lands; `lateregate release vX.Y.Z`
turns that into the tag's section, commits, tags and pushes.

A section says what changed for whoever uses the release. The commit log
already holds what was committed.

## Unreleased

- The repository page puts the name, the counts, the branch picker and the
  clone address in one block at the top. The branch picker opens in place.
  The clone address is the widest thing on the page, and HTTPS and SSH are
  two tabs. The last commit sits above the file list instead of in a panel
  of its own.
- You can manage your SSH keys here. The screen lists the keys on your
  account with the label from the line you pasted, the algorithm beside the
  fingerprint, the day you added each one and when it was last used, so you
  can tell two of your own apart and see which one is dead. "never used" is
  the only red on the page, because it is usually the one that means the key
  can go.
- Adding a key takes two steps. You paste the line, the screen shows you the
  fingerprint that will be stored, and nothing is saved until you confirm.
  Check it against `ssh-keygen -lf` on the machine that owns the key: that
  is the whole point of the step, and the fingerprint you are shown is
  computed by the store that will hold it, not by this page.
- Removing a key asks first, on a page that names the key, and tells you
  what stops working. A machine holding the matching private key loses its
  clone and push within a minute; nothing else changes.
- A key already on your account and a key on somebody else's are told apart.
  Yours is named back to you so you can find it in the table; another
  account's says only that the key is taken, because a public key is public
  and naming the holder would say who has which credential.
- The screen appears only where the installation runs a key store. If yours
  does not, nothing changes and no navigation entry for it appears. See
  "SSH keys" in the README for the three calls a store answers.

## v0.5.2 - 2026-09-11

- The repositories page groups repositories by owner, one table an owner,
  and counts what it shows. When the whole list fits one page, a filter
  box narrows it by name or owner.

- You can create a repository. **New repository** on the repositories page
  asks for a name to put it under and a name for it, and lands you on the
  empty repository with the commands to push to it. What you may put it
  under is your own name and any organisation you administer; if you have
  not claimed a name yet, the page says so and links you to your account.
  Nothing is imported and no first commit is made. The button appears only
  on an installation that has somewhere to record who owns a repository.
- The wording on the sign-in page, the repositories page and the agent
  documentation is shorter. Each line says what to do and stops.

## v0.5.1 - 2026-09-11

- The sign-in page is one column instead of three cards in a row. The
  sign-in button comes first. The clone address and the link to the
  project sit below it, separated by rules. The page fills the window
  instead of ending a third of the way down.
- The clone address said cloning needs "the same account", which assumed
  an account you may not have yet. It now says cloning uses the same
  sign-in.

## v0.5.0 - 2026-09-11

- The interface has a new look. The page is a warm paper tone. Panels
  and tables sit on it inside a visible rule. One rust accent marks
  links, focus and the main button on a screen.
- Text is set in IBM Plex Sans. Hashes, paths, refs, filenames and code
  are set in IBM Plex Mono.
- Grey text is darker in the light theme. Line numbers, breadcrumbs and
  column headers were below the readable contrast floor and now clear
  it.

## v0.4.2 - 2026-09-11

- The section list on the documentation screen sits beside the words it
  indexes instead of at the far right of the window, so there is no
  longer a wide empty band between the two. The tables and the samples
  still run the full width.

## v0.4.1 - 2026-09-11

- A repository with a long identifier no longer makes the choices on the
  agent tokens screen scroll sideways. The name wraps in place.

## v0.4.0 - 2026-09-11

- The bar at the top of every screen says which account you are signed
  in as. What you can see here is exactly what your account can read, so
  when a page is empty the account is the first thing that explains it.
  A screen with nothing on it says so in words too.
- A first visit no longer says a credential of yours was rejected. It
  says what this is and offers the way in. The refusal is kept for what
  it describes: a session that held a credential and was turned away.
- Every screen uses the width of the window. The documentation reads at
  a comfortable line length with its section list down the right side
  instead of wrapping across the top, the sign-in page is three panels
  across the width, and the token form fills its panel.
- No screen opens a menu drawn by your operating system. Choices are
  lists you can move through with the arrow keys, and a long list of
  repositories or branches scrolls in place.
- Text that does the same job now looks the same everywhere: a choice
  reads the same whether it names a scope, a lifetime, a repository or a
  branch, and headings line up with the text under them.
- The agent tokens screen says each thing once, where you need it. What
  a token cannot do, and why, is on the documentation screen.

## v0.3.0 - 2026-09-10

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
