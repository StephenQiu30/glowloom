import { apiRequest } from './request';
import type { ApiEnvelope, RoleCode } from './session';

export type MembershipStatus = 'active' | 'suspended' | 'removed';

type RequiredResponse<T> = { [Key in keyof T]-?: NonNullable<T[Key]> };

export type WorkspaceMember = RequiredResponse<API.WorkspaceMember>;
export type WorkspaceMemberPage = RequiredResponse<API.WorkspaceMemberPage> & {
  items: WorkspaceMember[];
};
export type AccessAuditEvent = RequiredResponse<API.AccessAuditEvent>;
export type AccessAuditPage = RequiredResponse<API.AccessAuditPage> & {
  items: AccessAuditEvent[];
};

function requirePage<T>(data: T | undefined, resource: string): RequiredResponse<T> {
  if (!data) throw new Error(`${resource}响应缺少 data`);
  return data as RequiredResponse<T>;
}

export async function listMembers(search = '') {
  const params = new URLSearchParams({ page: '1', page_size: '100' });
  if (search.trim()) params.set('search', search.trim());
  const response = await apiRequest<ApiEnvelope<WorkspaceMemberPage>>(`/api/admin/members?${params.toString()}`, { method: 'GET' });
  return requirePage(response.data, '成员列表') as WorkspaceMemberPage;
}

export async function listAccessAudit(query: API.adminListAccessAuditParams = {}) {
  const params = new URLSearchParams();
  for (const key of [
    'search',
    'actor',
    'object',
    'action',
    'result',
    'occurred_from',
    'occurred_to',
  ] as const) {
    const value = query[key];
    if (typeof value === 'string' && value.trim()) params.set(key, value.trim());
  }
  params.set('page', String(query.page ?? 1));
  params.set('page_size', String(query.page_size ?? 20));
  const response = await apiRequest<ApiEnvelope<AccessAuditPage>>(
    `/api/admin/audit-events?${params.toString()}`,
    { method: 'GET' },
  );
  return requirePage(response.data, '访问审计') as AccessAuditPage;
}

export async function updateMember(
  membershipID: string,
  update: {
    reason: string;
    role?: RoleCode;
    status?: MembershipStatus;
  },
) {
  const response = await apiRequest<ApiEnvelope<WorkspaceMember>>(`/api/admin/members/${membershipID}`, {
    method: 'PATCH',
    data: update,
  });
  return response.data;
}
