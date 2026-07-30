# Higurashi Loop — implementation plan

Status: implementation in progress
Plan version: 1
Date: 2026-07-29

Implementation progress:

- Phase 0 repository scaffold: complete.
- Phase 1 strict configuration: complete.
- Guarded project initialization and runner selection: complete.
- Phase 2 project and CodeGraph doctor: complete.
- Phase 3 artifact inspection: complete.
- Phase 4 guarded transitions: complete.
- Phase 5 canonical skill and rendering: complete.
- Phase 6 OpenCode adapter parity: complete.
- Phase 7 Claude Code adapter: complete; live MCP release smoke remains an
  explicit compatibility gate.
- Cross-cutting durable repair handoff and explicit authorization protocol:
  complete.
- Cross-cutting explicit optional refinement workflow: complete.
- Next: Phase 8 end-to-end fixture.

## 1. Outcome

Build a reusable, runner-neutral development workflow that delivers one
well-scoped work item through:

```text
REFINE (optional) → PLAN → APPLY → VERIFY
```

The first release must preserve the behavior already proven by the working
prototype:

- durable repository state instead of session-dependent state;
- resumable work without replanning;
- one compact delivery artifact;
- vertical TDD implementation batches;
- bounded loops with measurable progress;
- independent contract and risk review;
- one bounded repair attempt;
- fail-closed state transitions;
- no commits, pushes, or pull requests unless separately requested.

The workflow must run through both OpenCode and Claude Code without making
either runner the source of truth. A future Codex adapter must fit without
changing the core state machine or artifact format.

## 2. Fixed decisions

These decisions are settled for the initial implementation and should not be
reopened unless implementation evidence proves them unworkable.

1. **Project name:** Higurashi Loop.
2. **CLI binary:** `higurashi`.
3. **Deterministic implementation language:** Go.
4. **Workflow language:** Markdown skills and runner-native agent definitions.
5. **Configuration format:** strict JSON.
6. **Repository configuration directory:** `.higurashi/`.
7. **Default artifact directory:** `docs/higurashi/`.
8. **Canonical explicit workflow command:** `higurashi-deliver`.
9. **First runner adapters:** OpenCode and Claude Code.
10. **Code intelligence:** CodeGraph is required for optimal mode.
11. **Model execution:** delegated to the runner; the Go CLI never calls model
    providers directly.
12. **Memory:** no memory dependency in the first release.
13. **Runner-session retry:** after a correctable non-terminal stop, a clear
    retry reply re-enters PRECHECK for the same accepted invocation; it never
    authorizes a repair round or bypasses durable inspection.
14. **Skill generation:** no dynamic generation in the first release.
15. **Migration strategy:** reproduce proven behavior through generic names and
    interfaces; do not copy project-specific policy.
15. **Isolation:** the new project does not modify or install files into any
    existing consumer repository during development.
16. **Development toolchain:** project-local mise configuration pins the
    minimum supported Go release used for local development.

## 3. Scope

### 3.1 Initial release

The initial release includes:

- strict project configuration and defaults;
- work-item ID validation;
- requirement-source discovery;
- artifact parsing and progress inspection;
- legal, guarded, atomic status transitions;
- configurable APPLY and repair limits;
- CodeGraph capability diagnostics;
- one canonical runner-neutral delivery skill;
- generated OpenCode adapter files;
- generated Claude Code plugin/standalone files;
- adapter drift detection;
- fixture-based integration tests;
- cross-platform CLI builds;
- documentation for installation, upgrades, and project adoption.

### 3.2 Deferred

The following remain designed extension points, not initial implementation:

- interactive product-owner refinement;
- Engram or another durable recall adapter;
- scheduled interaction analysis;
- generated skill proposals;
- a richer skill registry;
- Codex-specific delegation files;
- remote orchestration;
- a hosted control plane;
- direct model-provider calls;
- automatic commits, pushes, or pull requests;
- graphical or terminal user interfaces;
- automatic global installation of CodeGraph or any other external tool.

## 4. Design principles

### 4.1 Deep module

The Go CLI is one deep module. Its external interface is intentionally small:

```text
higurashi init
higurashi doctor
higurashi inspect
higurashi transition
higurashi repair
higurashi adapter
higurashi config
higurashi version
```

Behind this interface, the implementation hides:

- project-root resolution;
- path safety;
- strict configuration parsing;
- defaults and schema migration;
- requirement lookup;
- artifact naming and parsing;
- checklist progress calculation;
- transition legality;
- atomic writes;
- generated adapter rendering;
- source-hash drift detection;
- CodeGraph health checks;
- structured results and stable exit codes.

If this module were deleted, every runner adapter would have to reimplement
those rules. That is the leverage the module must provide.

### 4.2 Real seams only

Two runner adapters exist from the beginning, so the runner seam is real.

Do not introduce hypothetical interfaces for:

- alternative code-intelligence providers;
- remote artifact stores;
- model providers;
- memory providers;
- schedulers.

CodeGraph is the only code-intelligence implementation in the first release.
Artifacts live in the local repository. The runner owns model execution.

### 4.3 Interface as test surface

Tests should exercise CLI commands and exported core operations through their
observable results. Internal parsers may have focused table tests, but behavior
must also be proven through temporary repository fixtures.

Tests must not depend on implementation-specific package layout.

### 4.4 Living installation documentation

`README.md` is the authoritative installation and prerequisites guide from the
first implementation phase onward. Keep it accurate in the same change whenever
implementation alters:

- prerequisites or supported versions;
- installation, upgrade, or rollback steps;
- CLI commands or flags;
- generated project files;
- runner or CodeGraph setup;
- verification commands;
- release availability.

Before a command is implemented, the README must label it as planned rather
than installable. Documented installation commands and compatibility claims
must have a corresponding build, installation, or smoke test.

### 4.5 Language split

Go owns deterministic behavior because the CLI must be portable across consumer
projects that may not have Node, Bun, or Python. Prefer the Go standard library
for the first release: `encoding/json`, `os`, `os/exec`, `path/filepath`,
`regexp`, `crypto/sha256`, `embed`, and `testing` are sufficient for the
initial module.

Markdown owns agent behavior because skills and agent definitions are the
portable interface understood by runners. JSON owns project configuration and
machine-readable results. Do not encode the delivery protocol twice in Go and
Markdown: Go enforces deterministic state/configuration invariants; the
canonical skill defines agent behavior.

## 5. Proposed repository layout

```text
higurashi-loop/
├── .github/
│   └── workflows/
│       ├── ci.yml
│       └── release.yml
├── cmd/
│   └── higurashi/
│       └── main.go
├── internal/
│   ├── artifact/
│   │   ├── artifact.go
│   │   ├── parser.go
│   │   ├── transition.go
│   │   └── *_test.go
│   ├── cli/
│   │   ├── root.go
│   │   ├── init.go
│   │   ├── doctor.go
│   │   ├── inspect.go
│   │   ├── transition.go
│   │   ├── adapter.go
│   │   ├── config.go
│   │   └── *_test.go
│   ├── codegraph/
│   │   ├── doctor.go
│   │   └── doctor_test.go
│   ├── config/
│   │   ├── config.go
│   │   ├── defaults.go
│   │   ├── validate.go
│   │   └── *_test.go
│   ├── project/
│   │   ├── root.go
│   │   ├── requirements.go
│   │   └── *_test.go
│   ├── render/
│   │   ├── render.go
│   │   ├── manifest.go
│   │   └── *_test.go
│   └── result/
│       ├── result.go
│       └── exit.go
├── protocol/
│   └── higurashi-deliver/
│       ├── SKILL.md
│       └── references/
│           ├── artifact-contract.md
│           └── reviewer-contract.md
├── adapters/
│   ├── opencode/
│   │   ├── commands/
│   │   │   └── higurashi-deliver.md.tmpl
│   │   └── agents/
│   │       ├── higurashi-orchestrator.md.tmpl
│   │       ├── higurashi-plan.md.tmpl
│   │       ├── higurashi-apply.md.tmpl
│   │       ├── higurashi-verify-contract.md.tmpl
│   │       └── higurashi-verify-risk.md.tmpl
│   └── claude-code/
│       ├── .claude-plugin/
│       │   └── plugin.json.tmpl
│       ├── skills/
│       │   └── deliver/
│       │       └── SKILL.md.tmpl
│       ├── agents/
│       │   ├── higurashi-plan.md.tmpl
│       │   ├── higurashi-apply.md.tmpl
│       │   ├── higurashi-verify-contract.md.tmpl
│       │   └── higurashi-verify-risk.md.tmpl
│       └── .mcp.json.tmpl
├── schemas/
│   ├── config.schema.json
│   ├── doctor.schema.json
│   ├── inspection.schema.json
│   └── adapter-manifest.schema.json
├── testdata/
│   ├── repositories/
│   ├── artifacts/
│   ├── configs/
│   └── golden/
├── scripts/
│   ├── check-generic-language.sh
│   └── smoke-adapters.sh
├── .gitignore
├── AGENTS.md
├── CHANGELOG.md
├── CONTRIBUTING.md
├── go.mod
├── mise.toml
├── README.md
└── IMPLEMENTATION_PLAN.md
```

Avoid adding folders until their first file is implemented.
Do not add a license file until the intended publication and ownership model is
explicitly chosen; this does not block local implementation.

## 6. Public CLI interface

### 6.1 General rules

- Human-readable output is the default.
- `--json` returns versioned machine-readable output on stdout.
- Diagnostics go to stderr.
- Commands never print secrets or full environment contents.
- Every path is resolved relative to the discovered Git project root.
- Commands reject paths that escape the project root.
- Unknown JSON configuration fields fail validation.
- Read-only commands never mutate the repository.
- Mutating commands report every changed path.

### 6.2 Commands

#### `higurashi init`

```text
higurashi init \
  --runner opencode \
  --runner claude-code \
  [--project-root PATH] \
  [--force-generated] \
  [--json]
```

Behavior:

1. Resolve and validate a Git project root.
2. Refuse to initialize a home directory or non-project directory.
3. Create `.higurashi/config.json` from conservative defaults.
4. Create the configured artifact directory if absent.
5. Render only the selected adapters.
6. Never alter unrelated default-agent settings.
7. Refuse to overwrite non-generated files.
8. Discover supported project-owned verification commands without executing
   them or granting command authority.
9. Print the next required setup, verification-review, and health-check
   commands.

`--force-generated` may replace only files carrying a valid Higurashi generated
header whose previous source hash is recognized.

#### `higurashi doctor`

```text
higurashi doctor [--runner NAME] [--json]
```

Checks:

- Git project root;
- configuration presence and schema;
- artifact directory safety;
- adapter installation and source hashes;
- selected runner executable availability;
- CodeGraph CLI availability;
- per-worktree `.codegraph/` presence;
- `codegraph status` health and root consistency;
- pending synchronization state;
- CodeGraph watcher/auto-sync state when the installed status interface exposes
  it; otherwise report `not_reported` rather than inferring health;
- Claude Code MCP visibility when the Claude adapter is selected after Phase 7;
- OpenCode effective adapter discoverability when the OpenCode adapter is
  selected after Phase 6;
- no conflicting generated files.

The command is read-only. A future explicit `doctor --fix-project-index` may
initialize a missing project index, but Higurashi must never silently install,
upgrade, uninstall, or globally reconfigure CodeGraph.

#### `higurashi config validate`

```text
higurashi config validate [--json]
```

Returns normalized effective configuration or structured validation failures.
It does not rewrite configuration.

#### `higurashi verification suggest`

```text
higurashi verification suggest [--json]
```

Reads supported project manifests and returns conservative verification-command
suggestions, their source locations, exact argv arrays, timeouts, and any
detected risk that requires review. It never executes a suggestion or rewrites
configuration. A command becomes workflow authority only after the user adds it
to the corresponding field in `.higurashi/config.json`.

#### `higurashi inspect`

```text
higurashi inspect WORK-123 [--json]
```

Possible result kinds:

```text
ready
resume
complete
handoff_required
repair_ready
repair_recovery_required
invalid_repair_state
unknown
canceled
invalid_artifact
missing_requirement_source
codegraph_unavailable
configuration_invalid
conflict
```

For resumable work, return:

- work-item ID;
- artifact path;
- artifact status;
- artifact content hash;
- completed, pending, and total task counts;
- stable next-task ID and full multiline text;
- blocked-from state when applicable;
- current repair round, expected handoff path, validation state, blocker count,
  candidate strategy, explicit-authorization flag, and exact next command;
- configured loop limits;
- CodeGraph capability state;
- warnings.

#### `higurashi transition`

```text
higurashi transition WORK-123 implementing \
  --expected-hash HASH \
  [--reason TEXT] \
  [--json]
```

Behavior:

- validate the current artifact before changing it;
- reject stale `--expected-hash`;
- enforce the legal transition graph;
- enforce checklist invariants;
- update only machine-owned header fields;
- write atomically in the artifact directory;
- preserve unrelated artifact content byte-for-byte;
- return the new status, hash, and progress.

#### `higurashi repair authorize`

```text
higurashi repair authorize WORK-123 [--json]
```

Behavior:

- require blocked state and the exact next versioned ready handoff;
- validate one newly appended bounded pending task per blocker;
- consume the handoff before updating the artifact;
- increment `Repair-Round` and enter `implementing`;
- preserve completed tasks and all non-machine artifact bytes;
- recover idempotently from interruption between the two atomic file writes;
- reject normal resume, wrong rounds, consumed reuse, and stale or invalid
  evidence.

#### `higurashi adapter install`

```text
higurashi adapter install opencode
higurashi adapter install claude-code
```

Render the adapter into the current project without changing global runner
configuration.

#### `higurashi adapter diff`

```text
higurashi adapter diff opencode
higurashi adapter diff claude-code
```

Show generated-file drift without changing files.

#### `higurashi adapter update`

```text
higurashi adapter update opencode
higurashi adapter update claude-code
```

Update recognized generated files. Refuse to replace locally edited generated
files until the user explicitly resolves or adopts those changes.

## 7. Configuration contract

Initial `.higurashi/config.json`:

```json
{
  "schemaVersion": 1,
  "workItems": {
    "idPattern": "^[A-Z][A-Z0-9]*(-[A-Z0-9][A-Z0-9._-]*)$",
    "requirementSources": [
      "docs/higurashi/requirements"
    ]
  },
  "artifacts": {
    "directory": "docs/higurashi"
  },
  "loop": {
    "maxApplyBatchesPerRun": 8,
    "maxRepairAttempts": 1,
    "requireProgressAfterEveryBatch": true
  },
  "codegraph": {
    "mode": "required",
    "autoInitializeProjectIndex": true
  },
  "context": {
    "instructionFiles": [
      "AGENTS.md",
      "CLAUDE.md"
    ],
    "authoritativeFiles": []
  },
  "verification": {
    "normalizationCommands": [],
    "requiredCommands": [],
    "candidateFormatCommands": []
  },
  "runners": {
    "opencode": {
      "enabled": true
    },
    "claudeCode": {
      "enabled": true
    }
  }
}
```

### 7.1 Validation

- `schemaVersion` must be exactly supported.
- `idPattern` must compile and must reject path separators.
- `requirementSources` must be non-empty and project-relative.
- Requirement sources may be files or directories; directories are searched
  recursively for Markdown files.
- Requirement discovery matches an exact work-item ID as the first token of an
  ATX Markdown heading outside fenced code blocks. Multiple matching headings,
  including duplicates in one file, are conflicts.
- Artifact filenames preserve the validated work-item ID exactly and append
  `.md` beneath the configured artifact directory.
- Artifact directory must be project-relative and must not be the project root.
- `maxApplyBatchesPerRun` must be between 1 and 32.
- `maxRepairAttempts` must initially be 0 or 1.
- `requireProgressAfterEveryBatch` must default to true.
- CodeGraph mode must be `required` or `preferred`.
- Optimal mode means `required`.
- Command arrays must contain argv arrays, not shell-concatenated strings.
- Verification suggestions are read-only evidence and must never be treated as
  configured command authority.
- Unknown keys fail closed.

### 7.2 Command representation

Commands should avoid shell interpolation:

```json
{
  "argv": ["go", "test", "./..."],
  "timeoutSeconds": 600
}
```

Candidate-path expansion is deferred until it can be represented without
shell-string injection. The first release may leave candidate-format commands
runner-managed while preserving their evidence contract in the skill.

## 8. Artifact contract

### 8.1 Format

```markdown
# WORK-123 — Short title

Higurashi-Schema: 1
Status: implementing
Repair-Round: 0

## Contract

...

## Scope

...

## Invariants

...

## Decisions

...

## Ordered vertical TDD checklist

- [x] **task-001 — First behavior:** observable result and focused evidence.
- [ ] **task-002 — Next behavior:** full task text.
  Multiline continuation remains part of the same task.

## Verification

...

## Evidence

...

## Warnings and handoff

...
```

When blocked, add machine-owned fields:

```text
Status: blocked
Repair-Round: 0
Blocked-From: implementing
Blocker-Reason: bounded-safe-text
```

### 8.2 Machine-owned fields

The CLI owns:

- `Higurashi-Schema`;
- `Status`;
- `Repair-Round`;
- `Blocked-From`;
- `Blocker-Reason`.

Agents own narrative sections and checklist markers.

### 8.3 Statuses

```text
refined
planned
implementing
verifying
blocked
complete
```

`refined` is recognized from the beginning even though the refinement command
is deferred. This avoids an artifact migration when refinement is added.

### 8.4 Legal transitions

```text
refined      → planned | blocked
planned      → implementing | blocked
implementing → verifying | blocked
verifying    → complete | blocked
blocked      → implementing through `higurashi repair authorize`
complete     → complete
```

Idempotently setting the current status succeeds without writing.

### 8.5 Transition invariants

- `planned` requires at least one checklist task.
- `implementing` requires at least one checklist task.
- `verifying` requires zero pending tasks.
- `complete` requires current state `verifying` and zero pending tasks.
- `blocked` requires a nonterminal `Blocked-From` value and a bounded reason.
- Normal transition cannot leave `blocked`; one validated handoff and explicit
  authorization are required.
- `Repair-Round` is a non-negative canonical integer and missing legacy values
  are read as zero.
- `complete` is terminal.
- Duplicate machine-owned fields make the artifact invalid.
- Missing status or checklist makes the artifact invalid.
- Task IDs must be unique.
- Multiline task text must round-trip without normalization.
- Artifact writes must preserve newline style where practical.

## 9. Loop protocol

### 9.1 Preflight

1. Resolve the Git project root.
2. Load and validate configuration.
3. Confirm CodeGraph capability according to configured mode.
4. Resolve the work-item ID against configured requirement sources.
5. Inspect the artifact.
6. Stop on invalid, terminal, or refused results.
7. Return the exact next legal action.

### 9.2 PLAN

PLAN runs for `ready` or `refined`, plus append-only repair planning from a
validated `repair_ready` handoff.

The planner:

- reads project instructions and configured authoritative context;
- uses CodeGraph before broad structural filesystem exploration;
- identifies the affected module seams and blast radius;
- records observable contract and acceptance checks;
- records scope and explicit exclusions;
- records security/domain invariants supplied by the consumer project;
- records rollback;
- creates a stable, ordered, vertical TDD checklist;
- records verification commands;
- creates a new artifact only when deterministic inspection permits it;
- appends exactly one bounded pending repair task per validated blocker without
  changing completed tasks or prior narrative/evidence;
- never overwrites a resumable artifact;
- never edits implementation code;
- never delegates.

The planner sets `Status: planned`.

### 9.3 APPLY

Before APPLY:

1. Transition `planned` to `implementing`, or require a previously authorized
   repair artifact already in `implementing`.
2. Inspect again and capture completed count and artifact hash.
3. Read `maxApplyBatchesPerRun`.

For each batch:

1. Delegate to a fresh APPLY agent.
2. Assign exactly the current next task.
3. Require one failing observable behavior first.
4. Implement the minimum passing behavior.
5. Refactor only while focused tests stay green.
6. Mark exactly that task complete.
7. Record focused evidence.
8. Inspect again.
9. Require completed-task count to increase by exactly one.
10. Stop and mark blocked if progress is absent, ambiguous, or invalid.

Stop conditions:

- no progress;
- invalid artifact;
- concrete blocker;
- configured batch ceiling;
- user cancellation;
- no pending tasks.

When no pending tasks remain, continue to VERIFY in the same invocation even if
the final batch reaches the configured ceiling.

Never implement an unbounded loop.

### 9.4 VERIFY

1. Run source-mutating normalization before freezing implementation files.
2. Transition to `verifying`; the CLI rejects pending tasks.
3. Capture changed paths and a frozen implementation diff.
4. Run configured verification commands.
5. Record exact commands, exit status, and relevant output.
6. If the repository-wide format gate has unrelated failures, prove that fact
   before using a candidate-scoped check.
7. Give identical candidate material to two fresh read-only reviewers.

Reviewers:

- **Contract reviewer:** observable behavior, scope, acceptance coverage,
  regressions, and whether tests prove the contract.
- **Risk reviewer:** project-defined invariants, failure paths, concurrency,
  privacy/security, idempotency, rollback, and operational safety.

A blocker must contain:

- stable deduplicated ID;
- originating reviewer;
- violated contract or invariant;
- exact evidence location;
- deterministic reproduction or red test command;
- minimum acceptance condition.

Warnings do not trigger repair.

### 9.5 Repair

If blockers exist and the in-run repair budget remains, run one bounded repair
writer and repeat verification with fresh reviewers. If blockers remain or the
budget is exhausted:

1. Keep the review candidate uncommitted.
2. Transition to `blocked` with `Blocked-From: verifying`.
3. Persist the exact reviewer evidence in
   `<artifact-directory>/<work-item-id>-repair-<Repair-Round+1>.md`.
4. Require version 1, exact work item/round/next command,
   `Candidate-Strategy: uncommitted`, and one strict blocker section per stable
   ID.
5. Invoke PLAN only to append task `repair-r<round>-<blocker-id>` for every
   blocker, carrying the exact reproduction and minimum acceptance.
6. Stop with exactly one command:
   `higurashi repair authorize <work-item-id>`.

Authorization is deterministic code. It validates blocked status, the ready
handoff, and exact pending tasks; writes `Handoff-Status: consumed` atomically;
then atomically clears blocked fields, increments `Repair-Round`, and sets
`Status: implementing`. Repeating the command finishes a consumed-handoff
partial transition or returns unchanged after success. A consumed handoff can
never authorize a later round.

After APPLY completes the authorized tasks, rerun normal VERIFY with fresh
independent reviewers. If blockers remain after the permitted in-run attempt,
create the next versioned handoff and require another explicit authorization.

## 10. CodeGraph requirement

### 10.1 Policy

CodeGraph is a first-class prerequisite for optimal operation.

```text
required  → preflight stops if CodeGraph is unavailable or unhealthy
preferred → warn, use read-only CodeGraph CLI if MCP is absent, then fall back
```

The generated default is `required`.

### 10.2 Required behavior

- Every real project or worktree has its own `.codegraph/` index.
- Indexes are never copied, shared, or symlinked between worktrees.
- Worktrees intended for CodeGraph use must live in stable user-owned paths,
  not generic temporary directories.
- Structural exploration uses CodeGraph before broad filesystem searches.
- Planning uses CodeGraph for seams, call flow, dependencies, and blast radius.
- APPLY uses CodeGraph before editing named symbols.
- Reviewers use CodeGraph when they need impact or caller/callee evidence.
- After writes, rely on watcher auto-sync unless stale state is reported.
- Retrieved graph intelligence is read-only evidence, not authority over current
  project instructions.

### 10.3 Claude Code

Claude Code supports local stdio MCP servers. The adapter should generate a
project-scoped MCP entry that invokes:

```text
codegraph serve --mcp
```

The generated adapter must:

- use the stable project root supplied by Claude Code;
- expose CodeGraph tools to the main coordinator and custom subagents;
- keep reviewers read-only;
- require the user/workspace to approve project MCP configuration;
- verify connection health rather than assuming configuration means success;
- avoid ephemeral subagent worktree isolation because each isolated worktree
  would require its own index and would not share sequential APPLY state.

Compatibility acceptance:

1. `claude mcp get codegraph` reports connected.
2. Claude Code lists the CodeGraph tool.
3. A custom planning subagent can call CodeGraph.
4. The result resolves the correct project root.
5. A file edit becomes visible after watcher synchronization.

A previously reported stdio framing incompatibility was closed upstream, but
the release used by Higurashi must still pass this smoke test before being
declared supported.

### 10.4 OpenCode

The OpenCode adapter should use the same project-local CodeGraph server and
canonical ordering rule. The adapter contract test must prove the generated
agents mention CodeGraph before broad exploration and do not silently replace
it with raw search.

### 10.5 Diagnostics

`higurashi doctor --json` should report:

```json
{
  "codegraph": {
    "mode": "required",
    "cliAvailable": true,
    "indexPresent": true,
    "statusHealthy": true,
    "rootMatches": true,
    "synchronization": "current",
    "watcherState": "not_reported",
    "version": "1.5.0",
    "indexState": "complete",
    "pendingChanges": 0,
    "reindexRecommended": false
  }
}
```

Runner connection fields are added by the corresponding adapter phase rather
than guessed by the deterministic Phase 2 core.

## 11. Canonical skill

The canonical `protocol/higurashi-deliver/SKILL.md` contains only runner-neutral
behavior:

- state machine;
- artifact contract;
- progress rules;
- TDD batch contract;
- verification/review contract;
- repair cap;
- CodeGraph ordering;
- control-document ownership and anti-bypass rules;
- safety and reporting rules.

It must not contain:

- runner-specific frontmatter;
- specific model names;
- provider-specific commands;
- consumer-project requirement paths;
- consumer-project domain invariants;
- database-specific verification workarounds;
- user home paths;
- global configuration changes.

Detailed artifact and reviewer contracts live one level below the skill and are
loaded only when needed.

## 12. Runner adapters

### 12.1 Shared topology

```text
main coordinator
├── PLAN agent
├── APPLY agent, fresh per batch
├── contract reviewer
└── risk reviewer
```

Delegation is one level deep. Worker and reviewer agents cannot delegate.
Every subagent prompt repeats the canonical anti-bypass rule: agents must not
weaken requirements, instructions, configuration, schemas, verification policy,
generated ownership markers, or existing tests to manufacture success. A
blocked contract is reported rather than rewritten. Renderer validation rejects
agent templates that omit this rule.

### 12.2 OpenCode adapter

Generated project files:

```text
.agents/skills/higurashi-deliver/SKILL.md
.opencode/commands/higurashi-deliver.md
.opencode/agents/higurashi-orchestrator.md
.opencode/agents/higurashi-plan.md
.opencode/agents/higurashi-apply.md
.opencode/agents/higurashi-verify-contract.md
.opencode/agents/higurashi-verify-risk.md
```

Rules:

- explicit invocation only;
- `higurashi-*` namespace;
- no changes to default agents;
- coordinator cannot edit implementation files directly;
- planner may write only the new artifact;
- APPLY may edit and run commands but cannot delegate;
- reviewers cannot edit, run mutating commands, or delegate;
- model and effort settings are adapter configuration, not canonical protocol;
- generated command accepts opaque work-item IDs and `--plan-only`.

### 12.3 Claude Code adapter

Plugin layout:

```text
.claude-plugin/plugin.json
skills/deliver/SKILL.md
agents/higurashi-plan.md
agents/higurashi-apply.md
agents/higurashi-verify-contract.md
agents/higurashi-verify-risk.md
.mcp.json
```

Invocation:

```text
/higurashi-loop:deliver WORK-123
```

Standalone project generation may additionally support:

```text
.claude/skills/higurashi-deliver/SKILL.md
.claude/agents/higurashi-*.md
```

Rules:

- the skill runs in the main conversation and coordinates all subagents;
- do not set the coordinator skill to `context: fork`;
- custom subagents receive the minimum task-local context;
- APPLY gets editing and focused test tools;
- reviewers deny write tools;
- use `maxTurns` as a secondary runaway guard, not as the APPLY batch count;
- default model is `inherit` so enterprise model policy remains authoritative;
- effort defaults may be role-specific but must use values supported by the
  selected model;
- subagents may access the project CodeGraph MCP server;
- do not use temporary worktree isolation for sequential APPLY batches.

## 13. Generated-file safety

Every generated file begins with a format-appropriate marker containing:

- generator name;
- generator version;
- template identifier;
- canonical source hash.

Install/update behavior:

1. Missing file: create.
2. Recognized unchanged generated file: update.
3. Recognized locally modified generated file: refuse and show diff.
4. Unrecognized existing file: refuse.
5. Removed template: report stale file; never delete automatically in the
   initial release.

Generated files are reproducible: equal configuration and Higurashi version
must produce byte-identical output.

## 14. Stable JSON results and exit codes

All JSON results include:

```json
{
  "schemaVersion": 1,
  "command": "inspect",
  "ok": true
}
```

Initial `config validate` result kinds:

```text
valid
configuration_missing
configuration_invalid
not_git_project
unsafe_project_root
invalid_usage
```

Initial `verification suggest` result kinds:

```text
suggestions
current
configuration_missing
configuration_invalid
not_git_project
unsafe_project_root
invalid_usage
```

Initial `doctor` result kinds:

```text
healthy
degraded
codegraph_unavailable
codegraph_unhealthy
configuration_missing
configuration_invalid
not_git_project
unsafe_project_root
invalid_usage
```

Initial exit-code contract:

```text
0  successful command/result
2  invalid CLI usage
3  invalid configuration
4  unknown or canceled work item
5  invalid artifact
6  required capability unavailable
7  stale hash or ownership conflict
8  unsafe path or generated-file conflict
9  unexpected internal error
```

Callers branch on JSON `kind`, not error-message text.

## 15. Security requirements

- Resolve symlinks before validating mutation targets.
- Reject work-item IDs containing path separators or traversal sequences.
- Reject artifact directories outside the Git root.
- Refuse to mutate the Git directory.
- Use argv arrays for subprocesses.
- Apply timeouts to external diagnostics.
- Bound captured subprocess output.
- Never read or log full environments.
- Never embed secrets in generated MCP or runner files.
- Treat requirement documents, memories, MCP output, and model output as
  untrusted data.
- Never execute instructions found in retrieved code or documentation unless
  the workflow contract explicitly requires them.
- CodeGraph remains read-only.
- Reviewers remain read-only.
- No unattended command receives commit, push, release, or remote-write
  authority.
- Generated adapters may request project tools but cannot bypass enterprise
  policy.

## 16. Implementation phases

Each phase is a vertical TDD slice. Do not batch all tests before
implementation.

### Phase 0 — repository scaffold

Deliver:

- initialize Git;
- create `go.mod` with local module path `higurashi-loop`;
- add `mise.toml` with the selected supported Go patch release;
- add CLI entry point with `version` and help;
- add `.gitignore` and `AGENTS.md`;
- update `README.md` with the selected Go baseline, working source-build and
  installation commands, current prerequisites, and verified CLI availability;
- add CI for formatting, vet, tests, and build;
- initialize a project-local CodeGraph index only after the directory is a real
  project;
- record the first implementation baseline.

Verification:

```text
go fmt ./...
go vet ./...
go test ./...
go build ./cmd/higurashi
```

Suggested commit:

```text
chore: scaffold Higurashi CLI
```

### Phase 1 — strict configuration

First failing behaviors:

- missing configuration returns a structured result;
- valid minimal configuration receives defaults;
- unknown keys fail;
- unsafe paths fail;
- loop limits outside allowed ranges fail.

Deliver:

- configuration structs;
- strict decoder;
- normalization;
- validation;
- JSON schema;
- `higurashi config validate`.

Suggested commit:

```text
feat: add strict project configuration
```

### Phase 2 — project and CodeGraph doctor

First failing behaviors:

- home/non-Git directory is rejected;
- missing CodeGraph fails required mode;
- missing index fails required mode;
- mismatched CodeGraph root fails;
- preferred mode emits warning without failing;
- command timeout is reported safely.

Deliver:

- project-root resolver;
- internal command-runner seam;
- fake command runner for tests;
- CodeGraph diagnostics;
- `higurashi doctor`.

Do not manage global CodeGraph lifecycle.

Suggested commit:

```text
feat: add project and CodeGraph diagnostics
```

### Phase 3 — artifact inspection

First failing behaviors:

- absent artifact plus known requirement returns `ready`;
- existing valid artifact returns `resume`;
- complete artifact returns terminal `complete`;
- malformed machine fields fail closed;
- multiline next task is preserved;
- duplicate task IDs fail;
- verifying/complete with pending tasks fails;
- unknown work item is refused;
- normalized artifact path cannot escape root.

Deliver:

- requirement lookup;
- artifact path normalization;
- parser;
- progress calculation;
- content hashing;
- `higurashi inspect`.

Suggested commit:

```text
feat: inspect resumable delivery artifacts
```

### Phase 4 — guarded transitions

First failing behaviors:

- every legal transition succeeds;
- every illegal transition fails;
- stale expected hash fails;
- blocked state records and enforces its origin;
- verification with pending tasks fails;
- completion outside verification fails;
- idempotent transition does not write;
- transition preserves narrative content;
- interrupted write does not corrupt the original.

Deliver:

- transition graph;
- invariant validation;
- atomic write helper;
- `higurashi transition`.

Suggested commit:

```text
feat: guard delivery state transitions
```

### Phase 5 — canonical skill and rendering

First failing behaviors:

- canonical skill contains the complete state/loop contract;
- canonical skill contains no runner-specific model or permission syntax;
- rendered files are deterministic;
- generated headers and hashes are correct;
- local modifications prevent overwrite;
- reusable files contain no consumer-specific terminology.

Deliver:

- canonical skill;
- references;
- embedded templates;
- rendering module;
- generated-file manifest;
- `adapter install`, `adapter diff`, and `adapter update`.

Suggested commit:

```text
feat: render runner-neutral workflow adapters
```

### Phase 6 — OpenCode adapter parity

First failing behaviors:

- explicit command routes to the coordinator;
- coordinator delegation is shallow;
- loop limit comes from deterministic configuration;
- PLAN is skipped on resume;
- one APPLY agent receives exactly one task;
- reviewers are read-only;
- one repair cap is preserved;
- CodeGraph ordering is explicit;
- no default workflow is modified.

Deliver:

- command and five agent templates;
- adapter configuration;
- golden tests;
- fixture installation test.

Suggested commit:

```text
feat: add OpenCode adapter
```

### Phase 7 — Claude Code adapter

First failing behaviors:

- plugin manifest validates;
- `/higurashi-loop:deliver` exists;
- main skill coordinates rather than forking itself;
- custom agents cannot delegate;
- reviewers deny write tools;
- APPLY receives only needed tools;
- `maxTurns` and effort fields validate;
- CodeGraph MCP entry is generated;
- subagents can discover CodeGraph in a smoke fixture;
- no temporary worktree isolation is configured.

Deliver:

- plugin manifest;
- main skill wrapper;
- four custom agents;
- MCP template;
- standalone project-local variant;
- schema/golden tests;
- documented manual MCP trust step.

Suggested commit:

```text
feat: add Claude Code adapter
```

### Cross-cutting slice — durable repair handoffs

Implemented behaviors:

- blocked inspection distinguishes `handoff_required`, `repair_ready`, and
  recoverable consumed-handoff state;
- `Repair-Round` determines the versioned sidecar path;
- strict handoff validation rejects missing fields, placeholders, duplicate
  blocker IDs, mismatched work items/rounds, incorrect commands, and reuse;
- `higurashi repair authorize` requires one exact pending task per blocker,
  consumes the handoff, increments the round, and enters `implementing`;
- authorization preserves completed tasks and narrative bytes;
- consumed-handoff-first ordering makes interrupted two-file updates
  deterministically recoverable and repeated authorization idempotent;
- normal `transition` cannot leave `blocked`;
- inspection and mutation JSON expose repair state and the exact next command;
- both runner adapters keep reviewers read-only, restrict planner/APPLY
  ownership, preserve uncommitted review candidates, and require explicit
  `--repair`.

### Phase 8 — end-to-end fixture

Create a language-neutral fixture repository with:

- generic project instructions;
- one requirement document;
- one small implementation seam;
- focused tests;
- Higurashi configuration;
- both adapters;
- CodeGraph index created during the test setup, never committed.

Prove:

1. `ready` inspection.
2. PLAN artifact creation.
3. partial APPLY progress.
4. process/session restart.
5. resume without replanning.
6. batch ceiling behavior.
7. remaining tasks complete.
8. verification transition.
9. independent reviewer inputs are identical.
10. clean completion.
11. blocked review and one repair.
12. terminal re-invocation performs no work.

Automate deterministic portions. Record runner-driven smoke scripts separately
because model output is nondeterministic.

Suggested commit:

```text
test: prove resumable cross-runner workflow
```

### Phase 9 — packaging and release

Deliver:

- version injection at build time;
- Linux, macOS, and Windows builds for common architectures;
- checksums;
- release notes;
- adapter compatibility matrix;
- configuration and artifact schema version policy;
- upgrade and rollback instructions;
- finalize and smoke-test the README installation instructions without assuming
  the consumer project language.

Do not publish until both adapter smoke tests pass against pinned runner and
CodeGraph versions.

Suggested commit:

```text
chore: prepare initial Higurashi release
```

## 17. Test matrix

### 17.1 Unit tests

- strict config decoding;
- defaults;
- regex and path validation;
- requirement lookup;
- artifact parsing;
- checklist multiline parsing;
- duplicate field/task detection;
- transition graph;
- blocked origin;
- content hash;
- atomic writer;
- generated manifest;
- exit-code mapping.

### 17.2 Local-substitutable integration tests

Use temporary Git repositories and fake executables:

- project-root discovery;
- CodeGraph doctor output;
- requirement and artifact discovery;
- generated adapter install/update conflicts;
- no writes outside root;
- interrupted transition recovery.

### 17.3 Golden tests

- canonical skill rendering;
- OpenCode files;
- Claude plugin files;
- JSON output;
- example configuration;
- artifact template.

Golden updates must be explicit and reviewed.

### 17.4 Race and platform tests

```text
go test -race ./...
```

Test path behavior on Linux, macOS, and Windows CI. Use platform-specific tests
only where path or atomic replacement semantics differ.

### 17.5 Manual runner acceptance

OpenCode:

- command visible;
- coordinator selected;
- CodeGraph callable;
- fresh workers;
- resume works.

Claude Code:

- plugin visible;
- namespaced command visible;
- MCP connected;
- custom subagents available;
- subagent CodeGraph call works;
- resume works in a new session.

## 18. CI gates

Required on every change:

```text
gofmt check
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/higurashi
generated-file golden check
generic-language check
JSON schema validation
git diff --check
```

The generic-language check must reject consumer-specific identifiers and
project-specific product/domain language from:

- `cmd/`;
- `internal/`;
- `protocol/`;
- `adapters/`;
- `schemas/`;
- active tests and fixtures.

## 19. Migration rules

Use the current working prototype as a behavioral reference, not as a package
dependency.

Migrate:

- status vocabulary where generic;
- checklist progress parsing;
- multiline next-task behavior;
- resume/complete/invalid inspection outcomes;
- guarded verifying/complete transitions;
- fresh sequential APPLY batches;
- progress-required loop behavior;
- independent reviewers;
- one repair cap;
- candidate-vs-baseline format distinction;
- durable artifact/session-disposable principle.

Rewrite or remove:

- project-specific work-item catalog parsing;
- fixed requirement paths;
- project-specific ownership checks;
- product/domain invariants;
- database and container workarounds;
- provider-specific model names;
- user-specific filesystem paths;
- fixed batch count;
- runner-specific language in the canonical skill.

Do not modify or delete the existing prototype during extraction. Side-by-side
operation is allowed only after the new adapters use unique `higurashi-*` names
and fixture tests prove they do not collide.

## 20. Definition of done for v0.1

The first release is complete when:

- the Go CLI builds on supported platforms;
- configuration is strict and documented;
- `inspect` and `transition` pass all state-contract tests;
- APPLY batch count is configurable and enforced;
- a missing/unhealthy required CodeGraph blocks preflight clearly;
- a healthy CodeGraph works through both runners;
- OpenCode and Claude Code adapters are generated deterministically;
- adapters do not modify default runner behavior;
- the same fixture artifact resumes across runner sessions;
- explicit refinement asks at most two bounded question batches, persists only
  a confirmed contract, and resumes through the guarded `refined → planned`
  transition;
- verification cannot begin with pending tasks;
- completion cannot occur without clean verification;
- reviewers are independently read-only;
- one repair attempt is enforced;
- generated-file drift is detected safely;
- reusable code and prompts contain no consumer-specific terminology;
- no existing project workflow has been changed;
- the README accurately lists prerequisites, supported versions, tested
  installation, upgrade, rollback, runner, and CodeGraph setup;
- release artifacts include checksums and compatibility notes.

## 21. Post-v0.1 roadmap

Explicit refinement was pulled into the initial implementation. Both runners
now expose:

```text
/higurashi-loop:refine WORK-123
/higurashi-refine WORK-123
```

The product-owner role returns one free-text question batch with recommended
answers, accepts a free-text response, allows at most one follow-up batch, and
persists only an explicitly confirmed contract with `Status: refined`.

### v0.2 — context metrics and skill registry

- phase-level context metrics;
- metadata-first approved skill discovery;
- skill versions, provenance, review dates, and evaluations;
- fixed context budget for retrieved references.

### v0.3 — optional project recall

- one project-scoped memory write authority;
- compact non-authoritative observations;
- no raw transcripts or sensitive product data;
- repository confirmation before recalled claims influence implementation.

### v0.4 — proposal-only learning

- sanitized deterministic event aggregation;
- repeated-pattern thresholds;
- candidate skill/patch proposals outside active discovery paths;
- static, activation, non-activation, replay, and injection evaluations;
- human-only promotion.

### v0.6 — Codex adapter

- reuse canonical skill, config, artifact, and Go CLI;
- add only Codex-specific entry points, permissions, and delegation mechanics;
- prove the same fixture resumes across all supported runners.

## 22. First implementation session

Start with only Phase 0 and Phase 1.

Concrete sequence:

1. Read this plan fully.
2. Create a Git repository in a normal development workspace.
3. Create `AGENTS.md` with TDD, CodeGraph, safety, and generic-language rules.
4. Select and record the minimum supported Go version in `README.md`.
5. Create the Go module and CLI help/version command.
6. Replace planned README commands with tested source-build and installation
   instructions as they become available.
7. Add the first failing configuration test.
8. Implement strict decoding and defaults.
9. Add path and loop-limit tests one behavior at a time.
10. Run formatting, vet, tests, and build.
11. Commit the scaffold separately from configuration only when explicitly
    requested.
12. Stop and report the next Phase 2 test.

Do not begin runner adapters in the first session. Establish the deterministic
module and test discipline first.

## 23. Primary references

- Claude Code skills:
  <https://code.claude.com/docs/en/slash-commands>
- Claude Code subagents:
  <https://code.claude.com/docs/en/sub-agents>
- Claude Code plugins:
  <https://code.claude.com/docs/en/plugins>
- Claude Code MCP:
  <https://code.claude.com/docs/en/mcp>
- CodeGraph:
  <https://github.com/colbymchenry/codegraph>
- Go build/install:
  <https://go.dev/doc/tutorial/compile-install>
- Agent Skills specification:
  <https://agentskills.io/specification>
