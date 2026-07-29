---
name: higurashi-refine
description: Clarify one ambiguous work item into a confirmed, durable contract before planning.
---

# Higurashi refinement

Refine one work item only when product ambiguity prevents safe planning. The
result is a confirmed contract, not an implementation plan.

Do not alter control documents, generated ownership markers, or existing tests to bypass required behavior; report a blocker instead.

## Entry

1. Accept one opaque work-item ID and, optionally, one explicitly supplied
   project-relative requirement path or pasted requirement.
2. Run `higurashi inspect <ID> --json`.
3. If inspection is not `kind: ready`, a runner coordinator may import only the
   exact requirement the user explicitly supplied. File input uses
   `higurashi requirements import <ID> --from <PATH> --json`. Pasted input must
   be displayed verbatim and explicitly confirmed as authoritative, then passed
   without rewriting to `higurashi requirements import <ID> --stdin --json`.
   Never discover, infer, paraphrase, replace, or silently select a requirement.
   Stop on a conflict.
4. Re-inspect after import and continue only for `kind: ready`. Refuse to
   overwrite or amend an existing artifact.
5. Use the exact requirement source and artifact path returned by inspection.
6. Read only the requirement, project instructions, and authoritative product
   documents needed to expose unresolved behavior. Never modify the requirement source or any other existing document.

## Question rounds

Return one batch of concise, free-text product questions. Every question must:

- identify the observable decision that blocks planning;
- explain why the answer changes externally visible behavior or acceptance;
- include one recommended answer with a short rationale;
- avoid asking for implementation details that PLAN can decide safely.

Accept a free-text response. Ask at most one follow-up batch, and only for
ambiguity introduced or left unresolved by that response. Do not turn a
recommendation into a decision without the user's answer.

If a required authority decision remains unavailable, stop without writing an
artifact. Report the unresolved decision and the exact artifact path that
remains absent.

## Confirmation

Draft a concise contract containing:

- observable behavior and acceptance evidence;
- included and excluded scope;
- invariants and compatibility constraints;
- confirmed decisions;
- explicit non-goals.

Show the complete draft and require explicit confirmation in the conversation.
Silence, an unrelated reply, or acceptance of an individual recommended answer
is not confirmation of the complete contract.

Persist only the confirmed contract. Do not persist raw questions, raw answers,
reasoning transcripts, speculative alternatives, or unconfirmed policy.

## Persisted artifact

After explicit confirmation, re-run inspection and require `kind: ready`.
Create only the exact returned artifact path with this structure:

```markdown
# WORK-123 — Confirmed short title

Higurashi-Schema: 1
Status: refined
Repair-Round: 0

## Contract

Confirmed observable behavior and acceptance evidence.

## Scope

Included scope, excluded scope, and non-goals.

## Invariants

Safety, compatibility, ownership, and policy constraints.

## Decisions

Confirmed product decisions only.

## Ordered vertical TDD checklist

## Verification

Not started.

## Evidence

Refinement confirmed by the user.

## Warnings and handoff

Ready for PLAN.
```

The refined artifact has no checklist tasks. Do not change machine-owned fields
after creation and do not transition it to `planned`; PLAN owns the
`refined → planned` transition after adding bounded vertical tasks.

Inspect once more and return the artifact path, `Status: refined`, exact hash,
and the runner-native delivery command that continues with PLAN. Do not
implement, plan tasks, delegate implementation work, commit, or push.
