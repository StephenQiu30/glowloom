import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export interface DeclarativeSkill {
  name: string;
  description: string;
  allowedTools: string[];
  instructions: string;
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

export async function loadDeclarativeSkills(
  skillsDir?: string,
): Promise<DeclarativeSkill[]> {
  const currentDir = path.dirname(fileURLToPath(import.meta.url));
  const dir = skillsDir ?? path.resolve(currentDir, '../../skills');
  let entries: string[];
  try {
    entries = await readdir(dir);
  } catch {
    return [];
  }

  const skills: DeclarativeSkill[] = [];
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
      skills.push({
        name,
        description,
        allowedTools,
        instructions: body,
      });
    } catch {
      // Ignore directories without SKILL.md.
    }
  }
  return skills;
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
