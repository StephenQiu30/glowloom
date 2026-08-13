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

  type ActionBeat = {
    /** Beat Key */
    beat_key: string;
    /** Order */
    order: number;
    /** Description */
    description: string;
  };

  type AdaptationCancelRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type AdaptationConstraintsResponse = {
    /** Target Duration Ms */
    target_duration_ms: number;
    /** Core Plot Points */
    core_plot_points: string[];
    /** Pacing */
    pacing: "slow" | "balanced" | "fast";
    /** Colloquial Dialogue */
    colloquial_dialogue: boolean;
  };

  type AdaptationDiffResponse = {
    /** Base Version Id */
    base_version_id: string;
    /** Adaptation Run Id */
    adaptation_run_id: string;
    /** Added Lines */
    added_lines: number;
    /** Removed Lines */
    removed_lines: number;
    /** Diff Lines */
    diff_lines: string[];
  };

  type AdaptationDraftUpdateRequest = {
    /** Body */
    body: string;
    /** Expected Revision */
    expected_revision: number;
  };

  type AdaptationPublishRequest = {
    /** Expected Run Revision */
    expected_run_revision: number;
    /** Expected Current Version Id */
    expected_current_version_id: string;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type AdaptationPublishResponse = {
    run: AdaptationRunResponse;
    version: ScriptVersionResponse;
    current: CurrentScriptVersionResponse;
  };

  type AdaptationRunCreateRequest = {
    /** Input Script Version Id */
    input_script_version_id: string;
    /** Target Duration Ms */
    target_duration_ms: number;
    /** Core Plot Points */
    core_plot_points: string[];
    /** Pacing */
    pacing: "slow" | "balanced" | "fast";
    /** Colloquial Dialogue */
    colloquial_dialogue: boolean;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type AdaptationRunResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Episode Id */
    episode_id: string;
    /** Source Id */
    source_id: string;
    /** Input Script Version Id */
    input_script_version_id: string;
    /** Input Hash */
    input_hash: string;
    constraints: AdaptationConstraintsResponse;
    /** Status */
    status:
      | "queued"
      | "running"
      | "succeeded"
      | "published"
      | "failed"
      | "cancelled"
      | "unknown";
    /** Revision */
    revision: number;
    /** Task Id */
    task_id: string | null;
    /** Candidate Body */
    candidate_body: string | null;
    /** Candidate Hash */
    candidate_hash: string | null;
    /** Draft Body */
    draft_body: string | null;
    /** Draft Hash */
    draft_hash: string | null;
    /** Change Summary */
    change_summary: string | null;
    /** Estimated Duration Ms */
    estimated_duration_ms: number | null;
    /** Error Code */
    error_code: string | null;
    /** Published Script Version Id */
    published_script_version_id: string | null;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
  };

  type ApiResponseAdaptationDiffResponse_ = {
    data: AdaptationDiffResponse;
  };

  type ApiResponseAdaptationPublishResponse_ = {
    data: AdaptationPublishResponse;
  };

  type ApiResponseAdaptationRunResponse_ = {
    data: AdaptationRunResponse;
  };

  type ApiResponseAssetAvailabilityResponse_ = {
    data: AssetAvailabilityResponse;
  };

  type ApiResponseAssetBibleResponse_ = {
    data: AssetBibleResponse;
  };

  type ApiResponseAssetDeletePreflightResponse_ = {
    data: AssetDeletePreflightResponse;
  };

  type ApiResponseAssetDeleteResponse_ = {
    data: AssetDeleteResponse;
  };

  type ApiResponseAssetImpactResponse_ = {
    data: AssetImpactResponse;
  };

  type ApiResponseAssetOccurrenceDecisionResponse_ = {
    data: AssetOccurrenceDecisionResponse;
  };

  type ApiResponseAssetReadinessResponse_ = {
    data: AssetReadinessResponse;
  };

  type ApiResponseAssetRenameResponse_ = {
    data: AssetRenameResponse;
  };

  type ApiResponseAssetResponse_ = {
    data: AssetResponse;
  };

  type ApiResponseAssetStateAvailabilityResponse_ = {
    data: AssetStateAvailabilityResponse;
  };

  type ApiResponseAssetStateCreateResponse_ = {
    data: AssetStateCreateResponse;
  };

  type ApiResponseAssetStateCurrentResponse_ = {
    data: AssetStateCurrentResponse;
  };

  type ApiResponseAssetStateReadinessResponse_ = {
    data: AssetStateReadinessResponse;
  };

  type ApiResponseAssetStateResponse_ = {
    data: AssetStateResponse;
  };

  type ApiResponseAssetUpgradeApplyResponse_ = {
    data: AssetUpgradeApplyResponse;
  };

  type ApiResponseAssetUpgradePreflightResponse_ = {
    data: AssetUpgradePreflightResponse;
  };

  type ApiResponseAssetVersionCreateResponse_ = {
    data: AssetVersionCreateResponse;
  };

  type ApiResponseAssetVersionResponse_ = {
    data: AssetVersionResponse;
  };

  type ApiResponseAuthResponse_ = {
    data: AuthResponse;
  };

  type ApiResponseCandidateDecisionResultResponse_ = {
    data: CandidateDecisionResultResponse;
  };

  type ApiResponseConfirmedStructureResponse_ = {
    data: ConfirmedStructureResponse;
  };

  type ApiResponseConsentDetailResponse_ = {
    data: ConsentDetailResponse;
  };

  type ApiResponseCostQueryResponse_ = {
    data: CostQueryResponse;
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

  type ApiResponseEpisodePlanDetailResponse_ = {
    data: EpisodePlanDetailResponse;
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

  type ApiResponseGenerationPreflightResponse_ = {
    data: GenerationPreflightResponse;
  };

  type ApiResponseGenerationSubmissionResponse_ = {
    data: GenerationSubmissionResponse;
  };

  type ApiResponseGenerationTaskCancellationResponse_ = {
    data: GenerationTaskCancellationResponse;
  };

  type ApiResponseImportCommitDetailResponse_ = {
    data: ImportCommitDetailResponse;
  };

  type ApiResponseListEpisodeResponse_ = {
    /** Data */
    data: EpisodeResponse[];
  };

  type ApiResponseListModelCapabilityResponse_ = {
    /** Data */
    data: ModelCapabilityResponse[];
  };

  type ApiResponseListShotResponse_ = {
    /** Data */
    data: ShotResponse[];
  };

  type ApiResponseListShotSpecVersionResponse_ = {
    /** Data */
    data: ShotSpecVersionResponse[];
  };

  type ApiResponseListWorkspaceResponse_ = {
    /** Data */
    data: WorkspaceResponse[];
  };

  type ApiResponseMediaAccessResponse_ = {
    data: MediaAccessResponse;
  };

  type ApiResponseMediaLocationsResponse_ = {
    data: MediaLocationsResponse;
  };

  type ApiResponseMediaObjectResponse_ = {
    data: MediaObjectResponse;
  };

  type ApiResponseMediaVersionResponse_ = {
    data: MediaVersionResponse;
  };

  type ApiResponseMeResponse_ = {
    data: MeResponse;
  };

  type ApiResponseNarrativeDependencyResponse_ = {
    data: NarrativeDependencyResponse;
  };

  type ApiResponseNarrativeImpactResponse_ = {
    data: NarrativeImpactResponse;
  };

  type ApiResponseNarrativeRevisionResponse_ = {
    data: NarrativeRevisionResponse;
  };

  type ApiResponseNarrativeStructureResponse_ = {
    data: NarrativeStructureResponse;
  };

  type ApiResponsePaginatedAssetOccurrences_ = {
    data: PaginatedAssetOccurrences;
  };

  type ApiResponsePaginatedAssets_ = {
    data: PaginatedAssets;
  };

  type ApiResponsePaginatedAssetShotUsages_ = {
    data: PaginatedAssetShotUsages;
  };

  type ApiResponsePaginatedAssetStates_ = {
    data: PaginatedAssetStates;
  };

  type ApiResponsePaginatedAssetVersions_ = {
    data: PaginatedAssetVersions;
  };

  type ApiResponsePaginatedAuditEvents_ = {
    data: PaginatedAuditEvents;
  };

  type ApiResponsePaginatedCandidateDecisions_ = {
    data: PaginatedCandidateDecisions;
  };

  type ApiResponsePaginatedConsents_ = {
    data: PaginatedConsents;
  };

  type ApiResponsePaginatedExtractionCandidates_ = {
    data: PaginatedExtractionCandidates;
  };

  type ApiResponsePaginatedMedia_ = {
    data: PaginatedMedia;
  };

  type ApiResponsePaginatedProjects_ = {
    data: PaginatedProjects;
  };

  type ApiResponsePaginatedSchedules_ = {
    data: PaginatedSchedules;
  };

  type ApiResponsePaginatedScriptDocuments_ = {
    data: PaginatedScriptDocuments;
  };

  type ApiResponsePaginatedScriptSources_ = {
    data: PaginatedScriptSources;
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

  type ApiResponseRegistrationVerificationAccepted_ = {
    data: RegistrationVerificationAccepted;
  };

  type ApiResponseRegistrationVerificationConfirmed_ = {
    data: RegistrationVerificationConfirmed;
  };

  type ApiResponseRevocationResponse_ = {
    data: RevocationResponse;
  };

  type ApiResponseScheduleFireResponse_ = {
    data: ScheduleFireResponse;
  };

  type ApiResponseScheduleResponse_ = {
    data: ScheduleResponse;
  };

  type ApiResponseScriptDocumentAnalysisResponse_ = {
    data: ScriptDocumentAnalysisResponse;
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

  type ApiResponseShotDeletePreflightResponse_ = {
    data: ShotDeletePreflightResponse;
  };

  type ApiResponseShotDeleteResponse_ = {
    data: ShotDeleteResponse;
  };

  type ApiResponseShotOrderResponse_ = {
    data: ShotOrderResponse;
  };

  type ApiResponseShotReadinessBatchResponse_ = {
    data: ShotReadinessBatchResponse;
  };

  type ApiResponseShotReadinessResponse_ = {
    data: ShotReadinessResponse;
  };

  type ApiResponseShotResponse_ = {
    data: ShotResponse;
  };

  type ApiResponseShotSpecCreateResponse_ = {
    data: ShotSpecCreateResponse;
  };

  type ApiResponseShotSpecVersionResponse_ = {
    data: ShotSpecVersionResponse;
  };

  type ApiResponseShotStateResponse_ = {
    data: ShotStateResponse;
  };

  type ApiResponseShotTransformPreflightResponse_ = {
    data: ShotTransformPreflightResponse;
  };

  type ApiResponseShotTransformResponse_ = {
    data: ShotTransformResponse;
  };

  type ApiResponseStructureConfirmationResponse_ = {
    data: StructureConfirmationResponse;
  };

  type ApiResponseTaskResponse_ = {
    data: TaskResponse;
  };

  type ApiResponseUploadCompletionResponse_ = {
    data: UploadCompletionResponse;
  };

  type ApiResponseUploadInitializationResponse_ = {
    data: UploadInitializationResponse;
  };

  type ApiResponseWorkspaceResponse_ = {
    data: WorkspaceResponse;
  };

  type appendAssetVersionApiV1AssetStatesStateIdVersionsPostParams = {
    state_id: string;
  };

  type appendSpecVersionApiV1ShotsShotIdSpecVersionsPostParams = {
    shot_id: string;
  };

  type AppendVersionRequest = {
    /** Workspace Id */
    workspace_id: string;
    /** Kind */
    kind: "image" | "video" | "audio" | "subtitle" | "delivery" | "document";
    /** Filename */
    filename: string;
    /** Size Bytes */
    size_bytes: number;
    /** Mime Type */
    mime_type: string;
    /** Sha256 */
    sha256: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Expected Current Version Id */
    expected_current_version_id: string;
  };

  type applyAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePostParams = {
    asset_version_id: string;
  };

  type archiveAssetApiV1AssetsAssetIdArchivePostParams = {
    asset_id: string;
  };

  type archiveEpisodeApiV1EpisodesEpisodeIdArchivePostParams = {
    episode_id: string;
  };

  type archiveMediaApiV1MediaObjectsMediaObjectIdArchivePostParams = {
    media_object_id: string;
  };

  type ArchiveMediaRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type archiveProjectApiV1ProjectsProjectIdArchivePostParams = {
    project_id: string;
  };

  type archiveShotApiV1ShotsShotIdArchivePostParams = {
    shot_id: string;
  };

  type archiveSourceApiV1ScriptSourcesSourceIdArchivePostParams = {
    source_id: string;
  };

  type archiveWorkspaceApiV1WorkspacesWorkspaceIdArchivePostParams = {
    workspace_id: string;
  };

  type AssetAvailabilityResponse = {
    asset: AssetResponse;
    impact: AssetImpactResponse;
  };

  type AssetBibleAsset = {
    asset: AssetResponse;
    /** States */
    states: AssetBibleState[];
  };

  type AssetBibleResponse = {
    /** Items */
    items: AssetBibleAsset[];
    summary: AssetBibleSummary;
  };

  type AssetBibleState = {
    state: AssetStateResponse;
    current_version: AssetVersionResponse | null;
    /** Occurrences */
    occurrences: AssetOccurrenceResponse[];
    readiness: AssetStateReadinessResponse;
  };

  type AssetBibleSummary = {
    /** Asset Count */
    asset_count: number;
    /** State Count */
    state_count: number;
    /** Ready */
    ready: number;
    /** Draft */
    draft: number;
    /** Blocked */
    blocked: number;
    /** Unavailable */
    unavailable: number;
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
      | "visual_style"
      | "voice";
    /** Name */
    name: string;
    /** Description */
    description: string;
  };

  type AssetCreateRequest = {
    /** Kind */
    kind:
      | "character"
      | "location"
      | "prop"
      | "costume"
      | "visual_style"
      | "voice";
    /** Name */
    name: string;
    /** Aliases */
    aliases: string[] | null;
    /** Tags */
    tags: string[] | null;
  };

  type AssetDeleteBlocker = {
    /** Code */
    code: string;
    /** Summary */
    summary: string;
    /** Version Count */
    version_count: number;
    /** Decision Count */
    decision_count: number;
    /** Related Version Count */
    related_version_count: number;
  };

  type assetDeletePreflightApiV1AssetsAssetIdDeletePreflightGetParams = {
    asset_id: string;
  };

  type AssetDeletePreflightResponse = {
    /** Allowed */
    allowed: boolean;
    /** Blockers */
    blockers: AssetDeleteBlocker[];
  };

  type AssetDeleteResponse = {
    /** Deleted */
    deleted: true | null;
  };

  type assetDisablePreflightApiV1AssetsAssetIdDisablePreflightPostParams = {
    asset_id: string;
  };

  type AssetDisablePreflightRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type AssetDisableRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Impact Hash */
    impact_hash: string;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type AssetEnableRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type AssetEpisodeImpact = {
    /** Episode Id */
    episode_id: string;
    /** Shot Count */
    shot_count: number;
    /** Prompt Snapshot Count */
    prompt_snapshot_count: number;
    /** Active Task Count */
    active_task_count: number;
  };

  type AssetImpactResponse = {
    /** Operation */
    operation: "rename" | "disable_asset" | "disable_state" | "set_current";
    /** Asset Id */
    asset_id: string;
    /** State Id */
    state_id: string | null;
    /** Old Version Id */
    old_version_id: string | null;
    /** New Version Id */
    new_version_id: string | null;
    summary: AssetImpactSummary;
    /** Episodes */
    episodes: AssetEpisodeImpact[];
    /** Shots */
    shots: AssetShotImpact[];
    /** Prompt Snapshots */
    prompt_snapshots: AssetPromptImpact[];
    /** Active Tasks */
    active_tasks: AssetTaskImpact[];
    /** Impact Hash */
    impact_hash: string;
  };

  type AssetImpactSummary = {
    /** Episode Count */
    episode_count: number;
    /** Shot Count */
    shot_count: number;
    /** Spec Version Count */
    spec_version_count: number;
    /** Prompt Snapshot Count */
    prompt_snapshot_count: number;
    /** Active Task Count */
    active_task_count: number;
  };

  type AssetMediaReferenceRequest = {
    /** Media Version Id */
    media_version_id: string;
    /** Purpose */
    purpose:
      | "portrait"
      | "full_body"
      | "expression"
      | "turnaround"
      | "environment"
      | "object"
      | "outfit"
      | "style_reference"
      | "voice_sample";
    /** Position */
    position: number;
  };

  type AssetMediaReferenceResponse = {
    /** Media Version Id */
    media_version_id: string;
    /** Purpose */
    purpose: string;
    /** Position */
    position: number;
  };

  type AssetOccurrenceDecisionResponse = {
    state: AssetStateResponse;
    decision: AssetOccurrenceResponse;
  };

  type AssetOccurrenceRequest = {
    /** Decision */
    decision: "link" | "unlink";
    /** Narrative Unit Id */
    narrative_unit_id: string;
    /** Narrative Unit Version Id */
    narrative_unit_version_id: string;
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type AssetOccurrenceResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Asset State Id */
    asset_state_id: string;
    /** Episode Id */
    episode_id: string;
    /** Narrative Unit Id */
    narrative_unit_id: string;
    /** Narrative Unit Version Id */
    narrative_unit_version_id: string;
    /** Sequence */
    sequence: number;
    /** Decision */
    decision: "link" | "unlink";
    /** Origin */
    origin: "manual" | "script_candidate";
    /** Evidence Hash */
    evidence_hash: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Freshness */
    freshness: "current" | "stale";
    /** Created By */
    created_by: string;
    /** Created At */
    created_at: string;
  };

  type AssetPromptImpact = {
    /** Generation Request Id */
    generation_request_id: string;
    /** Episode Id */
    episode_id: string;
    /** Shot Id */
    shot_id: string;
    /** Shot Spec Version Id */
    shot_spec_version_id: string;
    /** Input Hash */
    input_hash: string;
  };

  type AssetReadinessBlocker = {
    /** Code */
    code: string;
    /** Field Path */
    field_path: string | null | null;
    /** Dependency Type */
    dependency_type: string | null | null;
    /** Dependency Id */
    dependency_id: string | null | null;
    /** Summary */
    summary: string;
    /** Next Action */
    next_action: string;
  };

  type AssetReadinessDependencySnapshot = {
    /** Asset Version Id */
    asset_version_id: string;
    /** Asset State Id */
    asset_state_id: string;
    /** Asset State Revision */
    asset_state_revision: number;
    /** Media Version Ids */
    media_version_ids: string[];
    /** Consent Ids */
    consent_ids: string[];
    /** Evaluated At */
    evaluated_at: string;
  };

  type AssetReadinessResponse = {
    /** Status */
    status: "draft" | "ready" | "blocked";
    /** Blockers */
    blockers: AssetReadinessBlocker[];
    /** Warnings */
    warnings: string[];
    /** Next Actions */
    next_actions: string[];
    dependency_snapshot: AssetReadinessDependencySnapshot;
  };

  type AssetReferenceRequest = {
    /** Slot Key */
    slot_key: string;
    /** Role */
    role:
      | "location"
      | "character"
      | "prop"
      | "costume"
      | "visual_style"
      | "voice";
    /** Asset Version Id */
    asset_version_id: string;
    /** Subject Key */
    subject_key: string | null | null;
  };

  type AssetReferenceResponse = {
    /** Slot Key */
    slot_key: string;
    /** Role */
    role:
      | "location"
      | "character"
      | "prop"
      | "costume"
      | "visual_style"
      | "voice";
    /** Asset Version Id */
    asset_version_id: string;
    /** Asset State Id */
    asset_state_id: string;
    /** Asset Id */
    asset_id: string;
    /** Binding Source */
    binding_source: "manual";
    /** Subject Key */
    subject_key: string | null;
  };

  type assetRenamePreflightApiV1AssetsAssetIdRenamePreflightPostParams = {
    asset_id: string;
  };

  type AssetRenamePreflightRequest = {
    /** New Name */
    new_name: string;
    /** Expected Revision */
    expected_revision: number;
  };

  type AssetRenameRequest = {
    /** New Name */
    new_name: string;
    /** Expected Revision */
    expected_revision: number;
    /** Impact Hash */
    impact_hash: string;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type AssetRenameResponse = {
    asset: AssetResponse;
    impact: AssetImpactResponse;
  };

  type AssetResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Project Id */
    project_id: string;
    /** Kind */
    kind:
      | "character"
      | "location"
      | "prop"
      | "costume"
      | "visual_style"
      | "voice";
    /** Name */
    name: string;
    /** Aliases */
    aliases: string[];
    /** Tags */
    tags: string[];
    /** Status */
    status: "active" | "archived";
    /** Availability */
    availability: "enabled" | "disabled";
    /** Name Revision */
    name_revision: number;
    /** Revision */
    revision: number;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
    /** Warnings */
    warnings: "duplicate_name"[] | null;
  };

  type AssetShotImpact = {
    /** Shot Id */
    shot_id: string;
    /** Shot Title */
    shot_title: string;
    /** Episode Id */
    episode_id: string;
    /** Spec Version Ids */
    spec_version_ids: string[];
    /** Current Spec Version Id */
    current_spec_version_id: string | null;
    /** Slot Keys */
    slot_keys: string[];
  };

  type AssetShotUsageResponse = {
    /** Shot Id */
    shot_id: string;
    /** Shot Title */
    shot_title: string;
    /** Episode Id */
    episode_id: string;
    /** Spec Version Id */
    spec_version_id: string;
    /** Spec Version No */
    spec_version_no: number;
    /** Slot Keys */
    slot_keys: string[];
    /** Is Current */
    is_current: boolean;
  };

  type AssetStateAvailabilityResponse = {
    state: AssetStateResponse;
    impact: AssetImpactResponse;
  };

  type AssetStateCreateRequest = {
    /** State Key */
    state_key: string;
    /** Label */
    label: string;
    /** Description */
    description: string | null;
    /** Expected Asset Revision */
    expected_asset_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type AssetStateCreateResponse = {
    asset: AssetResponse;
    state: AssetStateResponse;
  };

  type AssetStateCurrentPreflightRequest = {
    /** Version Id */
    version_id: string;
    /** Expected Current Version Id */
    expected_current_version_id: string | null;
    /** Expected Revision */
    expected_revision: number;
  };

  type AssetStateCurrentRequest = {
    /** Version Id */
    version_id: string;
    /** Expected Current Version Id */
    expected_current_version_id: string | null;
    /** Expected Revision */
    expected_revision: number;
    /** Impact Hash */
    impact_hash: string;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type AssetStateCurrentResponse = {
    state: AssetStateResponse;
    impact: AssetImpactResponse;
  };

  type assetStateDisablePreflightApiV1AssetStatesStateIdDisablePreflightPostParams =
    {
      state_id: string;
    };

  type AssetStateEnableRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type AssetStateReadinessResponse = {
    /** Status */
    status: "draft" | "ready" | "blocked" | "unavailable";
    /** Blockers */
    blockers: AssetReadinessBlocker[];
    /** Warnings */
    warnings: string[];
    /** Next Actions */
    next_actions: string[];
    dependency_snapshot: AssetStateReadinessSnapshot;
  };

  type AssetStateReadinessSnapshot = {
    /** Asset State Id */
    asset_state_id: string;
    /** Asset State Revision */
    asset_state_revision: number;
    /** Current Version Id */
    current_version_id: string | null;
    /** Occurrence Decision Ids */
    occurrence_decision_ids: string[];
    /** Media Version Ids */
    media_version_ids: string[];
    /** Consent Ids */
    consent_ids: string[];
    /** Evaluated At */
    evaluated_at: string;
  };

  type AssetStateResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Asset Id */
    asset_id: string;
    /** State Key */
    state_key: string;
    /** Label */
    label: string;
    /** Description */
    description: string;
    /** Status */
    status: "active" | "disabled";
    /** Current Version Id */
    current_version_id: string | null;
    /** Revision */
    revision: number;
    /** Created By */
    created_by: string;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
  };

  type AssetStateUpdateRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Label */
    label: string | null | null;
    /** Description */
    description: string | null | null;
  };

  type AssetStatusRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type AssetSummary = {
    /** Status */
    status: "not_started" | "draft" | "blocked" | "ready" | "unavailable";
    /** Total */
    total: number | null;
    /** Versioned */
    versioned: number | null;
    /** Ready */
    ready: number | null;
    /** Draft */
    draft: number | null;
    /** Blocked */
    blocked: number | null;
    /** Ready Kinds */
    ready_kinds: string[];
    /** Required Kinds */
    required_kinds: string[];
  };

  type AssetTaskImpact = {
    /** Task Id */
    task_id: string;
    /** Generation Request Id */
    generation_request_id: string;
    /** Status */
    status: "queued" | "running" | "waiting_provider" | "unknown";
    /** Revision */
    revision: number;
  };

  type AssetUpdateRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Aliases */
    aliases: string[] | null | null;
    /** Tags */
    tags: string[] | null | null;
  };

  type AssetUpgradeApplyRequest = {
    /** New Asset Version Id */
    new_asset_version_id: string;
    /** Targets */
    targets: AssetUpgradeTargetRequest[];
    /** Preflight Hash */
    preflight_hash: string;
  };

  type AssetUpgradeApplyResponse = {
    /** Shots */
    shots: ShotResponse[];
    /** Spec Versions */
    spec_versions: ShotSpecVersionResponse[];
  };

  type AssetUpgradePreflightRequest = {
    /** New Asset Version Id */
    new_asset_version_id: string;
    /** Shot Ids */
    shot_ids: string[];
  };

  type AssetUpgradePreflightResponse = {
    /** Old Asset Version Id */
    old_asset_version_id: string;
    /** New Asset Version Id */
    new_asset_version_id: string;
    /** Targets */
    targets: AssetUpgradeTargetRequest[];
    /** Preflight Hash */
    preflight_hash: string;
  };

  type AssetUpgradeTargetRequest = {
    /** Shot Id */
    shot_id: string;
    /** Expected Spec Version Id */
    expected_spec_version_id: string;
    /** Expected Shot Revision */
    expected_shot_revision: number;
    /** Slot Keys */
    slot_keys: string[];
    /** New Input Hash */
    new_input_hash: string;
  };

  type AssetVersionCreateRequest = {
    /** Spec */
    spec:
      | CharacterSpec
      | LocationSpec
      | PropSpec
      | CostumeSpec
      | StyleSpec
      | VoiceSpec;
    /** Prompt Description */
    prompt_description: string | null;
    /** Media References */
    media_references: AssetMediaReferenceRequest[] | null;
    /** Source Type */
    source_type: "manual" | "script_extraction_candidate" | null;
    /** Source Id */
    source_id: string | null | null;
    /** Expected Revision */
    expected_revision: number;
    /** Expected Current Version Id */
    expected_current_version_id: string | null;
    /** Set As Current */
    set_as_current: boolean | null;
  };

  type AssetVersionCreateResponse = {
    state: AssetStateResponse;
    version: AssetVersionResponse;
    readiness: AssetReadinessResponse;
  };

  type AssetVersionResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Asset Id */
    asset_id: string;
    /** Asset State Id */
    asset_state_id: string;
    /** Version No */
    version_no: number;
    /** Schema Version */
    schema_version: number;
    /** Spec */
    spec:
      | CharacterSpec
      | LocationSpec
      | PropSpec
      | CostumeSpec
      | StyleSpec
      | VoiceSpec;
    /** Prompt Description */
    prompt_description: string;
    /** Source Type */
    source_type: "manual" | "script_extraction_candidate";
    /** Source Id */
    source_id: string | null;
    /** Content Hash */
    content_hash: string;
    /** Media References */
    media_references: AssetMediaReferenceResponse[];
    /** Created By */
    created_by: string;
    /** Created At */
    created_at: string;
  };

  type AudioIntent = {
    /** Ambient */
    ambient: string | null | null;
    /** Sound Effects */
    sound_effects: string[] | null;
  };

  type AuditEventResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Actor Id */
    actor_id: string;
    /** Action */
    action: string;
    /** Target Type */
    target_type: string;
    /** Target Id */
    target_id: string;
    /** Result */
    result: "succeeded" | "denied" | "failed";
    /** Trace Id */
    trace_id: string;
    /** Metadata */
    metadata: Record<string, any>;
    /** Occurred At */
    occurred_at: string;
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

  type cancelGenerationTaskApiV1TasksTaskIdCancelPostParams = {
    task_id: string;
  };

  type cancelRunApiV1AdaptationRunsRunIdCancelPostParams = {
    run_id: string;
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
      | LinkExistingDecision
      | MergeIntoDecision
      | IgnoreDecision;
    /** Downstream Type */
    downstream_type: "ASSET" | null;
    /** Downstream Id */
    downstream_id: string | null;
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
      | LinkExistingDecision
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

  type CapabilityPricingResponse = {
    /** Unit */
    unit: "per_request";
    /** Amount */
    amount: string;
    /** Currency */
    currency: string;
    /** High Cost Threshold */
    high_cost_threshold: string | null;
  };

  type ChangePasswordRequest = {
    /** Current Password */
    current_password: string;
    /** New Password */
    new_password: string;
  };

  type CharacterSpec = {
    /** Kind */
    kind: "character";
    /** Identity */
    identity: string | null;
    /** Appearance */
    appearance: string | null;
    /** Age Impression */
    age_impression: string | null;
    /** Temperament */
    temperament: string[] | null;
  };

  type completeUploadApiV1MediaUploadsUploadSessionIdCompletePostParams = {
    upload_session_id: string;
  };

  type configureScheduleApiV1SchedulesScheduleIdConfigurationPutParams = {
    schedule_id: string;
  };

  type ConfirmedStructureResponse = {
    /** Script Version Id */
    script_version_id: string;
    /** Scenes */
    scenes: SceneResponse[];
  };

  type confirmEpisodePlanApiV1EpisodePlansPlanIdConfirmPostParams = {
    plan_id: string;
  };

  type ConfirmEpisodePlanRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type confirmStructureApiV1ExtractionBatchesBatchIdConfirmStructurePostParams =
    {
      batch_id: string;
    };

  type ConsentCreateRequest = {
    /** Workspace Id */
    workspace_id: string;
    subject_identity: SubjectIdentity;
    scope: MediaUsageScope;
    /** Proof Media Version Ids */
    proof_media_version_ids: string[];
    /** Reason */
    reason: string;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type ConsentDetailResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    subject_identity: SubjectIdentity;
    status: ConsentStatus;
    /** Revision */
    revision: number;
    /** Current Revision Id */
    current_revision_id: string;
    current_revision: ConsentRevisionResponse;
    /** Created By */
    created_by: string;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
    /** Revisions */
    revisions: ConsentRevisionResponse[];
  };

  type ConsentRevisionRequest = {
    /** Expected Revision */
    expected_revision: number;
    scope: MediaUsageScope;
    /** Proof Media Version Ids */
    proof_media_version_ids: string[];
    /** Reason */
    reason: string;
  };

  type ConsentRevisionResponse = {
    /** Id */
    id: string;
    /** Revision No */
    revision_no: number;
    /** Action */
    action: "register" | "update" | "revoke";
    scope: MediaUsageScope;
    /** Proof Media Version Ids */
    proof_media_version_ids: string[];
    /** Reason */
    reason: string;
    /** Created By */
    created_by: string;
    /** Created At */
    created_at: string;
  };

  type ConsentRevokeRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Reason */
    reason: string;
  };

  type ConsentStatus = "active" | "expired" | "revoked";

  type ConsentSummaryResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    subject_identity: SubjectIdentity;
    status: ConsentStatus;
    /** Revision */
    revision: number;
    /** Current Revision Id */
    current_revision_id: string;
    current_revision: ConsentRevisionResponse;
    /** Created By */
    created_by: string;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
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

  type copyShotApiV1ShotsShotIdCopyPostParams = {
    shot_id: string;
  };

  type CopyShotRequest = {
    /** Title */
    title: string;
    /** Expected Source Spec Version Id */
    expected_source_spec_version_id: string;
    /** Expected Order Hash */
    expected_order_hash: string;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type CostEntryResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Reservation Id */
    reservation_id: string;
    /** Request Id */
    request_id: string;
    /** Task Id */
    task_id: string;
    /** Entry Type */
    entry_type: "reserve" | "settle" | "release" | "adjust";
    /** Amount */
    amount: string;
    /** Currency */
    currency: string;
    /** Provider Bill Ref */
    provider_bill_ref: string | null;
    /** Created At */
    created_at: string;
  };

  type CostQueryResponse = {
    /** Currency */
    currency: string;
    summary: CostSummaryResponse;
    /** Items */
    items: CostEntryResponse[];
    /** Total */
    total: number;
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
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

  type CostSummaryResponse = {
    /** Reserved */
    reserved: string;
    /** Settled */
    settled: string;
    /** Released */
    released: string;
    /** Adjustments */
    adjustments: string;
    /** Remaining Reserved */
    remaining_reserved: string;
  };

  type CostumeSpec = {
    /** Kind */
    kind: "costume";
    /** Appearance */
    appearance: string | null;
    /** Material */
    material: string | null;
    /** Usage Context */
    usage_context: string | null;
    /** Wearer Character Id */
    wearer_character_id: string | null | null;
  };

  type createAccessApiV1MediaVersionIdAccessPostParams = {
    version_id: string;
  };

  type createAssetApiV1ProjectsProjectIdAssetsPostParams = {
    project_id: string;
  };

  type createAssetStateApiV1AssetsAssetIdStatesPostParams = {
    asset_id: string;
  };

  type createEpisodeApiV1ProjectsProjectIdEpisodesPostParams = {
    project_id: string;
  };

  type createEpisodePlanApiV1DocumentRevisionsRevisionIdEpisodePlansPostParams =
    {
      revision_id: string;
    };

  type createFromConfirmedCandidateApiV1ExtractionCandidatesCandidateIdShotPostParams =
    {
      candidate_id: string;
    };

  type createManualShotApiV1EpisodesEpisodeIdShotsPostParams = {
    episode_id: string;
  };

  type createRunApiV1EpisodesEpisodeIdAdaptationRunsPostParams = {
    episode_id: string;
  };

  type currentAssetVersionPreflightApiV1AssetStatesStateIdCurrentVersionPreflightPostParams =
    {
      state_id: string;
    };

  type CurrentMediaVersionRequest = {
    /** Version Id */
    version_id: string;
    /** Expected Current Version Id */
    expected_current_version_id: string;
    /** Expected Revision */
    expected_revision: number;
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

  type decideAssetOccurrenceApiV1AssetStatesStateIdOccurrenceDecisionsPostParams =
    {
      state_id: string;
    };

  type decideExtractionCandidateApiV1ExtractionCandidatesCandidateIdDecisionsPostParams =
    {
      candidate_id: string;
    };

  type deleteAssetApiV1AssetsAssetIdDeleteParams = {
    asset_id: string;
    expected_revision: number;
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

  type deleteShotApiV1ShotsShotIdDeleteParams = {
    shot_id: string;
    expected_revision: number;
    expected_order_hash: string;
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

  type DialogueOrNarration = {
    /** Source Dialogue Id */
    source_dialogue_id: string;
    /** Beat Key */
    beat_key: string | null | null;
    /** Speaker Subject Key */
    speaker_subject_key: string | null | null;
    /** Render As Audio */
    render_as_audio: boolean | null;
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

  type diffRunApiV1AdaptationRunsRunIdDiffGetParams = {
    run_id: string;
  };

  type diffVersionsApiV1ScriptVersionsVersionIdDiffGetParams = {
    version_id: string;
    other_version_id: string;
  };

  type disableAssetApiV1AssetsAssetIdDisablePostParams = {
    asset_id: string;
  };

  type disableAssetStateApiV1AssetStatesStateIdDisablePostParams = {
    state_id: string;
  };

  type DocumentRevisionResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Document Id */
    document_id: string;
    /** Version No */
    version_no: number;
    /** Source Type */
    source_type: "text" | "media";
    /** Source Media Version Id */
    source_media_version_id: string | null;
    /** Raw Text */
    raw_text: string;
    /** Raw Hash */
    raw_hash: string;
    /** Normalized Text */
    normalized_text: string;
    /** Normalized Hash */
    normalized_hash: string;
    /** Normalizer Version */
    normalizer_version: string;
    /** Normalization Map */
    normalization_map: Record<string, any>;
    /** Codepoint Count */
    codepoint_count: number;
    /** Analysis Status */
    analysis_status: "deterministic" | "ai_candidate_required" | "rejected";
    /** Analyzer Version */
    analyzer_version: string;
    /** Created By */
    created_by: string;
    /** Created At */
    created_at: string;
  };

  type DownstreamEvidenceResponse = {
    /** Generation Request Ids */
    generation_request_ids: string[] | null;
    /** Candidate Ids */
    candidate_ids: string[] | null;
    /** Review Ids */
    review_ids: string[] | null;
    /** Issue Ids */
    issue_ids: string[] | null;
    /** Timeline Source Ids */
    timeline_source_ids: string[] | null;
  };

  type enableAssetApiV1AssetsAssetIdEnablePostParams = {
    asset_id: string;
  };

  type enableAssetStateApiV1AssetStatesStateIdEnablePostParams = {
    state_id: string;
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

  type EpisodePlanCreateRequest = {
    /** Strategy */
    strategy: "explicit_markers" | "target_duration_ai";
    /** Target Duration Ms */
    target_duration_ms: number;
    /** Requested Episode Count */
    requested_episode_count: number | null | null;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type EpisodePlanDetailResponse = {
    plan: EpisodePlanResponse;
    /** Proposals */
    proposals: EpisodeProposalResponse[];
    impact: EpisodePlanImpactResponse;
    source: EpisodePlanSourceResponse;
  };

  type EpisodePlanImpactBlocker = {
    /** Code */
    code: string;
    /** Summary */
    summary: string;
    /** Next Action */
    next_action: string;
  };

  type EpisodePlanImpactResponse = {
    /** Project Revision */
    project_revision: number;
    /** Active Episode Count */
    active_episode_count: number;
    /** Active Order Hash */
    active_order_hash: string;
    /** Projected Episode Count */
    projected_episode_count: number;
    /** Allowed */
    allowed: boolean;
    /** Blockers */
    blockers: EpisodePlanImpactBlocker[];
  };

  type EpisodePlanResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Project Id */
    project_id: string;
    /** Document Revision Id */
    document_revision_id: string;
    /** Strategy */
    strategy: "explicit_markers" | "target_duration_ai";
    /** Status */
    status:
      | "draft"
      | "review_ready"
      | "confirmed"
      | "materialized"
      | "superseded";
    /** Target Duration Ms */
    target_duration_ms: number;
    /** Requested Episode Count */
    requested_episode_count: number | null;
    /** Total Estimated Duration Ms */
    total_estimated_duration_ms: number;
    /** Input Hash */
    input_hash: string;
    /** Planning Engine Version */
    planning_engine_version: string;
    /** Model Name */
    model_name: string | null;
    /** Prompt Version */
    prompt_version: string | null;
    /** Schema Version */
    schema_version: string;
    /** Planning Task Id */
    planning_task_id: string | null;
    /** Planning Error Code */
    planning_error_code: string | null;
    /** Revision */
    revision: number;
    /** Confirmed By */
    confirmed_by: string | null;
    /** Confirmed At */
    confirmed_at: string | null;
    /** Created By */
    created_by: string;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
  };

  type EpisodePlanSourceResponse = {
    /** Document Revision Id */
    document_revision_id: string;
    /** Normalized Text */
    normalized_text: string;
    /** Normalized Hash */
    normalized_hash: string;
    /** Codepoint Count */
    codepoint_count: number;
    /** Blocks */
    blocks: NarrativeBlockResponse[];
  };

  type EpisodeProductionSnapshot = {
    /** Episode Id */
    episode_id: string;
    /** Current Stage */
    current_stage:
      | "script_import"
      | "structure_review"
      | "asset_preparation"
      | "storyboard_preparation";
    /** Completion */
    completion: number;
    /** Blocking Reasons */
    blocking_reasons: BlockingReason[];
    /** Next Actions */
    next_actions: NextAction[];
    script_summary: ScriptSummary;
    asset_summary: AssetSummary;
    storyboard_summary: StoryboardSummary;
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

  type EpisodeProposalResponse = {
    /** Id */
    id: string;
    /** Plan Id */
    plan_id: string;
    /** Position */
    position: number;
    /** Title */
    title: string;
    /** Start Block Id */
    start_block_id: string;
    /** End Block Id */
    end_block_id: string;
    /** Start Block Position */
    start_block_position: number;
    /** End Block Position */
    end_block_position: number;
    /** Source Start */
    source_start: number;
    /** Source End */
    source_end: number;
    /** Content Hash */
    content_hash: string;
    /** Estimated Duration Ms */
    estimated_duration_ms: number;
    /** Reason */
    reason: string;
    /** Confidence */
    confidence: number;
    /** Boundary Evidence */
    boundary_evidence: Record<string, any>;
    /** Is Locked */
    is_locked: boolean;
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

  type EpisodeSegmentOriginResponse = {
    /** Id */
    id: string;
    /** Import Commit Id */
    import_commit_id: string;
    /** Proposal Id */
    proposal_id: string;
    /** Document Revision Id */
    document_revision_id: string;
    /** Episode Id */
    episode_id: string;
    /** Source Id */
    source_id: string;
    /** Draft Version Id */
    draft_version_id: string;
    /** Published Version Id */
    published_version_id: string | null;
    /** Position */
    position: number;
    /** Source Start */
    source_start: number;
    /** Source End */
    source_end: number;
    /** Source Hash */
    source_hash: string;
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

  type EstimatedCostResponse = {
    /** Amount */
    amount: string;
    /** Currency */
    currency: string;
    /** Pricing Version */
    pricing_version: number;
    /** Unit */
    unit: "per_request" | null;
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

  type FormatIssueResponse = {
    /** Id */
    id: string;
    /** Document Revision Id */
    document_revision_id: string;
    /** Position */
    position: number;
    /** Code */
    code: string;
    /** Severity */
    severity: "warning" | "blocking";
    /** Source Start */
    source_start: number;
    /** Source End */
    source_end: number;
    /** Line Number */
    line_number: number;
    /** Column Number */
    column_number: number;
    /** Next Action */
    next_action: string;
    /** Details */
    details: Record<string, any>;
  };

  type GenerationBlocker = {
    /** Code */
    code: string;
    /** Summary */
    summary: string;
    /** Next Action */
    next_action: string;
  };

  type GenerationConfirmationRequirement = {
    /** Code */
    code: "ACKNOWLEDGE_WARNINGS" | "CONFIRM_HIGH_COST";
    /** Warning Codes */
    warning_codes: string[];
  };

  type GenerationIntent = {
    /** Mode */
    mode: "keyframe_then_video" | "reference_to_video" | "text_to_video";
    /** First Frame */
    first_frame: string | null | null;
    /** Last Frame */
    last_frame: string | null | null;
    /** Keyframe Notes */
    keyframe_notes: string | null | null;
  };

  type GenerationPreflightRequest = {
    /** Workspace Id */
    workspace_id: string;
    /** Shot Spec Version Id */
    shot_spec_version_id: string;
    /** Capability Id */
    capability_id: string;
    /** Parameters */
    parameters: Record<string, any>;
  };

  type GenerationPreflightResponse = {
    /** Shot Id */
    shot_id: string;
    /** Shot Spec Version Id */
    shot_spec_version_id: string;
    /** Capability Id */
    capability_id: string;
    /** Status */
    status: "ready" | "blocked" | "unavailable";
    /** Ready */
    ready: boolean;
    /** Blocking Reasons */
    blocking_reasons: GenerationBlocker[];
    /** Warning Codes */
    warning_codes: string[];
    /** Confirmation Requirements */
    confirmation_requirements: GenerationConfirmationRequirement[];
    estimated_cost: EstimatedCostResponse | null;
    /** Preflight Hash */
    preflight_hash: string;
    /** Expires At */
    expires_at: string;
  };

  type GenerationRequestResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Project Id */
    project_id: string;
    /** Episode Id */
    episode_id: string;
    /** Shot Id */
    shot_id: string;
    /** Shot Spec Version Id */
    shot_spec_version_id: string;
    /** Capability Id */
    capability_id: string;
    /** Capability Config Version */
    capability_config_version: number;
    /** Parameter Snapshot */
    parameter_snapshot: Record<string, any>;
    /** Warning Acknowledgements */
    warning_acknowledgements: string[];
    /** Shot Spec Input Hash */
    shot_spec_input_hash: string;
    /** Input Hash */
    input_hash: string;
    /** High Cost Confirmed */
    high_cost_confirmed: boolean;
    /** Idempotency Key */
    idempotency_key: string;
    /** Requested By */
    requested_by: string;
    /** Created At */
    created_at: string;
  };

  type GenerationSubmissionRequest = {
    /** Workspace Id */
    workspace_id: string;
    /** Shot Spec Version Id */
    shot_spec_version_id: string;
    /** Capability Id */
    capability_id: string;
    /** Parameters */
    parameters: Record<string, any>;
    /** Preflight Hash */
    preflight_hash: string;
    /** Preflight Expires At */
    preflight_expires_at: string;
    /** Warning Acknowledgements */
    warning_acknowledgements: string[] | null;
    /** High Cost Confirmed */
    high_cost_confirmed: boolean | null;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type GenerationSubmissionResponse = {
    request: GenerationRequestResponse;
    task: TaskResponse;
    reservation: ReservationResponse;
    initial_cost_entry: CostEntryResponse;
    /** Outbox Event Id */
    outbox_event_id: string;
    /** Replayed */
    replayed: boolean;
  };

  type GenerationTaskCancellationRequest = {
    /** Workspace Id */
    workspace_id: string;
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Reason */
    reason: "user_requested" | "input_changed" | "budget_changed";
  };

  type GenerationTaskCancellationResponse = {
    task: TaskResponse;
    reservation: ReservationResponse;
    release_cost_entry: CostEntryResponse;
    /** Replayed */
    replayed: boolean;
  };

  type getAssetApiV1AssetsAssetIdGetParams = {
    asset_id: string;
  };

  type getAssetBibleApiV1ProjectsProjectIdAssetBibleGetParams = {
    project_id: string;
    purpose: string;
    channel: string;
    region: string;
  };

  type getAssetReadinessApiV1AssetVersionsVersionIdReadinessGetParams = {
    version_id: string;
    purpose: string;
    channel: string;
    region: string;
  };

  type getAssetStateReadinessApiV1AssetStatesStateIdReadinessGetParams = {
    state_id: string;
    purpose: string;
    channel: string;
    region: string;
  };

  type getAssetVersionApiV1AssetVersionsVersionIdGetParams = {
    version_id: string;
  };

  type getConfirmedStructureApiV1ScriptVersionsVersionIdStructureGetParams = {
    version_id: string;
  };

  type getConsentApiV1ConsentsConsentIdGetParams = {
    consent_id: string;
  };

  type getCostsApiV1CostsGetParams = {
    workspace_id: string;
    project_id: string;
    limit: number | null | null;
    offset: number | null;
  };

  type getEpisodeApiV1EpisodesEpisodeIdGetParams = {
    episode_id: string;
  };

  type getEpisodePlanApiV1EpisodePlansPlanIdGetParams = {
    plan_id: string;
  };

  type getEpisodeReadinessApiV1EpisodesEpisodeIdShotReadinessGetParams = {
    episode_id: string;
  };

  type getExtractionBatchApiV1ExtractionBatchesBatchIdGetParams = {
    batch_id: string;
  };

  type getExtractionCandidateApiV1ExtractionCandidatesCandidateIdGetParams = {
    candidate_id: string;
  };

  type getLatestNarrativeImpactApiV1EpisodesEpisodeIdNarrativeImpactsLatestGetParams =
    {
      episode_id: string;
    };

  type getMediaApiV1MediaVersionIdGetParams = {
    version_id: string;
  };

  type getNarrativeDependencyApiV1EpisodesEpisodeIdNarrativeDependencyGetParams =
    {
      episode_id: string;
      evaluation_hash: string | null | null;
    };

  type getNarrativeStructureApiV1ScriptVersionsVersionIdNarrativeStructureGetParams =
    {
      version_id: string;
      revision: number | null | null;
    };

  type getProjectApiV1ProjectsProjectIdGetParams = {
    project_id: string;
  };

  type getReadinessApiV1ShotsShotIdReadinessGetParams = {
    shot_id: string;
    version_id: string | null | null;
  };

  type getRevisionApiV1DocumentRevisionsRevisionIdGetParams = {
    revision_id: string;
  };

  type getRunApiV1AdaptationRunsRunIdGetParams = {
    run_id: string;
  };

  type getShotApiV1ShotsShotIdGetParams = {
    shot_id: string;
  };

  type getSourceApiV1ScriptSourcesSourceIdGetParams = {
    source_id: string;
  };

  type getSpecVersionApiV1ShotSpecVersionsVersionIdGetParams = {
    version_id: string;
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

  type ImportCommitDetailResponse = {
    commit: ImportCommitResponse;
    /** Segments */
    segments: EpisodeSegmentOriginResponse[];
  };

  type ImportCommitResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Project Id */
    project_id: string;
    /** Plan Id */
    plan_id: string;
    /** Mode */
    mode: "append_new";
    /** Status */
    status:
      | "pending"
      | "materializing"
      | "materialized"
      | "publishing"
      | "published"
      | "conflict"
      | "failed";
    /** Input Hash */
    input_hash: string;
    /** Expected Project Revision */
    expected_project_revision: number;
    /** Expected Active Order Hash */
    expected_active_order_hash: string;
    /** Error Code */
    error_code: string | null;
    /** Revision */
    revision: number;
    /** Created By */
    created_by: string;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
  };

  type importDocumentApiV1ProjectsProjectIdScriptImportsPostParams = {
    project_id: string;
  };

  type importTextSourceApiV1EpisodesEpisodeIdScriptSourcesPostParams = {
    episode_id: string;
  };

  type initializeVersionUploadApiV1MediaObjectsMediaObjectIdVersionsPostParams =
    {
      media_object_id: string;
    };

  type LinkExistingDecision = {
    /** Action */
    action: "link_existing";
    /** Downstream Id */
    downstream_id: string;
  };

  type listArchivedShotsApiV1EpisodesEpisodeIdArchivedShotsGetParams = {
    episode_id: string;
  };

  type listAssetOccurrencesApiV1AssetStatesStateIdOccurrencesGetParams = {
    state_id: string;
    include_history: boolean | null;
  };

  type listAssetsApiV1ProjectsProjectIdAssetsGetParams = {
    project_id: string;
    kind:
      | "character"
      | "location"
      | "prop"
      | "costume"
      | "visual_style"
      | "voice"
      | null
      | null;
    include_archived: boolean | null;
    query: string | null | null;
    limit: number | null | null;
    offset: number | null;
  };

  type listAssetShotUsagesApiV1AssetVersionsAssetVersionIdShotUsagesGetParams =
    {
      asset_version_id: string;
      limit: number | null | null;
      offset: number | null;
    };

  type listAssetStatesApiV1AssetsAssetIdStatesGetParams = {
    asset_id: string;
  };

  type listAssetVersionsApiV1AssetStatesStateIdVersionsGetParams = {
    state_id: string;
    limit: number | null | null;
    offset: number | null;
  };

  type listAuditEventsApiV1AuditEventsGetParams = {
    workspace_id: string;
    actor_id: string | null | null;
    target_type: string | null | null;
    target_id: string | null | null;
    action: string | null | null;
    occurred_from: string | null | null;
    occurred_to: string | null | null;
    limit: number | null | null;
    offset: number | null;
  };

  type listCandidateDecisionsApiV1ExtractionCandidatesCandidateIdDecisionsGetParams =
    {
      candidate_id: string;
      limit: number | null | null;
      offset: number | null;
    };

  type listConsentsApiV1ConsentsGetParams = {
    workspace_id: string;
    limit: number | null | null;
    offset: number | null;
  };

  type listDocumentsApiV1ProjectsProjectIdScriptDocumentsGetParams = {
    project_id: string;
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

  type listMediaApiV1MediaGetParams = {
    workspace_id: string;
    kind:
      | "image"
      | "video"
      | "audio"
      | "subtitle"
      | "delivery"
      | "document"
      | null
      | null;
    source_type: "upload" | "generated" | "rendered" | null | null;
    include_archived: boolean | null;
    created_from: string | null | null;
    created_to: string | null | null;
    limit: number | null | null;
    offset: number | null;
  };

  type listMediaLocationsApiV1MediaVersionIdLocationsGetParams = {
    version_id: string;
  };

  type listModelCapabilitiesApiV1ModelCapabilitiesGetParams = {
    workspace_id: string;
    kind: "image" | "video" | null | null;
    model: string | null | null;
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

  type listSchedulesApiV1SchedulesGetParams = {
    workspace_id: string;
    status:
      | "active"
      | "paused"
      | "completed"
      | "manual_attention"
      | null
      | null;
    limit: number | null | null;
    offset: number | null;
  };

  type listShotsApiV1EpisodesEpisodeIdShotsGetParams = {
    episode_id: string;
  };

  type listSourcesApiV1EpisodesEpisodeIdScriptSourcesGetParams = {
    episode_id: string;
    limit: number | null | null;
    offset: number | null;
  };

  type listSpecVersionsApiV1ShotsShotIdSpecVersionsGetParams = {
    shot_id: string;
  };

  type listTasksApiV1TasksGetParams = {
    workspace_id: string;
    task_type:
      | "script_extraction"
      | "episode_planning"
      | "script_adaptation"
      | "image_generation"
      | "video_generation"
      | "media_probe"
      | "upload_expiration"
      | "upload_cleanup"
      | "media_location_migration"
      | "media_location_retirement"
      | null
      | null;
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

  type LocationSpec = {
    /** Kind */
    kind: "location";
    /** Spatial Description */
    spatial_description: string | null;
    /** Time Weather */
    time_weather: string | null;
    /** Visual Elements */
    visual_elements: string[] | null;
    /** Lighting */
    lighting: string | null;
  };

  type LoginRequest = {
    /** Email */
    email: string;
    /** Password */
    password: string;
  };

  type materializeEpisodePlanApiV1EpisodePlansPlanIdMaterializationsPostParams =
    {
      plan_id: string;
    };

  type MaterializeEpisodePlanRequest = {
    /** Mode */
    mode: "append_new";
    /** Expected Plan Revision */
    expected_plan_revision: number;
    /** Expected Project Revision */
    expected_project_revision: number;
    /** Expected Active Order Hash */
    expected_active_order_hash: string;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type MediaAccessRequest = {
    /** Purpose */
    purpose: "preview" | "download";
  };

  type MediaAccessResponse = {
    /** Method */
    method: "GET" | null;
    /** Url */
    url: string;
    /** Purpose */
    purpose: "preview" | "download";
    /** Expires At */
    expires_at: string;
  };

  type MediaLocationMigrationRequest = {
    /** Idempotency Key */
    idempotency_key: string;
  };

  type MediaLocationResponse = {
    /** Id */
    id: string;
    /** Media Version Id */
    media_version_id: string;
    /** Status */
    status: "verified" | "active" | "retiring" | "retired" | "quarantined";
    /** Rollback Available */
    rollback_available: boolean;
    /** Verified At */
    verified_at: string | null;
    /** Retire After */
    retire_after: string | null;
    /** Retired At */
    retired_at: string | null;
    /** Created At */
    created_at: string;
  };

  type MediaLocationRollbackRequest = {
    /** Idempotency Key */
    idempotency_key: string;
    /** Target Location Id */
    target_location_id: string;
  };

  type MediaLocationsResponse = {
    /** Items */
    items: MediaLocationResponse[];
  };

  type MediaObjectResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Kind */
    kind: "image" | "video" | "audio" | "subtitle" | "delivery" | "document";
    /** Source Type */
    source_type: "upload" | "generated" | "rendered";
    /** Status */
    status: "active" | "archived";
    /** Current Version Id */
    current_version_id: string | null;
    /** Revision */
    revision: number;
  };

  type MediaUsageScope = {
    /** Type */
    type: "media_usage";
    subject_type: SubjectType;
    /** Subject Id */
    subject_id: string;
    /** Rights Holder Role */
    rights_holder_role: string;
    /** Rights Types */
    rights_types: string[];
    /** Authorized Purposes */
    authorized_purposes: string[];
    /** Channels */
    channels: string[];
    /** Regions */
    regions: string[];
    /** Valid From */
    valid_from: string;
    /** Valid To */
    valid_to: string;
  };

  type MediaVersionResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Media Object Id */
    media_object_id: string;
    /** Media Object Kind */
    media_object_kind:
      | "image"
      | "video"
      | "audio"
      | "subtitle"
      | "delivery"
      | "document";
    /** Media Object Source Type */
    media_object_source_type: "upload" | "generated" | "rendered";
    /** Media Object Status */
    media_object_status: "active" | "archived";
    /** Media Object Current Version Id */
    media_object_current_version_id: string | null;
    /** Media Object Revision */
    media_object_revision: number;
    /** Version No */
    version_no: number;
    /** Filename */
    filename: string;
    /** Sha256 */
    sha256: string;
    /** Size Bytes */
    size_bytes: number;
    /** Mime Type */
    mime_type: string;
    /** Probe Status */
    probe_status: "pending" | "ready" | "failed" | "quarantined";
    /** Probe Attempt */
    probe_attempt: number;
    /** Probe Error Code */
    probe_error_code: string | null;
    /** Probe Error Summary */
    probe_error_summary: string | null;
    /** Probe Next Action */
    probe_next_action: string | null;
    /** Width */
    width: number | null;
    /** Height */
    height: number | null;
    /** Duration Ms */
    duration_ms: number | null;
    /** Codec */
    codec: string | null;
    /** Container */
    container: string | null;
    /** Created At */
    created_at: string;
  };

  type MeResponse = {
    user: UserResponse;
    workspace: WorkspaceResponse;
  };

  type MergeEpisodeProposalRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Left Proposal Id */
    left_proposal_id: string;
  };

  type mergeEpisodeProposalsApiV1EpisodePlansPlanIdMergePostParams = {
    plan_id: string;
  };

  type MergeIntoDecision = {
    /** Action */
    action: "merge_into";
    /** Target Candidate Id */
    target_candidate_id: string;
  };

  type MergePreflightRequest = {
    /** Shot Ids */
    shot_ids: string[];
    /** Expected Spec Version Ids */
    expected_spec_version_ids: string[];
    /** Expected Order Hash */
    expected_order_hash: string;
  };

  type MergeShotRequest = {
    /** Shot Ids */
    shot_ids: string[];
    /** Expected Spec Version Ids */
    expected_spec_version_ids: string[];
    /** Expected Order Hash */
    expected_order_hash: string;
    /** Impact Hash */
    impact_hash: string;
    /** Idempotency Key */
    idempotency_key: string;
    target: TargetShotSpecRequest;
  };

  type ModelCapabilityResponse = {
    /** Id */
    id: string;
    /** Provider */
    provider: string;
    /** Model */
    model: string;
    /** Kind */
    kind: "image" | "video";
    /** Config Version */
    config_version: number;
    /** Input Types */
    input_types: string[];
    /** Parameter Schema */
    parameter_schema: Record<string, any>;
    /** Limits */
    limits: Record<string, any>;
    pricing: CapabilityPricingResponse | null;
    /** Status */
    status: "active" | "inactive" | "unavailable";
    /** Unavailable Reason */
    unavailable_reason: string | null;
  };

  type moveEpisodeBoundaryApiV1EpisodePlansPlanIdMoveBoundaryPostParams = {
    plan_id: string;
  };

  type MoveEpisodeBoundaryRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Left Proposal Id */
    left_proposal_id: string;
    /** Source Offset */
    source_offset: number;
  };

  type NarrativeBlockResponse = {
    /** Id */
    id: string;
    /** Document Revision Id */
    document_revision_id: string;
    /** Position */
    position: number;
    /** Kind */
    kind:
      | "preamble"
      | "episode_marker"
      | "scene_heading"
      | "dialogue"
      | "narration"
      | "action"
      | "separator";
    /** Source Start */
    source_start: number;
    /** Source End */
    source_end: number;
    /** Text Hash */
    text_hash: string;
    /** Metadata */
    metadata: Record<string, any>;
  };

  type NarrativeDependencyResponse = {
    /** Episode Id */
    episode_id: string;
    /** Current Script Version Id */
    current_script_version_id: string;
    /** Current Structure Id */
    current_structure_id: string;
    /** Current Structure Revision */
    current_structure_revision: number;
    /** Current Dependency Hash */
    current_dependency_hash: string;
    /** Evaluated Hash */
    evaluated_hash: string | null;
    /** Status */
    status: "fresh" | "stale";
  };

  type NarrativeImpactResponse = {
    /** Id */
    id: string;
    /** Episode Id */
    episode_id: string;
    /** Sequence */
    sequence: number;
    /** Trigger */
    trigger: "current_changed" | "structure_corrected";
    /** Episode Revision */
    episode_revision: number;
    /** Previous Script Version Id */
    previous_script_version_id: string | null;
    /** Current Script Version Id */
    current_script_version_id: string;
    /** Previous Structure Hash */
    previous_structure_hash: string | null;
    /** Current Structure Hash */
    current_structure_hash: string;
    /** Previous Dependency Hash */
    previous_dependency_hash: string | null;
    /** Current Dependency Hash */
    current_dependency_hash: string;
    /** Previous Unit Count */
    previous_unit_count: number;
    /** Current Unit Count */
    current_unit_count: number;
    /** Affected Shot Ids */
    affected_shot_ids: string[];
    /** Invalidated Scopes */
    invalidated_scopes: ("shot_readiness" | "coverage" | "export")[];
    /** Impact Hash */
    impact_hash: string;
    /** Created At */
    created_at: string;
  };

  type NarrativeRevisionResponse = {
    structure: NarrativeStructureResponse;
    impact: NarrativeImpactResponse;
  };

  type NarrativeSpec = {
    /** Purpose */
    purpose: string;
    /** Continuity Note */
    continuity_note: string | null | null;
  };

  type NarrativeStructureResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Episode Id */
    episode_id: string;
    /** Script Version Id */
    script_version_id: string;
    /** Input Hash */
    input_hash: string;
    /** Parser Version */
    parser_version: string;
    /** Structure Hash */
    structure_hash: string;
    /** Dependency Hash */
    dependency_hash: string;
    /** Revision */
    revision: number;
    /** Units */
    units: NarrativeUnitResponse[];
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
  };

  type NarrativeStructureRevisionRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Expected Current Script Version Id */
    expected_current_script_version_id: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Units */
    units: NarrativeUnitRevisionItem[];
  };

  type NarrativeUnitResponse = {
    /** Id */
    id: string;
    /** Unit Id */
    unit_id: string;
    /** Kind */
    kind: "scene_heading" | "action" | "dialogue" | "narration";
    /** Position */
    position: number;
    /** Version No */
    version_no: number;
    source_range: SourceRange;
    /** Exact Text */
    exact_text: string;
    /** Text Hash */
    text_hash: string;
    /** Prefix Text */
    prefix_text: string;
    /** Suffix Text */
    suffix_text: string;
    /** Required For Coverage */
    required_for_coverage: boolean;
    /** Source Scene Id */
    source_scene_id: string | null;
    /** Source Dialogue Id */
    source_dialogue_id: string | null;
    /** Origin */
    origin: "deterministic" | "manual";
    /** Created At */
    created_at: string;
  };

  type NarrativeUnitRevisionItem = {
    /** Unit Id */
    unit_id: string;
    /** Kind */
    kind: "scene_heading" | "action" | "dialogue" | "narration";
    /** Source Start */
    source_start: number;
    /** Source End */
    source_end: number;
    /** Required For Coverage */
    required_for_coverage: boolean;
  };

  type NextAction = {
    /** Code */
    code: string;
    /** Label */
    label: string;
    /** Href */
    href: string;
  };

  type PaginatedAssetOccurrences = {
    /** Items */
    items: AssetOccurrenceResponse[];
    /** Total */
    total: number;
  };

  type PaginatedAssets = {
    /** Items */
    items: AssetResponse[];
    /** Total */
    total: number;
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
  };

  type PaginatedAssetShotUsages = {
    /** Items */
    items: AssetShotUsageResponse[];
    /** Total */
    total: number;
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
  };

  type PaginatedAssetStates = {
    /** Items */
    items: AssetStateResponse[];
    /** Total */
    total: number;
  };

  type PaginatedAssetVersions = {
    /** Items */
    items: AssetVersionResponse[];
    /** Total */
    total: number;
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
  };

  type PaginatedAuditEvents = {
    /** Items */
    items: AuditEventResponse[];
    /** Total */
    total: number;
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
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

  type PaginatedConsents = {
    /** Items */
    items: ConsentSummaryResponse[];
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

  type PaginatedMedia = {
    /** Items */
    items: MediaVersionResponse[];
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

  type PaginatedSchedules = {
    /** Items */
    items: ScheduleResponse[];
    /** Total */
    total: number;
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
  };

  type PaginatedScriptDocuments = {
    /** Items */
    items: ScriptDocumentResponse[];
    /** Total */
    total: number;
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
  };

  type PaginatedScriptSources = {
    /** Items */
    items: ScriptSourceResponse[];
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

  type pauseScheduleApiV1SchedulesScheduleIdPausePostParams = {
    schedule_id: string;
  };

  type preflightAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePreflightPostParams =
    {
      asset_version_id: string;
    };

  type preflightGenerationApiV1ShotsShotIdGenerationPreflightPostParams = {
    shot_id: string;
  };

  type ProbeRetryRequest = {
    /** Idempotency Key */
    idempotency_key: string;
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
    current_stage:
      | "project_setup"
      | "script_import"
      | "structure_review"
      | "asset_preparation"
      | "storyboard_preparation";
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

  type PropSpec = {
    /** Kind */
    kind: "prop";
    /** Appearance */
    appearance: string | null;
    /** Material */
    material: string | null;
    /** Usage Context */
    usage_context: string | null;
    /** Holder Character Id */
    holder_character_id: string | null | null;
  };

  type publishImportCommitApiV1ImportCommitsCommitIdPublishPostParams = {
    commit_id: string;
  };

  type PublishImportCommitRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type publishRunApiV1AdaptationRunsRunIdPublishPostParams = {
    run_id: string;
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
    /** Registration Ticket */
    registration_ticket: string;
    /** Password */
    password: string;
    /** Display Name */
    display_name: string;
  };

  type RegistrationVerificationAccepted = {
    /** Accepted */
    accepted: true | null;
    /** Retry After Seconds */
    retry_after_seconds: number;
  };

  type RegistrationVerificationConfirmed = {
    /** Registration Ticket */
    registration_ticket: string;
    /** Expires In */
    expires_in: number;
  };

  type RegistrationVerificationConfirmRequest = {
    /** Email */
    email: string;
    /** Code */
    code: string;
  };

  type RegistrationVerificationRequest = {
    /** Email */
    email: string;
  };

  type renameAssetApiV1AssetsAssetIdRenamePostParams = {
    asset_id: string;
  };

  type renameEpisodeProposalApiV1EpisodePlansPlanIdRenamePostParams = {
    plan_id: string;
  };

  type RenameEpisodeProposalRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Proposal Id */
    proposal_id: string;
    /** Title */
    title: string;
  };

  type reorderEpisodesApiV1ProjectsProjectIdEpisodesReorderPostParams = {
    project_id: string;
  };

  type reorderShotsApiV1EpisodesEpisodeIdShotsReorderPostParams = {
    episode_id: string;
  };

  type requestMediaLocationMigrationApiV1MediaVersionIdLocationMigrationsPostParams =
    {
      version_id: string;
    };

  type requestMediaLocationRollbackApiV1MediaVersionIdLocationRollbacksPostParams =
    {
      version_id: string;
    };

  type ReservationResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Request Id */
    request_id: string;
    /** Currency */
    currency: string;
    /** Estimated Amount */
    estimated_amount: string;
    /** Reserved Amount */
    reserved_amount: string;
    /** Status */
    status: "active" | "settled" | "released";
    /** Revision */
    revision: number;
    /** Created At */
    created_at: string;
  };

  type restoreAssetApiV1AssetsAssetIdRestorePostParams = {
    asset_id: string;
  };

  type restoreEpisodeApiV1EpisodesEpisodeIdRestorePostParams = {
    episode_id: string;
  };

  type restoreMediaApiV1MediaObjectsMediaObjectIdRestorePostParams = {
    media_object_id: string;
  };

  type restoreProjectApiV1ProjectsProjectIdRestorePostParams = {
    project_id: string;
  };

  type restoreShotApiV1ShotsShotIdRestorePostParams = {
    shot_id: string;
  };

  type restoreSourceApiV1ScriptSourcesSourceIdRestorePostParams = {
    source_id: string;
  };

  type restoreWorkspaceApiV1WorkspacesWorkspaceIdRestorePostParams = {
    workspace_id: string;
  };

  type resumeScheduleApiV1SchedulesScheduleIdResumePostParams = {
    schedule_id: string;
  };

  type retryProbeApiV1MediaVersionIdProbeRetryPostParams = {
    version_id: string;
  };

  type ReviewSummary = {
    /** Status */
    status: "not_started" | "pending" | "completed" | "unavailable";
    /** Pending */
    pending: number | null;
  };

  type reviseConsentApiV1ConsentsConsentIdRevisionsPostParams = {
    consent_id: string;
  };

  type reviseNarrativeStructureApiV1NarrativeStructuresStructureIdRevisionsPostParams =
    {
      structure_id: string;
    };

  type RevocationResponse = {
    /** Revoked */
    revoked: true | null;
  };

  type revokeConsentApiV1ConsentsConsentIdRevokePostParams = {
    consent_id: string;
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

  type ScheduleConfigurationRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Effective From */
    effective_from: string;
    /** Kind */
    kind: "interval" | "cron";
    /** Interval Seconds */
    interval_seconds: number | null | null;
    /** Cron Expression */
    cron_expression: string | null | null;
    /** Timezone */
    timezone: string | null;
    /** Misfire Policy */
    misfire_policy: "skip" | "run_once" | "catch_up";
    /** Max Catch Up */
    max_catch_up: number | null;
    /** Misfire Grace Seconds */
    misfire_grace_seconds: number | null;
  };

  type ScheduleCronRuleResponse = {
    /** Kind */
    kind: "cron" | null;
    /** Expression */
    expression: string;
    /** Misfire Grace Seconds */
    misfire_grace_seconds: number;
  };

  type ScheduleFireResponse = {
    /** Id */
    id: string;
    /** Schedule Id */
    schedule_id: string;
    /** Scheduled For */
    scheduled_for: string;
    /** Trigger Kind */
    trigger_kind: "scheduled" | "manual";
    task: TaskResponse;
  };

  type ScheduleIntervalRuleResponse = {
    /** Kind */
    kind: "interval" | null;
    /** Seconds */
    seconds: number;
    /** Misfire Grace Seconds */
    misfire_grace_seconds: number;
  };

  type ScheduleOneOffRuleResponse = {
    /** Kind */
    kind: "one_off" | null;
    /** At */
    at: string;
    /** Misfire Grace Seconds */
    misfire_grace_seconds: number;
  };

  type ScheduleResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Schedule Key */
    schedule_key: string;
    /** Handler Name */
    handler_name:
      | "expire_upload_session"
      | "cleanup_expired_uploads"
      | "retire_media_location"
      | "unregistered";
    scope: ScheduleScopeResponse;
    /** Kind */
    kind: "one_off" | "interval" | "cron";
    /** Rule */
    rule:
      | ScheduleOneOffRuleResponse
      | ScheduleIntervalRuleResponse
      | ScheduleCronRuleResponse
      | UnknownScheduleRuleResponse;
    /** Timezone */
    timezone: string;
    /** Status */
    status: "active" | "paused" | "completed" | "manual_attention";
    /** Next Fire At */
    next_fire_at: string | null;
    /** Next Attempt At */
    next_attempt_at: string | null;
    /** Misfire Policy */
    misfire_policy: "skip" | "run_once" | "catch_up";
    /** Max Catch Up */
    max_catch_up: number;
    /** Failure Count */
    failure_count: number;
    /** Last Error */
    last_error: string | null;
    /** Revision */
    revision: number;
  };

  type ScheduleResumeRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Resume From */
    resume_from: string;
    /** Misfire Policy */
    misfire_policy: "skip" | "run_once" | "catch_up";
    /** Max Catch Up */
    max_catch_up: number | null;
  };

  type ScheduleScopeResponse = {
    /** Usage Type */
    usage_type: "upload_session" | "workspace" | "media_location";
    /** Usage Id */
    usage_id: string;
  };

  type ScheduleStateRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type ScheduleTriggerRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type ScriptDocumentAnalysisResponse = {
    document: ScriptDocumentResponse;
    revision: DocumentRevisionResponse;
    /** Blocks */
    blocks: NarrativeBlockResponse[];
    /** Issues */
    issues: FormatIssueResponse[];
  };

  type ScriptDocumentImportRequest = {
    /** Input Type */
    input_type: "text" | "media";
    /** Title */
    title: string;
    /** Text */
    text: string | null | null;
    /** Media Version Id */
    media_version_id: string | null | null;
    /** Language */
    language: string;
    /** Rights Declaration */
    rights_declaration: string;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type ScriptDocumentResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Project Id */
    project_id: string;
    /** Title */
    title: string;
    /** Source Type */
    source_type: "text" | "media";
    /** Source Media Version Id */
    source_media_version_id: string | null;
    /** Language */
    language: string;
    /** Rights Declaration */
    rights_declaration: string;
    /** Status */
    status: "active" | "archived";
    /** Revision */
    revision: number;
    /** Created By */
    created_by: string;
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

  type ScriptReference = {
    /** Confirmed Script Version Id */
    confirmed_script_version_id: string;
    /** Scene Id */
    scene_id: string;
    /** Dialogue Ids */
    dialogue_ids: string[] | null;
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

  type ScriptSummary = {
    /** Status */
    status:
      | "not_started"
      | "published"
      | "extracting"
      | "extraction_blocked"
      | "review_required"
      | "confirmation_required"
      | "set_current_required"
      | "confirmed"
      | "unavailable";
    /** Current Version Id */
    current_version_id: string | null;
    /** Extraction Batch Id */
    extraction_batch_id: string | null;
    /** Pending Required Candidates */
    pending_required_candidates: number | null;
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
    /** Narrative Impact Id */
    narrative_impact_id: string;
    /** Previous Narrative Dependency Hash */
    previous_narrative_dependency_hash: string | null;
    /** Current Narrative Dependency Hash */
    current_narrative_dependency_hash: string;
    /** Invalidated Scopes */
    invalidated_scopes: ("shot_readiness" | "coverage" | "export")[];
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

  type setCurrentAssetVersionApiV1AssetStatesStateIdCurrentVersionPostParams = {
    state_id: string;
  };

  type setCurrentMediaVersionApiV1MediaObjectsMediaObjectIdCurrentVersionPostParams =
    {
      media_object_id: string;
    };

  type setCurrentSpecVersionApiV1ShotsShotIdCurrentSpecVersionPostParams = {
    shot_id: string;
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

  type ShotCreateRequest = {
    /** Title */
    title: string;
    /** Source Script Version Id */
    source_script_version_id: string;
    /** Source Scene Id */
    source_scene_id: string;
    /** Creation Key */
    creation_key: string;
  };

  type ShotCurrentSpecRequest = {
    /** Version Id */
    version_id: string;
    /** Expected Current Spec Version Id */
    expected_current_spec_version_id: string | null;
    /** Expected Revision */
    expected_revision: number;
  };

  type ShotDeleteBlocker = {
    /** Code */
    code: "SOURCE_CANDIDATE_EVIDENCE" | "SPEC_VERSION_EVIDENCE";
    /** Summary */
    summary: string;
  };

  type shotDeletePreflightApiV1ShotsShotIdDeletePreflightGetParams = {
    shot_id: string;
  };

  type ShotDeletePreflightResponse = {
    /** Allowed */
    allowed: boolean;
    /** Blockers */
    blockers: ShotDeleteBlocker[];
  };

  type ShotDeleteResponse = {
    /** Deleted */
    deleted: true | null;
    order: ShotOrderResponse;
  };

  type ShotOrderResponse = {
    /** Items */
    items: ShotResponse[];
    /** Order Hash */
    order_hash: string;
  };

  type ShotReadinessBatchResponse = {
    /** Episode Id */
    episode_id: string;
    /** Items */
    items: ShotReadinessResponse[];
    summary: ShotReadinessSummary;
    /** Evaluation Hash */
    evaluation_hash: string;
  };

  type ShotReadinessDependencies = {
    /** Shot Spec Version Id */
    shot_spec_version_id: string | null;
    /** Confirmed Script Version Id */
    confirmed_script_version_id: string;
    /** Current Script Version Id */
    current_script_version_id: string | null;
    /** Narrative Structure Id */
    narrative_structure_id: string | null;
    /** Narrative Structure Revision */
    narrative_structure_revision: number | null;
    /** Narrative Dependency Hash */
    narrative_dependency_hash: string | null;
    /** Scene Id */
    scene_id: string;
    /** Dialogue Ids */
    dialogue_ids: string[];
    /** Asset Version Ids */
    asset_version_ids: string[];
    /** Media Version Ids */
    media_version_ids: string[];
    /** Consent Ids */
    consent_ids: string[];
    /** Asset Evaluation Hashes */
    asset_evaluation_hashes: Record<string, any>;
  };

  type ShotReadinessIssue = {
    /** Code */
    code:
      | "CURRENT_SPEC_MISSING"
      | "SPEC_FIELD_MISSING"
      | "DURATION_OUT_OF_RANGE"
      | "SCRIPT_VERSION_UNAVAILABLE"
      | "SCRIPT_REVISION_NOT_CURRENT"
      | "SOURCE_SCENE_INVALID"
      | "SOURCE_DIALOGUE_INVALID"
      | "LOCATION_REFERENCE_MISSING"
      | "CHARACTER_REFERENCE_MISSING"
      | "VOICE_REFERENCE_MISSING"
      | "ASSET_KIND_MISMATCH"
      | "ASSET_VERSION_UNAVAILABLE"
      | "ASSET_DISABLED"
      | "ASSET_NOT_READY"
      | "MEDIA_REFERENCE_UNAVAILABLE"
      | "RIGHTS_BLOCKED"
      | "DEPENDENCY_UNAVAILABLE";
    /** Field Path */
    field_path: string | null | null;
    /** Dependency Type */
    dependency_type: string | null | null;
    /** Dependency Id */
    dependency_id: string | null | null;
    /** Summary */
    summary: string;
    /** Next Action */
    next_action: string;
  };

  type ShotReadinessResponse = {
    /** Shot Id */
    shot_id: string;
    /** Status */
    status: "ready" | "blocked" | "unavailable";
    /** Ready */
    ready: boolean;
    /** Blocking Reasons */
    blocking_reasons: ShotReadinessIssue[];
    /** Warnings */
    warnings: ShotReadinessWarning[];
    /** Next Actions */
    next_actions: string[];
    evaluated_dependencies: ShotReadinessDependencies;
    /** Evaluation Hash */
    evaluation_hash: string;
  };

  type ShotReadinessSummary = {
    /** Total */
    total: number;
    /** Ready */
    ready: number;
    /** Blocked */
    blocked: number;
    /** Unavailable */
    unavailable: number;
  };

  type ShotReadinessWarning = {
    /** Code */
    code:
      | "DURATION_ABOVE_RECOMMENDED"
      | "ACTION_DENSITY_HIGH"
      | "STYLE_REFERENCE_MISSING";
    /** Field Path */
    field_path: string | null | null;
    /** Summary */
    summary: string;
    /** Next Action */
    next_action: string;
  };

  type ShotReorderRequest = {
    /** Shot Ids */
    shot_ids: string[];
    /** Expected Order Hash */
    expected_order_hash: string;
  };

  type ShotResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Episode Id */
    episode_id: string;
    /** Position */
    position: number;
    /** Title */
    title: string;
    /** Source Script Version Id */
    source_script_version_id: string;
    /** Source Scene Id */
    source_scene_id: string;
    /** Source Candidate Id */
    source_candidate_id: string | null;
    /** Status */
    status: "active" | "archived";
    /** Current Spec Version Id */
    current_spec_version_id: string | null;
    /** Revision */
    revision: number;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
  };

  type ShotSpec = {
    /** Schema Version */
    schema_version: 1 | null;
    script_reference: ScriptReference;
    narrative: NarrativeSpec;
    visual: VisualSpec;
    /** Action Beats */
    action_beats: ActionBeat[];
    /** Dialogue Or Narration */
    dialogue_or_narration: DialogueOrNarration[] | null;
    /** Duration Ms */
    duration_ms: number | null;
    audio_intent: AudioIntent | null | null;
    generation_intent: GenerationIntent;
  };

  type ShotSpecCreateRequest = {
    /** Expected Current Spec Version Id */
    expected_current_spec_version_id: string | null;
    spec: ShotSpec;
    /** Asset References */
    asset_references: AssetReferenceRequest[] | null;
  };

  type ShotSpecCreateResponse = {
    shot: ShotResponse;
    version: ShotSpecVersionResponse;
  };

  type ShotSpecVersionResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Shot Id */
    shot_id: string;
    /** Version No */
    version_no: number;
    /** Schema Version */
    schema_version: 1;
    spec: ShotSpec;
    /** Content Hash */
    content_hash: string;
    /** Input Hash */
    input_hash: string;
    /** Asset References */
    asset_references: AssetReferenceResponse[];
    /** Created By */
    created_by: string;
    /** Created At */
    created_at: string;
  };

  type ShotStateRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Expected Order Hash */
    expected_order_hash: string;
  };

  type ShotStateResponse = {
    shot: ShotResponse;
    order: ShotOrderResponse;
  };

  type ShotTransformEvidenceResponse = {
    /** Id */
    id: string;
    /** Operation */
    operation: "copy" | "split" | "merge";
    /** Source Shot Ids */
    source_shot_ids: string[];
    /** Source Spec Version Ids */
    source_spec_version_ids: string[];
    /** Result Shot Ids */
    result_shot_ids: string[];
    /** Impact Hash */
    impact_hash: string;
    /** Input Hash */
    input_hash: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Actor Id */
    actor_id: string;
    /** Created At */
    created_at: string;
  };

  type ShotTransformPreflightResponse = {
    /** Operation */
    operation: "split" | "merge";
    /** Source Shot Ids */
    source_shot_ids: string[];
    /** Source Spec Version Ids */
    source_spec_version_ids: string[];
    /** Order Hash */
    order_hash: string;
    downstream_evidence: DownstreamEvidenceResponse;
    /** Impact Hash */
    impact_hash: string;
  };

  type ShotTransformResponse = {
    transform: ShotTransformEvidenceResponse;
    /** Shots */
    shots: ShotResponse[];
    /** Spec Versions */
    spec_versions: ShotSpecVersionResponse[];
    order: ShotOrderResponse;
  };

  type ShotUpdateRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Title */
    title: string;
  };

  type SourceRange = {
    /** Start */
    start: number;
    /** End */
    end: number;
  };

  type splitEpisodeProposalApiV1EpisodePlansPlanIdSplitPostParams = {
    plan_id: string;
  };

  type SplitEpisodeProposalRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Proposal Id */
    proposal_id: string;
    /** Source Offset */
    source_offset: number;
    /** New Title */
    new_title: string;
  };

  type splitPreflightApiV1ShotsShotIdSplitPreflightPostParams = {
    shot_id: string;
  };

  type SplitPreflightRequest = {
    /** Expected Source Spec Version Id */
    expected_source_spec_version_id: string;
    /** Expected Order Hash */
    expected_order_hash: string;
  };

  type splitShotApiV1ShotsShotIdSplitPostParams = {
    shot_id: string;
  };

  type SplitShotRequest = {
    /** Expected Source Spec Version Id */
    expected_source_spec_version_id: string;
    /** Expected Order Hash */
    expected_order_hash: string;
    /** Impact Hash */
    impact_hash: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Targets */
    targets: TargetShotSpecRequest[];
  };

  type startExtractionApiV1ScriptVersionsVersionIdExtractionsPostParams = {
    version_id: string;
  };

  type StoryboardSummary = {
    /** Status */
    status: "not_started" | "blocked" | "ready" | "unavailable";
    /** Total */
    total: number | null;
    /** Ready */
    ready: number | null;
    /** Blocked */
    blocked: number | null;
    /** Unavailable */
    unavailable: number | null;
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

  type StyleSpec = {
    /** Kind */
    kind: "visual_style";
    /** Visual Language */
    visual_language: string | null;
    /** Palette */
    palette: string | null;
    /** Lighting Language */
    lighting_language: string | null;
    /** Negative Constraints */
    negative_constraints: string[] | null;
  };

  type SubjectIdentity = {
    /** Reference */
    reference: string;
    kind: SubjectIdentityKind;
  };

  type SubjectIdentityKind =
    | "adult"
    | "fictional_adult"
    | "organization"
    | "minor";

  type SubjectPlacement = {
    /** Subject Key */
    subject_key: string;
    /** Placement */
    placement: string;
  };

  type SubjectType =
    | "SCRIPT_VERSION"
    | "ASSET_VERSION"
    | "SHOT_SPEC_VERSION"
    | "CANDIDATE"
    | "MEDIA_VERSION"
    | "TIMELINE_VERSION"
    | "DELIVERY";

  type submitGenerationApiV1ShotsShotIdGenerationRequestsPostParams = {
    shot_id: string;
  };

  type TargetShotSpecRequest = {
    /** Title */
    title: string;
    spec: ShotSpec;
    /** Asset References */
    asset_references: AssetReferenceRequest[] | null;
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
    task_type:
      | "script_extraction"
      | "episode_planning"
      | "script_adaptation"
      | "image_generation"
      | "video_generation"
      | "media_probe"
      | "upload_expiration"
      | "upload_cleanup"
      | "media_location_migration"
      | "media_location_retirement";
    /** Request Type */
    request_type:
      | "extraction_batch"
      | "episode_plan"
      | "adaptation_run"
      | "generation_request"
      | "media_version"
      | "upload_session"
      | "workspace"
      | "media_location";
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
    status: "not_started" | "running" | "failed" | "succeeded" | "unavailable";
    /** Running */
    running: number | null;
    /** Failed */
    failed: number | null;
    /** Succeeded */
    succeeded: number | null;
    /** Unknown */
    unknown: number | null;
  };

  type triggerScheduleApiV1SchedulesScheduleIdTriggerPostParams = {
    schedule_id: string;
  };

  type UnknownScheduleRuleResponse = {
    /** Kind */
    kind: "unknown" | null;
  };

  type updateAssetApiV1AssetsAssetIdPatchParams = {
    asset_id: string;
  };

  type updateAssetStateApiV1AssetStatesStateIdPatchParams = {
    state_id: string;
  };

  type updateBudgetLimitApiV1ProjectsProjectIdBudgetLimitPostParams = {
    project_id: string;
  };

  type updateDraftApiV1AdaptationRunsRunIdDraftPatchParams = {
    run_id: string;
  };

  type updateEpisodeApiV1EpisodesEpisodeIdPatchParams = {
    episode_id: string;
  };

  type updateProjectApiV1ProjectsProjectIdPatchParams = {
    project_id: string;
  };

  type updateShotApiV1ShotsShotIdPatchParams = {
    shot_id: string;
  };

  type updateWorkspaceApiV1WorkspacesWorkspaceIdPatchParams = {
    workspace_id: string;
  };

  type UploadCapabilityResponse = {
    /** Method */
    method: "PUT" | null;
    /** Url */
    url: string;
    /** Headers */
    headers: Record<string, any>;
    /** Expires At */
    expires_at: string;
  };

  type UploadCompletionResponse = {
    media_object: MediaObjectResponse;
    version: MediaVersionResponse;
    probe_task: TaskResponse;
  };

  type UploadDeclaration = {
    /** Workspace Id */
    workspace_id: string;
    /** Kind */
    kind: "image" | "video" | "audio" | "subtitle" | "delivery" | "document";
    /** Filename */
    filename: string;
    /** Size Bytes */
    size_bytes: number;
    /** Mime Type */
    mime_type: string;
    /** Sha256 */
    sha256: string;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type UploadInitializationResponse = {
    upload_session: UploadSessionResponse;
    upload: UploadCapabilityResponse;
  };

  type UploadSessionResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Media Object Id */
    media_object_id: string | null;
    /** Status */
    status: "pending" | "completed" | "expired" | "failed";
    /** Kind */
    kind: "image" | "video" | "audio" | "subtitle" | "delivery" | "document";
    /** Filename */
    filename: string;
    /** Size Bytes */
    size_bytes: number;
    /** Mime Type */
    mime_type: string;
    /** Sha256 */
    sha256: string;
    /** Expires At */
    expires_at: string;
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

  type VisualSpec = {
    /** Shot Size */
    shot_size:
      | "extreme_wide"
      | "wide"
      | "full"
      | "medium"
      | "medium_close_up"
      | "close_up"
      | "extreme_close_up";
    /** Camera Angle */
    camera_angle: "eye_level" | "high" | "low" | "bird_eye" | "dutch";
    /** Camera Movement */
    camera_movement:
      | "static"
      | "pan"
      | "tilt"
      | "dolly"
      | "truck"
      | "pedestal"
      | "zoom"
      | "handheld"
      | "orbit";
    /** Composition */
    composition: string;
    /** Environment */
    environment: string;
    /** Subject Placements */
    subject_placements: SubjectPlacement[] | null;
    /** Mood Lighting */
    mood_lighting: string;
  };

  type VoiceSpec = {
    /** Kind */
    kind: "voice";
    /** Source Kind */
    source_kind:
      | "synthetic_recording"
      | "human_recording"
      | "voice_clone"
      | null
      | null;
    /** Language */
    language: string | null;
    /** Performance Traits */
    performance_traits: string[] | null;
    /** Allowed Usage */
    allowed_usage: string[] | null;
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
