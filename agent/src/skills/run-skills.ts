import { createHash } from 'node:crypto';
import { z } from 'zod';
import { pinSkill, type PinnedSkill } from './loader.js';

const skillPin = z
  .object({
    skill_id: z.uuid(),
    version: z.string().min(1).max(100),
    resource_hash: z.string().regex(/^[a-f0-9]{64}$/),
  })
  .strict();
const bundleSchema = skillPin.extend({
  name: z.string(),
  description: z.string(),
  content: z.base64(),
  resources: z.record(z.string(), z.base64()).optional(),
});
const grantSchema = z.object({
  grant_id: z.uuid(),
  root_run_id: z.uuid(),
  submission_id: z.uuid(),
  owner_epoch: z.number().int(),
  authorization_revision: z.number().int(),
  expires_at: z.string(),
  scope: z.record(z.string(), z.unknown()),
});

export interface SkillServiceConfig {
  businessOrigin: string;
  businessToken: string;
}

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

export async function loadPrivateSkills(
  config: SkillServiceConfig,
  runId: string,
  submissionId: string,
  ttlMs: number,
  requested: unknown,
): Promise<PinnedSkill[]> {
  const pins = z
    .array(skillPin)
    .max(32)
    .parse(requested ?? []);
  if (pins.length === 0) return [];
  const grant = grantSchema.parse(
    await post(config, '/grants', {
      root_run_id: runId,
      submission_id: submissionId,
      owner_epoch: 1,
      authorization_revision: 1,
      lease_until: new Date(Date.now() + ttlMs).toISOString(),
    }),
  );
  if (grant.root_run_id !== runId || grant.submission_id !== submissionId) {
    throw new Error('Skill grant scope mismatch');
  }

  return Promise.all(
    pins.map(async (pin) => {
      const bundle = bundleSchema.parse(
        await post(config, `/skills/${pin.skill_id}/load`, {
          grant,
          version: pin.version,
          resource_hash: pin.resource_hash,
        }),
      );
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

async function post(
  config: SkillServiceConfig,
  route: string,
  body: unknown,
): Promise<unknown> {
  const response = await fetch(
    config.businessOrigin + '/internal/agent-business' + route,
    {
      method: 'POST',
      redirect: 'error',
      signal: AbortSignal.timeout(15_000),
      headers: {
        Authorization: `Bearer ${config.businessToken}`,
        'X-Agent-Audience': 'agent-business',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(body),
    },
  );
  if (!response.ok)
    throw new Error(`Skill service rejected ${route}: ${response.status}`);
  return response.json();
}
