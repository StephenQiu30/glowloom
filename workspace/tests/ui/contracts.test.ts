import { afterEach, expect, test, vi } from 'vitest';
import { request } from '../../src/online/components/api';
import {
  statusContract,
  documentsContract,
  runsContract,
} from '../../src/online/components/contracts';
afterEach(() => vi.unstubAllGlobals());

test('rejects malformed HTTP data instead of presenting false connected or TTS states', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ database: 'yes', bridge: true, tts: true }),
      ),
  );
  await expect(request('/status', statusContract)).rejects.toThrow(
    '服务响应格式异常',
  );
});

test('preserves valid disabled TTS and rejects incomplete run context', async () => {
  const status = {
    database: true,
    bridge: true,
    tts: false,
    actor: 'local-owner',
    message: null,
  };
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json(status)));
  await expect(request('/status', statusContract)).resolves.toEqual(status);
  expect(
    runsContract.safeParse([
      {
        id: 'run',
        status: 'running',
        context: { goal: 'missing authorization' },
      },
    ]).success,
  ).toBe(false);
});

test('does not display arbitrary server internals in a user-facing error', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        Response.json(
          { error: '/private/path: internal stack' },
          { status: 500 },
        ),
      ),
  );
  await expect(request('/status', statusContract)).rejects.toThrow(
    '请求失败，请保留当前内容后重试。',
  );
});

test('accepts document metadata without pretending the collection contains live source', () => {
  expect(
    documentsContract.parse([
      { id: 'design-43', title: '设计', revision: 'hash', editable: true },
    ]),
  ).toEqual([
    { id: 'design-43', title: '设计', revision: 'hash', editable: true },
  ]);
});
