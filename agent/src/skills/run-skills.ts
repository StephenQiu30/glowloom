import { createHash } from 'node:crypto';
import { z } from 'zod';
import {
  createBusinessMcpClient,
  type BusinessMcpConfig,
} from '../business/mcp-client.js';
import type { RunIdentityHeaders } from '../business/gateway-provider.js';
import { pinSkill, type PinnedSkill } from './loader.js';

const skillPinSchema = z
  .object({
    skill_id: z.uuid(),
    version: z.string().min(1).max(100),
    resource_hash: z.string().regex(/^[a-f0-9]{64}$/),
  })
  .strict();

const bundleSchema = skillPinSchema.extend({
  name: z.string(),
  description: z.string(),
  content: z.base64(),
  resources: z.record(z.string(), z.base64()).optional(),
});

export type PrivateSkillPin = z.infer<typeof skillPinSchema>;

export type PrivateSkillBundleFetcher = (
  pin: PrivateSkillPin,
  context: { runId: string; submissionId: string; ttlMs: number },
) => Promise<unknown>;

export function runIdentifier(namespace: string, label: string): string {
  const uuid = z.uuid().parse(namespace);
  const bytes = createHash('sha1')
    .update(Buffer.from(uuid.replaceAll('-', ''), 'hex'))
    .update(label)
    .digest()
    .subarray(0, 16);
  bytes.writeUInt8((bytes.readUInt8(6) & 0x0f) | 0x50, 6);
  bytes.writeUInt8((bytes.readUInt8(8) & 0x3f) | 0x80, 8);
  const hex = bytes.toString('hex');
  return [
    hex.slice(0, 8),
    hex.slice(8, 12),
    hex.slice(12, 16),
    hex.slice(16, 20),
    hex.slice(20),
  ].join('-');
}

function extractMcpToolPayload(result: unknown): unknown {
  if (typeof result !== 'object' || result === null) return result;
  const record = result as Record<string, unknown>;
  if (record.structuredContent !== undefined) return record.structuredContent;
  if (Array.isArray(record.content)) {
    const textPart = record.content.find(
      (part): part is { type: string; text: string } =>
        typeof part === 'object' &&
        part !== null &&
        (part as Record<string, unknown>).type === 'text' &&
        typeof (part as Record<string, unknown>).text === 'string',
    );
    if (textPart) {
      return JSON.parse(textPart.text) as unknown;
    }
  }
  return result;
}

/**
 * Loads pinned private skills via the Go internal MCP Server (`@mastra/mcp`)
 * without any hand-written outbound HTTP/fetch calls.
 */
export async function loadPrivateSkills(
  config: BusinessMcpConfig,
  identity: RunIdentityHeaders,
  runId: string,
  submissionId: string,
  ttlMs: number,
  requested: unknown,
  fetcher?: PrivateSkillBundleFetcher,
): Promise<PinnedSkill[]> {
  const pins = z
    .array(skillPinSchema)
    .max(32)
    .parse(requested ?? []);
  if (pins.length === 0) return [];

  if (fetcher) {
    return loadWithFetcher(pins, { runId, submissionId, ttlMs }, fetcher);
  }

  const mcpClient = createBusinessMcpClient(config, {
    ...identity,
    runId,
  });
  try {
    const tools = await mcpClient.listTools();
    const loaderTool = Object.entries(tools).find(
      ([name]) =>
        name === 'skill_load_bundle' || name.endsWith('_skill_load_bundle'),
    )?.[1];
    if (!loaderTool || typeof loaderTool.execute !== 'function') {
      throw new Error('Go MCP server does not expose skill_load_bundle');
    }
    const execute = loaderTool.execute as (
      args: Record<string, unknown>,
    ) => Promise<unknown>;

    return await loadWithFetcher(
      pins,
      { runId, submissionId, ttlMs },
      async (pin, ctx) => {
        const raw = await execute({
          root_run_id: ctx.runId,
          submission_id: ctx.submissionId,
          lease_until: new Date(Date.now() + ctx.ttlMs).toISOString(),
          skill_id: pin.skill_id,
          version: pin.version,
          resource_hash: pin.resource_hash,
        });
        return extractMcpToolPayload(raw);
      },
    );
  } finally {
    await mcpClient.disconnect().catch(() => undefined);
  }
}

async function loadWithFetcher(
  pins: PrivateSkillPin[],
  context: { runId: string; submissionId: string; ttlMs: number },
  fetcher: PrivateSkillBundleFetcher,
): Promise<PinnedSkill[]> {
  return Promise.all(
    pins.map(async (pin) => {
      const bundle = bundleSchema.parse(await fetcher(pin, context));
      if (
        bundle.skill_id !== pin.skill_id ||
        bundle.version !== pin.version ||
        bundle.resource_hash !== pin.resource_hash
      ) {
        throw new Error('Skill resource version mismatch');
      }
      const files = { ...bundle.resources, 'SKILL.md': bundle.content };
      return pinSkill(
        bundle.name,
        new Map(
          Object.entries(files).map(([file, encoded]) => [
            file,
            Buffer.from(encoded, 'base64'),
          ]),
        ),
      );
    }),
  );
}
