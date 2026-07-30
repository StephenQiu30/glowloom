declare namespace API {
  type AcceptNewDecision = {
    /** Action */
    action: "accept_new";
  };

  type AcceptWithChangesDecision = {
    /** Action */
    action: "accept_with_changes";
    /** Proposal */
    proposal:
      | SceneCandidateProposal
      | DialogueCandidateProposal
      | AssetCandidateProposal
      | ShotCandidateProposal
      | ContinuityCandidateProposal;
  };

  type ApiResponseAuthResponse_ = {
    data: AuthResponse;
  };

  type ApiResponseCandidateDecisionResultResponse_ = {
    data: CandidateDecisionResultResponse;
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

  type ApiResponseExtractionBatchResponse_ = {
    data: ExtractionBatchResponse;
  };

  type ApiResponseExtractionCandidateResponse_ = {
    data: ExtractionCandidateResponse;
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

  type ApiResponsePaginatedCandidateDecisions_ = {
    data: PaginatedCandidateDecisions;
  };

  type ApiResponsePaginatedExtractionCandidates_ = {
    data: PaginatedExtractionCandidates;
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

  type ApiResponseScriptVersionDeleteResponse_ = {
    data: ScriptVersionDeleteResponse;
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

  type ApiResponseStructureConfirmationResponse_ = {
    data: StructureConfirmationResponse;
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

  type AssetCandidateProposal = {
    /** Kind */
    kind: "asset";
    /** Asset Kind */
    asset_kind:
      | "character"
      | "location"
      | "prop"
      | "costume"
      | "style"
      | "voice";
    /** Name */
    name: string;
    /** Description */
    description: string;
  };

  type AuthResponse = {
    user: UserResponse;
    workspace: WorkspaceResponse;
    /** Access Token */
    access_token: string;
    /** Token Type */
    token_type: "bearer" | null;
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

  type CandidateDecisionEvidenceResponse = {
    /** Id */
    id: string;
    /** Candidate Id */
    candidate_id: string;
    /** Sequence */
    sequence: number;
    /** Decision Key */
    decision_key: string;
    /** Decision */
    decision:
      | AcceptNewDecision
      | AcceptWithChangesDecision
      | MergeIntoDecision
      | IgnoreDecision;
    /** Actor Id */
    actor_id: string;
    /** Created At */
    created_at: string;
  };

  type CandidateDecisionRequest = {
    /** Decision Key */
    decision_key: string;
    /** Expected Revision */
    expected_revision: number;
    /** Decision */
    decision:
      | AcceptNewDecision
      | AcceptWithChangesDecision
      | MergeIntoDecision
      | IgnoreDecision;
  };

  type CandidateDecisionResultResponse = {
    candidate: ExtractionCandidateResponse;
    evidence: CandidateDecisionEvidenceResponse;
  };

  type CandidateSourceRange = {
    /** Start */
    start: number;
    /** End */
    end: number;
  };

  type ChangePasswordRequest = {
    /** Current Password */
    current_password: string;
    /** New Password */
    new_password: string;
  };

  type confirmStructureApiV1ExtractionBatchesBatchIdConfirmStructurePostParams =
    {
      batch_id: string;
    };

  type ContinuityCandidateProposal = {
    /** Kind */
    kind: "continuity";
    /** Severity */
    severity: "info" | "warning" | "blocking";
    /** Issue */
    issue: string;
    /** Suggestion */
    suggestion: string;
  };

  type CostSummary = {
    /** Status */
    status: "not_started" | null;
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
    impact: ScriptVersionImpactResponse;
  };

  type DeactivateAccountRequest = {
    /** Confirmation */
    confirmation: "DEACTIVATE";
  };

  type decideExtractionCandidateApiV1ExtractionCandidatesCandidateIdDecisionsPostParams =
    {
      candidate_id: string;
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

  type deleteDraftVersionApiV1ScriptVersionsVersionIdDeleteParams = {
    version_id: string;
    confirm: boolean;
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
    deleted: true | null;
  };

  type DependencyStatus = {
    /** Critical */
    critical: boolean;
    /** Status */
    status: "available" | "degraded" | "unavailable";
    /** Reason */
    reason: string | null | null;
  };

  type DialogueCandidateProposal = {
    /** Kind */
    kind: "dialogue";
    /** Scene Candidate Key */
    scene_candidate_key: string;
    /** Speaker Candidate */
    speaker_candidate: string;
    /** Dialogue Kind */
    dialogue_kind: "spoken" | "narration" | "internal" | "voice_over";
    /** Text */
    text: string;
    /** Performance Note */
    performance_note: string | null | null;
  };

  type DialogueResponse = {
    /** Id */
    id: string;
    /** Scene Id */
    scene_id: string;
    /** Position */
    position: number;
    /** Speaker Candidate */
    speaker_candidate: string;
    /** Dialogue Kind */
    dialogue_kind: "spoken" | "narration" | "internal" | "voice_over";
    /** Text */
    text: string;
    /** Performance Note */
    performance_note: string | null;
    source_range: CandidateSourceRange;
    /** Created At */
    created_at: string;
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
    current_stage: "script_import";
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

  type ExtractionBatchResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Script Version Id */
    script_version_id: string;
    /** Scope */
    scope: "full";
    /** Extractor Version */
    extractor_version: string;
    /** Input Hash */
    input_hash: string;
    /** Status */
    status:
      | "queued"
      | "running"
      | "waiting_provider"
      | "succeeded"
      | "failed"
      | "cancelled"
      | "unknown";
    /** Confirmed Script Version Id */
    confirmed_script_version_id: string | null;
    /** Candidate Count */
    candidate_count: number;
    task: TaskResponse;
    /** Created At */
    created_at: string;
  };

  type ExtractionCandidateResponse = {
    /** Id */
    id: string;
    /** Batch Id */
    batch_id: string;
    /** Candidate Key */
    candidate_key: string;
    /** Kind */
    kind: "scene" | "dialogue" | "asset" | "shot" | "continuity";
    source_range: CandidateSourceRange;
    /** Proposal */
    proposal:
      | SceneCandidateProposal
      | DialogueCandidateProposal
      | AssetCandidateProposal
      | ShotCandidateProposal
      | ContinuityCandidateProposal;
    /** Confidence Note */
    confidence_note: string | null;
    /** Required */
    required: boolean;
    /** Status */
    status: "pending" | "accepted" | "linked" | "merged" | "ignored";
    /** Revision */
    revision: number;
    /** Created At */
    created_at: string;
  };

  type getEpisodeApiV1EpisodesEpisodeIdGetParams = {
    episode_id: string;
  };

  type getExtractionBatchApiV1ExtractionBatchesBatchIdGetParams = {
    batch_id: string;
  };

  type getExtractionCandidateApiV1ExtractionCandidatesCandidateIdGetParams = {
    candidate_id: string;
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
    status: "ok" | null;
  };

  type HTTPValidationError = {
    /** Detail */
    detail: ValidationError[] | null;
  };

  type IgnoreDecision = {
    /** Action */
    action: "ignore";
  };

  type importTextSourceApiV1EpisodesEpisodeIdScriptSourcesPostParams = {
    episode_id: string;
  };

  type listCandidateDecisionsApiV1ExtractionCandidatesCandidateIdDecisionsGetParams =
    {
      candidate_id: string;
      limit: number | null | null;
      offset: number | null;
    };

  type listEpisodesApiV1ProjectsProjectIdEpisodesGetParams = {
    project_id: string;
    include_archived: boolean | null;
  };

  type listExtractionCandidatesApiV1ExtractionBatchesBatchIdCandidatesGetParams =
    {
      batch_id: string;
      kind:
        | "scene"
        | "dialogue"
        | "asset"
        | "shot"
        | "continuity"
        | null
        | null;
      status:
        | "pending"
        | "accepted"
        | "linked"
        | "merged"
        | "ignored"
        | null
        | null;
      limit: number | null | null;
      offset: number | null;
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
    task_type: "script_extraction" | null | null;
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

  type MergeIntoDecision = {
    /** Action */
    action: "merge_into";
    /** Target Candidate Id */
    target_candidate_id: string;
  };

  type NextAction = {
    /** Code */
    code: string;
    /** Label */
    label: string;
    /** Href */
    href: string;
  };

  type PaginatedCandidateDecisions = {
    /** Items */
    items: CandidateDecisionEvidenceResponse[];
    /** Total */
    total: number;
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
  };

  type PaginatedExtractionCandidates = {
    /** Items */
    items: ExtractionCandidateResponse[];
    /** Total */
    total: number;
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
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
    status: "not_started" | null;
    /** Pending */
    pending: number | null;
  };

  type RevocationResponse = {
    /** Revoked */
    revoked: true | null;
  };

  type SceneCandidateProposal = {
    /** Kind */
    kind: "scene";
    /** Heading */
    heading: string;
    /** Location */
    location: string;
    /** Time Of Day */
    time_of_day: string;
    /** Summary */
    summary: string;
  };

  type SceneResponse = {
    /** Id */
    id: string;
    /** Script Version Id */
    script_version_id: string;
    /** Position */
    position: number;
    /** Heading */
    heading: string;
    /** Location */
    location: string;
    /** Time Of Day */
    time_of_day: string;
    /** Summary */
    summary: string;
    source_range: CandidateSourceRange;
    /** Dialogues */
    dialogues: DialogueResponse[];
    /** Created At */
    created_at: string;
  };

  type ScriptExtractionRequest = {
    /** Scope */
    scope: "full";
    /** Idempotency Key */
    idempotency_key: string;
  };

  type ScriptImportRequest = {
    /** Input Type */
    input_type: "text";
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

  type ScriptVersionDeleteResponse = {
    /** Deleted */
    deleted: true | null;
    /** Script Version Id */
    script_version_id: string;
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

  type ScriptVersionImpactResponse = {
    /** Previous Script Version Id */
    previous_script_version_id: string | null;
    /** Current Script Version Id */
    current_script_version_id: string;
    /** Affected Shot Ids */
    affected_shot_ids: string[];
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

  type ShotCandidateProposal = {
    /** Kind */
    kind: "shot";
    /** Scene Candidate Key */
    scene_candidate_key: string;
    /** Title */
    title: string;
    /** Purpose */
    purpose: string;
  };

  type startExtractionApiV1ScriptVersionsVersionIdExtractionsPostParams = {
    version_id: string;
  };

  type StructureConfirmationResponse = {
    /** Batch Id */
    batch_id: string;
    /** Source Script Version Id */
    source_script_version_id: string;
    confirmed_version: ScriptVersionResponse;
    /** Scenes */
    scenes: SceneResponse[];
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
    task_type: "script_extraction";
    /** Request Type */
    request_type: "extraction_batch";
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
    status: "not_started" | null;
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
