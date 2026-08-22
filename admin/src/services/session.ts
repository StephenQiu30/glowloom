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

const workspaceStorageKey = 'lanverse.workspace_id';
let accessToken: string | undefined;
let currentAuth: AuthData | undefined;

export function getAccessToken() {
  return accessToken;
}

export function getSessionAuth() {
  return currentAuth;
}

export function getWorkspaceId() {
  if (typeof window === 'undefined') return undefined;
  return window.localStorage.getItem(workspaceStorageKey) || undefined;
}

export function setSession(auth: AuthData) {
  accessToken = auth.access_token;
  currentAuth = auth;
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(workspaceStorageKey, auth.workspace.id);
  }
}

export function clearSession() {
  accessToken = undefined;
  currentAuth = undefined;
  if (typeof window !== 'undefined') {
    window.localStorage.removeItem(workspaceStorageKey);
  }
}

export function authRequestOptions(options: RequestOptions = {}): RequestOptions {
  const headers = {
    ...((options.headers || {}) as Record<string, string>),
  };
  const token = getAccessToken();
  const workspaceId = getWorkspaceId();
  if (token) headers.Authorization = `Bearer ${token}`;
  if (workspaceId) headers['X-Workspace-Id'] = workspaceId;
  return {
    ...options,
    credentials: 'include',
    headers,
  };
}

export function isUnauthorized(error: unknown) {
  return (error as { response?: { status?: number } })?.response?.status === 401;
}
