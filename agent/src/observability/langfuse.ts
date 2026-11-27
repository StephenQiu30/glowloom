import { readFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import path from 'node:path';
import { resourceDirectory } from '../config.js';
import { Langfuse } from 'langfuse';
import { LangfuseExporter } from '@mastra/langfuse';
import type { Logger } from 'pino';
import { z } from 'zod';

export const promptByRole = {
  producer: 'producer',
  scriptwriter: 'scriptwriter',
  asset_director: 'asset-director',
  production: 'production',
  continuity_reviewer: 'continuity-reviewer',
} as const;
const promptNames = Object.values(promptByRole);
const promptRevisionSchema = z.object({
  text: z.string().min(1),
  digest: z.string().regex(/^[a-f0-9]{64}$/),
  remoteVersion: z.number().int().positive().optional(),
});
export const promptSnapshotSchema = z.record(
  z.enum(promptNames),
  promptRevisionSchema,
);
export type PromptSnapshot = z.infer<typeof promptSnapshotSchema>;
type PromptRevision = z.infer<typeof promptRevisionSchema>;

export interface LangfuseConfig {
  baseUrl: string;
  publicKey: string;
  secretKey: string;
}

interface CachedPrompt {
  revision: PromptRevision;
  expiresAt: number;
}

const promptTtlMs = 60_000;

export class PromptRegistry {
  private readonly client: Langfuse | undefined;
  private readonly cache = new Map<string, CachedPrompt>();
  private readonly promptsDir: string;

  constructor(
    config: LangfuseConfig | undefined,
    private readonly logger: Logger,
    promptsDir?: string,
  ) {
    this.promptsDir = promptsDir ?? resourceDirectory('prompts');
    if (config) {
      this.client = new Langfuse({
        baseUrl: config.baseUrl,
        publicKey: config.publicKey,
        secretKey: config.secretKey,
      });
    }
  }

  async resolve(name: string, fallbackText?: string): Promise<string> {
    return (await this.resolveRevision(name, fallbackText)).text;
  }

  async validateLocal(): Promise<void> {
    await Promise.all(
      promptNames.map(async (name) => {
        const file = path.join(this.promptsDir, `${name}.md`);
        if (!(await readFile(file, 'utf8')).trim()) {
          throw new Error(`Required local prompt is empty: ${file}`);
        }
      }),
    );
  }

  async snapshot(): Promise<PromptSnapshot> {
    const revisions = await Promise.all(
      promptNames.map(
        async (name) => [name, await this.resolveRevision(name)] as const,
      ),
    );
    return promptSnapshotSchema.parse(Object.fromEntries(revisions));
  }

  private async resolveRevision(
    name: string,
    fallbackText?: string,
  ): Promise<PromptRevision> {
    const now = Date.now();
    const cached = this.cache.get(name);
    if (cached && cached.expiresAt > now) {
      return cached.revision;
    }

    if (this.client) {
      try {
        const prompt = await this.client.getPrompt(name, undefined, {
          cacheTtlSeconds: 60,
          type: 'text',
        });
        const compiled = prompt.compile({});
        if (compiled.trim().length > 0) {
          return this.cacheRevision(name, compiled, now, prompt.version);
        }
      } catch (error: unknown) {
        this.logger.warn(
          {
            promptName: name,
            err: error instanceof Error ? error.message : String(error),
          },
          'Langfuse prompt fetch failed; falling back to local prompt',
        );
      }
    }

    const local = await this.readLocalPrompt(name, fallbackText);
    return this.cacheRevision(name, local, now);
  }

  private cacheRevision(
    name: string,
    text: string,
    now: number,
    remoteVersion?: number,
  ): PromptRevision {
    const revision = {
      text,
      digest: createHash('sha256').update(text).digest('hex'),
      ...(remoteVersion === undefined ? {} : { remoteVersion }),
    };
    this.cache.set(name, { revision, expiresAt: now + promptTtlMs });
    return revision;
  }

  async shutdown(): Promise<void> {
    if (this.client) {
      await this.client.shutdownAsync();
    }
  }

  private async readLocalPrompt(
    name: string,
    fallbackText?: string,
  ): Promise<string> {
    const safeName = name.replace(/[^a-zA-Z0-9_-]/g, '');
    const filePath = path.join(this.promptsDir, `${safeName}.md`);
    try {
      return (await readFile(filePath, 'utf8')).trim();
    } catch {
      return (
        fallbackText ??
        `You are the Glowloom ${safeName} creative assistant. Follow project truth via MCP tools.`
      );
    }
  }
}

export function createLangfuseExporter(
  config: LangfuseConfig | undefined,
): LangfuseExporter | undefined {
  if (!config) {
    return undefined;
  }
  return new LangfuseExporter({
    publicKey: config.publicKey,
    secretKey: config.secretKey,
    baseUrl: config.baseUrl,
  });
}
