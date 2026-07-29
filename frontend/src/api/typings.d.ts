declare namespace API {
  type ApiResponseAuthResponse_ = {
    data: AuthResponse;
  };

  type ApiResponseCurrentScriptVersionResponse_ = {
    data: CurrentScriptVersionResponse;
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

  type ApiResponseEpisodeProductionSnapshot_ = {
    data: EpisodeProductionSnapshot;
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

  type ApiResponsePaginatedScriptVersions_ = {
    data: PaginatedScriptVersions;
  };

  type ApiResponsePaginatedTasks_ = {
    data: PaginatedTasks;
  };

  type ApiResponseProjectProductionSnapshot_ = {
    data: ProjectProductionSnapshot;
  };

  type ApiResponseProjectResponse_ = {
    data: ProjectResponse;
  };

  type ApiResponseRevocationResponse_ = {
    data: RevocationResponse;
  };

  type ApiResponseScriptImportResponse_ = {
    data: ScriptImportResponse;
  };

  type ApiResponseScriptSourceResponse_ = {
    data: ScriptSourceResponse;
  };

  type ApiResponseScriptVersionDiffResponse_ = {
    data: ScriptVersionDiffResponse;
  };

  type ApiResponseScriptVersionPublishResponse_ = {
    data: ScriptVersionPublishResponse;
  };

  type ApiResponseScriptVersionResponse_ = {
    data: ScriptVersionResponse;
  };

  type ApiResponseTaskResponse_ = {
    data: TaskResponse;
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

  type archiveSourceApiV1ScriptSourcesSourceIdArchivePostParams = {
    source_id: string;
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

  type BlockingReason = {
    /** Code */
    code: string;
    /** Summary */
    summary: string;
    /** Resource Type */
    resource_type: "project" | "episode";
    /** Resource Id */
    resource_id: string;
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

  type CostSummary = {
    /** Status */
    status: string | null;
    /** Currency */
    currency: string;
    /** Reserved */
    reserved: string | null;
    /** Used */
    used: string | null;
  };

  type createEpisodeApiV1ProjectsProjectIdEpisodesPostParams = {
    project_id: string;
  };

  type CurrentScriptVersionRequest = {
    /** Version Id */
    version_id: string;
    /** Expected Current Version Id */
    expected_current_version_id: string | null;
  };

  type CurrentScriptVersionResponse = {
    /** Episode Id */
    episode_id: string;
    /** Current Script Version Id */
    current_script_version_id: string;
    /** Episode Revision */
    episode_revision: number;
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

  type diffVersionsApiV1ScriptVersionsVersionIdDiffGetParams = {
    version_id: string;
    other_version_id: string;
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

  type EpisodeProductionSnapshot = {
    /** Episode Id */
    episode_id: string;
    /** Current Stage */
    current_stage: string;
    /** Completion */
    completion: number;
    /** Blocking Reasons */
    blocking_reasons: BlockingReason[];
    /** Next Actions */
    next_actions: NextAction[];
    task_summary: TaskSummary;
    review_summary: ReviewSummary;
    cost_summary: CostSummary;
    /** Partial Failures */
    partial_failures: PartialFailure[];
    /** Computed At */
    computed_at: string;
  };

  type episodeProductionSnapshotApiV1EpisodesEpisodeIdProductionSnapshotGetParams =
    {
      episode_id: string;
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

  type getSourceApiV1ScriptSourcesSourceIdGetParams = {
    source_id: string;
  };

  type getTaskApiV1TasksTaskIdGetParams = {
    task_id: string;
  };

  type getVersionApiV1ScriptVersionsVersionIdGetParams = {
    version_id: string;
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

  type importTextSourceApiV1EpisodesEpisodeIdScriptSourcesPostParams = {
    episode_id: string;
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

  type listTasksApiV1TasksGetParams = {
    workspace_id: string;
    task_type: string | null | null;
    status:
      | "queued"
      | "running"
      | "waiting_provider"
      | "succeeded"
      | "failed"
      | "cancelled"
      | "unknown"
      | null
      | null;
    limit: number | null | null;
    offset: number | null;
  };

  type listVersionsApiV1ScriptSourcesSourceIdVersionsGetParams = {
    source_id: string;
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

  type NextAction = {
    /** Code */
    code: string;
    /** Label */
    label: string;
    /** Href */
    href: string;
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

  type PaginatedScriptVersions = {
    /** Items */
    items: ScriptVersionResponse[];
    /** Total */
    total: number;
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
  };

  type PaginatedTasks = {
    /** Items */
    items: TaskResponse[];
    /** Total */
    total: number;
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
  };

  type PartialFailure = {
    /** Module */
    module: string;
    /** Code */
    code: string;
    /** Summary */
    summary: string;
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

  type ProjectProductionSnapshot = {
    /** Project Id */
    project_id: string;
    /** Current Stage */
    current_stage: "project_setup" | "script_import";
    /** Completion */
    completion: number;
    /** Blocking Reasons */
    blocking_reasons: BlockingReason[];
    /** Next Actions */
    next_actions: NextAction[];
    /** Episodes */
    episodes: EpisodeProductionSnapshot[];
    /** Partial Failures */
    partial_failures: PartialFailure[];
    /** Computed At */
    computed_at: string;
  };

  type projectProductionSnapshotApiV1ProjectsProjectIdProductionSnapshotGetParams =
    {
      project_id: string;
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

  type publishVersionApiV1ScriptSourcesSourceIdVersionsPostParams = {
    source_id: string;
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

  type restoreSourceApiV1ScriptSourcesSourceIdRestorePostParams = {
    source_id: string;
  };

  type restoreWorkspaceApiV1WorkspacesWorkspaceIdRestorePostParams = {
    workspace_id: string;
  };

  type ReviewSummary = {
    /** Status */
    status: string | null;
    /** Pending */
    pending: number | null;
  };

  type RevocationResponse = {
    /** Revoked */
    revoked: boolean | null;
  };

  type ScriptImportRequest = {
    /** Input Type */
    input_type: string;
    /** Title */
    title: string;
    /** Body */
    body: string;
    /** Rights Declaration */
    rights_declaration: string;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type ScriptImportResponse = {
    source: ScriptSourceResponse;
    version: ScriptVersionResponse;
  };

  type ScriptSourceResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Episode Id */
    episode_id: string;
    /** Input Type */
    input_type: "text" | "media";
    /** Title */
    title: string;
    /** Source Media Version Id */
    source_media_version_id: string | null;
    /** Rights Declaration */
    rights_declaration: string;
    /** Status */
    status: "active" | "archived";
    /** Revision */
    revision: number;
    /** Created At */
    created_at: string;
  };

  type ScriptSourceStateRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type ScriptVersionDiffResponse = {
    /** Base Version Id */
    base_version_id: string;
    /** Target Version Id */
    target_version_id: string;
    /** Added Lines */
    added_lines: number;
    /** Removed Lines */
    removed_lines: number;
    /** Diff Lines */
    diff_lines: string[];
  };

  type ScriptVersionPublishRequest = {
    /** Body */
    body: string;
    /** Expected Current Version Id */
    expected_current_version_id: string | null;
  };

  type ScriptVersionPublishResponse = {
    version: ScriptVersionResponse;
    current: CurrentScriptVersionResponse;
  };

  type ScriptVersionResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Source Id */
    source_id: string;
    /** Version No */
    version_no: number;
    /** Status */
    status: "draft" | "published";
    /** Body */
    body: string;
    /** Content Hash */
    content_hash: string;
    /** Created By */
    created_by: string;
    /** Created At */
    created_at: string;
  };

  type setCurrentVersionApiV1EpisodesEpisodeIdCurrentScriptVersionPostParams = {
    episode_id: string;
  };

  type TaskErrorResponse = {
    /** Code */
    code: string;
    /** Retryable */
    retryable: boolean;
    /** Summary */
    summary: string;
  };

  type TaskResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Task Type */
    task_type: string;
    /** Request Type */
    request_type: string;
    /** Request Id */
    request_id: string;
    scope: TaskScopeResponse;
    /** Status */
    status:
      | "queued"
      | "running"
      | "waiting_provider"
      | "succeeded"
      | "failed"
      | "cancelled"
      | "unknown";
    /** Progress Stage */
    progress_stage: string;
    error: TaskErrorResponse | null;
    /** Next Action */
    next_action: string | null;
    /** Cancel Status */
    cancel_status: "none" | "requested" | "accepted" | "rejected";
    /** Revision */
    revision: number;
  };

  type TaskScopeResponse = {
    /** Episode Id */
    episode_id: string | null;
    /** Render Snapshot Id */
    render_snapshot_id: string | null;
    /** Usage Type */
    usage_type: string | null;
    /** Usage Id */
    usage_id: string | null;
    /** Input Version Id */
    input_version_id: string | null;
    /** Input Hash */
    input_hash: string | null;
  };

  type TaskSummary = {
    /** Status */
    status: string | null;
    /** Running */
    running: number | null;
    /** Failed */
    failed: number | null;
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
