import type { RequestOptions } from '@umijs/max';

export type RoleCode = 'admin' | 'user' | 'ban';

export type AuthData = {
  access_token: string;
  token_type: 'Bearer';
  expires_at: string;
  user: {
    id: string;
    email: string;
    display_name: string;
  };
  workspace: {
    id: string;
    name: string;
  };
  role: RoleCode;
};

export type ApiEnvelope<T> = {
  data: T;
};

let accessToken: string | undefined;
let currentAuth: AuthData | undefined;

export function getAccessToken() {
  return accessToken;
}

export function getSessionAuth() {
  return currentAuth;
}

export function setSession(auth: AuthData) {
  accessToken = auth.access_token;
  currentAuth = auth;
}

export function clearSession() {
  accessToken = undefined;
  currentAuth = undefined;
}

export function authRequestOptions(options: RequestOptions = {}): RequestOptions {
  const headers = {
    ...((options.headers || {}) as Record<string, string>),
  };
  const token = getAccessToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  return {
    ...options,
    withCredentials: true,
    headers,
  };
}

export function isUnauthorized(error: unknown) {
  return (error as { response?: { status?: number } })?.response?.status === 401;
}
