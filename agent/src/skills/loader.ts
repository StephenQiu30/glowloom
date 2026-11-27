import { createHash } from 'node:crypto';
import { isUtf8 } from 'node:buffer';
import { readFile, readdir } from 'node:fs/promises';
import path from 'node:path';
import { validateSkillContent } from '@mastra/core/skills';
import {
  InMemoryBlobStore,
  type SkillVersionTree,
  type StorageBlobEntry,
} from '@mastra/core/storage';
import {
  CompositeVersionedSkillSource,
  Workspace,
} from '@mastra/core/workspace';
import { parse } from 'yaml';
import { z } from 'zod';

import { resourceDirectory } from '../config.js';

export const declarativeSkillSchema = z.object({
  name: z.string(),
  description: z.string(),
  instructions: z.string(),
  allowedTools: z.array(z.string()),
});
export type DeclarativeSkill = z.infer<typeof declarativeSkillSchema>;

const fileSchema = z.object({
  blobHash: z.string().regex(/^[a-f0-9]{64}$/),
  size: z.number().int().nonnegative(),
  mimeType: z.string().optional(),
  encoding: z.enum(['utf-8', 'base64']).optional(),
});

export const pinnedSkillSchema = declarativeSkillSchema.extend({
  digest: z.string().regex(/^[a-f0-9]{64}$/),
  tree: z.object({ entries: z.record(z.string(), fileSchema) }),
  blobs: z.array(
    z.object({
      hash: z.string(),
      content: z.string(),
      size: z.number().int().nonnegative(),
      mimeType: z.string().optional(),
      createdAt: z.coerce.date(),
    }),
  ),
});
export type PinnedSkill = z.infer<typeof pinnedSkillSchema>;

async function readPackage(directory: string) {
  const files = new Map<string, Buffer>();
  async function walk(relative: string) {
    const entries = await readdir(path.join(directory, relative), {
      withFileTypes: true,
    });
    for (const entry of entries) {
      const file = path.posix.join(relative, entry.name);
      if (entry.isDirectory()) {
        await walk(file);
      } else if (entry.isFile()) {
        files.set(file, await readFile(path.join(directory, file)));
      } else {
        throw new Error(`Unsupported entry in Skill package: ${file}`);
      }
    }
  }
  await walk('');
  return files;
}

export function pinSkill(
  name: string,
  files: ReadonlyMap<string, Buffer>,
): PinnedSkill {
  const content = files.get('SKILL.md');
  if (!content || !isUtf8(content) || content.includes(0))
    throw new Error(`Missing or invalid UTF-8 SKILL.md: ${name}`);
  const body = content.toString('utf8');
  const validation = validateSkillContent({
    content: body,
    directoryName: name,
  });
  if (!validation.valid || !validation.instructions) {
    throw new Error(`Invalid Skill ${name}: ${validation.errors.join(', ')}`);
  }
  const meta = z
    .object({
      name: z.string(),
      description: z.string(),
      'allowed-tools': z.union([z.string(), z.array(z.string())]).optional(),
    })
    .parse(
      parse(/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/.exec(body)?.[1] ?? ''),
    );
  const tools = meta['allowed-tools'];
  const tree: PinnedSkill['tree'] = { entries: Object.create(null) };
  const blobs = new Map<string, PinnedSkill['blobs'][number]>();
  const identities = [];
  for (const file of [...files.keys()].sort()) {
    if (
      file.startsWith('/') ||
      file.includes('\\') ||
      file.includes('\0') ||
      file.split('/').some((part) => !part || part === '.' || part === '..')
    ) {
      throw new Error('Invalid Skill resource path');
    }
    const data = files.get(file);
    if (!data) throw new Error(`Missing Skill file: ${file}`);
    const hash = createHash('sha256').update(data).digest('hex');
    const encoding = isUtf8(data) && !data.includes(0) ? 'utf-8' : 'base64';
    tree.entries[file] = { blobHash: hash, size: data.length, encoding };
    blobs.set(hash, {
      hash,
      content: data.toString(encoding),
      size: data.length,
      createdAt: new Date(0),
    });
    identities.push([file, hash]);
  }
  return {
    name,
    description: meta.description,
    instructions: validation.instructions,
    allowedTools:
      typeof tools === 'string'
        ? tools.split(/\s+/).filter(Boolean)
        : (tools ?? []),
    digest: createHash('sha256')
      .update(JSON.stringify(identities))
      .digest('hex'),
    tree,
    blobs: [...blobs.values()],
  };
}

export async function skillWorkspace(
  packages: readonly PinnedSkill[],
): Promise<Workspace> {
  const names = new Set<string>();
  const blobs = new InMemoryBlobStore();
  const entries = [];
  for (const pkg of packages) {
    if (names.has(pkg.name)) throw new Error(`Duplicate Skill: ${pkg.name}`);
    names.add(pkg.name);
    const tree: SkillVersionTree = { entries: Object.create(null) };
    for (const [name, file] of Object.entries(pkg.tree.entries)) {
      tree.entries[name] = {
        blobHash: file.blobHash,
        size: file.size,
        ...(file.mimeType === undefined ? {} : { mimeType: file.mimeType }),
        ...(file.encoding === undefined ? {} : { encoding: file.encoding }),
      };
    }
    await blobs.putMany(
      pkg.blobs.map((file): StorageBlobEntry => ({
        ...file,
        mimeType: file.mimeType ?? '',
      })),
    );
    entries.push({ dirName: pkg.name, tree, versionCreatedAt: new Date(0) });
  }
  return new Workspace({
    skills: ['/'],
    skillSource: new CompositeVersionedSkillSource(entries, blobs),
  });
}

export class SkillRegistry {
  private skills = new Map<string, DeclarativeSkill>();
  private pending: Promise<PinnedSkill[]> | undefined;
  constructor(private readonly dirs = [resourceDirectory('skills')]) {}

  discover(): Promise<PinnedSkill[]> {
    this.pending ??= this.scan().finally(() => {
      this.pending = undefined;
    });
    return this.pending;
  }

  private async scan(): Promise<PinnedSkill[]> {
    const packages = new Map<string, PinnedSkill>();
    for (const dir of this.dirs) {
      const entries = await readdir(dir, { withFileTypes: true });
      for (const entry of entries.sort((a, b) =>
        a.name.localeCompare(b.name),
      )) {
        if (entry.name.startsWith('.')) continue;
        if (entry.isSymbolicLink())
          throw new Error(`Unsupported Skill symlink: ${entry.name}`);
        if (!entry.isDirectory()) continue;
        const directory = path.join(dir, entry.name);
        const contents = await readdir(directory, { withFileTypes: true });
        if (!contents.some((file) => file.name === 'SKILL.md')) continue;
        if (packages.has(entry.name))
          throw new Error(`Duplicate Skill: ${entry.name}`);
        packages.set(
          entry.name,
          pinSkill(entry.name, await readPackage(directory)),
        );
      }
    }
    return [...packages.values()];
  }

  async reload(): Promise<DeclarativeSkill[]> {
    const packages = await this.discover();
    this.skills = new Map(packages.map((pkg) => [pkg.name, pkg]));
    return this.list();
  }
  register(skill: DeclarativeSkill): void {
    this.skills.set(skill.name, skill);
  }
  get(name: string): DeclarativeSkill | undefined {
    return this.skills.get(name);
  }
  list(): DeclarativeSkill[] {
    return [...this.skills.values()];
  }
}

export function loadDeclarativeSkills(
  dir?: string,
): Promise<DeclarativeSkill[]> {
  return new SkillRegistry(dir ? [dir] : undefined).reload();
}

export function filterToolsByAllowlist<T>(
  tools: Record<string, T>,
  allowedTools: readonly string[],
): Record<string, T> {
  const allowSet = new Set(allowedTools);
  return Object.fromEntries(
    Object.entries(tools).filter(
      ([key]) =>
        allowSet.has(key) || allowSet.has(key.replace(/^glowloom_/, '')),
    ),
  );
}
