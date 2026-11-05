import { expect, it } from 'vitest';
import {
  BusinessClient,
  BusinessError,
  type Grant,
} from '../src/integrations/go-business-api/client.js';
import {
  readPinnedSkill,
  businessToolContent,
} from '../src/runtime/pi-durable/skills.js';
import { freezeManifest } from '../src/sessions/manifest.js';
const manifest = freezeManifest(
  {
    model_key: 'test',
    skills: [{ skill_id: 'script', version: '1', resource_hash: 'fixed-hash' }],
  },
  [],
);
const grant = {} as Grant;
it('rejects unpinned Skills before loading any tenant content', async () => {
  let loaded = false;
  class Client extends BusinessClient {
    override async skill() {
      loaded = true;
      return {
        skill_id: 'other',
        version: '1',
        resource_hash: 'fixed-hash',
        content: 'bad',
      };
    }
  }
  await expect(
    readPinnedSkill(
      new Client('http://127.0.0.1:1', 'test'),
      grant,
      manifest,
      'other',
    ),
  ).rejects.toThrow(/not pinned/);
  expect(loaded).toBe(false);
});
it.each([
  { version: '2', resource_hash: 'fixed-hash' },
  { version: '1', resource_hash: 'changed-hash' },
])(
  'rejects version or resource drift in an authorized Skill response: %o',
  async (changed) => {
    class Client extends BusinessClient {
      override async validate(g: Grant) {
        return g;
      }
      override async skill() {
        return {
          skill_id: 'script',
          content: 'Changed tenant content',
          ...changed,
        };
      }
    }
    await expect(
      readPinnedSkill(
        new Client('http://127.0.0.1:1', 'test'),
        grant,
        manifest,
        'script',
      ),
    ).rejects.toThrow(/resource changed/);
  },
);
it('rejects a revoked Skill at the Go authorization gate before loading content', async () => {
  let loaded = false;
  class Client extends BusinessClient {
    override async validate(): Promise<Grant> {
      throw new BusinessError(403, 'skill_revoked');
    }
    override async skill() {
      loaded = true;
      return {
        skill_id: 'script',
        version: '1',
        resource_hash: 'fixed-hash',
        content: 'revoked',
      };
    }
  }
  await expect(
    readPinnedSkill(
      new Client('http://127.0.0.1:1', 'test'),
      grant,
      manifest,
      'script',
    ),
  ).rejects.toThrow('skill_revoked');
  expect(loaded).toBe(false);
});
it('never interprets an unsupported media reference or a shell/script block as executable host output', () => {
  expect(() =>
    businessToolContent({ content: [{ type: 'image', path: '/etc/passwd' }] }),
  ).toThrow();
  expect(() =>
    businessToolContent({
      content: [{ type: 'shell', command: 'touch /tmp/should-never-exist' }],
    }),
  ).toThrow();
  expect(() =>
    businessToolContent({
      content: [{ type: 'image', data: '!!!!', mimeType: 'image/png' }],
    }),
  ).toThrow();
});
it('rejects a response for a different Skill even when its version and resource hash match', async () => {
  class Client extends BusinessClient {
    override async validate(g: Grant) {
      return g;
    }
    override async skill() {
      return {
        skill_id: 'other',
        version: '1',
        resource_hash: 'fixed-hash',
        content: 'wrong Skill',
      };
    }
  }
  await expect(
    readPinnedSkill(
      new Client('http://127.0.0.1:1', 'test'),
      grant,
      manifest,
      'script',
    ),
  ).rejects.toThrow(/resource changed/);
});
it('allocates child budgets from the root remainder and refuses a fifth concurrently active allocation', async () => {
  const { allocateBudget } =
    await import('../src/runtime/pi-durable/delegation.js');
  expect(
    allocateBudget({ model_steps: 38, tool_steps: 39, delegations: [] }),
  ).toEqual({ model_budget: 2, tool_budget: 1, model_used: 0, tool_used: 0 });
  expect(() =>
    allocateBudget(
      { model_steps: 35, tool_steps: 35, delegations: [] },
      { model_steps: 6, tool_steps: 1 },
    ),
  ).toThrow(/remaining root/);
  const active = Array.from({ length: 4 }, (_, i) => ({
    task_id: String(i),
    status: 'running',
    model_budget: 1,
    model_used: 0,
    tool_budget: 1,
    tool_used: 0,
  }));
  expect(() =>
    allocateBudget({ model_steps: 0, tool_steps: 0, delegations: active }),
  ).toThrow(/concurrency/);
  expect(() =>
    allocateBudget(
      { model_steps: 35, tool_steps: 0, delegations: active.slice(0, 3) },
      { model_steps: 3, tool_steps: 1 },
    ),
  ).toThrow(/remaining root/);
});
