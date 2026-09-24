# Using origo-web

For people who browse, clone, and manage repositories through the
interface. The addresses below are relative to the installation, such as
`https://code.example.com`.

## Signing in

**Sign in** sends you to the identity provider your installation uses, and
back to the page you asked for. There is no password, no sign-up, and no
account here: the account is the provider's.

A session lasts twelve hours from sign-in. Your access token is kept in one
encrypted cookie and sent to Origo on your behalf. It never appears in a
page, an address, or a log line.

**Sign out** ends the session here and at the identity provider, so the next
visit asks for an account again. Signing out of the identity provider
somewhere else ends the session here too, on installations whose operator
has set that up.

Without a session, a page shows whatever Origo lets a caller with no
credential read. Origo does not resolve a repository's name for such a
caller, so every repository page asks a signed-out visitor to sign in,
public repositories included. A public repository is readable without an
account through git and through Origo's API.

## Finding a repository

The home page, **Repositories**, lists the repositories you may read,
grouped by owner. When the whole list arrives at once, a filter box narrows
it by owner and name.

Some installations do not list repositories. Their home page instead has a
box that opens `owner/name` or a repository identifier, and lists the
repositories you opened in this session.

Repositories are addressed by owner and name, the way git addresses them,
so renaming a repository moves its address. Three other forms land on the
same page:

| You have | Open | You land on |
|---|---|---|
| a repository identifier | `/r/{id}`, or any screen under it | the same screen under the name |
| a clone address, pasted into the browser | `/{owner}/{name}.git` | the overview |
| `owner/name` or an identifier | the box on the home page | the overview |

The identifier never changes. It is what Origo's API and agent tokens use,
and the agent tokens page shows it.

## Screens

| Screen | Address |
|---|---|
| Repositories | `/` |
| Sign in | `/sign-in` |
| New repository | `/new` |
| Overview: clone address, branch picker, root tree, readme | `/{owner}/{name}` |
| Branches and tags | `/{owner}/{name}/refs` |
| Commit log, filterable by path | `/{owner}/{name}/log` |
| Commit: message, parents, diff | `/{owner}/{name}/commit/{sha}` |
| Patch download | `/{owner}/{name}/patch/{sha}` |
| Compare two revisions | `/{owner}/{name}/compare?base=&head=` |
| File tree | `/{owner}/{name}/tree/{path}` |
| File, with an anchor per line (`#L12`) | `/{owner}/{name}/blob/{path}` |
| Raw file, as a download | `/{owner}/{name}/raw/{path}` |
| Visibility | `/{owner}/{name}/visibility` |
| Delete | `/{owner}/{name}/delete` |
| Agent tokens | `/tokens` |
| SSH keys, when the installation runs a key store | `/keys` |
| How to configure an agent | `/docs/agents` |

`?ref=` selects a branch or tag on any repository screen. The branch picker
on the overview sets it, so every screen at every revision has an address
of its own.

## Reading

The overview shows the repository's readme, up to 512 KiB, rendered as
Markdown when its name ends in `.md` or `.markdown` and as plain text
otherwise. Raw HTML inside a readme is dropped. A larger readme links to its
file page instead.

A file page renders up to the first 1 MiB of a text file and says when it
stopped there. A binary file is offered as a download. A file over 50 MiB is
not shown, and Origo does not send it in one response, so clone the
repository to get it.

A diff renders each file collapsed when it changes more than 500 lines, and
without its body when it changes more than 5,000. When a whole commit or
comparison changes more than 20,000 lines, every file starts collapsed. A
file whose body was left out has a link that renders it anyway.

Pages may carry a notice:

- **frozen, read-only**: an administrator froze the repository at Origo.
  Reads work; pushes are refused.
- **This page may be out of date**: Origo is serving its cached copy
  because its storage is unreachable, and the notice says how long ago the
  copy was last refreshed.

## Cloning

The overview shows the clone address in the forms the installation serves:
SSH, when the installation runs it, and HTTPS.

```sh
git clone git@code.example.com:owner/name.git
git clone https://code.example.com/owner/name.git
```

SSH authenticates with a key you registered on the **SSH keys** page. Over
HTTPS a public repository clones with no credential, when the installation
admits reads without one. A private repository asks for a password: give an
agent token, with any username.

## SSH keys

The **SSH keys** page lists the public keys registered to your account, with
each key's comment, type, fingerprint, and when it was added and last used.
The page exists only on installations that run a key store.

Adding a key takes two steps. Paste the public key, the contents of a file
such as `~/.ssh/id_ed25519.pub`; its trailing comment becomes its label. The
page then shows the fingerprint the key store computed. Compare it with the
output of `ssh-keygen -lf ~/.ssh/id_ed25519.pub` on the machine that holds
the key, then confirm. The interface never parses a key itself, so the key
you confirmed is the key that is stored.

A key already on your account, a key registered to another account, and a
paste that is not one public key are each refused with a sentence of
their own.
Removing a key asks once more; clones and pushes with it stop working within
a minute.

## Agent tokens

An agent token gives a program access to one repository until it expires.
**Agent tokens** asks for three things:

| Field | Choices |
|---|---|
| Repository | one, by name or identifier |
| Scope | `read`: clone and read. `write`: read and push. Neither grants admin |
| Lifetime | 5 minutes, 15 minutes, or 1 hour, the longest allowed |

Minting a token needs admin access to the repository. The token is shown
once, on the next page, with a clone command and an API call that use it:

```sh
git clone https://x-access-token:$ORIGO_TOKEN@code.example.com/owner/name.git
curl -H "Authorization: Bearer $ORIGO_TOKEN" https://code.example.com/v1/repos/<id>
```

Origo signs tokens and keeps no record of them, so a token cannot be listed
or revoked. If one leaks, wait for it to expire, or ask an administrator to
change who may access the repository. `/docs/agents` is a page to hand to
whoever configures an agent, with the addresses of this installation filled
in.

## Creating a repository

**New repository** asks for an owner and a name, and creates an empty
repository with a default branch and no commit. Nothing is imported. You
land on its overview, which shows the clone address to push to.

The owners offered are your own account name and the organizations you
administer. If you have not claimed a name, the page links to your account
at the identity provider. A name uses letters, digits, `.`, `_`, and `-`,
up to 64 characters.

The installation decides which names are yours and how many repositories
each may hold, and Origo checks again before it writes anything. A refusal
from either keeps your form as you typed it. The button appears only on
installations that run a repository registry.

## Visibility

An administrator of a repository can make it public or private from its
overview. A public repository can be read and cloned without an account,
through git and through Origo's API; pushing still needs access, and the
repository is not listed anywhere publicly. Making a repository private does
not affect clones that already exist.

The overview shows a `public` badge on a public repository. When the
registry that records visibility cannot be reached, the badge says
visibility is unavailable, so a failed read never looks like a private
repository.

## Deleting

An administrator can delete a repository from its overview. The page says
what happens and asks for the repository's name, typed exactly as shown.

Origo keeps the content for seven days before removing it for good. Deleting
here also withdraws the registry's record of who owns the repository, so the
name is free to use again at once. The interface has no restore button:
restoring within the seven days is an operator's call to Origo's API, and on
an installation whose permissions come from that record, it is registered
again first.

## When something is refused

| You see | It means |
|---|---|
| the sign-in page on a repository address | there is no session, or your session was refused; sign in again |
| "This repository does not exist or you do not have access to it." | exactly one of the two, and the page does not say which, on purpose |
| "The identity provider did not answer for this installation." | you are signed in, but the provider did not issue the token this page needs. Pages that need no such token keep working |
| "This form has expired" | the form was open across a sign-out; go back and submit it again |
| "Repository creation is not available on this installation" | the installation runs no repository registry |
