// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** Read public registration and accepted agreement policy GET /api/auth-policy */
export async function getAuthPolicy(options?: RequestOptions) {
  return request<API.AuthPolicyView>("/api/auth-policy", {
    method: "GET",
    ...(options || {}),
  });
}

/** AcceptInvitation POST /api/invitations/accept */
export async function acceptInvitation(
  body: API.InvitationTokenRequest,
  options?: RequestOptions,
) {
  return request<any>("/api/invitations/accept", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** DeclineInvitation POST /api/invitations/decline */
export async function declineInvitation(
  body: API.InvitationTokenRequest,
  options?: RequestOptions,
) {
  return request<any>("/api/invitations/decline", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Preview minimal public invitation metadata POST /api/invitations/preview */
export async function previewInvitation(
  body: API.InvitationTokenRequest,
  options?: RequestOptions,
) {
  return request<API.InvitationPreview>("/api/invitations/preview", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/me */
export async function getMe(options?: RequestOptions) {
  return request<API.MeView>("/api/me", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 PATCH /api/me */
export async function updateMe(
  body: API.ProfileUpdateRequest,
  options?: RequestOptions,
) {
  return request<API.MeView>("/api/me", {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/me/authority-proposals */
export async function myProposals(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.myProposalsParams,
  options?: RequestOptions,
) {
  return request<API.ProposalListPage>("/api/me/authority-proposals", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** AcceptProposal POST /api/me/authority-proposals/${param0}/accept */
export async function acceptProposal(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.acceptProposalParams,
  body: API.GovernanceVersion,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/me/authority-proposals/${param0}/accept`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** DeclineProposal POST /api/me/authority-proposals/${param0}/decline */
export async function declineProposal(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.declineProposalParams,
  body: API.GovernanceVersion,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/me/authority-proposals/${param0}/decline`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/me/deletion */
export async function confirmAccountDeletion(
  body: API.DeletionConfirmRequest,
  options?: RequestOptions,
) {
  return request<any>("/api/me/deletion", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/me/deletion/challenge */
export async function startAccountDeletion(options?: RequestOptions) {
  return request<any>("/api/me/deletion/challenge", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/me/deletion/preview */
export async function previewAccountDeletion(options?: RequestOptions) {
  return request<API.DeletionImpactView>("/api/me/deletion/preview", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/me/email-change */
export async function startEmailChange(
  body: API.EmailChangeStartRequest,
  options?: RequestOptions,
) {
  return request<any>("/api/me/email-change", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 DELETE /api/me/email-change/${param0} */
export async function cancelEmailChange(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelEmailChangeParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/me/email-change/${param0}`, {
    method: "DELETE",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/me/email-change/${param0}/complete */
export async function completeEmailChange(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.completeEmailChangeParams,
  body: API.EmailChangeCompleteRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/me/email-change/${param0}/complete`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/me/email-change/${param0}/verify */
export async function verifyEmailChange(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.verifyEmailChangeParams,
  body: API.EmailChangeVerifyRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/me/email-change/${param0}/verify`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/me/password */
export async function changePassword(
  body: API.EmailPasswordChangeRequest,
  options?: RequestOptions,
) {
  return request<API.EmailSessionView>("/api/me/password", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/me/preferences */
export async function getMyPreferences(options?: RequestOptions) {
  return request<API.PreferencesView>("/api/me/preferences", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 PATCH /api/me/preferences */
export async function updateMyPreferences(
  body: API.PreferencesUpdateRequest,
  options?: RequestOptions,
) {
  return request<API.PreferencesView>("/api/me/preferences", {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/me/reauthentication */
export async function reauthenticate(
  body: API.ReauthenticationRequest,
  options?: RequestOptions,
) {
  return request<any>("/api/me/reauthentication", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/me/security-events */
export async function getMySecurityEvents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMySecurityEventsParams,
  options?: RequestOptions,
) {
  return request<API.SecurityEventPageView>("/api/me/security-events", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/me/sessions */
export async function getMySessions(options?: RequestOptions) {
  return request<API.SessionPageView>("/api/me/sessions", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/me/sessions/revoke-others */
export async function revokeOtherSessions(options?: RequestOptions) {
  return request<any>("/api/me/sessions/revoke-others", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/me/spaces */
export async function listMySpaces(options?: RequestOptions) {
  return request<API.SpacePageView>("/api/me/spaces", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/password-resets */
export async function startPasswordReset(
  body: API.PasswordResetStartRequest,
  options?: RequestOptions,
) {
  return request<any>("/api/password-resets", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/password-resets/complete */
export async function completePasswordReset(
  body: API.PasswordResetCompleteRequest,
  options?: RequestOptions,
) {
  return request<any>("/api/password-resets/complete", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** accountList GET /api/platform/account-invitations */
export async function accountListInvitations(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.accountListInvitationsParams,
  options?: RequestOptions,
) {
  return request<API.InvitationListPage>("/api/platform/account-invitations", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** Invite an ordinary platform account POST /api/platform/account-invitations */
export async function createAccountInvitation(
  body: API.AccountInvitationRequest,
  options?: RequestOptions,
) {
  return request<API.Invitation>("/api/platform/account-invitations", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** resendAccount POST /api/platform/account-invitations/${param0}/resend */
export async function resendAccountInvitation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.resendAccountInvitationParams,
  body: API.GovernanceVersion,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/platform/account-invitations/${param0}/resend`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** revokeAccount POST /api/platform/account-invitations/${param0}/revoke */
export async function revokeAccountInvitation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.revokeAccountInvitationParams,
  body: API.GovernanceMutation,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/platform/account-invitations/${param0}/revoke`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/platform/admin-grants */
export async function platformProposals(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.platformProposalsParams,
  options?: RequestOptions,
) {
  return request<API.ProposalListPage>("/api/platform/admin-grants", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** ProposeAdmin POST /api/platform/admin-grants */
export async function proposeAdmin(
  body: API.AuthorityProposalRequest,
  options?: RequestOptions,
) {
  return request<API.AuthorityProposal>("/api/platform/admin-grants", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** CancelProposal POST /api/platform/admin-grants/${param0}/cancel */
export async function cancelProposal(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelProposalParams,
  body: API.GovernanceMutation,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/platform/admin-grants/${param0}/cancel`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/platform/audit-events */
export async function getPlatformSecurityEvents(options?: RequestOptions) {
  return request<API.SecurityEventPageView>("/api/platform/audit-events", {
    method: "GET",
    ...(options || {}),
  });
}

/** Read platform settings GET /api/platform/settings */
export async function platformSettings(options?: RequestOptions) {
  return request<API.PlatformSettings>("/api/platform/settings", {
    method: "GET",
    ...(options || {}),
  });
}

/** Update platform settings PATCH /api/platform/settings */
export async function updatePlatformSettings(
  body: API.SettingsGovernanceRequest,
  options?: RequestOptions,
) {
  return request<API.PlatformSettings>("/api/platform/settings", {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** List platform team metadata GET /api/platform/teams */
export async function platformTeams(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.platformTeamsParams,
  options?: RequestOptions,
) {
  return request<API.PlatformTeamPage>("/api/platform/teams", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** Read platform team governance metadata GET /api/platform/teams/${param0} */
export async function platformTeam(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.platformTeamParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.PlatformTeam>(`/api/platform/teams/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Update platform team governance profile PATCH /api/platform/teams/${param0} */
export async function updatePlatformTeam(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updatePlatformTeamParams,
  body: API.TeamProfileRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.PlatformTeam>(`/api/platform/teams/${param0}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** ProposeTakeover POST /api/platform/teams/${param0}/admin-transfers */
export async function proposeTakeover(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.proposeTakeoverParams,
  body: API.AuthorityProposalRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.AuthorityProposal>(
    `/api/platform/teams/${param0}/admin-transfers`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** Preview platform team governance avatar GET /api/platform/teams/${param0}/avatar */
export async function platformTeamAvatar(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.platformTeamAvatarParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.TeamAvatarPreview>(
    `/api/platform/teams/${param0}/avatar`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** List platform-governed team invitations GET /api/platform/teams/${param0}/invitations */
export async function listPlatformTeamInvitations(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listPlatformTeamInvitationsParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.InvitationListPage>(
    `/api/platform/teams/${param0}/invitations`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** Invite a team member under platform governance POST /api/platform/teams/${param0}/invitations */
export async function createPlatformTeamInvitation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createPlatformTeamInvitationParams,
  body: API.PlatformTeamInvitationRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.Invitation>(`/api/platform/teams/${param0}/invitations`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Revoke a team invitation under platform governance POST /api/platform/teams/${param0}/invitations/${param1}/revoke */
export async function revokePlatformTeamInvitation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.revokePlatformTeamInvitationParams,
  body: API.GovernanceMutation,
  options?: RequestOptions,
) {
  const { id: param0, iid: param1, ...queryParams } = params;
  return request<any>(
    `/api/platform/teams/${param0}/invitations/${param1}/revoke`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** Restore platform team original state POST /api/platform/teams/${param0}/restore */
export async function restorePlatformTeam(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.restorePlatformTeamParams,
  body: API.TeamLifecycleRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/platform/teams/${param0}/restore`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Suspend platform team POST /api/platform/teams/${param0}/suspend */
export async function suspendPlatformTeam(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.suspendPlatformTeamParams,
  body: API.TeamLifecycleRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/platform/teams/${param0}/suspend`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/platform/user-exports */
export async function createPlatformUserExport(
  body: API.DirectoryExportRequest,
  options?: RequestOptions,
) {
  return request<any>("/api/platform/user-exports", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/platform/user-exports/${param0} */
export async function getPlatformUserExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getPlatformUserExportParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.DirectoryExportView>(
    `/api/platform/user-exports/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/platform/user-exports/${param0}/download */
export async function downloadPlatformUserExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.downloadPlatformUserExportParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<Blob>(`/api/platform/user-exports/${param0}/download`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || { responseType: "blob" }),
  });
}

/** 此处后端没有提供注释 GET /api/platform/user-statistics */
export async function getPlatformUserStatistics(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getPlatformUserStatisticsParams,
  options?: RequestOptions,
) {
  return request<API.PlatformStatisticsView>("/api/platform/user-statistics", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/platform/users */
export async function listPlatformUsers(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listPlatformUsersParams,
  options?: RequestOptions,
) {
  return request<API.PlatformUserPageView>("/api/platform/users", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/platform/users/${param0} */
export async function getPlatformUser(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getPlatformUserParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.PlatformUserDetailView>(`/api/platform/users/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Recover account governance command GET /api/platform/users/${param0}/commands/${param1} */
export async function getUserGovernanceReceipt(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getUserGovernanceReceiptParams,
  options?: RequestOptions,
) {
  const { id: param0, request: param1, ...queryParams } = params;
  return request<API.UserCommandReceipt>(
    `/api/platform/users/${param0}/commands/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** Disable account POST /api/platform/users/${param0}/disable */
export async function platformDisable(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.platformDisableParams,
  body: API.GovernanceMutation,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/platform/users/${param0}/disable`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Enable account POST /api/platform/users/${param0}/enable */
export async function platformEnable(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.platformEnableParams,
  body: API.GovernanceMutation,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/platform/users/${param0}/enable`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Send platform requested password recovery POST /api/platform/users/${param0}/password-reset */
export async function platformSendPasswordReset(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.platformSendPasswordResetParams,
  body: API.GovernanceMutation,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/platform/users/${param0}/password-reset`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Reset account public profile POST /api/platform/users/${param0}/profile-reset */
export async function platformResetProfile(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.platformResetProfileParams,
  body: API.ProfileGovernanceRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/platform/users/${param0}/profile-reset`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** RevokeAdmin account POST /api/platform/users/${param0}/revoke-admin */
export async function platformRevokeAdmin(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.platformRevokeAdminParams,
  body: API.GovernanceMutation,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/platform/users/${param0}/revoke-admin`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Revoke account sessions POST /api/platform/users/${param0}/revoke-sessions */
export async function platformRevokeSessions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.platformRevokeSessionsParams,
  body: API.SessionGovernanceRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/platform/users/${param0}/revoke-sessions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Unlock account POST /api/platform/users/${param0}/unlock */
export async function platformUnlock(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.platformUnlockParams,
  body: API.GovernanceMutation,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/platform/users/${param0}/unlock`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/registrations */
export async function startRegistration(
  body: API.RegistrationStartRequest,
  options?: RequestOptions,
) {
  return request<any>("/api/registrations", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/registrations/${param0}/complete */
export async function completeRegistration(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.completeRegistrationParams,
  body: API.RegistrationCompleteRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.RegistrationCompleteView>(
    `/api/registrations/${param0}/complete`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/registrations/${param0}/resend */
export async function resendRegistration(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.resendRegistrationParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/registrations/${param0}/resend`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/registrations/${param0}/verify */
export async function verifyRegistration(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.verifyRegistrationParams,
  body: API.RegistrationVerifyRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.RegistrationVerifyView>(
    `/api/registrations/${param0}/verify`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/session */
export async function getSession(options?: RequestOptions) {
  return request<API.EmailSessionView>("/api/session", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/session/refresh */
export async function refreshSession(options?: RequestOptions) {
  return request<API.EmailSessionView>("/api/session/refresh", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/session/upgrade */
export async function upgradeSession(options?: RequestOptions) {
  return request<API.EmailSessionView>("/api/session/upgrade", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/sessions */
export async function createSession(
  body: API.EmailLoginRequest,
  options?: RequestOptions,
) {
  return request<API.EmailSessionView>("/api/sessions", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 DELETE /api/sessions/${param0} */
export async function deleteSession(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteSessionParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/sessions/${param0}`, {
    method: "DELETE",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/teams */
export async function createTeam(
  body: API.TeamCreateRequest,
  options?: RequestOptions,
) {
  return request<API.TeamView>("/api/teams", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/teams/${param0} */
export async function getTeam(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getTeamParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.TeamView>(`/api/teams/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 PATCH /api/teams/${param0} */
export async function renameTeam(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.renameTeamParams,
  body: API.TeamProfileRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.TeamView>(`/api/teams/${param0}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** List team administrator transfers GET /api/teams/${param0}/admin-transfers */
export async function teamAuthorityProposals(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.teamAuthorityProposalsParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ProposalListPage>(`/api/teams/${param0}/admin-transfers`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** ProposeTransfer POST /api/teams/${param0}/admin-transfers */
export async function proposeTransfer(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.proposeTransferParams,
  body: API.AuthorityProposalRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.AuthorityProposal>(
    `/api/teams/${param0}/admin-transfers`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** Cancel team authority transfer POST /api/teams/${param0}/admin-transfers/${param1}/cancel */
export async function cancelTeamTransfer(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelTeamTransferParams,
  body: API.GovernanceMutation,
  options?: RequestOptions,
) {
  const { id: param0, pid: param1, ...queryParams } = params;
  return request<any>(`/api/teams/${param0}/admin-transfers/${param1}/cancel`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/teams/${param0}/audit-events */
export async function getTeamSecurityEvents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getTeamSecurityEventsParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.SecurityEventPageView>(
    `/api/teams/${param0}/audit-events`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** Preview team avatar GET /api/teams/${param0}/avatar */
export async function teamAvatar(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.teamAvatarParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.TeamAvatarPreview>(`/api/teams/${param0}/avatar`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** teamList GET /api/teams/${param0}/invitations */
export async function teamListInvitations(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.teamListInvitationsParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.InvitationListPage>(`/api/teams/${param0}/invitations`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** Invite a team member POST /api/teams/${param0}/invitations */
export async function createTeamInvitation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createTeamInvitationParams,
  body: API.TeamInvitationRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.Invitation>(`/api/teams/${param0}/invitations`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** resendTeam POST /api/teams/${param0}/invitations/${param1}/resend */
export async function resendTeamInvitation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.resendTeamInvitationParams,
  body: API.GovernanceVersion,
  options?: RequestOptions,
) {
  const { id: param0, iid: param1, ...queryParams } = params;
  return request<any>(`/api/teams/${param0}/invitations/${param1}/resend`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** revokeTeam POST /api/teams/${param0}/invitations/${param1}/revoke */
export async function revokeTeamInvitation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.revokeTeamInvitationParams,
  body: API.GovernanceMutation,
  options?: RequestOptions,
) {
  const { id: param0, iid: param1, ...queryParams } = params;
  return request<any>(`/api/teams/${param0}/invitations/${param1}/revoke`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/teams/${param0}/leave */
export async function leaveTeam(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.leaveTeamParams,
  body: API.TeamLeaveRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/teams/${param0}/leave`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/teams/${param0}/member-exports */
export async function createTeamMemberExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createTeamMemberExportParams,
  body: API.DirectoryExportRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/teams/${param0}/member-exports`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/teams/${param0}/member-exports/${param1} */
export async function getTeamMemberExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getTeamMemberExportParams,
  options?: RequestOptions,
) {
  const { id: param0, eid: param1, ...queryParams } = params;
  return request<API.DirectoryExportView>(
    `/api/teams/${param0}/member-exports/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/teams/${param0}/member-exports/${param1}/download */
export async function downloadTeamMemberExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.downloadTeamMemberExportParams,
  options?: RequestOptions,
) {
  const { id: param0, eid: param1, ...queryParams } = params;
  return request<Blob>(
    `/api/teams/${param0}/member-exports/${param1}/download`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || { responseType: "blob" }),
    },
  );
}

/** 此处后端没有提供注释 GET /api/teams/${param0}/member-statistics */
export async function getTeamMemberStatistics(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getTeamMemberStatisticsParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.TeamMemberStatisticsView>(
    `/api/teams/${param0}/member-statistics`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/teams/${param0}/members */
export async function listTeamMembers(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listTeamMembersParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.TeamMemberPageView>(`/api/teams/${param0}/members`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/teams/${param0}/members/${param1} */
export async function getTeamMember(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getTeamMemberParams,
  options?: RequestOptions,
) {
  const { id: param0, uid: param1, ...queryParams } = params;
  return request<API.TeamMemberView>(`/api/teams/${param0}/members/${param1}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Preview directory member avatar GET /api/teams/${param0}/members/${param1}/avatar */
export async function teamMemberAvatar(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.teamMemberAvatarParams,
  options?: RequestOptions,
) {
  const { id: param0, uid: param1, ...queryParams } = params;
  return request<API.TeamAvatarPreview>(
    `/api/teams/${param0}/members/${param1}/avatar`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/teams/${param0}/members/${param1}/remove */
export async function removeTeamMember(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.removeTeamMemberParams,
  body: API.TeamMutation,
  options?: RequestOptions,
) {
  const { id: param0, uid: param1, ...queryParams } = params;
  return request<any>(`/api/teams/${param0}/members/${param1}/remove`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/teams/${param0}/members/${param1}/restore */
export async function restoreTeamMember(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.restoreTeamMemberParams,
  body: API.TeamMutation,
  options?: RequestOptions,
) {
  const { id: param0, uid: param1, ...queryParams } = params;
  return request<any>(`/api/teams/${param0}/members/${param1}/restore`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 PATCH /api/teams/${param0}/members/${param1}/role */
export async function changeTeamMemberRole(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.changeTeamMemberRoleParams,
  body: API.TeamRoleRequest,
  options?: RequestOptions,
) {
  const { id: param0, uid: param1, ...queryParams } = params;
  return request<any>(`/api/teams/${param0}/members/${param1}/role`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/teams/${param0}/members/${param1}/suspend */
export async function suspendTeamMember(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.suspendTeamMemberParams,
  body: API.TeamMutation,
  options?: RequestOptions,
) {
  const { id: param0, uid: param1, ...queryParams } = params;
  return request<any>(`/api/teams/${param0}/members/${param1}/suspend`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
