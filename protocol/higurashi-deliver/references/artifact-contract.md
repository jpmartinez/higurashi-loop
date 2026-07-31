# Artifact contract

Use one Markdown artifact at the path returned by `higurashi inspect`.

## Required structure

```markdown
# WORK-123 — Short title

Higurashi-Schema: 1
Status: implementing
Repair-Round: 0

## Contract

Observable behavior and acceptance evidence.

## Scope

Included and excluded work.

## Invariants

Safety, compatibility, and ownership constraints.

## Decisions

Decisions made and unresolved choices.

## Ordered vertical TDD checklist

- [x] **task-001 — First behavior:** focused evidence.
- [ ] **task-002 — Next behavior:** full task text.
  Preserve multiline continuation as part of this task.

## Verification

Commands and results.

## Evidence

Focused test output and review evidence.

## Warnings and handoff

Warnings, blockers, and the next legal action.
```

Use unique stable task IDs. Preserve task ordering and full multiline task text.
Count progress only from checklist markers.

## Machine-owned fields

Change `Higurashi-Schema`, `Status`, `Repair-Round`, `Blocked-From`,
`Blocker-Reason`, and `Completion-Note` only through Higurashi CLI operations.
Existing artifacts
without `Repair-Round` are interpreted as round zero and gain the field on the
next mutation. Never edit these fields to bypass a legal transition,
pending-task check, stale hash, or blocker.

Use these legal transitions:

```text
refined      → planned | blocked
planned      → implementing | blocked
implementing → verifying | blocked
verifying    → complete | blocked
blocked      → implementing only through `higurashi repair authorize`
blocked      → complete only through explicit follow-up deferral
complete     → complete
```

Require at least one task for `planned` and `implementing`. Require zero pending
tasks for `verifying` and normal `complete`. A human-ordered completion may
retain pending repair tasks only when `Completion-Note` records the explicit
follow-up disposition. Require a nonterminal origin and bounded reason for
`blocked`.

Before every transition, obtain the exact current SHA-256 hash from:

```text
higurashi inspect WORK-123 --json
```

Then pass it to:

```text
higurashi transition WORK-123 TARGET --expected-hash HASH --json
```

Use `--reason TEXT` only when entering `blocked`. Stop on `stale_hash`; inspect
again instead of overwriting concurrent work.

Reviewers classify every handoff blocker with one severity: `critical`, `high`,
`medium`, or `low`. A user may explicitly defer all blockers to named
follow-up requirements and complete the blocked item with:

```text
higurashi transition WORK-123 complete \
  --expected-hash HASH \
  --defer-blocker B-001=FOLLOW-123 [--defer-blocker B-002=FOLLOW-124 ...]
```

The command requires exactly one mapping for every blocker, creates each
follow-up requirement in the managed requirement directory, preserves the
review evidence and minimum acceptance condition, and records the unresolved
decision in `Completion-Note`. It does not mark the blocker repaired.

## Durable repair handoff

For current `Repair-Round: N`, the expected sidecar is:

```text
<artifact-directory>/<work-item-id>-repair-<N+1>.md
```

Use this exact strict structure:

```markdown
# Repair handoff for WORK-123

Higurashi-Repair-Handoff: 1
Work-Item: WORK-123
Handoff-Status: ready
Repair-Round: 1
Next-Command: higurashi repair authorize WORK-123
Candidate-Strategy: uncommitted

## Blocker B-001
Severity: high
Originating-Reviewer: contract
Violated-Contract: exact violated contract or invariant
Evidence-Location: path/to/file.go:42
Reproduction: go test ./path -run TestName
Minimum-Acceptance: exact minimum condition that removes this blocker
```

Every field is required. Values must be concrete, not placeholders. Blocker IDs
must be stable and unique. Preserve reviewer evidence exactly; never infer it
later. The initial strategy is always `uncommitted`, so all verification rounds
inspect the complete working-tree diff.

Before authorization, PLAN appends exactly one pending task per blocker:

```markdown
- [ ] **repair-r1-B-001 — Bounded repair title:** scoped work.
  Blocker-ID: B-001
  Reproduction: go test ./path -run TestName
  Minimum-Acceptance: exact minimum condition that removes this blocker
```

The reproduction and minimum acceptance text must exactly match the handoff.
Do not edit completed tasks or earlier narrative/evidence. The planner cannot
consume the handoff. Only this deterministic command may consume it and
increment the round:

```text
higurashi repair authorize WORK-123
```

The command writes the consumed handoff before the artifact transition. If
interrupted between files, repeat the same command; it completes the recorded
intent without skipping or consuming a round twice.

## Ownership and anti-bypass

Do not alter control documents to manufacture success. PLAN owns initial
artifact narrative and checklist structure, plus append-only repair tasks
derived from a validated handoff. APPLY owns only its assigned task marker and
task-local evidence. VERIFY owns verification and review evidence. The
coordinator owns creation of the expected handoff only. Reviewers remain
read-only. APPLY cannot change status, reviewer receipts, repair round, or
handoffs and cannot invoke repair authorization. No role may weaken requirement
sources, instructions, configuration, schemas, verification policy, generated
ownership markers, or existing tests to silence a failure.

These role permissions are workflow guardrails, not a security boundary against
a malicious process with unrestricted filesystem access.

Repeat the anti-bypass rule in every subagent prompt. If required behavior
cannot be met without changing a protected contract outside explicit scope,
record a blocker and stop.
