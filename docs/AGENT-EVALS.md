<!-- generated-by: groundrules v1.10.0 -->
# Agent evals — mcp-remote-bridge

> A log of the **agent's own** observed failure modes on this project — recurring mistakes,
> hallucinations, drifts — and the guard added for each. Reverse-chronological (newest at
> the top). This is **meta**: it's about how the agent behaves *here*, not about the
> project's domain.

**How this differs from `docs/LEARNINGS.md`**: LEARNINGS captures rules about the *project*
(domain gotchas, stack pitfalls, conventions). AGENT-EVALS captures patterns about the
*agent* (what it gets wrong on this repo, and the rule/guard that should stop it). An eval
entry usually produces a fix in `CLAUDE.md` or `.claude/rules/` — link it.

**When to add an entry**: when the agent repeats a mistake, fabricates a fact/API, drifts
from an instruction, or you catch a hallucination. Capture it at the next checkpoint
(see `CLAUDE.md` → "Capture at checkpoints" — typically before a push/release).

---

## 2026-08-27 — The agent verified with an instrument that could not see the fault

**Observed**: three times in one release session, a check was run that could not have detected
the thing it was supposed to detect.

1. `make notarize` gated the release on `spctl --assess`, which judges **app bundles** and
   answers `rejected (the code is valid but does not seem to be an app)` for a bare CLI whether
   or not it is notarised. Written from memory, never exercised — it would have failed every
   release.
2. A GoReleaser fix was declared verified on a green `--snapshot` run. GoReleaser **skips the
   entire `signs` pipe in snapshot mode**, so the broken code path never executed. The green run
   was not evidence of anything.
3. `scripts/notarize.sh` read the **exit code** of `notarytool submit --wait` and never the
   `status:` it printed. A submission Apple *rejected* would have passed silently, and
   unnotarised archives would have shipped as notarised.

**Pattern**: the agent reaches for a plausible-looking instrument and then treats "the
instrument returned green" as "the property holds". The failure is not a wrong hypothesis — it
is never asking *what would this command print if the thing I fear were true?* All three were
found by accident (a failed publish, a second run), not by the check.

**Related, same session**: an architectural claim — "the notarisation check cannot resolve a
foreign architecture" — was written into the `Makefile` and `RELEASE.md` on two data points that
differed in several ways at once. It was wrong; both architectures verified later, unchanged. A
false explanation reached published documentation because two samples were mistaken for a
controlled comparison.

**Guard**: the project already had the rule — *verify the effect, never trust the write*, and
*a check that cannot fail is worse than none*. What it lacked was the same rule applied to the
**checking tool itself**. Before relying on a new check, run it against a case that must fail: a
mutation, a negative control, a deliberately broken input. Every check written this session that
was mutation-tested (the doctor coverage guard, the secret-path test, the DNS bound, the
non-darwin guard) was correct; every one that was not, was wrong.

**Rationalization table** — the excuses that appeared under time pressure, and the rebuttal:

| The excuse | Why it does not hold |
|---|---|
| "It ran green, so it works." | Green from an instrument you have not falsified is green from an instrument that may be blind. |
| "The negative case is expensive to produce." | The doctor guard cost one throwaway check; the release cost three runs of thirty minutes. |
| "It's just a verification step, not product code." | A wrong check is worse than no check: it converts an unknown into a false certainty. |
| "Two runs disagreed, so the difference must be X." | Two samples differing in several ways at once identify nothing. Vary one thing, or wait and re-measure. |

**Red-flag stop-line**: *about to write a check, or an explanation of a failure, that has never
been observed failing* → stop, construct the failing case first.

**Status**: watching — at the next release, check that every new gate has a mutation or negative
control alongside it.

## 2026-08-20 — A headless loop agent added an AI attribution trailer

**Observed**: the second commit of the first `loop/run-loop.sh` run carried
`Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`. The ten commits before it had none,
and the loop's *own* first commit had none either — so the drift was inconsistent even with
itself. Amended out before any push (`c54f89d`, tree unchanged).

**Trigger**: an autonomous loop iteration (`claude -p`, fresh agent, no carried context). It
followed its harness's default attribution guidance because the repo gave it nothing to
follow: the no-attribution convention was applied by `/groundrules:bootstrap` and by the
interactive session out of consistency, but was written **nowhere** — not in `CLAUDE.md`, and
`.groundrules.json` carried `policies.noAiAttribution: false`.

**Root cause — the generalisable one**: *an unwritten convention is not a convention.* It
survives only as long as the agent carrying it has the context, which is exactly what a
fresh, headless, or subagent run lacks. Any convention that matters must be written where a
context-free agent will read it. Expect this class of drift on **every** loop run, for every
habit this session holds implicitly.

**Guard added**: the rule is stated in the global `~/.claude/CLAUDE.md` (Identity / contact),
explicitly overriding harness defaults and explicitly naming headless runs, subagents and
loops. `.groundrules.json` now carries `policies.noAiAttribution: true` so the policy is
readable from inside the repo.

**Known hole**: the global file does not travel with `git clone`. A contributor's agent will
not inherit the rule. If the convention must hold for contributors, it needs a line in the
project's `CLAUDE.md` → `### Commits` too — deliberately not added yet.

**Status**: watching — re-check the trailers after the next loop run
(`git log --format='%(trailers:key=Co-Authored-By)'`).

<!-- Example:

## YYYY-MM-DD — Invents config keys that don't exist

**Observed**: proposed `app.config.ts` keys (`retryBudget`, `edgeRegion`) that aren't in the
schema — twice in one session.
**Trigger**: asked to "tune performance config" without being pointed at the schema file.
**Guard added**: `CLAUDE.md` now says "never propose a config key without first reading
`src/config/schema.ts`; if unsure, say so." (or a `.claude/rules/config.md` with `paths:`)
**Status**: watching — re-evaluate after a few sessions.

-->

<!-- Hardening a *stubborn* guard (one that keeps getting rationalized away): add a
**rationalization table** (the excuse the agent makes under pressure → the rebuttal) and a
**red-flag stop-line**. Example:

| Rationalization | Reality |
|---|---|
| "the schema probably has that key" | you haven't read it — read it |
| "it's obviously a standard key" | obvious-but-unverified = invented |

**Red flag — STOP**: proposing any config key without having read the schema *this turn*.

-->
