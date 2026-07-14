#!/usr/bin/env bash
set -euo pipefail

required_files=(
  "README.md"
  "AGENTS.md"
  "AGENTS.local.md"
  "WORKFLOW.md"
  ".env.example"
  ".codex/skills/agent-browser/SKILL.md"
  ".codex/skills/harness-local-server/SKILL.md"
  ".codex/skills/harness-playwright-evidence/SKILL.md"
  ".codex/skills/harness-linear-loop/SKILL.md"
  ".codex/skills/harness-quality-gate/SKILL.md"
  ".codex/skills/debug/SKILL.md"
  ".codex/skills/commit/SKILL.md"
  ".codex/skills/pull/SKILL.md"
  ".codex/skills/push/SKILL.md"
  ".codex/skills/land/SKILL.md"
  ".codex/skills/land/land_watch.py"
  ".codex/skills/linear/SKILL.md"
  ".github/pull_request_template.md"
  "docs/README.md"
  "docs/prd/README.md"
  "docs/plans/README.md"
  "docs/design/README.md"
  "docs/acceptance/README.md"
  "docs/operations/README.md"
)

for file in "${required_files[@]}"; do
  test -f "$file"
done

grep -q "tracker:" WORKFLOW.md
grep -q "kind: linear" WORKFLOW.md
grep -q "project_slug" WORKFLOW.md
grep -q "## Codex Workpad" WORKFLOW.md
grep -q "Human Review" WORKFLOW.md
grep -q "harness-quality-gate" WORKFLOW.md

test ! -d .agents
test ! -f skills-lock.json

git diff --check
