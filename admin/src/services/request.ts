import type { RequestOptions } from '@umijs/max';
import { request } from '@umijs/max';
import {
  ApiEnvelope,
  authRequestOptions,
  clearSession,
  getWorkspaceId,
  isUnauthorized,
  setSession,
  type AuthData,
} from './session';

let refreshing: Promise<boolean> | undefined;

export async function refreshSession() {
  if (!getWorkspaceId()) return false;
  if (!refreshing) {
    refreshing = request<ApiEnvelope<AuthData>>('/api/auth/refresh', {
      method: 'POST',
      ...authRequestOptions({ skipErrorHandler: true }),
    })
      .then((response) => {
        setSession(response.data);
        return true;
      })
      .catch(() => {
        clearSession();
        return false;
      })
      .finally(() => {
        refreshing = undefined;
      });
  }
  return refreshing;
}

export async function apiRequest<T>(
  url: string,
  options: RequestOptions = {},
  retry = true,
): Promise<T> {
  try {
    return await request<T>(url, authRequestOptions(options));
  } catch (error) {
    if (retry && isUnauthorized(error) && (await refreshSession())) {
      return apiRequest<T>(url, options, false);
    }
    throw error;
  }
}
