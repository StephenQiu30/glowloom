import { describe, expect, it } from 'vitest';
import { authRequestOptions } from '@/services/session';

describe('admin authenticated request options', () => {
  it('uses the axios credential contract required for HttpOnly refresh cookies', () => {
    const options = authRequestOptions({ headers: { 'X-Test': 'value' } });

    expect(options.withCredentials).toBe(true);
    expect(options).not.toHaveProperty('credentials');
    expect(options.headers).toEqual({ 'X-Test': 'value' });
  });
});
