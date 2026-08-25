<!-- generated-by: groundrules v1.10.0 -->
# Learnings — mcp-remote-bridge

Rules learned from corrections and non-trivial discoveries during the project. Reverse-chronological order (newest at the top). **Re-read at session start.**

One entry = one **actionable rule**, not a journal note. Each entry has:
- a title that states the rule (imperative or "X: do Y");
- **Why** — the story behind it: what happened, what it cost (a revert, a lost CI cycle, a confused user…);
- **When to apply** — the concrete trigger conditions, so the rule fires at the right moment instead of being remembered too late.

Include the minimal code snippet / command when it is the fix.

---

## Declare authentication when creating a resource — some systems will not let you add it later

**Why**: a Cloudflare MCP Portal server created against an unauthenticated origin records *"this
server does not require authentication"*, and that state has **no way back**. Guarding the origin
afterwards produces a deadlock:

- the authentication field never appears, because the server is recorded as needing none — the
  edit screen only offers *Update headers* on a server that already has them;
- `Sync capabilities` fails with `failed to get information`, because the origin now answers
  `403` — it fails for exactly the reason you are trying to fix;
- the server goes to `Error` and stays there.

The only way out was to **delete a running resource and recreate it**, declaring header-based
authentication in the creation form. On a server carrying live traffic, with a client secret that
Cloudflare shows once, in the middle of a form.

Working backwards, the correct order was: create the Access application on the hostname
**first**, then create the Portal server declaring authentication from the start. Nothing in the
UI suggests that order, and the reference guide walks the other way.

**When to apply**: before creating any resource that talks to a protected origin — a webhook, a
scraper, an API integration, a monitoring probe. Ask whether authentication can be **added
later** or only **declared at creation**. If the system infers it from a probe of the target, it
has recorded a fact about the past that it may never re-check.

Two corollaries worth stealing:
- **A "sync" or "refresh" button is not a way out**: it re-probes with the credentials it has,
  which is precisely what is missing. It fails hardest exactly when you need it most.
- Any resource whose creation form has more fields than its edit form is a resource whose
  creation order matters. Read both screens before creating the first one.

## The cost of a manual procedure is its confusion, not its length

**Why**: this project was founded on replacing a 489-line guide — length as the measure of the
problem. Guarding one MCP by hand turned out to be four steps, and the maintainer, who had built
the infrastructure himself and knew it, spent twenty minutes stuck on it. Not because it was
long. Because:

- two different screens present a field labelled **authentication**, meaning different things:
  who may reach the hostname, versus what the Portal presents when it reaches it;
- the second field **does not appear** until the first is configured, since the Portal only
  offers to authenticate once the origin starts refusing it — so the correct order is
  discoverable only by breaking something first;
- one of the values is shown **exactly once** and never again;
- the source guide calls the unguarded path *"that's fine"*, so following it faithfully produces
  the hole.

The stuck-ness was the useful signal, not the page count. It is also what made the scope decision
obvious: the API can do three of the four steps, and returns the one-time secret that the
dashboard does not.

**When to apply**: when justifying a tool that automates a manual procedure, measure the
procedure's **confusion**, not its size. Ask where a knowledgeable person hesitates, which fields
share a name, what order is non-obvious, and what value cannot be recovered if missed. Those are
the parts worth automating — a long but linear procedure is often fine as a script or a checklist.

Corollary observed here: an API surface can be *safer* than its dashboard, not merely faster.
Returning a secret once in a response body that goes straight to a keychain beats displaying it
once in a browser where the only place to put it is a clipboard.

## Integration tests that share a global resource are not isolated, they are sequenced

**Why**: the walking-skeleton tests derived their entry name from the process id, so every test
in the file used the **same** name — and the name determines both the launchd label and the
auto-assigned port. Two global resources, shared by five tests.

The failure it produced was not a clean one. One test's cleanup ran `launchctl bootout`, which
returns before the service is actually gone; the next test's `Ensure` then called `bootstrap` on
a label that was still loaded, and since bootstrap-on-a-loaded-label is correctly treated as
"already there", the second test **silently inherited the first test's service** — pointing at
the first test's binary and config, in a temp directory that was already being deleted.

Symptom: one test failed only when run with the others, and passed in isolation. It burned 30
seconds waiting for a state that could not arrive.

**When to apply**: any test that touches something the OS namespaces globally — a port, a
service label, a socket path, a well-known file, a database name. Derive the identity from
`t.Name()`, not from the process, the clock, or a constant. And treat "passes alone, fails in
the suite" as evidence of a shared resource rather than of flakiness: flakiness is what it looks
like, interference is what it is.

## A tool's exit codes and its error messages are two different things

**Why**: `launchctl bootstrap` on an already-loaded label fails with
`Bootstrap failed: 5: Input/output error`. There is no I/O problem — it means "already there".
Taken at face value the message sends you debugging disks and permissions; taken as an exit code
with a measured meaning, it is the ordinary case a reconciler must handle. Likewise `bootout` on
an absent label returns 3 `No such process` and `print` on an unknown label returns 113
`Bad request`, both of which are **answers**, not failures: already-gone is `Remove`'s desired
state, and unknown is `Status`'s honest reply.

The same session produced a second instance in the same tool. `launchctl print` reports
`state = xpcproxy` for a moment after bootstrap, before it becomes `running`, so deriving
"is it running?" from `state == "running"` is a race that calls a healthy service stopped. The
reliable signal is the presence of a `pid`. Three tests failed on it before the cause was
visible.

**When to apply**: whenever you shell out to a tool and branch on failure. Measure the exit code
for each state you care about — including the states you expect to be *normal* — and write the
meaning next to the constant, with the date. Never branch on message text, and do not assume the
message describes the code. Then, separately, check whether the field you are parsing is
**stable**: a value read immediately after an action may be a transient one.

## Validating a file format is not validating that its consumer accepts it

**Why**: the frozen acceptance test for `BuildPlist` validated generated plists with
`plutil -lint`, which proves the XML *parses* as a property list. It says nothing about whether
**launchd** accepts the document or interprets it as intended. The loop's verifier went further
on its own — `launchctl bootstrap gui/$UID <plist>` then `launchctl print` — and that is what
actually confirmed `minimum runtime = 60`, `successful exit => 0`, `after crash => 1` and the
arguments passed through verbatim. A lint could not have told us any of it.

The same gap bit the spec a second time in the same file: the test only ever used a 60s
`ThrottleInterval`, so nothing caught that a **zero value renders as `<integer>0</integer>`,
which disables throttling** — precisely the restart spin the spec introduced the field to
prevent. The implementation was correct against the spec; the spec and its test were both
incomplete.

**When to apply**: whenever you generate a file another program consumes — a plist, a systemd
unit, a cron entry, a k8s manifest, a Dockerfile. Two rules:

1. **Validate with the real consumer, not a linter.** Load it, then read back what the consumer
   understood. Use a throwaway name and tear it down afterwards.
2. **Test the zero value of every field.** A struct field nobody sets is the input most likely
   to reach production, and the one a happy-path fixture never exercises. If its rendering is
   dangerous, refuse it — the zero value must not be the dangerous one.

## Round-trip every byte class through an external tool before trusting its output

**Why**: `security find-generic-password -w` prints a password on stdout, which is exactly what
a `SecretSource` wants — for printable ASCII. Any other byte (an accent, a tab, a newline, a
backslash) makes it print a **bare hex string with no marker**, and storing the literal string
`636166c3a9` produces the same output as storing `café`. The ambiguity is irreducible: the
information is gone before the caller sees it.

Shipping that would have corrupted an accented password or a PEM private key into an ASCII hex
string, with **nothing to debug from**: the secret was found, it was injected, and the value is
correctly never logged. `-g` disambiguates with a `0x` prefix.

**When to apply**: whenever a value crosses a process boundary through a tool's *human-readable*
output — a CLI, a formatter, a serialiser. Do not test the happy path and generalise. Round-trip
a table of byte classes: plain ASCII, spaces, accents, tab, newline, backslash, quote, emoji, and
**a value that looks like the encoded form** (that last one is what catches an ambiguous
encoding rather than merely a lossy one).

The same test table caught it and proves the fix: reverting to `-w` fails four of twelve cases.
Write the mutation check, not just the assertion — a test that has never been observed failing
has not been verified.

## Verify an external tool's behaviour by killing something, not by reading its docs

**Why**: `SPEC-primitive.md` named the trap "the proxy can be up while the MCP inside it is
dead" and answered it with a probe driving "a real MCP `initialize` handshake". Measured
against `mcp-proxy 0.12.0` — wrap an MCP, `kill -9` the child, replay the requests — the
answer was wrong: **the proxy answers `initialize` itself**, from the state negotiated at
startup, and still reported `serverInfo.name` ten seconds after the process died.

`ping` — the method the MCP protocol advertises *for liveness* — is answered by the proxy too.
It is the worse trap: its name disarms the reader, so it would have shipped unquestioned.
Underneath both, `tools/list` against a dead MCP returns **HTTP 200**; the failure is only in
the JSON-RPC body.

Cost: nothing, because the check happened before implementation. Had it not, the project would
have shipped the exact defect its own rule 2 forbids — a health check that cannot fail — while
its README promised the opposite. Three layers of plausible-sounding verification, all
non-verifying.

**When to apply**: before specifying **any** check that depends on an external process
answering — a proxy, a sidecar, a gateway, a connection pool. Do not reason about what the
layer "should" forward. Break the thing on purpose and watch: kill the child, stop the
upstream, revoke the token, then run the check and require it to go red. A check never observed
failing is a check that has not been verified.

Two traps met in the doing, worth stealing:
- `pkill -f <script>` killed the proxy too, because the proxy's own `argv` contains the script
  path. Kill the child by PID.
- The first run of a negative test needs a **positive control**. "It failed when I broke it" is
  worthless without "it passed when I hadn't."

## An acceptance test constrains less than it looks — probe outside it before calling a task done

**Why**: `internal/bridge/probe_test.go` was written to freeze the spec of
`ProbeProxyListening`, *including* the loopback-only invariant ("never `0.0.0.0`, never the
public hostname"). It does not actually prove that. All three of its cases dial a listener
that is itself bound to `127.0.0.1`, so an implementation dialling **every** interface would
have passed all three green. The gap was caught by the loop's verifier probing *outside* the
frozen test — a listener bound to a LAN address only, expecting `OK=false` — not by the test
that was supposed to be the gate.

Cost: nothing this time, because the verifier looked further than its instructions required.
That is luck, not process.

**When to apply**: whenever a test is the acceptance gate for a **security-shaped invariant**
— a bind address, an auth requirement, path containment, a charset that keeps input out of a
path. Ask what the test would still *accept*, not only what it rejects. A frozen acceptance
test is a floor, never a proof.

Corollary when writing one: prefer a case that fails on a **wrong** implementation, not only
on a **missing** one. `ValidateName("../../etc/passwd")` is that kind of case; a probe test
whose every fixture satisfies the invariant by construction is not.

<!-- Example:

## Palette changes: one mock screen first, then propagate

**Why**: a new primary color was propagated to all 7 prototypes before the user
saw it in context. Verdict: "revert it all" — one full commit/push/deploy cycle lost.

**When to apply**: any *substitutive* visual change (primary color, font, layout
overhaul). Apply on ONE representative screen, get a visual validation, then
propagate. Additive changes (a new utility class) are lower-risk.

-->
