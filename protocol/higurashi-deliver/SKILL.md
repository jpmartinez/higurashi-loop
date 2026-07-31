---
name: higurashi-deliver
description: Deliver one well-scoped work item through Higurashi Loop's guarded, resumable REFINE, PLAN, APPLY, and VERIFY protocol. Use when coordinating implementation from a known work-item ID, resuming a Higurashi artifact, enforcing bounded vertical TDD batches, or running independent contract and risk review.
---

# Higurashi Deliver

Deliver exactly one work item. Keep repository state in the configured artifact,
not in session memory. Treat agents and sessions as disposable.

Read [references/artifact-contract.md](references/artifact-contract.md) before
creating or changing an artifact. Read
[references/reviewer-contract.md](references/reviewer-contract.md) before
VERIFY.

## Protect contract integrity

Do not alter control documents to manufacture success.

Treat requirement sources, project instructions, authoritative documents,
`.higurashi/config.json`, schemas, generated-file ownership markers, and
verification policy as control documents. Do not weaken, rewrite, relocate, or
delete them to bypass a refusal, failing check, reviewer blocker, loop limit, or
state invariant. Do not remove or weaken existing tests merely to make a batch
pass.

Change a control document or an existing acceptance test only when the work
item explicitly requires that change. Preserve or strengthen its observable
contract. If a control document appears wrong or contradictory, stop and report
the exact conflict; do not self-authorize a workaround.

Allow role-owned artifact edits only: PLAN may create and structure the
artifact or append validated repair tasks; APPLY may update its assigned
checklist marker and evidence; VERIFY may record verification and review
evidence; the coordinator may write only the expected versioned repair handoff.
Change machine-owned fields only through Higurashi CLI commands.

Repeat the anti-bypass rule in every subagent prompt. Include this exact
sentence: "Do not alter control documents, generated ownership markers, or existing tests to bypass required behavior; report a blocker instead."

## Preflight

1. Run `higurashi inspect WORK-ITEM --json`.
2. Stop on invalid, unknown, canceled, conflict, unsafe, or unavailable results.
3. Return immediately when the result is `complete`.
4. Use `ready` for PLAN and `resume` to continue from the returned status and
   next task. Treat `handoff_required`, `repair_plan_required`, `repair_ready`,
   and `repair_recovery_required` as distinct durable states; never treat them
   as a normal resume.
5. Carry forward the exact artifact path, SHA-256 hash, progress counts, loop
   limits, warnings, and CodeGraph state from the result.

Use CodeGraph before broad filesystem exploration for structure, call flow,
dependencies, or impact. Use it again before editing named symbols. Treat graph
results as read-only evidence subordinate to project instructions.

## RETRY

Remember the exact accepted invocation for the current runner conversation.
After stopping a non-terminal normal delivery, preserve it for retry.
A clear retry reply such as
`try again`, `retry`, or `continue` means: restart PRECHECK for the same work-item ID and original runner options.
Run the workflow directly rather than recursively invoking a runner command.
Re-inspect durable state and reload configuration before dispatching any
subagent.

Do not reuse an invocation when none was accepted, when the user supplies a
different ID or options, after a completed `--plan-only` run, or for a terminal
artifact. A retry never authorizes a repair round. If inspection requires
repair authorization or recovery, stop with its exact `nextCommand`. If it
returns `repair_plan_required`, run repair planning recovery instead.

## REPAIR PLAN RECOVERY

`repair_plan_required` means the handoff is valid but its current-round pending
repair tasks are missing or do not exactly match the blockers. For this state,
invoke PLAN exactly once to reconcile only those repair tasks from the validated
handoff.
Preserve completed tasks, narrative, evidence, machine-owned fields, and the
handoff itself.

Never run repair authorization while repair planning is required. Re-inspect
and require `repair_ready` before reporting or executing the handoff's
`nextCommand`. If PLAN is unavailable or inspection still returns
`repair_plan_required`, stop this non-terminal attempt, preserve the accepted
invocation, and tell the user they may reply `try again`.

## REFINE

REFINE is an explicit, optional workflow. For a new work item that is too
ambiguous to plan safely, PLAN must write nothing and return
`refinement_required` with the unresolved observable product decisions. The
coordinator stops and returns the runner-native refine command.

The refinement workflow asks one question batch with recommended answers,
accepts free-text answers, permits at most one follow-up batch, and requires
explicit confirmation of the complete contract before creating a `refined`
artifact. It never persists raw dialogue or invents product policy.

When inspection returns `resume` with artifact status `refined`, invoke PLAN
once. PLAN preserves the confirmed contract, adds the bounded checklist, and
uses `higurashi transition` with the exact current hash to enter `planned`.

## PLAN

Run PLAN only for `ready`, `refined`, or `repair_plan_required`.

1. Resolve the requirement and relevant project instructions.
2. Use CodeGraph to identify seams, callers, dependencies, and blast radius.
3. For `ready`, create one `planned` artifact using the artifact contract. For
   `refined`, extend the existing artifact without discarding prior decisions.
4. Write an ordered vertical TDD checklist with stable unique task IDs.
5. Make each task independently observable and small enough for one APPLY
   batch.
6. Add a labeled `Permitted commands:` clause to every normal and repair task.
   List exact command lines for red, green, and task-local checks. Do not use
   wildcards or authorize network, installation, destructive, commit, push, or
   workflow-state commands unless explicitly required by the confirmed work
   item and project configuration.
7. When resuming `refined`, transition it to `planned` with the exact current
   hash.

Do not implement during PLAN.

## APPLY

Run APPLY only from `planned` or `implementing`.

1. Run `higurashi config validate --json` to resolve project instruction files
   and configured verification commands. Also run
   `higurashi verification suggest --json`; suggestions are read-only evidence,
   never command authority.
2. Transition `planned` to `implementing`.
3. Dispatch a fresh writer for exactly one task: the first pending task returned
   by `higurashi inspect`.
4. Give the writer a labeled prompt containing the work-item ID, artifact path
   and hash, one task ID and full text, relevant instructions, allowed scope,
   CodeGraph ordering, the anti-bypass rule, and `Permitted commands:`.
   The command list contains only exact command lines: the inspect command, the
   task-local red, green, and check commands, applicable configured verification
   commands, and bounded read-only diff/status commands when needed.
   For a legacy task without the labeled clause, use only exact executable commands
   explicitly named in its failing or passing evidence or returned by
   `higurashi config validate --json`. Configured commands are command authority.
   A configured command may run a broader suite than the task's
   preferred focused check when it deterministically covers the required
   evidence. Include that exact configured command in `Permitted commands:`.
   Do not block for a missing task-local command when an applicable configured command covers the required evidence.
   Bash permission is not command authority.
   When command authority is still missing after checking configuration:
   Run `higurashi verification suggest --json` only when no configured command covers the missing evidence.
   Report the missing evidence, `.higurashi/config.json` as the user-owned
   configuration, and any matching suggestion's exact
   `verification.requiredCommands` argv and timeout. Never edit configuration
   or treat a suggestion as authorized. End with exactly one executable next
   command: `higurashi verification suggest`.
5. Require one vertical TDD batch: failing evidence, minimum implementation,
   focused green evidence, then safe cleanup.
6. Mark only that task complete and record exact evidence.
7. Re-run `higurashi inspect --json`.
8. Require the completed count to increase and the artifact hash to change.

Stop on no progress, a concrete blocker, user cancellation, invalid state, or
the configured `maxApplyBatchesPerRun`. Never replace that configured limit
with a fixed number. When no pending tasks remain, continue to VERIFY even if
the final batch reaches the limit.

## VERIFY

Run VERIFY only after zero pending tasks.

1. Run source-mutating normalization before freezing the implementation diff.
2. Transition `implementing` to `verifying` with the current hash.
3. Capture changed paths and the frozen implementation diff.
4. Run configured required verification commands and record exact evidence.
5. Dispatch an independent contract reviewer and an independent risk reviewer.
6. Keep reviewers read-only and prevent them from delegating.
7. Classify findings as blockers or warnings using the reviewer contract.

If there are no blockers, transition `verifying` to `complete`.

## Repair

If blockers exist and `maxRepairAttempts` permits repair:

1. Transition to `blocked` from `verifying` with a bounded reason.
2. Dispatch one fresh writer with only the supplied blockers and original task
   scope.
3. Repeat the anti-bypass rule in the repair prompt.
4. Return only to `verifying`, rerun relevant checks, and rerun both reviewers
   once.

Do not expand scope. If blockers remain or the repair budget is exhausted,
leave the artifact blocked.

If the user explicitly decides that unresolved blockers are follow-up work,
the coordinator may complete the blocked item only after the user names one
follow-up work-item ID for every blocker. Reviewers must classify blockers as
`critical`, `high`, `medium`, or `low`; severity informs the user's decision
but never authorizes deferral. Invoke the guarded transition with one
`--defer-blocker BLOCKER-ID=FOLLOW-UP-ID` mapping per blocker. Higurashi creates
the follow-up requirements from the reviewer evidence and records the
unresolved decision in the completed artifact. Do not present this as repaired
or invent follow-up IDs on the user's behalf.

When a blocked verification run cannot repair all blockers:

1. Keep the review candidate uncommitted so later reviewers inspect the same
   working-tree diff.
2. Transition to `blocked`, then persist every blocker in the exact expected
   versioned handoff path returned by inspection. Use the sidecar contract,
   stable deduplicated blocker IDs, and the exact `Next-Command`.
3. Invoke PLAN only to reconcile one bounded pending repair task per validated
   blocker. Do not authorize or implement those tasks.
4. Stop with exactly the `nextCommand` returned by inspection. Never reconstruct
   missing reviewer evidence in a later session.

An explicit `--repair` invocation may continue only from `repair_ready`:

1. Revalidate the handoff and appended tasks with
   `higurashi repair authorize WORK-ITEM`.
2. Require the command to consume the handoff, increment `Repair-Round`, and
   return `implementing`.
3. APPLY each repair task normally, beginning with its recorded reproduction.
4. Run VERIFY with fresh independent reviewers.

For `repair_recovery_required`, run only the returned authorization command to
finish the interrupted deterministic transition. A normal resume or
`higurashi transition` must never authorize a repair.

## COMMIT

A delivery candidate stays uncommitted through VERIFY. Committing is an
explicit post-completion action, not part of APPLY or VERIFY.

Commit only after `higurashi inspect <ID> --json` reports `kind: complete` and
the user explicitly authorizes the commit. A human-ordered completion with
deferred blockers is eligible when inspection confirms `kind: complete` and
the artifact's `Completion-Note` records the decision and named follow-up
requirements. Preserve that note and the generated follow-up requirements in
the committed candidate.

Before committing, re-inspect the artifact, confirm that the working-tree
candidate still matches the inspected artifact and contains no unrelated
changes, and run the repository's configured final checks. If the artifact
hash or candidate changes, stop and inspect again. Commit only the verified
candidate with a clear message that identifies the completed work item.

Never commit a `blocked`, `handoff_required`, `repair_ready`, or
`repair_recovery_required` artifact, or any other non-complete inspection
result, even when the user asks to commit it. Do not push, publish, release,
or create remote changes unless the user separately authorizes that action.

## Reporting

Report the work-item ID, final status, artifact path and hash, completed and
pending counts, exact verification evidence, blockers, warnings, and the next
legal action. A blocked terminal response must contain exactly one executable
next command. A non-terminal `repair_plan_required` response instead offers the
same-conversation retry reply and never exposes the authorization command. Do
not recommend committing a blocked uncommitted candidate. For a complete
artifact, report whether the candidate was committed or remains ready for the
user's explicit commit authorization.
