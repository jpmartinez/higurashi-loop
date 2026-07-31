# Multi-Team Agent Orchestration Product — Session Handoff

Date: 2026-07-31
Status: Product direction agreed; no product name, repository, stack, or implementation exists yet.

## Purpose of the next session

Start a separate product for durable, interactive, multi-team software delivery.
Do not turn the existing Higurashi Loop repository into this product. Higurashi
should remain the small guarded delivery loop it is today, and may be used as
the development process for building the new product.

The next session should use this handoff as its starting context, resolve the
remaining foundational choices, create the new repository, and define the
first vertical requirement. Do not modify Higurashi unless the user explicitly
requests a Higurashi change.

## Product thesis

The product coordinates a small software organization made of persistent,
specialized AI-agent teams. Each team is a stateful subgraph inside a durable
top-level mission graph. The human can inspect and interact directly with
agents, while a deterministic controller owns workflow state, gates,
scheduling, recovery, and authorization.

The product should preserve Higurashi's strongest ideas:

- guarded state transitions;
- durable state outside model memory;
- optimistic hashes and stale-output rejection;
- resumability and idempotent recovery;
- explicit authorization for sensitive transitions;
- frozen review candidates;
- independent reviewers;
- bounded repair loops;
- stable, structured outcomes.

These principles should be reimplemented for the new domain rather than making
the new product depend on Higurashi's current internal Go packages or fixed
`REFINE -> PLAN -> APPLY -> VERIFY` artifact format.

## Core architectural decision

Agents do not own lifecycle state.

The top-level **Mission Controller** is deterministic. It owns the mission
graph, team activation, phase gates, checkpointing, task readiness, concurrency
limits, retries, repair budgets, and human interrupts. A conversational lead
agent may explain state and coordinate discussion, but it cannot manufacture a
transition or declare work accepted.

Terminal sessions, multiplexer state, transcripts, and model memory are
operational context. They are never the source of truth.

## Initial organizational model

Only these team types are initially supported:

1. Product Owner
2. Design Team
3. Implementation Team
4. QA Team

The initial topology is fixed. Do not begin with a generic user-defined team
DSL. Make the fixed topology excellent first; generalize only after another
real topology proves what must vary.

### Product Owner

One Product Owner agent owns refinement of the initial requirement.

Responsibilities:

- read and challenge the initial requirement;
- ask bounded question batches with recommended answers;
- interact directly with the human;
- use a grilling strategy when rigorous challenge is useful;
- never invent product policy silently;
- raise a human decision when it cannot answer authoritatively;
- produce the confirmed requirement;
- remain available to later teams for product questions.

Recommended refinement: expose grilling as a configurable strategy such as
`standard` or `rigorous`, rather than making every interaction relentless.

There should be one product authority. The Design Team should consult the same
Product Owner seat rather than creating an independent second Product Owner
with divergent memory or authority.

### Design Team

Roles:

- **Designer:** produces the technical design, technology choices, key seams,
  constraints, and risks.
- **Product Owner:** answers requirement questions and escalates unresolved
  product decisions to the human.
- **Slicer:** converts the accepted requirement and design into a dependency
  graph of ready-to-implement slices.
- **Design Reviewer:** independently reviews the requirement, design, and slice
  graph for gaps, contradictions, unsafe assumptions, and missing evidence.

The Slicer's objective is not raw maximum parallelism. It should maximize
useful concurrency while preserving vertical cohesion and minimizing
integration risk.

Every slice should eventually declare:

- an observable outcome;
- dependencies;
- allowed scope;
- required evidence;
- a parallelization group or independence claim;
- integration order;
- risk tags;
- whether it touches control files, schemas, migrations, or other
  serialization-sensitive areas.

Design Review findings are routed by ownership:

- requirement gap -> Product Owner or human;
- design gap -> Designer;
- slice/dependency gap -> Slicer.

The review/fix cycle is bounded. The Design Reviewer remains read-only.

### Implementation Team

Roles:

- **N Implementers:** one seat per ready independent slice, subject to a
  configured concurrency cap.
- **Team Lead:** coordinates work, serializes integration, resolves conflicts,
  and maintains the integrated candidate.
- **Build Verifier:** runs deterministic configured checks against the
  integrated candidate.
- **Code Reviewer:** reviews maintainability, architectural fit, clarity, and
  overall code quality.

The graph engine, not the Team Lead agent, determines slice readiness from the
dependency graph.

Parallel implementation is allowed only when independence is proven. Each
writer needs a stable isolated worktree, immutable base identity, bounded
assignment, and its own CodeGraph index when CodeGraph is used. Candidate
integration is serialized, and checks are rerun after integration.

The Implementation Team emits one integrated candidate plus verification and
review evidence. Individual agent claims are not enough to advance the mission.

### QA Team

Roles/capabilities:

- **Contract Reviewer:** checks the frozen integrated candidate against the
  confirmed requirement.
- **Risk Reviewer:** searches for operational, security, concurrency,
  regression, and failure-mode risks.
- **Specialized Reviewers:** selected from slice/design risk tags when useful,
  for example accessibility, database, security, or performance review.
- **Test Designer:** creates manual acceptance scenarios and identifies which
  scenarios are automatable.
- **Automation QA:** executes applicable real-case scenarios through Playwright
  or another tool adapter.

The two primary reviewers should use different models when possible, receive
the same frozen candidate and evidence, and remain unaware of one another's
findings until both verdicts are sealed.

QA agents never fix implementation directly. Findings return to the
Implementation Team through a bounded repair edge, followed by fresh
verification and QA.

Useful safe overlap: the Test Designer may draft acceptance scenarios after
the Design Gate while implementation proceeds, then finalize and execute them
against the integrated candidate.

## Mission flow

```text
Initial Requirement
        |
        v
Product Owner
        |
        | Requirement Gate
        v
Design Team
        |
        | Design Gate
        v
Implementation Team
        |
        | Implementation Gate
        v
QA Team
        |
        | Acceptance Gate
        v
Complete
```

Bounded findings route back to their owning team. A proposed product-policy
change always returns to the Product Owner and may interrupt for human input.

## Canonical domain language

Use these terms in the new product unless later modeling disproves them:

- **Mission:** one end-to-end requirement delivery.
- **Team Run:** one team's execution within a mission.
- **Role:** a defined responsibility such as Designer or Implementer.
- **Seat:** one running agent instance assigned to a role.
- **Work Product:** a requirement, design, slice graph, candidate patch,
  review, test plan, or evidence bundle.
- **Slice:** an independently deliverable implementation assignment.
- **Gate:** deterministic conditions required to advance the mission.
- **Question:** a request for missing information.
- **Decision:** an accepted answer, possibly requiring the human.
- **Finding:** a structured reviewer or QA observation.
- **Candidate:** the integrated implementation being evaluated.
- **Checkpoint:** durable graph state after a meaningful step.
- **Interrupt:** a durable pause waiting for external input or authorization.

"Galaxy" may be used as a product or TUI metaphor, but should not replace the
precise core terms Mission and Team Run.

## Work products and ownership

| Team | Canonical work product | Gate |
| --- | --- | --- |
| Product Owner | Confirmed Requirement | Required decisions are confirmed |
| Design Team | Technical Design and Slice Graph | Design review has no blockers |
| Implementation Team | Integrated Candidate and evidence | Checks and code review pass |
| QA Team | Test Plan, independent reviews, scenario evidence | Acceptance scenarios pass |

Only the role/team that owns a work product may propose its replacement.
The Mission Controller validates freshness, required evidence, and gate rules
before accepting it.

## Graph and persistence semantics

The desired model is LangGraph-like, whether or not LangGraph is selected as
the first engine adapter:

- parent mission graph with team subgraphs;
- checkpoint after meaningful node completion;
- typed state and typed node outputs;
- durable human interrupts and resume;
- parallel nodes only when dependencies allow;
- replay, stale-output rejection, and fault recovery;
- per-invocation memory for independent workers/reviewers;
- per-thread memory only for roles that genuinely need a continuing
  conversation, such as Product Owner;
- successful sibling outputs survive another node's failure;
- side effects before an interrupt or retry are idempotent.

Keep the domain independent of LangGraph-specific types. Define a graph-engine
seam so a LangGraph adapter, deterministic in-memory adapter, or future custom
engine can execute the same mission semantics.

## Questions, decisions, and direct interaction

Agents should not coordinate by scraping or typing into one another's terminal
buffers. Use typed durable events such as:

```text
QuestionRaised
AnswerProposed
DecisionRequired
DecisionConfirmed
WorkProductSubmitted
FindingReported
SliceAssigned
CandidateIntegrated
GatePassed
GateBlocked
```

The human may attach directly to a seat. Direct conversation does not broaden
the seat's permissions or silently alter mission state. A product answer becomes
authoritative only after it is confirmed as a Decision and materialized into
the appropriate work product.

Every mutating assignment should include the mission, team, role, input hashes,
scope, permitted actions, worktree, and assignment generation. Late output from
a stale generation is rejected even if it appears useful.

## Agent runtime and terminal integration

Persistent agent terminals are valuable for visibility, attachment, and
recovery, but should sit behind an adapter seam.

Likely adapters:

- Herdr team/session adapter;
- cmux adapter for macOS;
- current runner-native or headless process adapter;
- deterministic fake adapter for tests.

Keep the terminal/session runtime seam separate from the coding-agent runner
seam. Avoid an adapter matrix such as Herdr+OpenCode, Herdr+Claude,
cmux+OpenCode, and cmux+Claude.

The first product version should use an existing multiplexer/runtime rather
than building PTY persistence and terminal multiplexing from scratch. The new
TUI should consume durable mission projections and runtime observations.

## TUI responsibilities

The TUI is a projection and command surface, not the state owner.

Initial views should include:

- mission graph and current gate;
- team and role tree;
- active seats, models, and statuses;
- pending human questions and decisions;
- slice dependency graph and ready slices;
- worktree/integration status;
- findings and their owners;
- verification and QA evidence;
- checkpoint timeline;
- direct attachment to an agent terminal.

If the TUI exits, the mission continues. Restarting it reconstructs the view
from durable mission state and runtime observations.

## Safety and workflow invariants

- One authoritative Mission Controller owns transitions.
- Agent or terminal status never proves a gate passed.
- Product decisions cannot be invented silently.
- Direct human interaction cannot bypass role permissions.
- Canonical work products are versioned and hash-addressed.
- Parallel writers never share one mutable checkout.
- Integration is serialized.
- Reviews operate on frozen candidates.
- Independent reviewers remain independent until verdict fan-in.
- Repair loops are bounded.
- Sensitive external actions require explicit authorization.
- A crash or restart resumes from the last accepted checkpoint, not model
  memory.

## Recommended first vertical slice

Prove the new orchestration model before implementation worktrees or a full
TUI:

1. Accept one initial requirement.
2. Run the Product Owner.
3. Interrupt for one human decision.
4. Persist and resume from that checkpoint.
5. Run Designer, Slicer, and Design Reviewer.
6. Route one Design Reviewer finding to its owner.
7. Produce an accepted Confirmed Requirement, Technical Design, and Slice
   Graph.
8. Restart the process and reconstruct the mission correctly.

Use deterministic fake agents for vertical tests and one real agent runtime for
the smoke test.

Acceptance criteria for this slice:

- the same mission can resume after process restart;
- a human interrupt survives restart;
- no agent output advances state without gate validation;
- a stale work-product submission is rejected;
- a reviewer finding returns to the correct owner;
- a bounded correction produces a new versioned work product;
- the final slice graph preserves explicit dependencies and concurrency claims;
- the complete mission state is inspectable without reading model memory.

## Suggested delivery sequence

1. Domain model, mission state, checkpoints, and fake graph engine.
2. Product Owner with durable human interrupt.
3. Design Team review/correction loop and Slice Graph.
4. Runtime adapter and direct agent attachment.
5. Implementation worktrees, readiness scheduling, and serialized integration.
6. Build verification and implementation code review.
7. Independent QA review and Test Designer.
8. Automation QA adapters.
9. TUI projections and terminal attachment.
10. Specialized reviewers and carefully bounded topology extension.

## Foundational decisions still open

The next session should resolve or explicitly defer:

1. Product name. "Galaxy" is available as a metaphor but not yet accepted as
   the canonical product name.
2. New repository name and filesystem location.
3. Implementation language and runtime.
4. Whether LangGraph is the first engine adapter or only an architectural
   reference.
5. Durable checkpoint store for the first version.
6. First real agent runner and model-provider integration.
7. First terminal runtime adapter: Herdr, cmux, or headless process runner.
8. TUI framework.
9. Exact human authorization policy beyond product decisions.
10. Initial concurrency cap and resource/cost budgets.
11. Whether Product Owner grilling defaults to standard or rigorous mode.
12. Which specialized QA reviewers are in scope for the first release.

Do not let these open choices delay the deterministic fake-agent vertical
slice unless they materially affect its interface.

## Explicit non-goals for the first slice

- Arbitrary user-defined teams or roles.
- Parallel implementation.
- A production-ready TUI.
- Building a terminal multiplexer.
- Automatic commits, pushes, releases, or deployments.
- Unbounded agent-to-agent conversation.
- Long-term semantic memory.
- Multiple simultaneous missions sharing one checkout.
- Replacing or modifying Higurashi Loop.

## References for the next session

- Existing Higurashi repository: `/home/jp/code/higurashi-loop`
- LangGraph persistence:
  <https://docs.langchain.com/oss/python/langgraph/persistence>
- LangGraph interrupts:
  <https://docs.langchain.com/oss/python/langgraph/interrupts>
- LangGraph subgraphs:
  <https://docs.langchain.com/oss/python/langgraph/use-subgraphs>
- Herdr documentation: <https://herdr.dev/docs/>
- cmux overview: <https://cmux.com/>

## Suggested opening prompt for the new session

```text
Read NEW_PRODUCT_HANDOFF.md completely. We are starting a separate product;
do not modify Higurashi Loop except when explicitly asked. Use the handoff's
domain language and treat its safety/workflow invariants as constraints.

First, help me resolve the product name, repository location, implementation
stack, graph-engine approach, and first runtime adapter. Then create the new
repository's domain glossary and an implementation-ready first requirement for
the fake-agent Product Owner + Design Team vertical slice. Use Higurashi Loop
as the delivery process where appropriate, but do not make the new product
depend on Higurashi at runtime.
```
