import { beforeEach, describe, expect, it, vi } from 'vitest';
import { updateMember } from '@/services/admin';
import { apiRequest } from '@/services/request';

vi.mock('@/services/request', () => ({
  apiRequest: vi.fn(),
}));

const mockApiRequest = vi.mocked(apiRequest);

describe('admin member service', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('submits the explicit audit reason with the current member update contract', async () => {
    mockApiRequest.mockResolvedValue({
      data: {
        membership_id: 'membership-1',
        role: 'ban',
      },
    });

    await updateMember('membership-1', {
      role: 'ban',
      reason: '违规访问处置',
    });

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/api/admin/members/membership-1',
      {
        method: 'PATCH',
        data: { role: 'ban', reason: '违规访问处置' },
      },
    );
  });
});
