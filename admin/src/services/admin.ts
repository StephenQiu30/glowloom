import { apiRequest } from './request';
import type { ApiEnvelope, RoleCode } from './session';

export type MembershipStatus = 'active' | 'suspended' | 'removed';

export type WorkspaceMember = {
  membership_id: string;
  user_id: string;
  email: string;
  display_name: string;
  account_status: 'active' | 'suspended' | 'removed';
  membership_status: MembershipStatus;
  role: RoleCode;
  created_at: string;
};

export type WorkspaceMemberPage = {
  items: WorkspaceMember[];
  total: number;
  page: number;
  page_size: number;
};

export async function listMembers(search = '') {
  const params = new URLSearchParams({ page: '1', page_size: '100' });
  if (search.trim()) params.set('search', search.trim());
  const response = await apiRequest<ApiEnvelope<WorkspaceMemberPage>>(`/api/admin/members?${params.toString()}`, { method: 'GET' });
  return response.data;
}

export async function updateMember(
  membershipID: string,
  update: { role?: RoleCode; status?: MembershipStatus },
) {
  const response = await apiRequest<ApiEnvelope<WorkspaceMember>>(`/api/admin/members/${membershipID}`, {
    method: 'PATCH',
    data: update,
  });
  return response.data;
}
