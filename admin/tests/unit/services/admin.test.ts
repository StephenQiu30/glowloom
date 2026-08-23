import { beforeEach, describe, expect, it, vi } from 'vitest';
import { listAccessAudit, updateMember } from '@/services/admin';
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

  it('queries the current workspace audit with explicit filters and pagination', async () => {
    mockApiRequest.mockResolvedValue({
      data: { items: [], total: 0, page: 2, page_size: 10 },
    });

    await listAccessAudit({
      actor: 'Audit Actor',
      object: 'Audit Target',
      action: 'iam.membership.updated',
      result: 'succeeded',
      occurred_from: '2026-08-23T00:00:00Z',
      occurred_to: '2026-08-24T00:00:00Z',
      page: 2,
      page_size: 10,
    });

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/api/admin/audit-events?actor=Audit+Actor&object=Audit+Target&action=iam.membership.updated&result=succeeded&occurred_from=2026-08-23T00%3A00%3A00Z&occurred_to=2026-08-24T00%3A00%3A00Z&page=2&page_size=10',
      { method: 'GET' },
    );
  });
});
