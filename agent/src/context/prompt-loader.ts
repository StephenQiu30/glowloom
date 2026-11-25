import { existsSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const defaultProducerPrompt =
  'You are the creative producer. Maintain a concrete versioned plan, delegate professional work in parallel when independent, read real evidence, and use authorized business tools. A plan or text answer never means media was generated or approved. Request a business proposal before expensive or destructive work.';

const roleGuidance: Readonly<Record<string, string>> = {
  script:
    'Break down narrative beats, dialogue, pacing, and scene structure grounded in actual script sources.',
  art: 'Define visual direction, palette, lighting, costume, and consistent character/environment references.',
  storyboard:
    'Compose concrete ordered shots with clear visible action, camera framing, and positive integer duration_ms.',
  shot: 'Prepare shot-level generation specs and verify frozen references and revisions before proposing spend.',
  voice:
    'Design voiceover, dialogue delivery, ambient sound, and audio pacing aligned to shot durations.',
  edit: 'Organize timeline pacing, transitions, and assembly checks based on adopted shot artifacts.',
  qa: 'Inspect specified artifact versions against creative intent and report concrete defects without initiating paid retries.',
};

function findPromptsRoot(): string | undefined {
  let current = dirname(fileURLToPath(import.meta.url));
  for (let i = 0; i < 6; i++) {
    const candidate = join(current, 'prompts');
    if (existsSync(candidate)) return candidate;
    const parent = dirname(current);
    if (parent === current) break;
    current = parent;
  }
  return undefined;
}

const promptsRoot = findPromptsRoot();
const cache = new Map<string, string>();

function readPromptFile(relativePath: string): string | undefined {
  if (!promptsRoot) return undefined;
  const cached = cache.get(relativePath);
  if (cached !== undefined) return cached;
  const fullPath = resolve(promptsRoot, relativePath);
  if (!existsSync(fullPath)) return undefined;
  const text = readFileSync(fullPath, 'utf8').trim();
  cache.set(relativePath, text);
  return text;
}

export function loadProducerInstructions(): string {
  return readPromptFile('producer.md') ?? defaultProducerPrompt;
}

export function loadSpecialistInstructions(role: string): string {
  const fromFile = readPromptFile(`roles/${role}.md`);
  if (fromFile) return fromFile;
  const specialty =
    roleGuidance[role] ??
    'Execute the assigned creative task accurately within authorized tool and budget limits.';
  return `You are the ${role} specialist. ${specialty} Maximum depth one; never delegate or change producer plans. Use only provided adopted sources and authorized role tools. Return artifact and source references. Plain text never establishes business completion or approval.`;
}
