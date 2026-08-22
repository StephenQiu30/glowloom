import { apiRequest } from '../request';
import { getSessionAuth } from '../session';

type CurrentIdentity = {
  user_id: string;
  workspace_id: string;
  membership_id: string;
  session_id: string;
  role: API.RoleCode;
};

export async function currentUser() {
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
