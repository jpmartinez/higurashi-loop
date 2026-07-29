# Reviewer contract

Run two independent read-only reviews after verification commands pass.

## Shared rules

- Review the frozen implementation diff and exact verification evidence.
- Use CodeGraph before broad structural exploration when impact evidence is
  needed.
- Do not edit files, run mutating commands, transition status, or delegate.
- Do not alter control documents to manufacture success.
- Repeat the anti-bypass rule in every subagent prompt.
- Treat a proposed change to requirements, instructions, configuration,
  schemas, verification policy, generated ownership markers, or existing tests
  as a blocker when its purpose is to evade required behavior.
- Give each blocker a stable deduplicated ID.
- Report its originating reviewer, violated contract or invariant, exact
  evidence location, deterministic reproduction or red test command, and
  minimum acceptance condition.
- Separate warnings from blockers. Warnings alone do not authorize repair.

## Contract reviewer

Check:

- work-item acceptance behavior;
- artifact contract and scope;
- each checklist claim against the implementation and evidence;
- missing, weakened, or contradicted requirements;
- compatibility promises and public interface changes;
- whether tests demonstrate behavior instead of encoding an implementation
  accident.

Do not request unrelated improvements.

## Risk reviewer

Check:

- project-defined invariants and failure paths;
- concurrency, stale state, idempotency, and rollback;
- privacy, security, path, command, and ownership boundaries;
- partial failure and interrupted operation behavior;
- operational safety and bounded loops;
- whether any agent changed a document, generated marker, verification rule, or
  test to bypass a refusal or make evidence appear green.

## Output

Return:

```text
blockers:
- id:
  originating-reviewer:
  violated-contract:
  evidence-location:
  reproduction:
  minimum-acceptance:

warnings:
- location:
  evidence:
```

Return empty lists when clean. Do not rewrite the artifact or implementation.
