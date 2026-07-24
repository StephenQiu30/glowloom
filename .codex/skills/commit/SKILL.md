---
name: commit
description:
  Create a well-formed git commit from current changes using session history for
  rationale and summary; use when asked to commit, prepare a commit message, or
  finalize staged work.
---

# Commit

## Goals

- Produce a commit that reflects the actual code changes and the session context.
- Follow all `AGENTS.md` and applicable `AGENTS.local.md` rules (allowed types and test-first ordering).
- Include both summary and rationale in the body.

## Allowed commit types

Only these subject prefixes are allowed:

- `test:` — failing tests, fixtures, mocks, acceptance scripts, test-only expectations
- `impl:` — smallest implementation that makes existing red tests pass
- `feat:` — user-visible capability or behavior changes (after prior `test:` unless documented as not scriptable)
- `refactor:` — behavior-preserving cleanup after tests are green
- `docs:` — documentation, examples, and acceptance notes
- `chore:` — CI, configuration, dependency metadata, generated housekeeping

For feature or behavior work, preserve order: `test:` first, then `impl:`/`feat:`, then optional `refactor:`, `docs:`, or `chore:`.

Do not mix unrelated types in one commit. Split by type when practical.

## Inputs

- Session history for intent and rationale.
- The true Git root, current branch, `git status`, `git diff`, and
  `git diff --staged` for actual changes.
- `AGENTS.md`, every applicable `AGENTS.local.md`, and the current task context.

## Steps

1. Resolve the true repository root with `git rev-parse --show-toplevel`, then
   read every `AGENTS.md` and applicable `AGENTS.local.md` for the current path.
2. Read session history to identify scope, intent, and rationale.
3. Confirm the current branch with `git branch --show-current` and inspect
   `git status`; if the branch name is empty, stop and report detached HEAD.
4. Inspect the working tree and staged changes (`git diff`, `git diff --staged`).
5. Build an explicit whitelist of task-owned paths, then stage only those paths with
   `git add -- <path>...`; do not use `git add -A`, broad globs, or repository-wide
   staging shortcuts.
6. Compare `git diff --name-only --staged` with the whitelist and inspect
   `git diff --staged`; exclude unrelated files, secrets, caches, logs, build
   artifacts, generated output, and temporary files before committing.
7. If the staged set differs from the whitelist, staging is incomplete, or a path
   cannot be attributed to the task, fix the index or ask for confirmation.
8. Choose the allowed type that matches the staged diff. Do not use `fix:` or scoped conventional types unless documented as an exception.
9. Write a subject line in imperative mood, <= 72 characters. Format: `<type> <short summary>`.
10. Write a body with summary, rationale, and tests or validation run (or why not run).
11. Append `Co-authored-by: Codex <codex@openai.com>` unless the user requests a different identity.
12. Wrap body lines at 72 characters.
13. Create the commit message with a here-doc or temp file and use `git commit -F <file>`.
14. Commit only when the message matches the staged changes.

## Output

- A single commit whose message reflects the session and discipline above.

## Template

```
<type> <short summary>

Summary:
- <what changed>

Rationale:
- <why>

Tests:
- <command or "not run (reason)">

Co-authored-by: Codex <codex@openai.com>
```
