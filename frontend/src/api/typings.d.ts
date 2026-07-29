declare namespace API {
  type ApiResponseAuthResponse_ = {
    data: AuthResponse;
  };

  type ApiResponseDeletePreflightResponse_ = {
    data: DeletePreflightResponse;
  };

  type ApiResponseDeleteResponse_ = {
    data: DeleteResponse;
  };

  type ApiResponseEpisodeOrderResponse_ = {
    data: EpisodeOrderResponse;
  };

  type ApiResponseEpisodeResponse_ = {
    data: EpisodeResponse;
  };

  type ApiResponseListEpisodeResponse_ = {
    /** Data */
    data: EpisodeResponse[];
  };

  type ApiResponseListWorkspaceResponse_ = {
    /** Data */
    data: WorkspaceResponse[];
  };

  type ApiResponseMeResponse_ = {
    data: MeResponse;
  };

  type ApiResponsePaginatedProjects_ = {
    data: PaginatedProjects;
  };

  type ApiResponseProjectResponse_ = {
    data: ProjectResponse;
  };

  type ApiResponseRevocationResponse_ = {
    data: RevocationResponse;
  };

  type ApiResponseWorkspaceResponse_ = {
    data: WorkspaceResponse;
  };

  type archiveEpisodeApiV1EpisodesEpisodeIdArchivePostParams = {
    episode_id: string;
  };

  type archiveProjectApiV1ProjectsProjectIdArchivePostParams = {
    project_id: string;
  };

  type archiveWorkspaceApiV1WorkspacesWorkspaceIdArchivePostParams = {
    workspace_id: string;
  };

  type AuthResponse = {
    user: UserResponse;
    workspace: WorkspaceResponse;
    /** Access Token */
    access_token: string;
    /** Token Type */
    token_type: string | null;
    /** Expires In */
    expires_in: number;
  };

  type BudgetLimitRequest = {
    /** Amount */
    amount: number | string;
    /** Currency */
    currency: string;
    /** Expected Revision */
    expected_revision: number;
  };

  type ChangePasswordRequest = {
    /** Current Password */
    current_password: string;
    /** New Password */
    new_password: string;
  };

  type createEpisodeApiV1ProjectsProjectIdEpisodesPostParams = {
    project_id: string;
  };

  type DeactivateAccountRequest = {
    /** Confirmation */
    confirmation: string;
  };

  type DeleteBlocker = {
    /** Code */
    code: string;
    /** Resource Type */
    resource_type: string;
    /** Resource Id */
    resource_id: string;
    /** Summary */
    summary: string;
  };

  type deleteEpisodeApiV1EpisodesEpisodeIdDeleteParams = {
    episode_id: string;
    expected_revision: number;
  };

  type deletePreflightApiV1ProjectsProjectIdDeletePreflightPostParams = {
    project_id: string;
  };

  type DeletePreflightResponse = {
    /** Allowed */
    allowed: boolean;
    /** Blockers */
    blockers: DeleteBlocker[];
  };

  type deleteProjectApiV1ProjectsProjectIdDeleteParams = {
    project_id: string;
    expected_revision: number;
  };

  type DeleteResponse = {
    /** Deleted */
    deleted: boolean | null;
  };

  type DependencyStatus = {
    /** Critical */
    critical: boolean;
    /** Status */
    status: "available" | "degraded" | "unavailable";
    /** Reason */
    reason: string | null | null;
  };

  type EpisodeCreateRequest = {
    /** Name */
    name: string;
    /** Target Duration Ms */
    target_duration_ms: number | null;
  };

  type episodeDeletePreflightApiV1EpisodesEpisodeIdDeletePreflightPostParams = {
    episode_id: string;
  };

  type EpisodeOrderResponse = {
    /** Items */
    items: EpisodeResponse[];
    /** Project Revision */
    project_revision: number;
  };

  type EpisodeReorderRequest = {
    /** Episode Ids */
    episode_ids: string[];
    /** Expected Revision */
    expected_revision: number;
  };

  type EpisodeResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Project Id */
    project_id: string;
    /** Name */
    name: string;
    /** Position */
    position: number;
    /** Target Duration Ms */
    target_duration_ms: number;
    /** Status */
    status: "active" | "archived";
    /** Revision */
    revision: number;
    /** Current Script Version Id */
    current_script_version_id: string | null;
    /** Current Timeline Version Id */
    current_timeline_version_id: string | null;
  };

  type EpisodeStateRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type EpisodeUpdateRequest = {
    /** Name */
    name: string | null | null;
    /** Target Duration Ms */
    target_duration_ms: number | null | null;
    /** Expected Revision */
    expected_revision: number;
  };

  type getEpisodeApiV1EpisodesEpisodeIdGetParams = {
    episode_id: string;
  };

  type getProjectApiV1ProjectsProjectIdGetParams = {
    project_id: string;
  };

  type getWorkspaceApiV1WorkspacesWorkspaceIdGetParams = {
    workspace_id: string;
  };

  type HealthResponse = {
    /** Status */
    status: string | null;
  };

  type HTTPValidationError = {
    /** Detail */
    detail: ValidationError[] | null;
  };

  type listEpisodesApiV1ProjectsProjectIdEpisodesGetParams = {
    project_id: string;
    include_archived: boolean | null;
  };

  type listProjectsApiV1ProjectsGetParams = {
    workspace_id: string;
    include_archived: boolean | null;
    search: string | null | null;
    sort: "name" | "created_at" | "updated_at" | null | null;
    order: "asc" | "desc" | null | null;
    limit: number | null | null;
    offset: number | null;
  };

  type listWorkspacesApiV1WorkspacesGetParams = {
    include_archived: boolean | null;
  };

  type LoginRequest = {
    /** Email */
    email: string;
    /** Password */
    password: string;
  };

  type MeResponse = {
    user: UserResponse;
    workspace: WorkspaceResponse;
  };

  type PaginatedProjects = {
    /** Items */
    items: ProjectResponse[];
    /** Total */
    total: number;
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
  };

  type ProfileUpdateRequest = {
    /** Display Name */
    display_name: string | null | null;
    /** Avatar Url */
    avatar_url: string | null | null;
  };

  type ProjectCreateRequest = {
    /** Workspace Id */
    workspace_id: string;
    /** Name */
    name: string;
    /** Description */
    description: string | null | null;
    /** Aspect Ratio */
    aspect_ratio: "9:16" | "16:9" | "1:1" | null;
    /** Language */
    language: string | null;
    /** Visual Style */
    visual_style: string | null | null;
    /** Target Duration Ms */
    target_duration_ms: number | null;
  };

  type ProjectResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Name */
    name: string;
    /** Description */
    description: string | null;
    /** Aspect Ratio */
    aspect_ratio: "9:16" | "16:9" | "1:1";
    /** Language */
    language: string;
    /** Visual Style */
    visual_style: string | null;
    /** Target Duration Ms */
    target_duration_ms: number;
    /** Budget Limit */
    budget_limit: string;
    /** Currency */
    currency: string;
    /** Status */
    status: "active" | "archived";
    /** Revision */
    revision: number;
  };

  type ProjectStateRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type ProjectUpdateRequest = {
    /** Name */
    name: string | null | null;
    /** Description */
    description: string | null | null;
    /** Aspect Ratio */
    aspect_ratio: "9:16" | "16:9" | "1:1" | null | null;
    /** Language */
    language: string | null | null;
    /** Visual Style */
    visual_style: string | null | null;
    /** Target Duration Ms */
    target_duration_ms: number | null | null;
    /** Expected Revision */
    expected_revision: number;
  };

  type ReadinessResponse = {
    /** Status */
    status: "ready" | "degraded" | "unavailable";
    /** Dependencies */
    dependencies: Record<string, any>;
  };

  type RegisterRequest = {
    /** Email */
    email: string;
    /** Password */
    password: string;
    /** Display Name */
    display_name: string;
  };

  type reorderEpisodesApiV1ProjectsProjectIdEpisodesReorderPostParams = {
    project_id: string;
  };

  type restoreEpisodeApiV1EpisodesEpisodeIdRestorePostParams = {
    episode_id: string;
  };

  type restoreProjectApiV1ProjectsProjectIdRestorePostParams = {
    project_id: string;
  };

  type restoreWorkspaceApiV1WorkspacesWorkspaceIdRestorePostParams = {
    workspace_id: string;
  };

  type RevocationResponse = {
    /** Revoked */
    revoked: boolean | null;
  };

  type updateBudgetLimitApiV1ProjectsProjectIdBudgetLimitPostParams = {
    project_id: string;
  };

  type updateEpisodeApiV1EpisodesEpisodeIdPatchParams = {
    episode_id: string;
  };

  type updateProjectApiV1ProjectsProjectIdPatchParams = {
    project_id: string;
  };

  type updateWorkspaceApiV1WorkspacesWorkspaceIdPatchParams = {
    workspace_id: string;
  };

  type UserResponse = {
    /** Id */
    id: string;
    /** Email */
    email: string;
    /** Display Name */
    display_name: string;
    /** Avatar Url */
    avatar_url: string | null;
  };

  type ValidationError = {
    /** Location */
    loc: (string | number)[];
    /** Message */
    msg: string;
    /** Error Type */
    type: string;
    /** Input */
    input: any | null;
    /** Context */
    ctx: Record<string, any> | null;
  };

  type WorkspaceCreateRequest = {
    /** Name */
    name: string;
  };

  type WorkspaceResponse = {
    /** Id */
    id: string;
    /** Name */
    name: string;
    /** Status */
    status: "active" | "archived";
    /** Role */
    role: "owner" | "editor" | "viewer";
    /** Revision */
    revision: number;
  };

  type WorkspaceStateRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type WorkspaceUpdateRequest = {
    /** Name */
    name: string;
    /** Expected Revision */
    expected_revision: number;
  };
}
