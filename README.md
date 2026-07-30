# Higurashi Loop

Higurashi Loop is a runner-neutral development workflow for delivering one
well-scoped work item through a bounded, resumable loop:

```text
REFINE (optional) → PLAN → APPLY → VERIFY
```

The workflow keeps its authoritative state in the consumer repository rather
than in a runner session. A Go CLI will enforce deterministic configuration,
artifact, transition, rendering, and diagnostic rules. Runner-native Markdown
skills and agents will coordinate model work.

## Project status

Higurashi Loop is alpha software intended for early developer testing. The Go
CLI can be downloaded from GitHub Releases, built from source, or installed
with Go, and currently provides:

```text
higurashi init --runner <opencode|claude-code> [--runner NAME ...] [--requirement-source PATH ...] [--project-root PATH] [--force-generated] [--json]
higurashi help
higurashi version
higurashi config validate [--json]
higurashi config requirements set PATH [PATH ...] [--json]
higurashi requirements import WORK-123 (--from PATH | --stdin) [--json]
higurashi doctor [--json]
higurashi inspect WORK-123 [--json]
higurashi transition WORK-123 STATUS --expected-hash HASH [--reason TEXT] [--json]
higurashi repair authorize WORK-123 [--json]
higurashi adapter <install|diff|update> <opencode|claude-code> [--json]
higurashi models <show|set|validate> [--runner opencode] [OPTIONS]
higurashi verification suggest [--json]
```

Project-local OpenCode and Claude Code adapter generation is implemented. The
Claude Code adapter remains provisional until its trust-dependent live MCP
smoke test is run with an authenticated account.

See [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) for the complete contract
and phased delivery plan.

## Requirements

### Development requirements

- Git.
- Mise, recommended for the pinned project toolchain.
- GitHub CLI 2.96.0, pinned through mise for maintainers publishing releases.
- Go 1.25 or Go 1.26. `mise.toml` currently pins Go 1.25.12 for local
  development.
- Node 24.18.0, pinned only to install and launch the development Claude Code
  validator.
- Claude Code 2.1.220, pinned through mise as a development-only native
  validator for the generated Claude adapter. Authentication is not required
  for version and plugin-structure validation.
- CodeGraph CLI with a separate `.codegraph/` index for this checkout.

The implementation should otherwise prefer the Go standard library. Additional
development tools must be documented here when introduced.

### Consumer-project requirements for v0.1

- A Git repository or worktree.
- The `higurashi` binary.
- At least one supported runner:
- OpenCode 1.x (tested with 1.18.9); or
  - a current Claude Code release supporting plugin skills, custom-agent
    `maxTurns` and `effort`, and project MCP configuration.
- CodeGraph CLI and a healthy project-local `.codegraph/` index when the
  configured CodeGraph mode is `required`, which is the planned default.
- Permission to add project-local Higurashi configuration, artifact, skill,
  agent, command, and MCP files.

The alpha release is tested with CodeGraph 1.5.0 and Linux amd64. Release
archives are cross-built for Linux and macOS on amd64 and arm64, plus Windows
amd64; those additional targets still need tester validation.

The Higurashi binary and consumer projects do not require Node, Bun, Python, a
hosted control plane, direct model-provider credentials, or a memory provider.
This source checkout pins Node only for its Claude adapter validator. A selected
runner may have its own runtime and authentication requirements.

## Installation

### Install a tester release

Download the archive for your platform from the
[GitHub Releases page](https://github.com/jpmartinez/higurashi-loop/releases).
Verify it against `checksums.txt`, extract `higurashi` (or `higurashi.exe`), and
place it in a directory on `PATH`.

For example, on Linux amd64:

```text
version=v0.1.0-alpha.3
curl -LO "https://github.com/jpmartinez/higurashi-loop/releases/download/${version}/higurashi_${version}_linux_amd64.tar.gz"
curl -LO "https://github.com/jpmartinez/higurashi-loop/releases/download/${version}/checksums.txt"
sha256sum --check --ignore-missing checksums.txt
tar -xzf "higurashi_${version}_linux_amd64.tar.gz"
install -m 0755 higurashi ~/.local/bin/higurashi
higurashi version
```

Use a versioned URL rather than a moving “latest” URL while the project is in
alpha. To upgrade, repeat these steps with the new version. To roll back,
reinstall the previous versioned archive.

### Install the development toolchain

```text
mise trust mise.toml
mise install
mise exec -- go version
mise exec -- gh --version
mise exec -- node --version
mise exec -- claude --version
```

Review `mise.toml` before trusting it. Its Claude Code entry allows only the
package's own lifecycle script, which installs the native executable. The
version commands should report Go 1.25.12, GitHub CLI 2.96.0, Node 24.18.0, and
Claude Code 2.1.220. Claude Code is installed here only to validate the adapter.
Using generated skills, agents, or MCP connections requires a supported
authenticated Claude Code account.

### Build a repository-local binary

```text
mise exec -- go build -o ./bin/higurashi ./cmd/higurashi
./bin/higurashi version
```

### Install into the Go binary directory

```text
go install github.com/jpmartinez/higurashi-loop/cmd/higurashi@v0.1.0-alpha.3
```

This installs `higurashi` into `GOBIN`, or into the current Go environment's
default binary directory when `GOBIN` is unset. That directory must be on
`PATH` to invoke `higurashi` by name.

The source build, source installation, version, and help commands above have
been exercised with the pinned Mise toolchain. Release archives include the
MIT license.

## Current usage

Create `.higurashi/config.json` in a Git repository:

```json
{
  "schemaVersion": 1
}
```

Validate it and inspect the normalized effective configuration:

```text
higurashi config validate
higurashi config validate --json
```

The command is read-only. It discovers the containing Git project, rejects
unknown configuration fields and unsafe paths, applies conservative defaults,
and returns exit code `3` for missing or invalid configuration.

Discover project-owned verification commands before authorizing agents to run
them:

```text
higurashi verification suggest
higurashi verification suggest --json
```

Suggestion discovery is read-only: it never executes commands or changes
configuration. It recognizes high-confidence Go commands and package-manager
scripts from `package.json`, using Bun, pnpm, Yarn, or npm based on the declared
manager or lockfile. Each suggestion includes its exact argv, timeout, source,
destination under `verification`, and whether its script requires review for
operations such as a local database reset, deployment, publication, push,
forced mutation, or removal. Commands already configured by argv are omitted.

The human-readable result includes a complete suggested `verification` value
for `.higurashi/config.json`. Review and copy only the commands the project
intends to authorize. Detection rules live in Higurashi, but accepted commands
remain durable, project-owned configuration.

Check project and CodeGraph health:

```text
higurashi doctor
higurashi doctor --json
```

Doctor is read-only. It checks the Git project root, strict configuration,
configured requirement sources, CodeGraph executable, project-local
`.codegraph/` index, status health, root consistency, and pending
synchronization state. Missing requirement sources fail closed with exit code
`3`. In the default `required` mode, an unavailable or unhealthy CodeGraph
returns exit code `6`. In `preferred` mode, the same condition returns success
with a `degraded` result and warnings.

The installed CodeGraph status interface does not report watcher health, so
doctor reports `watcherState: "not_reported"` rather than guessing. Runner and
adapter discoverability checks will be added with their corresponding adapter
phases.

Inspect a known work item and its durable delivery artifact:

```text
higurashi inspect WORK-123
higurashi inspect WORK-123 --json
```

Inspection is read-only. A work item is known when its exact ID is the first
token of an ATX Markdown heading in a configured requirement source, for
example `## WORK-123 — Add inspection`. Requirement-source directories are
searched recursively for Markdown files. Multiple matching headings are
reported as a conflict instead of being selected implicitly.

Import a requirement from an existing Markdown document:

```text
higurashi requirements import WORK-123 \
  --from "requirements/Product requirements.md"
```

Or paste a requirement through standard input:

```text
higurashi requirements import WORK-123 --stdin
```

File imports extract the unique Markdown section headed by the work-item ID.
Headingless stdin text receives only the required ID heading. Both forms create
`docs/higurashi/requirements/<ID>.md`, preserve the imported bytes beneath
machine-owned provenance metadata, record a source hash, and activate the
managed requirement directory as the authoritative source. Existing differing
snapshots fail as conflicts and are never silently replaced. The file-first,
configuration-second transition is recoverable by repeating the same import.

The artifact path is the configured artifact directory plus the exact
validated ID and `.md`; with defaults, `WORK-123` maps to
`docs/higurashi/WORK-123.md`. A missing artifact returns `ready`. A valid
artifact returns `resume`, or terminal `complete`, with its status, exact-byte
SHA-256 hash, checklist progress, and first pending task. Invalid or ambiguous
machine fields, duplicate task IDs, illegal progress states, and symlink path
escapes fail closed. See the artifact contract in
[IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md#8-artifact-contract).

A blocked artifact uses a machine-owned `Repair-Round` field; legacy artifacts
without it are read as round zero. Inspection computes the next sidecar path
deterministically and returns `handoff_required` when the sidecar is absent or
invalid, `repair_ready` when a valid ready handoff exists, or
`repair_recovery_required` when an interrupted authorization left a consumed
handoff and blocked artifact. JSON includes the current round, handoff path and
validation, blocker count, candidate strategy, authorization requirement, and
the one exact next command when authorization or recovery is possible.

Guard and update an existing artifact status:

```text
higurashi transition WORK-123 implementing \
  --expected-hash SHA256_FROM_INSPECT

higurashi transition WORK-123 blocked \
  --expected-hash SHA256_FROM_INSPECT \
  --reason waiting-for-explicit-user-decision
```

Transition first repeats project, configuration, CodeGraph, requirement, path,
and artifact validation. It rejects a stale expected hash with exit code `7`
and illegal state changes or artifact invariants with exit code `5`. Entering
`blocked` records the current nonterminal status in `Blocked-From` and requires
a bounded, single-line reason. A normal transition cannot leave `blocked`;
repair resumption requires the explicit guarded authorization command.

Successful changes replace only the machine-owned header fields through a
same-directory temporary file and atomic rename. Narrative bytes, checklist
content, newline style, and file permissions are preserved. Setting the current
status is idempotent and does not replace the file. Mutation targets resolving
outside the project or beneath `.git` are refused.

Independent verification blockers are persisted in:

```text
<artifact-directory>/<work-item-id>-repair-<round>.md
```

The strict versioned handoff records the work item, ready/consumed state, round,
exact next command, `uncommitted` candidate strategy, and one structured section
per stable blocker ID. Every blocker requires its originating reviewer,
violated contract, exact evidence location, deterministic reproduction, and
minimum acceptance condition. Missing fields, placeholders, duplicate IDs,
wrong work items or rounds, wrong commands, and consumed handoffs outside the
single recoverable current authorization fail closed.

After a planner appends exactly one pending task per validated blocker,
authorize one repair round explicitly:

```text
higurashi repair authorize WORK-123
higurashi repair authorize WORK-123 --json
```

Authorization requires `Status: blocked`, validates exact task-to-blocker
evidence, consumes the handoff, increments `Repair-Round`, and transitions to
`implementing`. It preserves completed tasks and all other artifact bytes.
The handoff is atomically consumed before the artifact update; repeating the
same command safely finishes an interrupted update or reports the completed
authorization unchanged. Blocked candidates remain uncommitted so every fresh
reviewer sees the complete working-tree diff.

Configure role-specific OpenCode models in Higurashi's project configuration:

```text
higurashi models show --runner opencode

higurashi models set --runner opencode \
  --orchestrator openai/gpt-5.6-luna \
  --orchestrator-effort high \
  --refine openai/gpt-5.6-sol \
  --refine-effort high \
  --plan openai/gpt-5.6-sol \
  --plan-effort high \
  --apply openai/gpt-5.6-luna \
  --apply-effort max \
  --verify-contract openai/gpt-5.6-luna \
  --verify-contract-effort max \
  --verify-risk openai/gpt-5.6-terra \
  --verify-risk-effort high

higurashi models validate --runner opencode
```

Replace the example IDs with exact values from `opencode models`. The suggested
shape uses Luna at high effort for deterministic coordination, Sol at high
effort for product refinement and planning, Luna at maximum effort for code
production and contract verification, and Terra at high effort for a distinct
risk perspective. APPLY and contract verification remain separate sessions
with different prompts and permissions even though they use the same model.

Assignments are stored under `runners.opencode.models` in
`.higurashi/config.json`. The value `inherit` leaves a role on OpenCode's
active/default model. Effort flags compile to OpenCode's native
`provider/model#variant` reference; setting an effort to `inherit` clears that
role's explicit variant. `models set` preserves unspecified roles, validates
every value as a safe model reference, preflights generated-file ownership,
atomically replaces the configuration, and regenerates Higurashi-owned agent
files. It refuses locally modified or user-owned generated targets.

`models validate` is read-only and checks every explicit assignment against the
models and variants returned by `opencode models --verbose`. Both `show` and
`validate` support `--json`.

Render or inspect the canonical runner-neutral delivery protocol:

```text
higurashi adapter install opencode
higurashi adapter diff opencode
higurashi adapter update opencode

higurashi adapter install claude-code
```

For OpenCode, installation generates:

```text
.agents/skills/higurashi-deliver/SKILL.md
.agents/skills/higurashi-deliver/references/artifact-contract.md
.agents/skills/higurashi-deliver/references/reviewer-contract.md
.agents/skills/higurashi-refine/SKILL.md
.opencode/commands/higurashi-deliver.md
.opencode/commands/higurashi-refine.md
.opencode/agents/higurashi-orchestrator.md
.opencode/agents/higurashi-refine.md
.opencode/agents/higurashi-plan.md
.opencode/agents/higurashi-apply.md
.opencode/agents/higurashi-verify-contract.md
.opencode/agents/higurashi-verify-risk.md
```

The OpenCode adapter uses the current 1.x Markdown `permission` format and has
been fixture-tested with OpenCode 1.18.9. It does not change built-in or default
agents. Roles default to `inherit`; explicit Higurashi project assignments are
rendered into the generated agent frontmatter. OpenCode remains authoritative
for provider authentication and model availability.

Verify project-local discovery:

```text
opencode debug config
opencode agent list
```

Then invoke the workflow explicitly inside OpenCode:

```text
/higurashi-refine WORK-123
/higurashi-deliver WORK-123
/higurashi-deliver WORK-123 --plan-only
/higurashi-deliver WORK-123 --repair
```

Use refine first only when observable product behavior is too ambiguous to plan
safely. The product-owner role asks one free-text question batch with
recommended answers, permits at most one follow-up batch, shows the complete
contract, and writes a `Status: refined` artifact only after explicit
confirmation. It never changes the requirement source or persists raw dialogue.
The following delivery invocation preserves that confirmed contract, adds the
TDD checklist, and transitions it to `planned`.

The OpenCode coordinator also supports ordinary conversation. Any message that
is not an exact delivery invocation or a valid session retry is handled as a
question or proposal: the coordinator may explain the current behavior,
inspect the project with read-only tools, identify tradeoffs, and suggest
concrete changes or work-item acceptance criteria. It does not invoke delivery
subagents, mutate workflow state, or edit files in this mode. Starting
implementation still requires an exact work-item invocation.

The normal delivery form resumes an existing durable artifact automatically.
When a normal delivery stops on a correctable, non-terminal blocker, resolve
the reported cause and reply `try again`, `retry`, or `continue` in the same
runner conversation. The coordinator re-enters PRECHECK with the same work-item
ID and options, reloads durable state and configuration, and continues without
requiring the slash command again. Retry never authorizes a repair round; an
explicit `--repair` invocation is still required when inspection reports
`repair_ready`.
`--plan-only` creates or inspects the plan and stops before APPLY. The
`higurashi` and `codegraph` executables must be available to OpenCode through
`PATH`.

For Claude Code, installation generates a reusable plugin and a standalone
project-local variant:

```text
.claude-plugin/plugin.json
skills/deliver/SKILL.md
skills/refine/SKILL.md
agents/higurashi-refine.md
agents/higurashi-plan.md
agents/higurashi-apply.md
agents/higurashi-verify-contract.md
agents/higurashi-verify-risk.md
.mcp.json
.claude/skills/higurashi-deliver/SKILL.md
.claude/skills/higurashi-refine/SKILL.md
.claude/agents/higurashi-refine.md
.claude/agents/higurashi-plan.md
.claude/agents/higurashi-apply.md
.claude/agents/higurashi-verify-contract.md
.claude/agents/higurashi-verify-risk.md
```

The canonical skill references are installed beneath
`.claude/skills/higurashi-deliver/references/`. Validate and load the plugin
from the project root:

```text
claude plugin validate . --strict
claude --plugin-dir .
```

Invoke the plugin skill with:

```text
/higurashi-loop:refine WORK-123
/higurashi-loop:deliver WORK-123
/higurashi-loop:deliver WORK-123 --plan-only
/higurashi-loop:deliver WORK-123 --repair
```

Without `--plugin-dir`, Claude Code can use the standalone project skill as:

```text
/higurashi-refine WORK-123
/higurashi-deliver WORK-123
```

Refinement is runner-native because it requires a model/user question and
confirmation exchange. The Go binary remains deterministic: `inspect` supplies
the exact requirement and artifact paths, parses the resulting refined
artifact, and guards the later `refined → planned` transition.

The generated `.mcp.json` runs
`codegraph serve --mcp --path ${CLAUDE_PROJECT_DIR}`. Claude Code requires an
explicit trust decision before using a project MCP server. Review the generated
file, approve it in the workspace prompt, and then verify:

```text
claude mcp get codegraph
```

Inside Claude Code, `/mcp` must show CodeGraph connected and list its tools.
Run one `--plan-only` fixture and confirm the planning agent can query CodeGraph
for the correct project root. After a controlled file edit, confirm the change
appears after watcher synchronization. Configuration presence alone is not
connection proof; do not declare the Claude adapter supported for a release
until this smoke test passes.

Claude agents inherit the session model, use bounded `maxTurns`, cannot
delegate, and are denied temporary-worktree tools. APPLY receives only read,
search, edit, shell, and CodeGraph tools. Reviewers receive only read, search,
and CodeGraph tools and explicitly deny write, shell, delegation, and worktree
tools. The `higurashi` and `codegraph` executables must be on Claude Code's
`PATH`.

Generated files carry the generator version, template identifier, and canonical
source hash. `.higurashi/generated.json` records their exact content hashes.
Installation and update refuse unrecognized files and locally modified
generated files without writing anything; `adapter diff` is read-only and
reports missing, outdated, conflicting, and stale paths.

The canonical protocol prohibits agents from weakening requirements,
instructions, configuration, schemas, verification policy, generated ownership
markers, or existing tests to manufacture success. It requires the exact
anti-bypass rule in every subagent prompt, and the renderer rejects future agent
templates that omit that rule. Both adapters enforce shallow delegation.
Coordinators do not implement directly, workers cannot delegate, and reviewers
cannot edit or run shell commands. Role-owned artifact narrative, checklist,
and evidence updates remain allowed.

## Project setup

Run initialization from the root of an existing Git project:

```text
higurashi init --runner opencode
```

When requirements should remain live instead of being imported into the
default managed `docs/higurashi/requirements` directory,
configure one or more existing project-relative Markdown files or directories
during initialization:

```text
higurashi init --runner opencode \
  --requirement-source "requirements/Product requirements.md"
```

To install both supported adapters:

```text
higurashi init --runner opencode --runner claude-code
```

Initialization creates:

- a complete conservative `.higurashi/config.json`;
- the configured `docs/higurashi` artifact directory;
- the default managed `docs/higurashi/requirements` requirement directory;
- `.higurashi/generated.json`;
- only the selected project-local runner skills, commands, and agents.

It never changes global runner settings or overwrites user-owned files.
Repeating the same command is idempotent. If a recognized generated file was
locally changed and must be restored deliberately, use `--force-generated`;
this flag still refuses unrecognized or user-owned files.

When known project manifests contain unconfigured verification commands, init
also returns their structured suggestions and adds this read-only next command:

```text
higurashi verification suggest
```

Adapter installation and updates expose the same guidance. Neither operation
silently adds command authority to project configuration.

CodeGraph keeps a separate index per checkout or worktree. If the project does
not yet have `.codegraph/`, initialize it locally and do not commit the index:

```text
codegraph init .
```

Then run the read-only health check:

```text
higurashi doctor
```

The default requirement source is the managed
`docs/higurashi/requirements` directory. It starts empty so `higurashi doctor`
is healthy immediately after initialization. Import a requirement from a file
or pasted text before refinement. Managed files define every work item exactly
once as an ATX Markdown heading whose first token is the ID:

```text
## WORK-123 Add an observable behavior

Describe the behavior, constraints, non-goals, and acceptance evidence here.
```

For an existing initialized project, correct the configured sources explicitly:

```text
higurashi config requirements set \
  "requirements/Product requirements.md"
higurashi doctor
```

The command validates every source before atomically replacing the project
configuration. Refinement agents never invoke this broad source replacement
from chat context or silently override the configured source list.

When a requirement arrives as an attached file or pasted message, prefer a
durable managed snapshot:

```text
higurashi requirements import WORK-123 \
  --from "requirements/Product requirements.md"
higurashi inspect WORK-123 --json
```

Runner refinement commands may perform this deterministic import only after the
user explicitly identifies the file or confirms the exact pasted text as
authoritative. The model may transport those bytes but may not rewrite them.

Verify deterministic discovery before opening the runner:

```text
higurashi config validate
higurashi models show --runner opencode
higurashi models validate --runner opencode
higurashi inspect WORK-123 --json
opencode debug config
opencode agent list
opencode
```

Inside OpenCode, refine only when product behavior remains ambiguous:

```text
/higurashi-refine WORK-123
```

Otherwise start or resume delivery directly:

```text
/higurashi-deliver WORK-123
```

Commit project configuration, requirements, and generated adapter files
according to the repository's normal policy. Keep `.codegraph/` local.

## Development verification

Install the pinned toolchain once with `mise install`, then run:

```text
mise exec -- go fmt ./...
mise exec -- go vet ./...
mise exec -- go test ./...
mise exec -- go test -race ./...
mise exec -- go build ./cmd/higurashi
```

The regular test suite generates an isolated Claude adapter fixture and runs
`claude plugin validate --strict` against it. This structural check does not
need authentication. The trust-dependent live skill, subagent, and CodeGraph
MCP smoke test described above still requires an authenticated supported
account.

## Documentation policy

Update this README in the same change whenever implementation alters:

- prerequisites or supported versions;
- installation, upgrade, or rollback steps;
- CLI commands or flags;
- generated project files;
- runner or CodeGraph setup;
- verification commands;
- release availability.

Installation commands and compatibility claims are part of the tested product
interface, not release-day notes.
