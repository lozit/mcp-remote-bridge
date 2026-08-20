<!-- generated-by: groundrules v1.10.0 -->
# Process — mcp-remote-bridge

> The **working method contract** for this project: how we work together, phase by phase,
> with explicit validation gates. This is neither the intent (`intake/`) nor the vision
> (`docs/VISION.md`) — it's the *how we proceed*. Claude must follow it and **never skip a
> phase**.

> ⚠️ **This file was seeded from what the repo already demonstrates, not from a stated
> preference.** The phase shape below is inferred from the commit history (two full specs
> written and committed before a single line of Go). The `<fill in>` markers are the parts
> only you can answer. Correct it — an inferred process contract that nobody agreed to is
> worse than none.

## Phases

The repo's own history sets the pattern: **specify, then build against the spec.**

1. **Spec** — the design is written and committed as a document before code exists.
   `docs/SPEC-primitive.md` fixed the atomic operation and the three seams;
   `docs/SPEC-config-cli.md` fixed the layer above it. A spec names its **resolved
   decisions** and its **deferred** scope explicitly, so the boundary is a written artifact
   rather than a memory.
   **GATE**: `<fill in — who validates a spec, and against what?>`

2. **Build against the spec** — implement one milestone from `docs/ROADMAP.md`. The spec is
   the acceptance criteria; if the code wants to differ, the spec changes first (or an ADR
   records why it diverged).
   **GATE**: `<fill in>`

3. **Verify the effect** — the project's own rule, applied to its own delivery: a milestone
   is done when it has been *probed*, not when it compiles and the mocks pass. See the
   end-to-end item in `RELEASE.md`'s checklist.
   **GATE**: a real end-to-end run on a clean machine state. Non-negotiable — this is the
   one gate the project's thesis makes mandatory.

4. **Capture, then ship** — before a push or a tag: decisions → ADR, learnings → 
   `docs/LEARNINGS.md`, agent drift → `docs/AGENT-EVALS.md`. Then release per `RELEASE.md`.
   **GATE**: `<fill in>`

## Validation gates

- A phase is **done** only when explicitly validated by `<fill in — you? a test suite? a
  probe?>`.
- Intermediate deliverables are marked `[~] in review` in `PLAN.md` until validated.
- **The one gate that is already decided**: nothing counts as working on the strength of
  mocks alone. Every seam is mocked in unit tests, and everything that actually breaks
  (launchd semantics, DNS propagation, tunnel auth) lives on the other side of the mock.

## Working style

- **Interviews**: grouped questions (3–4 at a time, with options) rather than one long
  questionnaire.
- **Spec before code** on anything structural — the two existing specs are the reference
  for the level of detail expected (a contract, the load-bearing rules, the failure modes
  handled explicitly, and a named deferred list).
- **Name the deferred scope in writing.** Both specs end with a "Deferred (wanted, out of
  the MVP)" section. This is what keeps "not a gateway" enforceable under pressure.
- `<other preferences: plan mode by default? push-back threshold? when to open an ADR vs.
  just decide?>`

## Where artifacts live

- **Specs** — `docs/SPEC-*.md`, one per layer. Normative.
- **Per-phase working artifacts** — `<fill in, if you want a convention; otherwise
  alongside the code>`
- **Synthesized, stable outcomes** migrate to `docs/` (`VISION.md`, `ARCHITECTURE.md`,
  `GLOSSARY.md`, `SECURITY.md`).
- **Raw upstream** stays in `intake/` and is never edited to "fix" it.
