<!-- generated-by: groundrules v1.10.0 -->
# Learnings — mcp-remote-bridge

Rules learned from corrections and non-trivial discoveries during the project. Reverse-chronological order (newest at the top). **Re-read at session start.**

One entry = one **actionable rule**, not a journal note. Each entry has:
- a title that states the rule (imperative or "X: do Y");
- **Why** — the story behind it: what happened, what it cost (a revert, a lost CI cycle, a confused user…);
- **When to apply** — the concrete trigger conditions, so the rule fires at the right moment instead of being remembered too late.

Include the minimal code snippet / command when it is the fix.

---

## A wildcard DNS record makes `hostname_resolves` a check that cannot fail

**Why**: the v0.1.0 release check was run against `paranoid.foo`, a zone carrying a wildcard
`*` record. Every name under it resolves — measured: `relcheck-mcp`,
`ceci-nexiste-pas-du-tout-42` and `xyzzy-quux` all returned the same address before anything was
created.

So on such a zone `ProbeHostnameResolves` passes for a hostname the tool has never published. It
reports "resolves" as evidence that a record exists, when it is evidence of nothing — which is
precisely what rule 2 forbids: *a check that cannot fail is worse than none, because it
manufactures confidence.*

There is a discriminator, found in the same run: before `apply` the name returned the wildcard's
address, and after it the Cloudflare edge addresses. So the record's existence IS observable from
DNS alone — just not by asking whether the name resolves.

**When to apply**: whenever a probe asks "does this name resolve", "does this path exist", "does
this key have a value" — ask what a *default* would do to it. Wildcards, catch-alls, fallback
routes and default values all turn an existence check into a tautology. And when verifying a
deletion on a zone like this, `dig` cannot answer: the authoritative API said the record was gone
while a cached lookup still returned the edge addresses and the hostname still answered `522`.

---

## `go run` defeats a per-binary firewall; build once to a stable path

**Why**: three API probes in a row timed out while `git` and `xcrun notarytool` reached the
network from the same shell, seconds apart. Signing the probe changed nothing, and disabling the
agent sandbox changed nothing either — both were wrong guesses, made because the only evidence
was a timeout.

The cause was Little Snitch. `go run` compiles to a temporary path whose **basename is always
`main`** and whose **checksum changes every run**, so the filter did not see an unknown program:
it saw a *known* program that had been modified, and raised "The program has been modified!"
every single time. Nobody was at the screen, the dialog was never answered, and the connection
died as a timeout rather than a refusal.

The fix is not to sign and not to ask for more permission — it is to stop moving:

```sh
go build -o <stable-path>/tool main.go   # once
<stable-path>/tool                       # many times, same checksum, one approval
```

**When to apply**: whenever a program you *just compiled* cannot reach the network while system
binaries can. Check for a per-application filter (Little Snitch, LuLu, a VPN's split tunnelling)
before blaming DNS, TLS, IPv6 or a sandbox — and prefer `go build` to a fixed path over `go run`
for anything that opens a socket, on any machine that might have one.

**The pattern worth carrying**: this is the third time in this project that an unanswered
authorisation dialog surfaced in code as a network timeout — the keychain prompt, `notarytool
--wait`, and now this. *An invisible prompt and a dead network are indistinguishable from
inside the process.* When a timeout makes no sense, look for a dialog nobody answered.

---

## A dead client says nothing about the server it was waiting on

**Why**: `make release` failed on `HTTPClientError.connectTimeout` from `notarytool submit
--wait`. That reads as a network fault, so the reflex is to retry the whole thing — which would
have resubmitted an archive Apple had already accepted for processing. The upload had in fact
succeeded: `notarytool history` showed the submission with an id and `status: In Progress`.

Then it happened twice more, and the shape became legible. `notarytool info` answered instantly
throughout, while `--wait` died against the same host. They are not the same request: `wait`
holds a long-lived connection that some environments kill, `info` is a short one. **The client
that timed out was reporting on itself, not on the work.**

This is the same failure as the keychain prompt earlier in this project, where a blocked dialog
surfaced in code as a network timeout — a transport error standing in for a state nobody had
asked about. Both times the error named the messenger.

The fix is to ask the server: poll with `info`, never re-drive the operation on a dead waiter.

**When to apply**: whenever a long-running remote operation reports a client-side timeout —
notarisation, a CI run, a deploy, a queued job. Before retrying, find the read-only query that
reports the *server's* state and run it. Retrying on a timeout you have not diagnosed risks
duplicating work that already succeeded. And when writing such a step, make the recovery path
a poll, not a longer wait.

**A follow-on, and a correction of my own**: the same episode produced a *second* local check
that lagged the server — `codesign --test-requirement="=notarized"`, which turns true only once
Apple's ticket reaches the service it asks. Seeing an arm64 binary pass while an x86_64 one
failed on the same host, I concluded the check could not resolve a foreign architecture and
wrote that into the release tooling. It was wrong: both verify now, unchanged. They had simply
been submitted at different moments, and I had turned a timing difference into an architectural
law because the two samples I had happened to differ that way. *Two data points that differ in
several ways at once cannot tell you which difference matters* — vary one thing, or wait and
re-measure, before writing the explanation down.

---

## A name is not an identifier unless the system enforces it

**Why**: Cloudflare allows two Access service tokens to share a name. `setup` looked one up by
name, got the first match, and printed its client id next to a secret belonging to the **other**
— a pair that authenticates as `403`, long after being copied into a config, with nothing at the
time of the mistake to suggest it.

Measured on a real account: two tokens named `mcp-remote-bridge`, and the stored secret paired
with the second. The first returned `403`, the second `200`. The lookup had a 50% chance of being
right and no way to know.

The fix is not better matching, it is refusing: ambiguity has no correct answer, so the caller is
told and the candidates are listed.

**When to apply**: whenever you look something up by a human-chosen name — a token, a tag, a
container, a DNS record, a branch. Ask whether the system *enforces* uniqueness or merely makes
duplicates unlikely. If it does not enforce it, a lookup returning one match is a guess. Handle
`len(matches) > 1` explicitly, and prefer erroring to picking.

Corollary on symptoms: the mismatch surfaced as an authentication failure at a completely
different time and place from the mistake. Errors that travel like that are worth spending a
guard on at the point they are created.

## Wait for the state you depend on, not for a symptom of it

**Why**: a drift-repair test booted a launchd job out, waited for the entry to become
**unhealthy**, then repaired it. It failed about one run in ten, and two plausible fixes missed:
verifying the bootstrap's effect (right in itself, not the cause) and shortening the restart
throttle (wrong hypothesis, measured unchanged at 3/15).

Instrumenting showed the repair *working* — loaded, running, with a pid — and then **vanishing
two seconds later**. Health falls before registration does: the port closes while the job is
still registered. Repairing in that window produced a service the tail of the original bootout
then removed. Waiting for the label to actually be gone: 0 failures in 20.

The two states looked interchangeable and were not. "Unhealthy" was a *symptom* of the unload;
"not registered" was the *state* the next step depended on.

**When to apply**: whenever a test or a reconciler waits before acting on shared state. Name the
precise condition the next step requires, and wait for that — not for the first observable
consequence of it. They usually differ by a window just wide enough to be intermittent.

Corollary on method: two hypotheses were tried before instrumenting, and both were plausible
enough to feel like progress. For an intermittent failure, print the state over time on the
failing run first. Guessing costs more than measuring, and a fix that reduces a flake rate
without explaining it has not been verified — it has been perturbed.

## Pair every "must fail" fixture with a "must succeed" control on the same dependency

**Why**: `TestProbeHostnameResolvesFailsForANameThatDoesNot` looks airtight — the `.invalid` TLD
is reserved by RFC 2606 so it can never resolve. But the assertion passes just as happily when
the **resolver itself is unreachable**. On a machine with no DNS, the probe would look correct
while being unable to resolve anything at all, and the test would say nothing about it.

The same shape had already appeared twice in this project: `plutil -lint` proving a plist parses
but not that launchd accepts it, and a `GET /mcp` returning `406` whether or not the hostname was
guarded. A negative assertion is only as strong as the assumption that the dependency was
*working and said no* — rather than *absent*.

The fix is cheap: in the same run, resolve names that must succeed (`example.com`,
`one.one.one.one`) and check the failure unwraps to the specific error you expect —
`*net.DNSError` with `IsNotFound=true`, not just "some error".

**When to apply**: any test asserting that something fails — a name that must not resolve, a port
that must be closed, a request that must be refused, a permission that must be denied. Ask what
else produces that same failure, and add a positive control that rules it out. Prefer asserting
on the *specific* error over "an error occurred".

## Distinguish a flaky test from a regression before chasing it

**Why**: `TestEnsureExposedRepairsDrift` fails roughly one run in ten, independently of any diff —
measured 1/20 at a clean HEAD and 1/8 with an unrelated change. It boots out a real launchd job
and races its re-registration; a failing run takes ~30s against ~2.5s for a green one.

Left unmeasured, it costs one debugging session per person who meets it, each concluding their own
change broke it.

**When to apply**: before investigating any test failure in a suite that touches real OS state,
re-run it at a clean tree (`git stash` then `-count=10`). Record the observed rate somewhere the
next person reads. A flake with a measured rate is an annoyance; an unmeasured one is a trap that
misattributes itself to whatever diff is in flight.

## Before deleting a resource, find out what else points at it

**Why**: recreating a Cloudflare MCP Portal server (the only way out of the authentication
deadlock above) silently removed it from the Portal's **server selection**. Everything else came
back: the server was recreated, the auto-generated Access application was recreated, the
dashboard showed `status: ready`. Only a client calling a tool revealed it — `Tool Server not
found`.

The recovery hunt then went four steps too far. `portal_list_servers` showed the server was not
merely disabled but *absent from the selection*; `portal_toggle_single_server` answered
`Server not found`; the documented re-selection flow led to a Cloudflare Access login the account
had no policy for; and the next suggestion on that path was to add an identity policy to the
Portal — widening access to a production application in order to tick a box.

The actual fix was to tick the box, in the dashboard, as the account owner.

**When to apply**: before deleting anything that another system references, enumerate what points
at it and how each pointer is restored — automatically, or by hand. Recreating a resource
restores the resource, not the references to it.

And when a recovery path starts requiring *new permissions, new policies, or wider access*, stop.
That is the signal that you have left the intended path. The intended one is usually shorter and
duller; here it was one checkbox on a screen nobody had opened.

Corollary: `ready` describes the resource, not the service. A component can be healthy and
unreferenced at the same time, and only an end-to-end call tells them apart — the same reason
this project probes rather than reading configuration.

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
