# Higurashi workflow context

## Terms

### Work item

A scoped requirement with its own durable delivery artifact and lifecycle.

### Blocker

Reviewer evidence that prevents the current work item from being accepted as
implemented under its present contract. A blocker is unresolved until its
minimum acceptance condition is met.

### Blocker severity

The reviewer's classification of impact: `critical`, `high`, `medium`, or
`low`. Severity informs a human disposition; it does not authorize deferral
by itself.

### Disposition

The user's explicit decision about a blocker. A blocker may be repaired in the
current work item or deferred as follow-up work. Deferral accepts that the
blocker remains unresolved for the current item.

### Follow-up work item

A new, independently trackable work item created from a deferred blocker. It
retains the blocker evidence, reproduction, and minimum acceptance condition.

### Human-ordered completion

Completion of a blocked work item after the user explicitly defers every
current blocker to a named follow-up work item. It is not evidence that the
deferred blockers were fixed.
