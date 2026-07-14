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
  if [[ ! -f "$file" ]]; then
    printf 'Missing required file: %s\n' "$file" >&2
    exit 1
  fi
done

for script in scripts/*.sh; do
  [[ -f "$script" ]] || continue
  bash -n "$script"
done

python -m py_compile .codex/skills/land/land_watch.py

if [[ -d .agents ]]; then
  printf 'Unexpected generated directory: .agents\n' >&2
  exit 1
fi

if [[ -f skills-lock.json ]]; then
  printf 'Unexpected generated lock file: skills-lock.json\n' >&2
  exit 1
fi
