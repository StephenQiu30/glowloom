import { describe, it, expect } from 'vitest';
import {
  freezeManifest,
  stableId,
  canonicalHash,
} from '../src/sessions/manifest.js';
import { PhaseTelemetry, modelPhase } from '../src/pi-durable/telemetry.js';

describe('frozen execution identity', () => {
  it('uses immutable content, distinct run/continuation namespaces, and rejects duplicate tools', () => {
    const sid = '00000000-0000-4000-8000-000000000001';
    expect(stableId(sid, 'run:r')).toBe(stableId(sid, 'run:r'));
    expect(stableId(sid, 'run:r')).not.toBe(stableId(sid, 'continuation:r'));
    expect(canonicalHash({ b: 1, a: 2 })).toBe(canonicalHash({ a: 2, b: 1 }));
    expect(canonicalHash({ name: '<角色>&' })).toBe(
      '49f209bb03e92a15616a0ef206d13f7cc751253fb4315b141b3aaceec55f04c7',
    );
    expect(() =>
      freezeManifest({ model_key: 'model' }, [
        {
          name: 'x',
          description: 'x',
          parameters: { type: 'object' },
          contract_version: '1',
          replay: 'unsafe',
          effect: 'write',
        },
        {
          name: 'x',
          description: 'x',
          parameters: { type: 'object' },
          contract_version: '1',
          replay: 'unsafe',
          effect: 'write',
        },
      ]),
    ).toThrow('duplicate');
  });
  it('requires a model and pins Pi version and selected tool contents', () => {
    expect(() => freezeManifest({}, [])).toThrow('model_key');
    const m = freezeManifest({ model_key: 'm' }, []);
    expect(m.pi_version).toBe('1.5.0');
    expect(m.sha256).toHaveLength(64);
    expect(Object.isFrozen(m)).toBe(true);
  });
  it('carries task identity through nested provider spans and refuses absent correlation', async () => {
    const telemetry = new PhaseTelemetry();
    expect(() => modelPhase(telemetry)).toThrow('correlation');
    await telemetry.startSpan(
      {
        name: 'pi.durable.phase',
        attributes: {
          'task.id': 42,
          'task.name': 'pi.compaction',
          phase: 'request',
          attempt: 1,
        },
      },
      async (phase) => {
        await phase.startSpan({ name: 'provider' }, (span) => {
          expect(modelPhase(span)).toEqual({
            taskId: 42,
            taskName: 'pi.compaction',
            phase: 'request',
            attempt: 1,
          });
        });
      },
    );
  });
});
