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

## Subject format and allowed commit types

Every subject must use `type(scope): 中文说明`. The type and scope stay in
lowercase English; the summary and body use Chinese except for commands, paths,
code identifiers, and fixed trailers such as `Co-authored-by`.

- `test(<scope>):` — failing tests, fixtures, mocks, acceptance scripts, test-only expectations
- `impl(<scope>):` — smallest implementation that makes existing red tests pass
- `feat(<scope>):` — user-visible capability or behavior changes (after prior `test` unless documented as not scriptable)
- `refactor(<scope>):` — behavior-preserving cleanup after tests are green
- `docs(<scope>):` — documentation, examples, and acceptance notes
- `chore(<scope>):` — CI, configuration, dependency metadata, generated housekeeping

The scope is required and names a specific business module or engineering area,
such as `auth`, `frontend`, `storyboard`, `docs`, or `ci`. Do not use vague
scopes such as `app` or `misc`.

For feature or behavior work, preserve order: `test(<scope>):` first, then
`impl(<scope>):`/`feat(<scope>):`, then optional `refactor(<scope>):`,
`docs(<scope>):`, or `chore(<scope>):`.

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
8. Choose the allowed type and the narrowest meaningful scope that match the staged diff. Do not use `fix` or an unscoped subject.
9. Write a concise Chinese subject, <= 72 characters. Format: `<type>(<scope>): <中文说明>`.
10. Write the summary, rationale, and tests or validation result in Chinese (or explain in Chinese why validation was not run).
11. Append `Co-authored-by: Codex <codex@openai.com>` unless the user requests a different identity.
12. Wrap body lines at 72 characters.
13. Create the commit message with a here-doc or temp file and use `git commit -F <file>`.
14. Commit only when the message matches the staged changes.

## Output

- A single commit whose message reflects the session and discipline above.

## Template

```
<type>(<scope>): <中文说明>

摘要：
- <修改内容>

原因：
- <修改原因>

验证：
- <命令与结果，或“未运行（原因）”>

Co-authored-by: Codex <codex@openai.com>
```
