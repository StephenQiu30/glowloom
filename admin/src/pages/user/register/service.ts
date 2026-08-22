import { request } from '@umijs/max';
import { setSession, type ApiEnvelope, type AuthData } from '@/services/session';

export interface UserRegisterParams {
  email: string;
  password: string;
  displayName?: string;
  workspaceName: string;
}

export async function register(params: UserRegisterParams) {
  const response = await request<ApiEnvelope<AuthData>>('/api/auth/register', {
    method: 'POST',
    credentials: 'include',
    data: {
      email: params.email,
      password: params.password,
      display_name: params.displayName,
      workspace_name: params.workspaceName,
    },
  });
  setSession(response.data);
  return response.data;
}
