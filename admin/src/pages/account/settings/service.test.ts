import { beforeEach, describe, expect, it, vi } from 'vitest';
import { queryCity } from './service';

vi.mock('@/services/ant-design-pro/api', () => ({
  currentUser: vi.fn(),
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
