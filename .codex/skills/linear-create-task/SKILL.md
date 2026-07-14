---
name: linear-create-task
description: Use when creating a new Linear task or rewriting a Linear task draft so the issue contains a complete execution template and the correct execution labels for routing and review.
---

# Linear Create Task


## Rules

- Create or rewrite the issue only when the task scope is concrete enough to execute.
- Keep the issue scoped to one deliverable or one tightly related change set.
- Write the body so it can stand alone as the source of truth for implementation kickoff.
- Add the correct Linear labels during task creation; do not leave routing or reviewer labels implicit.
- Require explicit scope boundaries; do not leave room for open-ended compatibility extras or non-essential tests.
- Make validation executable. Prefer commands, checklists, or concrete review evidence over vague statements.

## Required Metadata

Every created task must include the labels needed for execution routing and review.

- Always determine the repo or ownership label expected by the current project.
- Always add the execution agent label when the repo uses agent-based routing, for example `agent:claude`.
- Always add the reviewer label when the repo uses reviewer routing, for example `reviewer:claude`.
- Add any stable workflow labels required by the project or task family, such as repo, phase, platform, or dispatch labels.
- If the workspace already has established label conventions, reuse them exactly instead of inventing near-duplicates.
- If the correct labels cannot be inferred from repo context, existing issues, or explicit user instructions, stop and ask for the missing label choice before creating the task.

Typical label classes to check:

- execution: `agent:*`
- review: `reviewer:*`
- repo ownership: `repo:*`
- delivery phase: `phase:*`
- team or dispatch routing: project-specific operational labels

## Required Fields

Use this structure in the Linear issue body:

```md
## Goal
- [clear business or engineering outcome]

## Task Description
- Background:
- Expected change:

## Scope Boundaries
- In scope:
- Out of scope:
- Explicit non-goals:

## Acceptance Criteria
- [observable requirement]
- [observable requirement]

## Validation
- Automated:
- Manual:
- Agent Review focus:

## Deliverables
- [code / doc / spec / workflow artifact]
```

## Writing Guidance

### Goal

- State the desired end state, not the implementation plan.
- Prefer one sentence plus one measurable outcome.

### Task Description

- Capture the problem, target behavior, and why the task exists.

### Scope Boundaries

- Tighten boundaries aggressively.
- Call out what must not be added, especially compatibility padding, exploratory extras, and non-essential test expansions.

### Acceptance Criteria

- Write criteria the reviewer can falsify.
- Avoid vague phrases such as "works correctly" or "looks good".

### Validation

- `Automated`: list the exact useful tests or checks that should run.
- `Manual`: only include actions that validate user-visible or workflow-visible outcomes.
- `Agent Review focus`: tell the reviewer what implementation-to-spec gaps, boundary drift, and code quality risks to inspect.

## Label Guidance

- Prefer the smallest complete label set that preserves routing, review ownership, and reporting.
- Match existing spelling and punctuation exactly, including prefixes such as `agent:` and `reviewer:`.
- Reuse labels already visible in nearby tasks, project docs, or repo instructions before proposing new ones.
- When the user explicitly specifies labels, treat them as required unless they conflict with an existing project rule.
- Mention the chosen labels in the task creation summary so the operator can verify routing at a glance.

## Task Template

```md
## Goal
- <what outcome must be true after this task lands>

## Task Description
- Background: <current problem or missing capability>
- Expected change: <what should be implemented or updated>

## Scope Boundaries
- In scope: <what is allowed>
- Out of scope: <what is intentionally excluded>
- Explicit non-goals: <compatibility padding, wording-only checks, unrelated cleanup, etc.>

## Acceptance Criteria
- <criterion 1>
- <criterion 2>
- <criterion 3>

## Validation
- Automated: <exact test, lint, build, or verification commands worth running>
- Manual: <manual checks only if they add signal>

## Deliverables
```

## Final Check Before Creating The Issue

- Title is specific and implementation-scoped.
- Body includes all required fields.
- Correct labels are attached for execution, review, and repo/project routing.
- Validation is concrete and useful.
- Boundaries are explicit.
