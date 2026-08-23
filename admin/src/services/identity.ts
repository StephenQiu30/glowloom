import { request } from '@umijs/max';
import { apiRequest, restoreSession } from './request';
import {
  clearSession,
  getSessionAuth,
  setSession,
  type ApiEnvelope,
  type AuthData,
} from './session';

type CurrentIdentity = {
  user_id: string;
  workspace_id: string;
  membership_id: string;
  session_id: string;
  role: API.RoleCode;
};

export async function queryCurrentIdentity() {
  if (!(await restoreSession())) {
    throw new Error('登录会话已失效');
  }
  const response = await apiRequest<{ data: CurrentIdentity }>('/api/auth/me', {
    method: 'GET',
  });
  const auth = getSessionAuth();
  return {
    data: {
      id: response.data.user_id,
      userid: response.data.user_id,
      name: auth?.user.display_name || response.data.user_id,
      email: auth?.user.email,
      role: response.data.role,
      access: response.data.role,
      workspaceId: response.data.workspace_id,
      membershipId: response.data.membership_id,
      workspaceName: auth?.workspace.name,
    },
  };
}

export async function loginWithEmail(body: API.LoginParams) {
  const response = await request<ApiEnvelope<AuthData>>('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    withCredentials: true,
    data: { email: body.email, password: body.password },
  });
  setSession(response.data);
}

export async function logoutCurrentSession() {
  try {
    await request('/api/auth/logout', {
      method: 'POST',
      withCredentials: true,
    });
  } finally {
    clearSession();
  }
}
