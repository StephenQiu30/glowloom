import { afterEach, expect, it, vi } from 'vitest';
import { request } from '../src/business/request.js';
const call = () =>
  request('/fixture', {
    method: 'GET',
    transport: { origin: 'http://fixture.invalid', token: 'fixture' },
  });
afterEach(() => vi.unstubAllGlobals());
it('accepts a bounded image receipt and rejects a response beyond the shared 8 MiB limit', async () => {
  const image = 'a'.repeat(3 * 1024 * 1024);
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify({ image }))),
  );
  expect(await call()).toEqual({ image });
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response('"' + 'a'.repeat(8 * 1024 * 1024) + '"')),
  );
  await expect(call()).rejects.toMatchObject({
    code: 'business_response_too_large',
  });
});

it('uses generated routes with service identity and propagates cancellation', async () => {
  const fetchMock = vi.fn(
    async () =>
      new Response(
        JSON.stringify({
          receipt_id: 'receipt',
          root_run_id: 'run',
          submission_id: 'submission',
          call: {
            task_id: 'task',
            call_id: 'call',
            checkpoint_id: 'checkpoint',
            attempt: 1,
            contract_version: 'v1',
            input_hash: 'hash',
          },
          status: 'completed',
          result: {},
          cost_micros: 0,
        }),
      ),
  );
  vi.stubGlobal('fetch', fetchMock);
  const { BusinessClient } = await import('../src/business/client.js');
  const client = new BusinessClient('http://fixture.invalid', 'secret');
  const signal = new AbortController().signal;
  await client.execute(
    'tool',
    'a/b',
    {
      grant_id: 'grant',
      root_run_id: 'run',
      submission_id: 'submission',
      owner_epoch: 1,
      authorization_revision: 1,
    },
    {
      task_id: 'task',
      call_id: 'call',
      checkpoint_id: 'checkpoint',
      attempt: 1,
      contract_version: 'v1',
      input_hash: 'hash',
    },
    {},
    0,
    signal,
  );
  const [url, options] = fetchMock.mock.calls[0] as unknown as [
    URL,
    RequestInit,
  ];
  expect(url.pathname).toBe('/internal/agent-business/tools/a%2Fb/execute');
  expect(options.redirect).toBe('error');
  expect(options.headers).toMatchObject({
    authorization: 'Bearer secret',
    'x-agent-audience': 'agent-business',
  });
  expect(options.signal).toBeInstanceOf(AbortSignal);
});

it('rejects generated calls without injected transport', async () => {
  const { listTools } = await import('../src/business/generated/business.js');
  await expect(listTools()).rejects.toThrow('Business transport is required');
});

it('propagates caller cancellation to the generated model request', async () => {
  const controller = new AbortController();
  vi.stubGlobal(
    'fetch',
    vi.fn(
      (_url: URL, options: RequestInit) =>
        new Promise((_resolve, reject) => {
          options.signal?.addEventListener(
            'abort',
            () => reject(options.signal?.reason),
            { once: true },
          );
        }),
    ),
  );
  const { BusinessClient } = await import('../src/business/client.js');
  const client = new BusinessClient('http://fixture.invalid', 'secret');
  const pending = client.execute(
    'model',
    'chat',
    {
      grant_id: 'grant',
      root_run_id: 'run',
      submission_id: 'submission',
      owner_epoch: 1,
      authorization_revision: 1,
    },
    {
      task_id: 'task',
      call_id: 'call',
      checkpoint_id: 'checkpoint',
      attempt: 1,
      contract_version: 'v1',
      input_hash: 'hash',
    },
    {},
    0,
    controller.signal,
  );
  controller.abort();
  await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
});
