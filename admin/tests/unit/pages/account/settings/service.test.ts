import { beforeEach, describe, expect, it, vi } from 'vitest';
import { queryCity } from '@/pages/account/settings/service';

vi.mock('@/services/identity', () => ({
  queryCurrentIdentity: vi.fn(),
}));

describe('account settings service', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('returns local city options without the deleted mock API', async () => {
    const cities = await queryCity('330000');

    expect(cities.length).toBeGreaterThan(0);
  });
});
