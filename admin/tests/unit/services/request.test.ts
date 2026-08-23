import { request } from '@umijs/max';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { restoreSession } from '@/services/request';
import { clearSession, getAccessToken } from '@/services/session';

vi.mock('@umijs/max', () => ({
  request: vi.fn(),
}));

const mockRequest = vi.mocked(request);

describe('admin session restoration', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    clearSession();
  });

  it('restores an empty in-memory session from the HttpOnly refresh cookie first', async () => {
    mockRequest.mockResolvedValue({
      data: {
        access_token: 'new-access-token',
        token_type: 'Bearer',
        expires_at: '2026-08-23T18:00:00Z',
        user: {
          id: 'user-1',
          email: 'admin@example.test',
          display_name: 'Admin',
        },
        workspace: { id: 'workspace-1', name: 'Workspace' },
        role: 'admin',
      },
    });

    await expect(restoreSession()).resolves.toBe(true);

    expect(mockRequest).toHaveBeenCalledWith(
      '/api/auth/refresh',
      expect.objectContaining({ method: 'POST', withCredentials: true }),
    );
    expect(getAccessToken()).toBe('new-access-token');
  });
});
