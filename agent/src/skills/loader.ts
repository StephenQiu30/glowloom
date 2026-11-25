import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export interface DeclarativeSkill {
  name: string;
  description: string;
  role?: string;
  version?: string;
  tags: string[];
  allowedTools: string[];
  instructions: string;
  sourcePath?: string;
}

function parseFrontmatter(raw: string): {
  meta: Record<string, string | string[]>;
  body: string;
} {
  const match = raw.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/);
  if (!match) {
    return { meta: {}, body: raw.trim() };
  }
  const yamlBlock = match[1] ?? '';
  const body = (match[2] ?? '').trim();
  const meta: Record<string, string | string[]> = {};
  let currentArrayKey: string | undefined;

  for (const line of yamlBlock.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;
    if (trimmed.startsWith('- ') && currentArrayKey) {
      const existing = meta[currentArrayKey];
      if (Array.isArray(existing)) {
        existing.push(trimmed.slice(2).trim());
      }
      continue;
    }
    const colon = trimmed.indexOf(':');
    if (colon <= 0) continue;
    const key = trimmed.slice(0, colon).trim();
    const value = trimmed.slice(colon + 1).trim();
    if (value === '') {
      meta[key] = [];
      currentArrayKey = key;
    } else {
      meta[key] = value;
      currentArrayKey = undefined;
    }
  }
  return { meta, body };
}

async function scanSkillsDirectory(dir: string): Promise<DeclarativeSkill[]> {
  let entries: string[];
  try {
    entries = await readdir(dir);
  } catch {
    return [];
  }

  const found: DeclarativeSkill[] = [];
  for (const entry of entries.sort()) {
    const skillFile = path.join(dir, entry, 'SKILL.md');
    try {
      const raw = await readFile(skillFile, 'utf8');
      const { meta, body } = parseFrontmatter(raw);
      const name = typeof meta.name === 'string' ? meta.name : entry;
      const description =
        typeof meta.description === 'string' ? meta.description : '';
      const allowedTools = Array.isArray(meta.allowed_tools)
        ? meta.allowed_tools
        : [];
      const tags = Array.isArray(meta.tags) ? meta.tags : [];
      found.push({
        name,
        description,
        ...(typeof meta.role === 'string' ? { role: meta.role } : {}),
        ...(typeof meta.version === 'string' ? { version: meta.version } : {}),
        tags,
        allowedTools,
        instructions: body,
        sourcePath: skillFile,
      });
    } catch {
      // Ignore non-skill subdirectories.
    }
  }
  return found;
}

/**
 * Dynamic, hot-reloadable Skill registry modeled after BeefTV's SKILL.md plugin architecture.
 * Supports multiple skill roots (built-in + mounted/external directories) and runtime registration
 * with zero code changes or recompilation.
 */
export class SkillRegistry {
  private readonly skills = new Map<string, DeclarativeSkill>();
  private readonly dirs: string[];

  constructor(dirs?: string[]) {
    const currentDir = path.dirname(fileURLToPath(import.meta.url));
    const defaultDir = path.resolve(currentDir, '../../skills');
    const extraEnvDirs = (process.env.GLOWLOOM_AGENT_SKILLS_DIRS ?? '')
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    this.dirs = dirs ?? [defaultDir, ...extraEnvDirs];
  }

  async reload(): Promise<DeclarativeSkill[]> {
    this.skills.clear();
    for (const dir of this.dirs) {
      const loaded = await scanSkillsDirectory(dir);
      for (const skill of loaded) {
        this.skills.set(skill.name, skill);
      }
    }
    return this.list();
  }

  register(skill: DeclarativeSkill): void {
    this.skills.set(skill.name, skill);
  }

  get(name: string): DeclarativeSkill | undefined {
    return this.skills.get(name);
  }

  list(): DeclarativeSkill[] {
    return Array.from(this.skills.values());
  }

  resolveAllowedTools(skillNames: readonly string[]): string[] {
    const allowed = new Set<string>();
    for (const name of skillNames) {
      const skill = this.skills.get(name);
      if (!skill) continue;
      for (const tool of skill.allowedTools) {
        allowed.add(tool);
      }
    }
    return Array.from(allowed);
  }

  composeInstructions(skillNames: readonly string[]): string {
    const sections: string[] = [];
    for (const name of skillNames) {
      const skill = this.skills.get(name);
      if (!skill) continue;
      sections.push(
        `## Active Skill: ${skill.name}\n${skill.description ? `${skill.description}\n\n` : ''}${skill.instructions}`,
      );
    }
    return sections.join('\n\n---\n\n');
  }
}

export async function loadDeclarativeSkills(
  skillsDir?: string,
): Promise<DeclarativeSkill[]> {
  const registry = new SkillRegistry(skillsDir ? [skillsDir] : undefined);
  return registry.reload();
}

export function filterToolsByAllowlist<T extends Record<string, unknown>>(
  tools: T,
  allowedTools: readonly string[],
): Partial<T> {
  if (allowedTools.length === 0) {
    return {};
  }
  const allowSet = new Set(allowedTools);
  const filtered: Partial<T> = {};
  for (const [key, value] of Object.entries(tools)) {
    const bareName = key.includes('_') ? key.slice(key.indexOf('_') + 1) : key;
    if (allowSet.has(key) || allowSet.has(bareName)) {
      filtered[key as keyof T] = value as T[keyof T];
    }
  }
  return filtered;
}
