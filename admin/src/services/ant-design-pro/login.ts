import { request } from '@umijs/max';
import {
  clearSession,
  getWorkspaceId,
  setSession,
  type ApiEnvelope,
  type AuthData,
} from '../session';

export async function login(
  body: API.LoginParams,
  options?: { [key: string]: unknown },
) {
  const workspaceId = body.workspaceId || getWorkspaceId();
  if (!workspaceId) {
    throw new Error('请先填写 Workspace ID，或先完成注册');
  }
  const response = await request<ApiEnvelope<AuthData>>('/api/auth/login', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Workspace-Id': workspaceId,
    },
    credentials: 'include',
    data: { email: body.email, password: body.password },
    ...(options || {}),
  });
  setSession(response.data);
  return { status: 'ok', type: 'account', currentAuthority: response.data.role };
}

export async function outLogin() {
  try {
    await request('/api/auth/logout', {
      method: 'POST',
      credentials: 'include',
      headers: {
        'X-Workspace-Id': getWorkspaceId() || '',
      },
    });
  } finally {
    clearSession();
  }
}
