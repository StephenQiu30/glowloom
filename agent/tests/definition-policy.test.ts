import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, it } from 'vitest';
import { computeDefinitionHash } from '../src/pi-durable/definitions.js';
import { executionPolicy } from '../src/pi-durable/policy.js';
import { freezeManifest } from '../src/sessions/manifest.js';

it('hashes the complete executable tree and dependency lock reproducibly', async () => {
  const root = await mkdtemp(join(tmpdir(), 'agent-definition-'));
  try {
    const source = join(root, 'src');
    const lock = join(root, 'pnpm-lock.yaml');
    await mkdir(join(source, 'nested'), { recursive: true });
    await writeFile(join(source, 'entry.js'), 'export const main = 1;');
    await writeFile(
      join(source, 'nested', 'delegation.js'),
      'export const cap = 4;',
    );
    await writeFile(lock, 'version: 1');
    const original = computeDefinitionHash(source, '.js', lock);
    expect(computeDefinitionHash(source, '.js', lock)).toBe(original);
    await writeFile(
      join(source, 'nested', 'delegation.js'),
      'export const cap = 3;',
    );
    const changed = computeDefinitionHash(source, '.js', lock);
    expect(changed).not.toBe(original);
    await writeFile(lock, 'version: 2');
    expect(computeDefinitionHash(source, '.js', lock)).not.toBe(changed);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

it('freezes the actual execution policy and refuses request-selected limits', () => {
  const manifest = freezeManifest({ model_key: 'model' }, []);
  expect(manifest.execution_policy).toEqual(executionPolicy);
  expect(Object.isFrozen(manifest.execution_policy)).toBe(true);
  expect(() =>
    freezeManifest(
      { model_key: 'model', execution_policy: { modelSteps: 9999 } },
      [],
    ),
  ).toThrow('policy');
});
