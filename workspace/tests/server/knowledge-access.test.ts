import { afterEach, expect, it, vi } from 'vitest';
import { NextRequest } from 'next/server';
import { proxy } from '../../src/online/proxy';
afterEach(() => vi.unstubAllEnvs());

it('opens knowledge and search without a cookie or privileged session', async () => {
  for (const path of [
    '/',
    '/read/design-43/',
    '/search/',
    '/skills/',
    '/workflows/',
    '/tasks/',
    '/engineering/',
    '/graph/',
    '/raw/design-43.md',
    '/narration/index.json',
    '/static.json',
    '/api/search/?query=Workspace',
  ]) {
    const response = await proxy(
      new NextRequest('http://127.0.0.1:1130' + path),
    );
    expect(response.headers.get('x-middleware-next'), path).toBe('1');
    expect(response.headers.has('set-cookie'), path).toBe(false);
  }
});

it('keeps collaboration data and writes protected without granting anonymous ownership', async () => {
  for (const path of [
    '/api/documents/',
    '/api/status/',
    '/api/proposals/',
    '/api/runs/',
    '/api/search/',
  ]) {
    const response = await proxy(
      new NextRequest('http://127.0.0.1:1130' + path, { method: 'POST' }),
    );
    expect(response.status, path).toBe(401);
  }
  const response = await proxy(
    new NextRequest('http://127.0.0.1:1130/operations/'),
  );
  expect(response.headers.get('location')).toContain('/login/');
});
