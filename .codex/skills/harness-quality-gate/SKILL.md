---
name: harness-quality-gate
license: MIT
---


## Purpose



## When to use

- Any `Todo` / `In Progress` Linear ticket run via `WORKFLOW.md`.
- Before moving to `Human Review` or marking validation complete.

## Skill stack (order)

| Phase | Skill path | Outcome |
|-------|------------|---------|
| Linear loop | `.codex/skills/harness-linear-loop/SKILL.md` | Status, `## Codex Workpad`, PR linkage |
| Harness run | `.codex/skills/harness-local-server/SKILL.md` | App/runtime up for validation |
| E2E evidence | `.codex/skills/harness-playwright-evidence/SKILL.md` | UI flows: screenshot/trace/video in Workpad |
| Git | `commit` → `push` → `pull` → `land` skills | Typed commits and PR body per template |

## TDD + commit contract

2. Map phases to commits: `test:` (red) → `impl:` / `feat:` (green) → optional `refactor:` / `docs:` / `chore:`.
3. Record red/green commands in Workpad `Test-first Evidence` and PR `Test-first Evidence`.

## End-to-end bar

1. Unit/integration: project verify command (document in Workpad `Validation`).
2. UI/runtime: `harness-local-server` then `harness-playwright-evidence` when the ticket touches UI or user flows.

## Human Review gate

Do not move to `Human Review` until:

- [ ] Harness validation commands green on latest commit
- [ ] E2E evidence attached or linked when UI/app-touching
- [ ] PR body matches test-first template (`push` skill)

## Exceptions

