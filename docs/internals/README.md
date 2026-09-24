# Internals

For people changing origo-web. The user and operator documentation is one
level up, in [`docs/`](../README.md); these pages describe how the code
works and how it is tested.

| Page | |
|---|---|
| [Architecture](architecture.md) | the package map, how a request travels from the browser to Origo, the session and its tokens, sign-in and sign-out, rendering without script, and the order of the two writes |
| [Testing](testing.md) | the fakes, what each group of tests holds, the end-to-end tier against a real Origo, running the interface locally, and the gate |

The design records, with the reasoning behind each decision, are in
[`specs/`](../../specs/): the web interface itself, and the move of the
repository registry and the key store to their own audience.
[`CONTRIBUTING.md`](../../CONTRIBUTING.md) is how to send a change.
