import { readdir, readFile } from 'node:fs/promises';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { composeModelMessages } from '../src/context/composer.js';
import {
  loadProducerInstructions,
  loadSpecialistInstructions,
} from '../src/context/prompt-loader.js';
import { verifyStoryboardDraft } from '../src/harness/stage-verifier.js';
import { BusinessError } from '../src/business/client.js';
import { formatActionableToolError } from '../src/tools/tool-errors.js';
import { RunEventBus } from '../src/sessions/event-bus.js';
import {
  freezeManifest,
  hasCurrentDefinitions,
} from '../src/sessions/manifest.js';
import {
  currentTraceparent,
  formatTraceparent,
  parseTraceparent,
  setSpanSink,
  type GenAiSpanRecord,
} from '../src/observability/otel.js';
import { PhaseTelemetry } from '../src/observability/pi-telemetry.js';
import {
  gradeTrajectory,
  summarizeTrials,
} from '../evals/graders/trajectory-grader.js';

const sourceRoot = fileURLToPath(new URL('../src/', import.meta.url));

async function listHandwrittenSourceFiles(dir: string): Promise<string[]> {
  const entries = await readdir(dir, { withFileTypes: true });
  const files: string[] = [];
  for (const entry of entries) {
    if (entry.name === 'generated') continue;
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await listHandwrittenSourceFiles(full)));
    } else if (entry.isFile() && entry.name.endsWith('.ts')) {
      files.push(full);
    }
  }
  return files;
}

describe('Google TypeScript directory & module architecture invariants', () => {
  it('enforces flat two-level packages, kebab-case names, no barrels, no default exports, and no ../../ climbing', async () => {
    const files = await listHandwrittenSourceFiles(sourceRoot);
    const violations: string[] = [];
    for (const file of files) {
      const rel = relative(sourceRoot, file).replaceAll('\\', '/');
      const segments = rel.split('/');
      if (segments.length > 2) {
        violations.push(`${rel}: exceeds two-level package depth`);
      }
      const filename = segments.at(-1) ?? '';
      if (filename === 'index.ts') {
        violations.push(`${rel}: handwritten barrel index.ts is forbidden`);
      }
      if (!/^[a-z0-9]+(?:-[a-z0-9]+)*\.ts$/.test(filename)) {
        violations.push(`${rel}: filename must be lowercase kebab-case`);
      }
      const body = await readFile(file, 'utf8');
      if (/^\s*export\s+default\b/m.test(body)) {
        violations.push(`${rel}: export default is forbidden by Google TS`);
      }
      if (/(?:from|import\s*\()\s*['"]\.\.\/\.\.\//.test(body)) {
        violations.push(`${rel}: deep ../../ relative import is forbidden`);
      }
    }
    expect(violations).toEqual([]);
  });
});

describe('Agent Harness engineering capabilities', () => {
  it('keeps system prefix static and appends snapshots at the tail for Prompt Cache hits', () => {
    const baseTranscript = {
      messages: [
        { role: 'system' as const, content: 'Base system.' },
        { role: 'user' as const, content: 'First turn' },
      ],
      tools: [],
    };
    const turn1 = composeModelMessages(baseTranscript, {
      instructions: 'Static instructions.',
      snapshot: { rev: 1 },
      changedSincePrevious: false,
      isFirstModelStep: true,
    });
    const turn2Unchanged = composeModelMessages(baseTranscript, {
      instructions: 'Static instructions.',
      snapshot: { rev: 1 },
      changedSincePrevious: false,
      isFirstModelStep: false,
    });
    const turn3Changed = composeModelMessages(baseTranscript, {
      instructions: 'Static instructions.',
      snapshot: { rev: 2 },
      changedSincePrevious: true,
      isFirstModelStep: false,
    });

    expect(turn1.messages[0]).toEqual(turn2Unchanged.messages[0]);
    expect(turn1.messages[0]).toEqual(turn3Changed.messages[0]);
    expect(turn1.messages.at(-1)?.content).toContain('"rev":1');
    expect(turn2Unchanged.messages).toHaveLength(2);
    expect(turn3Changed.messages.at(-1)?.content).toContain('"rev":2');
  });

  it('decouples contract_version from non-breaking definition_hash changes and freezes role_models', () => {
    const manifest = freezeManifest(
      {
        model_key: 'gemini-2.5-pro',
        reasoning: true,
        role_models: {
          storyboard: { model_key: 'gemini-2.5-flash', reasoning: false },
        },
      },
      [],
    );
    expect(hasCurrentDefinitions(manifest)).toBe(true);
    expect(
      hasCurrentDefinitions({
        ...manifest,
        definition_hash: '0'.repeat(64),
      }),
    ).toBe(true);
    expect(
      hasCurrentDefinitions({
        ...manifest,
        contract_version: 'v999' as 'v1',
      }),
    ).toBe(false);
    expect(manifest.role_models?.storyboard?.model_key).toBe(
      'gemini-2.5-flash',
    );
  });

  it('wakes RunEventBus subscribers immediately on notify', async () => {
    const bus = new RunEventBus();
    const started = Date.now();
    const wait = bus.waitForUpdate('run-1', 5000);
    setTimeout(() => bus.notify('run-1'), 15);
    await wait;
    expect(Date.now() - started).toBeLessThan(500);
    bus.close();
  });

  it('emits structured OTel spans and propagates W3C traceparent', async () => {
    const recorded: GenAiSpanRecord[] = [];
    setSpanSink((span) => recorded.push(span));
    try {
      const telemetry = new PhaseTelemetry();
      let headerInside: string | undefined;
      await telemetry.startSpan(
        {
          name: 'pi.durable.phase',
          attributes: {
            'task.id': 7,
            'task.name': 'model',
            phase: 'request',
            attempt: 1,
          },
        },
        async () => {
          headerInside = currentTraceparent();
        },
      );
      expect(headerInside).toBeDefined();
      const parsed = parseTraceparent(headerInside);
      expect(parsed).toBeDefined();
      expect(formatTraceparent(parsed!)).toBe(headerInside);
      expect(recorded).toHaveLength(1);
      expect(recorded[0]?.status).toBe('ok');
    } finally {
      setSpanSink(undefined);
    }
  });

  it('formats actionable tool errors, validates storyboard drafts, loads prompts, and summarizes evals', () => {
    const err = formatActionableToolError(
      'canvas_storyboard_write',
      new BusinessError(409, 'revision_conflict'),
    );
    expect(err.retryable).toBe(true);
    expect(err.suggested_action).toContain('Re-read');

    expect(
      verifyStoryboardDraft([
        {
          title: 'Opening',
          visible_action: 'Ferry departs in mist',
          duration_ms: 5000,
        },
      ]).ok,
    ).toBe(true);
    expect(
      verifyStoryboardDraft([
        { title: '', visible_action: 'Missing title', duration_ms: -1 },
      ]).ok,
    ).toBe(false);

    expect(loadProducerInstructions()).toContain('creative producer');
    expect(loadSpecialistInstructions('qa')).toContain('qa specialist');

    const trial = gradeTrajectory(
      'creative-ambiguous-ferry',
      {
        must_offer_directions: true,
        max_media_submissions: 0,
        require_static_system_prefix: true,
      },
      {
        outputText: '方向：乘客视角观察\n方向：码头清晨群像',
        writeToolCalls: 0,
        mediaSubmissions: 0,
        unapprovedMediaSubmissions: 0,
        requestedProposal: false,
        systemPrefixStable: true,
      },
    );
    const summary = summarizeTrials([trial, trial]);
    expect(summary.passAt1).toBe(1);
    expect(summary.passAllK).toBe(1);
  });
});
