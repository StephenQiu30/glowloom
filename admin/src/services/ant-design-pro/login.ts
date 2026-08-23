import { request } from '@umijs/max';
import {
  clearSession,
  setSession,
  type ApiEnvelope,
  type AuthData,
} from '../session';

export async function login(
  body: API.LoginParams,
  options?: { [key: string]: unknown },
) {
  const response = await request<ApiEnvelope<AuthData>>('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
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
    });
  } finally {
    clearSession();
  }
}
