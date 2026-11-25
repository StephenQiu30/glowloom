import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { Langfuse } from 'langfuse';
import { LangfuseExporter } from '@mastra/langfuse';
import type { Logger } from 'pino';

export interface LangfuseConfig {
  baseUrl: string;
  publicKey: string;
  secretKey: string;
}

interface CachedPrompt {
  content: string;
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
    const currentDir = path.dirname(fileURLToPath(import.meta.url));
    this.promptsDir =
      promptsDir ?? path.resolve(currentDir, '../../prompts');
    if (config) {
      this.client = new Langfuse({
        baseUrl: config.baseUrl,
        publicKey: config.publicKey,
        secretKey: config.secretKey,
      });
    }
  }

  async resolve(name: string, fallbackText?: string): Promise<string> {
    const now = Date.now();
    const cached = this.cache.get(name);
    if (cached && cached.expiresAt > now) {
      return cached.content;
    }

    if (this.client) {
      try {
        const prompt = await this.client.getPrompt(name, undefined, {
          cacheTtlSeconds: 60,
          type: 'text',
        });
        const compiled = prompt.compile({});
        if (compiled.trim().length > 0) {
          this.cache.set(name, {
            content: compiled,
            expiresAt: now + promptTtlMs,
          });
          return compiled;
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
    this.cache.set(name, { content: local, expiresAt: now + promptTtlMs });
    return local;
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
