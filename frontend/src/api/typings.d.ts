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
    /** Description */
    description: string;
    /** Order */
    order: number;
  };

  type AdaptationCancelRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type AdaptationConstraintsResponse = {
    /** Colloquial Dialogue */
    colloquial_dialogue: boolean;
    /** Core Plot Points */
    core_plot_points: string[];
    /** Pacing */
    pacing: "slow" | "balanced" | "fast";
    /** Target Duration Ms */
    target_duration_ms: number;
  };

  type AdaptationDiffResponse = {
    /** Adaptation Run Id */
    adaptation_run_id: string;
    /** Added Lines */
    added_lines: number;
    /** Base Version Id */
    base_version_id: string;
    /** Diff Lines */
    diff_lines: string[];
    /** Removed Lines */
    removed_lines: number;
  };

  type AdaptationDraftUpdateRequest = {
    /** Body */
    body: string;
    /** Expected Revision */
    expected_revision: number;
  };

  type AdaptationPublishRequest = {
    /** Expected Current Version Id */
    expected_current_version_id: string;
    /** Expected Run Revision */
    expected_run_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type AdaptationPublishResponse = {
    current: CurrentScriptVersionResponse;
    run: AdaptationRunResponse;
    version: ScriptVersionResponse;
  };

  type AdaptationRunCreateRequest = {
    /** Colloquial Dialogue */
    colloquial_dialogue: boolean;
    /** Core Plot Points */
    core_plot_points: string[];
    /** Idempotency Key */
    idempotency_key: string;
    /** Input Script Version Id */
    input_script_version_id: string;
    /** Pacing */
    pacing: "slow" | "balanced" | "fast";
    /** Target Duration Ms */
    target_duration_ms: number;
  };

  type AdaptationRunResponse = {
    /** Candidate Body */
    candidate_body: string | null;
    /** Candidate Hash */
    candidate_hash: string | null;
    /** Change Summary */
    change_summary: string | null;
    constraints: AdaptationConstraintsResponse;
    /** Created At */
    created_at: string;
    /** Draft Body */
    draft_body: string | null;
    /** Draft Hash */
    draft_hash: string | null;
    /** Episode Id */
    episode_id: string;
    /** Error Code */
    error_code: string | null;
    /** Estimated Duration Ms */
    estimated_duration_ms: number | null;
    /** Id */
    id: string;
    /** Input Hash */
    input_hash: string;
    /** Input Script Version Id */
    input_script_version_id: string;
    /** Published Script Version Id */
    published_script_version_id: string | null;
    /** Revision */
    revision: number;
    /** Source Id */
    source_id: string;
    /** Status */
    status:
      | "queued"
      | "running"
      | "succeeded"
      | "published"
      | "failed"
      | "cancelled"
      | "unknown";
    /** Task Id */
    task_id: string | null;
    /** Updated At */
    updated_at: string;
    /** Workspace Id */
    workspace_id: string;
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

  type ApiResponseCoverageDecisionApplyResponse_ = {
    data: CoverageDecisionApplyResponse;
  };

  type ApiResponseCoverageReportResponse_ = {
    data: CoverageReportResponse;
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

  type ApiResponseDraftApplyPreflightResponse_ = {
    data: DraftApplyPreflightResponse;
  };

  type ApiResponseDraftApplyResponse_ = {
    data: DraftApplyResponse;
  };

  type ApiResponseDraftBatchResponse_ = {
    data: DraftBatchResponse;
  };

  type ApiResponseDraftDecisionResult_ = {
    data: DraftDecisionResult;
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

  type ApiResponseExportHistoryResponse_ = {
    data: ExportHistoryResponse;
  };

  type ApiResponseExportPreflightResponse_ = {
    data: ExportPreflightResponse;
  };

  type ApiResponseExportResponse_ = {
    data: ExportResponse;
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

  type ApiResponseNarrativeReferenceReplaceResponse_ = {
    data: NarrativeReferenceReplaceResponse;
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

  type ApiResponseScriptDocumentPreviewResponse_ = {
    data: ScriptDocumentPreviewResponse;
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
    /** Expected Current Version Id */
    expected_current_version_id: string;
    /** Filename */
    filename: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Kind */
    kind: "image" | "video" | "audio" | "subtitle" | "delivery" | "document";
    /** Mime Type */
    mime_type: string;
    /** Sha256 */
    sha256: string;
    /** Size Bytes */
    size_bytes: number;
    /** Workspace Id */
    workspace_id: string;
  };

  type applyAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePostParams = {
    asset_version_id: string;
  };

  type applyBatchApiV1StoryboardDraftBatchesBatchIdApplyPostParams = {
    batch_id: string;
  };

  type approveBatchApiV1StoryboardDraftBatchesBatchIdApprovePostParams = {
    batch_id: string;
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
    current_version: AssetVersionResponse | null;
    /** Occurrences */
    occurrences: AssetOccurrenceResponse[];
    readiness: AssetStateReadinessResponse;
    state: AssetStateResponse;
  };

  type AssetBibleSummary = {
    /** Asset Count */
    asset_count: number;
    /** Blocked */
    blocked: number;
    /** Draft */
    draft: number;
    /** Ready */
    ready: number;
    /** State Count */
    state_count: number;
    /** Unavailable */
    unavailable: number;
  };

  type AssetCandidateProposal = {
    /** Aliases */
    aliases: string[] | null;
    /** Appearance */
    appearance: string | null | null;
    /** Arc Summary */
    arc_summary: string | null | null;
    /** Asset Kind */
    asset_kind:
      | "character"
      | "location"
      | "prop"
      | "costume"
      | "visual_style"
      | "voice";
    /** Continuity Notes */
    continuity_notes: string[] | null;
    /** Description */
    description: string;
    /** Episode Numbers */
    episode_numbers: number[] | null;
    /** First Seen Episode */
    first_seen_episode: number | null | null;
    /** Goals */
    goals: string[] | null;
    /** Kind */
    kind: "asset";
    /** Name */
    name: string;
    /** Relationships */
    relationships: string[] | null;
    /** Role */
    role: string | null | null;
    /** Visual Identity */
    visual_identity: string | null | null;
    /** Voice Profile */
    voice_profile: string | null | null;
  };

  type AssetCreateRequest = {
    /** Aliases */
    aliases: string[] | null;
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
    /** Tags */
    tags: string[] | null;
  };

  type AssetDeleteBlocker = {
    /** Code */
    code: string;
    /** Decision Count */
    decision_count: number;
    /** Related Version Count */
    related_version_count: number;
    /** Summary */
    summary: string;
    /** Version Count */
    version_count: number;
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
    /** Idempotency Key */
    idempotency_key: string;
    /** Impact Hash */
    impact_hash: string;
  };

  type AssetEnableRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type AssetEpisodeImpact = {
    /** Active Task Count */
    active_task_count: number;
    /** Episode Id */
    episode_id: string;
    /** Prompt Snapshot Count */
    prompt_snapshot_count: number;
    /** Shot Count */
    shot_count: number;
  };

  type AssetImpactResponse = {
    /** Active Tasks */
    active_tasks: AssetTaskImpact[];
    /** Asset Id */
    asset_id: string;
    /** Episodes */
    episodes: AssetEpisodeImpact[];
    /** Impact Hash */
    impact_hash: string;
    /** New Version Id */
    new_version_id: string | null;
    /** Old Version Id */
    old_version_id: string | null;
    /** Operation */
    operation: "rename" | "disable_asset" | "disable_state" | "set_current";
    /** Prompt Snapshots */
    prompt_snapshots: AssetPromptImpact[];
    /** Shots */
    shots: AssetShotImpact[];
    /** State Id */
    state_id: string | null;
    summary: AssetImpactSummary;
  };

  type AssetImpactSummary = {
    /** Active Task Count */
    active_task_count: number;
    /** Episode Count */
    episode_count: number;
    /** Prompt Snapshot Count */
    prompt_snapshot_count: number;
    /** Shot Count */
    shot_count: number;
    /** Spec Version Count */
    spec_version_count: number;
  };

  type AssetMediaReferenceRequest = {
    /** Media Version Id */
    media_version_id: string;
    /** Position */
    position: number;
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
  };

  type AssetMediaReferenceResponse = {
    /** Media Version Id */
    media_version_id: string;
    /** Position */
    position: number;
    /** Purpose */
    purpose: string;
  };

  type AssetOccurrenceDecisionResponse = {
    decision: AssetOccurrenceResponse;
    state: AssetStateResponse;
  };

  type AssetOccurrenceRequest = {
    /** Decision */
    decision: "link" | "unlink";
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Narrative Unit Id */
    narrative_unit_id: string;
    /** Narrative Unit Version Id */
    narrative_unit_version_id: string;
  };

  type AssetOccurrenceResponse = {
    /** Asset State Id */
    asset_state_id: string;
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Decision */
    decision: "link" | "unlink";
    /** Episode Id */
    episode_id: string;
    /** Evidence Hash */
    evidence_hash: string;
    /** Freshness */
    freshness: "current" | "stale";
    /** Id */
    id: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Narrative Unit Id */
    narrative_unit_id: string;
    /** Narrative Unit Version Id */
    narrative_unit_version_id: string;
    /** Origin */
    origin: "manual" | "script_candidate";
    /** Sequence */
    sequence: number;
    /** Workspace Id */
    workspace_id: string;
  };

  type AssetPromptImpact = {
    /** Episode Id */
    episode_id: string;
    /** Generation Request Id */
    generation_request_id: string;
    /** Input Hash */
    input_hash: string;
    /** Shot Id */
    shot_id: string;
    /** Shot Spec Version Id */
    shot_spec_version_id: string;
  };

  type AssetReadinessBlocker = {
    /** Code */
    code: string;
    /** Dependency Id */
    dependency_id: string | null | null;
    /** Dependency Type */
    dependency_type: string | null | null;
    /** Field Path */
    field_path: string | null | null;
    /** Next Action */
    next_action: string;
    /** Summary */
    summary: string;
  };

  type AssetReadinessDependencySnapshot = {
    /** Asset State Id */
    asset_state_id: string;
    /** Asset State Revision */
    asset_state_revision: number;
    /** Asset Version Id */
    asset_version_id: string;
    /** Consent Ids */
    consent_ids: string[];
    /** Evaluated At */
    evaluated_at: string;
    /** Media Version Ids */
    media_version_ids: string[];
  };

  type AssetReadinessResponse = {
    /** Blockers */
    blockers: AssetReadinessBlocker[];
    dependency_snapshot: AssetReadinessDependencySnapshot;
    /** Next Actions */
    next_actions: string[];
    /** Status */
    status: "draft" | "ready" | "blocked";
    /** Warnings */
    warnings: string[];
  };

  type AssetReferenceRequest = {
    /** Asset Version Id */
    asset_version_id: string;
    /** Role */
    role:
      | "location"
      | "character"
      | "prop"
      | "costume"
      | "visual_style"
      | "voice";
    /** Slot Key */
    slot_key: string;
    /** Subject Key */
    subject_key: string | null | null;
  };

  type AssetReferenceResponse = {
    /** Asset Id */
    asset_id: string;
    /** Asset State Id */
    asset_state_id: string;
    /** Asset Version Id */
    asset_version_id: string;
    /** Binding Source */
    binding_source: "manual" | "ai";
    /** Role */
    role:
      | "location"
      | "character"
      | "prop"
      | "costume"
      | "visual_style"
      | "voice";
    /** Slot Key */
    slot_key: string;
    /** Subject Key */
    subject_key: string | null;
  };

  type assetRenamePreflightApiV1AssetsAssetIdRenamePreflightPostParams = {
    asset_id: string;
  };

  type AssetRenamePreflightRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** New Name */
    new_name: string;
  };

  type AssetRenameRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Impact Hash */
    impact_hash: string;
    /** New Name */
    new_name: string;
  };

  type AssetRenameResponse = {
    asset: AssetResponse;
    impact: AssetImpactResponse;
  };

  type AssetResponse = {
    /** Aliases */
    aliases: string[];
    /** Availability */
    availability: "enabled" | "disabled";
    /** Created At */
    created_at: string;
    /** Id */
    id: string;
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
    /** Name Revision */
    name_revision: number;
    /** Project Id */
    project_id: string;
    /** Revision */
    revision: number;
    /** Status */
    status: "active" | "archived";
    /** Tags */
    tags: string[];
    /** Updated At */
    updated_at: string;
    /** Warnings */
    warnings: "duplicate_name"[] | null;
    /** Workspace Id */
    workspace_id: string;
  };

  type AssetShotImpact = {
    /** Current Spec Version Id */
    current_spec_version_id: string | null;
    /** Episode Id */
    episode_id: string;
    /** Shot Id */
    shot_id: string;
    /** Shot Title */
    shot_title: string;
    /** Slot Keys */
    slot_keys: string[];
    /** Spec Version Ids */
    spec_version_ids: string[];
  };

  type AssetShotUsageResponse = {
    /** Episode Id */
    episode_id: string;
    /** Is Current */
    is_current: boolean;
    /** Shot Id */
    shot_id: string;
    /** Shot Title */
    shot_title: string;
    /** Slot Keys */
    slot_keys: string[];
    /** Spec Version Id */
    spec_version_id: string;
    /** Spec Version No */
    spec_version_no: number;
  };

  type AssetStateAvailabilityResponse = {
    impact: AssetImpactResponse;
    state: AssetStateResponse;
  };

  type AssetStateCreateRequest = {
    /** Description */
    description: string | null;
    /** Expected Asset Revision */
    expected_asset_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Label */
    label: string;
    /** State Key */
    state_key: string;
  };

  type AssetStateCreateResponse = {
    asset: AssetResponse;
    state: AssetStateResponse;
  };

  type AssetStateCurrentPreflightRequest = {
    /** Expected Current Version Id */
    expected_current_version_id: string | null;
    /** Expected Revision */
    expected_revision: number;
    /** Version Id */
    version_id: string;
  };

  type AssetStateCurrentRequest = {
    /** Expected Current Version Id */
    expected_current_version_id: string | null;
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Impact Hash */
    impact_hash: string;
    /** Version Id */
    version_id: string;
  };

  type AssetStateCurrentResponse = {
    impact: AssetImpactResponse;
    state: AssetStateResponse;
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
    /** Blockers */
    blockers: AssetReadinessBlocker[];
    dependency_snapshot: AssetStateReadinessSnapshot;
    /** Next Actions */
    next_actions: string[];
    /** Status */
    status: "draft" | "ready" | "blocked" | "unavailable";
    /** Warnings */
    warnings: string[];
  };

  type AssetStateReadinessSnapshot = {
    /** Asset State Id */
    asset_state_id: string;
    /** Asset State Revision */
    asset_state_revision: number;
    /** Consent Ids */
    consent_ids: string[];
    /** Current Version Id */
    current_version_id: string | null;
    /** Evaluated At */
    evaluated_at: string;
    /** Media Version Ids */
    media_version_ids: string[];
    /** Occurrence Decision Ids */
    occurrence_decision_ids: string[];
  };

  type AssetStateResponse = {
    /** Asset Id */
    asset_id: string;
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Current Version Id */
    current_version_id: string | null;
    /** Description */
    description: string;
    /** Id */
    id: string;
    /** Label */
    label: string;
    /** Revision */
    revision: number;
    /** State Key */
    state_key: string;
    /** Status */
    status: "active" | "disabled";
    /** Updated At */
    updated_at: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type AssetStateUpdateRequest = {
    /** Description */
    description: string | null | null;
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Label */
    label: string | null | null;
  };

  type AssetStatusRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type AssetSummary = {
    /** Blocked */
    blocked: number | null;
    /** Draft */
    draft: number | null;
    /** Ready */
    ready: number | null;
    /** Ready Kinds */
    ready_kinds: string[];
    /** Required Kinds */
    required_kinds: string[];
    /** Status */
    status: "not_started" | "draft" | "blocked" | "ready" | "unavailable";
    /** Total */
    total: number | null;
    /** Versioned */
    versioned: number | null;
  };

  type AssetTaskImpact = {
    /** Generation Request Id */
    generation_request_id: string;
    /** Revision */
    revision: number;
    /** Status */
    status: "queued" | "running" | "waiting_provider" | "unknown";
    /** Task Id */
    task_id: string;
  };

  type AssetUpdateRequest = {
    /** Aliases */
    aliases: string[] | null | null;
    /** Expected Revision */
    expected_revision: number;
    /** Tags */
    tags: string[] | null | null;
  };

  type AssetUpgradeApplyRequest = {
    /** New Asset Version Id */
    new_asset_version_id: string;
    /** Preflight Hash */
    preflight_hash: string;
    /** Targets */
    targets: AssetUpgradeTargetRequest[];
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
    /** New Asset Version Id */
    new_asset_version_id: string;
    /** Old Asset Version Id */
    old_asset_version_id: string;
    /** Preflight Hash */
    preflight_hash: string;
    /** Targets */
    targets: AssetUpgradeTargetRequest[];
  };

  type AssetUpgradeTargetRequest = {
    /** Expected Shot Revision */
    expected_shot_revision: number;
    /** Expected Spec Version Id */
    expected_spec_version_id: string;
    /** New Input Hash */
    new_input_hash: string;
    /** Shot Id */
    shot_id: string;
    /** Slot Keys */
    slot_keys: string[];
  };

  type AssetVersionCreateRequest = {
    /** Expected Current Version Id */
    expected_current_version_id: string | null;
    /** Expected Revision */
    expected_revision: number;
    /** Media References */
    media_references: AssetMediaReferenceRequest[] | null;
    /** Prompt Description */
    prompt_description: string | null;
    /** Set As Current */
    set_as_current: boolean | null;
    /** Source Id */
    source_id: string | null | null;
    /** Source Type */
    source_type: "manual" | "script_extraction_candidate" | null;
    /** Spec */
    spec:
      | CharacterSpec
      | LocationSpec
      | PropSpec
      | CostumeSpec
      | StyleSpec
      | VoiceSpec;
  };

  type AssetVersionCreateResponse = {
    readiness: AssetReadinessResponse;
    state: AssetStateResponse;
    version: AssetVersionResponse;
  };

  type AssetVersionResponse = {
    /** Asset Id */
    asset_id: string;
    /** Asset State Id */
    asset_state_id: string;
    /** Content Hash */
    content_hash: string;
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Id */
    id: string;
    /** Media References */
    media_references: AssetMediaReferenceResponse[];
    /** Prompt Description */
    prompt_description: string;
    /** Schema Version */
    schema_version: number;
    /** Source Id */
    source_id: string | null;
    /** Source Type */
    source_type: "manual" | "script_extraction_candidate";
    /** Spec */
    spec:
      | CharacterSpec
      | LocationSpec
      | PropSpec
      | CostumeSpec
      | StyleSpec
      | VoiceSpec;
    /** Version No */
    version_no: number;
    /** Workspace Id */
    workspace_id: string;
  };

  type AudioIntent = {
    /** Ambient */
    ambient: string | null | null;
    /** Sound Effects */
    sound_effects: string[] | null;
  };

  type AuditEventResponse = {
    /** Action */
    action: string;
    /** Actor Id */
    actor_id: string;
    /** Id */
    id: string;
    /** Metadata */
    metadata: Record<string, any>;
    /** Occurred At */
    occurred_at: string;
    /** Result */
    result: "succeeded" | "denied" | "failed";
    /** Target Id */
    target_id: string;
    /** Target Type */
    target_type: string;
    /** Trace Id */
    trace_id: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type AuthResponse = {
    /** Access Token */
    access_token: string;
    /** Expires In */
    expires_in: number;
    /** Token Type */
    token_type: "bearer" | null;
    user: UserResponse;
    workspace: WorkspaceResponse;
  };

  type BlockingReason = {
    /** Code */
    code: string;
    /** Resource Id */
    resource_id: string;
    /** Resource Type */
    resource_type: "project" | "episode";
    /** Summary */
    summary: string;
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
    /** Actor Id */
    actor_id: string;
    /** Candidate Id */
    candidate_id: string;
    /** Created At */
    created_at: string;
    /** Decision */
    decision:
      | AcceptNewDecision
      | AcceptWithChangesDecision
      | LinkExistingDecision
      | MergeIntoDecision
      | IgnoreDecision;
    /** Decision Key */
    decision_key: string;
    /** Downstream Id */
    downstream_id: string | null;
    /** Downstream Type */
    downstream_type: "ASSET" | null;
    /** Id */
    id: string;
    /** Sequence */
    sequence: number;
  };

  type CandidateDecisionRequest = {
    /** Decision */
    decision:
      | AcceptNewDecision
      | AcceptWithChangesDecision
      | LinkExistingDecision
      | MergeIntoDecision
      | IgnoreDecision;
    /** Decision Key */
    decision_key: string;
    /** Expected Revision */
    expected_revision: number;
  };

  type CandidateDecisionResultResponse = {
    candidate: ExtractionCandidateResponse;
    evidence: CandidateDecisionEvidenceResponse;
  };

  type CandidateSourceRange = {
    /** End */
    end: number;
    /** Start */
    start: number;
  };

  type CapabilityPricingResponse = {
    /** Amount */
    amount: string;
    /** Currency */
    currency: string;
    /** High Cost Threshold */
    high_cost_threshold: string | null;
    /** Unit */
    unit: "per_request";
  };

  type ChangePasswordRequest = {
    /** Current Password */
    current_password: string;
    /** New Password */
    new_password: string;
  };

  type CharacterSpec = {
    /** Age Impression */
    age_impression: string | null;
    /** Appearance */
    appearance: string | null;
    /** Arc Summary */
    arc_summary: string | null;
    /** Goals */
    goals: string[] | null;
    /** Identity */
    identity: string | null;
    /** Kind */
    kind: "character";
    /** Relationships */
    relationships: string[] | null;
    /** Temperament */
    temperament: string[] | null;
    /** Voice Profile */
    voice_profile: string | null;
  };

  type completeUploadApiV1MediaUploadsUploadSessionIdCompletePostParams = {
    upload_session_id: string;
  };

  type configureScheduleApiV1SchedulesScheduleIdConfigurationPutParams = {
    schedule_id: string;
  };

  type ConfirmedStructureResponse = {
    /** Scenes */
    scenes: SceneResponse[];
    /** Script Version Id */
    script_version_id: string;
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
    /** Idempotency Key */
    idempotency_key: string;
    /** Proof Media Version Ids */
    proof_media_version_ids: string[];
    /** Reason */
    reason: string;
    scope: MediaUsageScope;
    subject_identity: SubjectIdentity;
    /** Workspace Id */
    workspace_id: string;
  };

  type ConsentDetailResponse = {
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    current_revision: ConsentRevisionResponse;
    /** Current Revision Id */
    current_revision_id: string;
    /** Id */
    id: string;
    /** Revision */
    revision: number;
    /** Revisions */
    revisions: ConsentRevisionResponse[];
    status: ConsentStatus;
    subject_identity: SubjectIdentity;
    /** Updated At */
    updated_at: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type ConsentRevisionRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Proof Media Version Ids */
    proof_media_version_ids: string[];
    /** Reason */
    reason: string;
    scope: MediaUsageScope;
  };

  type ConsentRevisionResponse = {
    /** Action */
    action: "register" | "update" | "revoke";
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Id */
    id: string;
    /** Proof Media Version Ids */
    proof_media_version_ids: string[];
    /** Reason */
    reason: string;
    /** Revision No */
    revision_no: number;
    scope: MediaUsageScope;
  };

  type ConsentRevokeRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Reason */
    reason: string;
  };

  type ConsentStatus = "active" | "expired" | "revoked";

  type ConsentSummaryResponse = {
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    current_revision: ConsentRevisionResponse;
    /** Current Revision Id */
    current_revision_id: string;
    /** Id */
    id: string;
    /** Revision */
    revision: number;
    status: ConsentStatus;
    subject_identity: SubjectIdentity;
    /** Updated At */
    updated_at: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type ContinuityCandidateProposal = {
    /** Entities */
    entities: string[] | null;
    /** Episode Number */
    episode_number: number | null | null;
    /** Evidence */
    evidence: string | null | null;
    /** Facts */
    facts: string[] | null;
    /** Issue */
    issue: string;
    /** Kind */
    kind: "continuity";
    /** Logline */
    logline: string | null | null;
    /** Rules */
    rules: string[] | null;
    /** Scene Candidate Key */
    scene_candidate_key: string | null | null;
    /** Scene Candidate Keys */
    scene_candidate_keys: string[] | null;
    /** Scope */
    scope: "scene" | "episode" | "character" | "world" | null;
    /** Severity */
    severity: "info" | "warning" | "blocking";
    /** Suggestion */
    suggestion: string;
    /** Summary */
    summary: string | null | null;
    /** Title */
    title: string | null | null;
    /** Topic */
    topic: string | null | null;
  };

  type copyShotApiV1ShotsShotIdCopyPostParams = {
    shot_id: string;
  };

  type CopyShotRequest = {
    /** Expected Order Hash */
    expected_order_hash: string;
    /** Expected Source Spec Version Id */
    expected_source_spec_version_id: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Title */
    title: string;
  };

  type CostEntryResponse = {
    /** Amount */
    amount: string;
    /** Created At */
    created_at: string;
    /** Currency */
    currency: string;
    /** Entry Type */
    entry_type: "reserve" | "settle" | "release" | "adjust";
    /** Id */
    id: string;
    /** Provider Bill Ref */
    provider_bill_ref: string | null;
    /** Request Id */
    request_id: string;
    /** Reservation Id */
    reservation_id: string;
    /** Task Id */
    task_id: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type CostQueryResponse = {
    /** Currency */
    currency: string;
    /** Items */
    items: CostEntryResponse[];
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    summary: CostSummaryResponse;
    /** Total */
    total: number;
  };

  type CostSummary = {
    /** Currency */
    currency: string;
    /** Reserved */
    reserved: string | null;
    /** Status */
    status: "not_started" | null;
    /** Used */
    used: string | null;
  };

  type CostSummaryResponse = {
    /** Adjustments */
    adjustments: string;
    /** Released */
    released: string;
    /** Remaining Reserved */
    remaining_reserved: string;
    /** Reserved */
    reserved: string;
    /** Settled */
    settled: string;
  };

  type CostumeSpec = {
    /** Appearance */
    appearance: string | null;
    /** Kind */
    kind: "costume";
    /** Material */
    material: string | null;
    /** Usage Context */
    usage_context: string | null;
    /** Wearer Character Id */
    wearer_character_id: string | null | null;
  };

  type CoverageDecisionApplyResponse = {
    decision: CoverageDecisionResponse;
    report: CoverageReportResponse;
  };

  type CoverageDecisionRequest = {
    /** Action */
    action:
      | "approve_omission"
      | "revoke_omission"
      | "approve_invented"
      | "revoke_invented";
    /** Evidence */
    evidence: string | null | null;
    /** Expected Evaluation Hash */
    expected_evaluation_hash: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Reason */
    reason: string;
    /** Shot Spec Version Id */
    shot_spec_version_id: string | null;
    /** Unit Version Id */
    unit_version_id: string | null;
  };

  type CoverageDecisionResponse = {
    /** Action */
    action:
      | "approve_omission"
      | "revoke_omission"
      | "approve_invented"
      | "revoke_invented";
    /** Actor Id */
    actor_id: string;
    /** Basis Hash */
    basis_hash: string;
    /** Created At */
    created_at: string;
    /** Episode Id */
    episode_id: string;
    /** Evidence */
    evidence: string | null;
    /** Id */
    id: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Reason */
    reason: string;
    /** Sequence */
    sequence: number;
    /** Shot Spec Version Id */
    shot_spec_version_id: string | null;
    /** Unit Version Id */
    unit_version_id: string | null;
  };

  type CoverageReportResponse = {
    /** Basis Hash */
    basis_hash: string;
    /** Episode Id */
    episode_id: string;
    /** Evaluation Hash */
    evaluation_hash: string;
    /** Next Actions */
    next_actions: string[];
    /** Ready */
    ready: boolean;
    /** References */
    references: NarrativeReferenceResponse[];
    /** Shots */
    shots: ShotCoverageResponse[];
    /** Stale Decision Ids */
    stale_decision_ids: string[];
    /** Stale Reference Ids */
    stale_reference_ids: string[];
    /** Status */
    status: "ready" | "blocked" | "unavailable";
    summary: CoverageSummaryResponse;
    /** Units */
    units: UnitCoverageResponse[];
  };

  type CoverageSummaryResponse = {
    /** Approved Invented */
    approved_invented: number;
    /** Approved Omitted */
    approved_omitted: number;
    /** Covered */
    covered: number;
    /** Linked */
    linked: number;
    /** Orphan */
    orphan: number;
    /** Required Total */
    required_total: number;
    /** Shots Total */
    shots_total: number;
    /** Stale */
    stale: number;
    /** Uncovered */
    uncovered: number;
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

  type createBatchApiV1EpisodesEpisodeIdStoryboardDraftBatchesPostParams = {
    episode_id: string;
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
    /** Expected Current Version Id */
    expected_current_version_id: string;
    /** Expected Revision */
    expected_revision: number;
    /** Version Id */
    version_id: string;
  };

  type CurrentScriptVersionRequest = {
    /** Expected Current Version Id */
    expected_current_version_id: string | null;
    /** Version Id */
    version_id: string;
  };

  type CurrentScriptVersionResponse = {
    /** Current Script Version Id */
    current_script_version_id: string;
    /** Episode Id */
    episode_id: string;
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

  type decideCoverageApiV1EpisodesEpisodeIdCoverageDecisionsPostParams = {
    episode_id: string;
  };

  type decideDraftApiV1StoryboardDraftsDraftIdDecisionsPostParams = {
    draft_id: string;
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
    /** Resource Id */
    resource_id: string;
    /** Resource Type */
    resource_type: string;
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
    /** Reason */
    reason: string | null | null;
    /** Status */
    status: "available" | "degraded" | "unavailable";
  };

  type DialogueCandidateProposal = {
    /** Action Before */
    action_before: string | null | null;
    /** Dialogue Kind */
    dialogue_kind: "spoken" | "narration" | "internal" | "voice_over";
    /** Emotion */
    emotion: string | null | null;
    /** Kind */
    kind: "dialogue";
    /** Performance Note */
    performance_note: string | null | null;
    /** Scene Candidate Key */
    scene_candidate_key: string;
    /** Speaker Candidate */
    speaker_candidate: string;
    /** Subtext */
    subtext: string | null | null;
    /** Text */
    text: string;
  };

  type DialogueOrNarration = {
    /** Beat Key */
    beat_key: string | null | null;
    /** Performance Note */
    performance_note: string | null | null;
    /** Render As Audio */
    render_as_audio: boolean | null;
    /** Source Dialogue Id */
    source_dialogue_id: string;
    /** Speaker Subject Key */
    speaker_subject_key: string | null | null;
  };

  type DialogueResponse = {
    /** Created At */
    created_at: string;
    /** Dialogue Kind */
    dialogue_kind: "spoken" | "narration" | "internal" | "voice_over";
    /** Id */
    id: string;
    /** Performance Note */
    performance_note: string | null;
    /** Position */
    position: number;
    /** Scene Id */
    scene_id: string;
    source_range: CandidateSourceRange;
    /** Speaker Candidate */
    speaker_candidate: string;
    /** Text */
    text: string;
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
    /** Analysis Status */
    analysis_status: "deterministic" | "ai_candidate_required" | "rejected";
    /** Analyzer Version */
    analyzer_version: string;
    /** Codepoint Count */
    codepoint_count: number;
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Document Id */
    document_id: string;
    /** Id */
    id: string;
    /** Normalization Map */
    normalization_map: Record<string, any>;
    /** Normalized Hash */
    normalized_hash: string;
    /** Normalized Text */
    normalized_text: string;
    /** Normalizer Version */
    normalizer_version: string;
    /** Raw Hash */
    raw_hash: string;
    /** Raw Text */
    raw_text: string;
    /** Source Media Version Id */
    source_media_version_id: string | null;
    /** Source Type */
    source_type: "text" | "media";
    /** Version No */
    version_no: number;
    /** Workspace Id */
    workspace_id: string;
  };

  type DownstreamEvidenceResponse = {
    /** Candidate Ids */
    candidate_ids: string[] | null;
    /** Generation Request Ids */
    generation_request_ids: string[] | null;
    /** Issue Ids */
    issue_ids: string[] | null;
    /** Review Ids */
    review_ids: string[] | null;
    /** Timeline Source Ids */
    timeline_source_ids: string[] | null;
  };

  type DraftApplyDiff = {
    /** Archived */
    archived: 0 | null;
    /** Created */
    created: number;
    /** Kept */
    kept: number;
    /** Modified */
    modified: 0 | null;
  };

  type DraftApplyPreflightRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type DraftApplyPreflightResponse = {
    /** Batch Id */
    batch_id: string;
    /** Batch Revision */
    batch_revision: number;
    diff: DraftApplyDiff;
    /** Impact Hash */
    impact_hash: string;
    /** Order Hash */
    order_hash: string;
  };

  type DraftApplyRequest = {
    /** Expected Order Hash */
    expected_order_hash: string;
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Impact Hash */
    impact_hash: string;
  };

  type DraftApplyResponse = {
    batch: DraftBatchResponse;
    /** Created Shot Ids */
    created_shot_ids: string[];
  };

  type DraftApproveRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type DraftAssetReferenceResponse = {
    /** Asset Version Id */
    asset_version_id: string;
    /** Role */
    role:
      | "location"
      | "character"
      | "prop"
      | "costume"
      | "visual_style"
      | "voice";
    /** Slot Key */
    slot_key: string;
    /** Subject Key */
    subject_key: string | null;
  };

  type DraftBatchCreateRequest = {
    /** Asset State Ids */
    asset_state_ids: string[] | null;
    /** Idempotency Key */
    idempotency_key: string;
    /** Input Script Version Id */
    input_script_version_id: string;
  };

  type DraftBatchResponse = {
    /** Created At */
    created_at: string;
    decision_summary: DraftDecisionSummary;
    /** Drafts */
    drafts: DraftShotResponse[];
    /** Episode Id */
    episode_id: string;
    /** Error Code */
    error_code: string | null;
    /** Id */
    id: string;
    input: DraftInputSummary;
    /** Project Id */
    project_id: string;
    /** Revision */
    revision: number;
    /** Status */
    status:
      | "queued"
      | "running"
      | "needs_review"
      | "approved"
      | "applied"
      | "failed"
      | "unknown"
      | "cancelled";
    /** Task Id */
    task_id: string | null;
    /** Updated At */
    updated_at: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type DraftDecisionRequest = {
    /** Action */
    action: "accepted" | "modified" | "ignored";
    /** Expected Batch Revision */
    expected_batch_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    target: DraftTarget | null | null;
  };

  type DraftDecisionResponse = {
    /** Action */
    action: "accepted" | "modified" | "ignored";
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Id */
    id: string;
    /** Sequence */
    sequence: number;
    target: DraftTarget | null;
  };

  type DraftDecisionResult = {
    batch: DraftBatchResponse;
    draft: DraftShotResponse;
  };

  type DraftDecisionSummary = {
    /** Accepted */
    accepted: number;
    /** Ignored */
    ignored: number;
    /** Modified */
    modified: number;
    /** Pending */
    pending: number;
  };

  type DraftInputSummary = {
    /** Aspect Ratio */
    aspect_ratio: "9:16" | "16:9" | "1:1";
    /** Asset State Ids */
    asset_state_ids: string[];
    /** Asset Version Ids */
    asset_version_ids: string[];
    /** Input Hash */
    input_hash: string;
    /** Narrative Dependency Hash */
    narrative_dependency_hash: string;
    /** Narrative Revision */
    narrative_revision: number;
    /** Narrative Structure Id */
    narrative_structure_id: string;
    /** Narrative Unit Version Ids */
    narrative_unit_version_ids: string[];
    /** Script Version Id */
    script_version_id: string;
    /** Target Duration Ms */
    target_duration_ms: number;
    /** Visual Style */
    visual_style: string | null;
  };

  type DraftShotResponse = {
    /** Asset References */
    asset_references: DraftAssetReferenceResponse[];
    /** Decision History */
    decision_history: DraftDecisionResponse[];
    /** Id */
    id: string;
    /** Narrative Unit Version Ids */
    narrative_unit_version_ids: string[];
    /** Position */
    position: number;
    /** Proposal Key */
    proposal_key: string;
    /** Risk Codes */
    risk_codes: string[];
    spec: ShotSpec;
    /** Title */
    title: string;
  };

  type DraftTarget = {
    /** Asset References */
    asset_references: AssetReferenceRequest[] | null;
    /** Narrative Unit Version Ids */
    narrative_unit_version_ids: string[];
    spec: ShotSpec;
    /** Title */
    title: string;
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
    /** Idempotency Key */
    idempotency_key: string;
    /** Requested Episode Count */
    requested_episode_count: number | null | null;
    /** Strategy */
    strategy: "explicit_markers" | "target_duration_ai";
    /** Target Duration Ms */
    target_duration_ms: number;
  };

  type EpisodePlanDetailResponse = {
    impact: EpisodePlanImpactResponse;
    plan: EpisodePlanResponse;
    /** Proposals */
    proposals: EpisodeProposalResponse[];
    source: EpisodePlanSourceResponse;
  };

  type EpisodePlanImpactBlocker = {
    /** Code */
    code: string;
    /** Next Action */
    next_action: string;
    /** Summary */
    summary: string;
  };

  type EpisodePlanImpactResponse = {
    /** Active Episode Count */
    active_episode_count: number;
    /** Active Order Hash */
    active_order_hash: string;
    /** Allowed */
    allowed: boolean;
    /** Blockers */
    blockers: EpisodePlanImpactBlocker[];
    /** Project Revision */
    project_revision: number;
    /** Projected Episode Count */
    projected_episode_count: number;
  };

  type EpisodePlanResponse = {
    /** Confirmed At */
    confirmed_at: string | null;
    /** Confirmed By */
    confirmed_by: string | null;
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Document Revision Id */
    document_revision_id: string;
    /** Id */
    id: string;
    /** Input Hash */
    input_hash: string;
    /** Model Name */
    model_name: string | null;
    /** Planning Engine Version */
    planning_engine_version: string;
    /** Planning Error Code */
    planning_error_code: string | null;
    /** Planning Task Id */
    planning_task_id: string | null;
    /** Project Id */
    project_id: string;
    /** Prompt Version */
    prompt_version: string | null;
    /** Requested Episode Count */
    requested_episode_count: number | null;
    /** Revision */
    revision: number;
    /** Schema Version */
    schema_version: string;
    /** Status */
    status:
      | "draft"
      | "review_ready"
      | "confirmed"
      | "materialized"
      | "superseded";
    /** Strategy */
    strategy: "explicit_markers" | "target_duration_ai";
    /** Target Duration Ms */
    target_duration_ms: number;
    /** Total Estimated Duration Ms */
    total_estimated_duration_ms: number;
    /** Updated At */
    updated_at: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type EpisodePlanSourceResponse = {
    /** Blocks */
    blocks: NarrativeBlockResponse[];
    /** Codepoint Count */
    codepoint_count: number;
    /** Document Revision Id */
    document_revision_id: string;
    /** Normalized Hash */
    normalized_hash: string;
    /** Normalized Text */
    normalized_text: string;
  };

  type EpisodeProductionSnapshot = {
    asset_summary: AssetSummary;
    /** Blocking Reasons */
    blocking_reasons: BlockingReason[];
    /** Completion */
    completion: number;
    /** Computed At */
    computed_at: string;
    cost_summary: CostSummary;
    /** Current Stage */
    current_stage:
      | "script_import"
      | "structure_review"
      | "asset_preparation"
      | "storyboard_preparation";
    /** Episode Id */
    episode_id: string;
    /** Next Actions */
    next_actions: NextAction[];
    /** Partial Failures */
    partial_failures: PartialFailure[];
    review_summary: ReviewSummary;
    script_summary: ScriptSummary;
    storyboard_summary: StoryboardSummary;
    task_summary: TaskSummary;
  };

  type episodeProductionSnapshotApiV1EpisodesEpisodeIdProductionSnapshotGetParams =
    {
      episode_id: string;
    };

  type EpisodeProposalResponse = {
    /** Boundary Evidence */
    boundary_evidence: Record<string, any>;
    /** Confidence */
    confidence: number;
    /** Content Hash */
    content_hash: string;
    /** End Block Id */
    end_block_id: string;
    /** End Block Position */
    end_block_position: number;
    /** Estimated Duration Ms */
    estimated_duration_ms: number;
    /** Id */
    id: string;
    /** Is Locked */
    is_locked: boolean;
    /** Plan Id */
    plan_id: string;
    /** Position */
    position: number;
    /** Reason */
    reason: string;
    /** Source End */
    source_end: number;
    /** Source Start */
    source_start: number;
    /** Start Block Id */
    start_block_id: string;
    /** Start Block Position */
    start_block_position: number;
    /** Title */
    title: string;
  };

  type EpisodeReorderRequest = {
    /** Episode Ids */
    episode_ids: string[];
    /** Expected Revision */
    expected_revision: number;
  };

  type EpisodeResponse = {
    /** Current Script Version Id */
    current_script_version_id: string | null;
    /** Current Timeline Version Id */
    current_timeline_version_id: string | null;
    /** Id */
    id: string;
    /** Name */
    name: string;
    /** Position */
    position: number;
    /** Project Id */
    project_id: string;
    /** Revision */
    revision: number;
    /** Status */
    status: "active" | "archived";
    /** Target Duration Ms */
    target_duration_ms: number;
    /** Workspace Id */
    workspace_id: string;
  };

  type EpisodeSegmentOriginResponse = {
    /** Document Revision Id */
    document_revision_id: string;
    /** Draft Version Id */
    draft_version_id: string;
    /** Episode Id */
    episode_id: string;
    /** Id */
    id: string;
    /** Import Commit Id */
    import_commit_id: string;
    /** Position */
    position: number;
    /** Proposal Id */
    proposal_id: string;
    /** Published Version Id */
    published_version_id: string | null;
    /** Source End */
    source_end: number;
    /** Source Hash */
    source_hash: string;
    /** Source Id */
    source_id: string;
    /** Source Start */
    source_start: number;
  };

  type EpisodeStateRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type EpisodeUpdateRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Name */
    name: string | null | null;
    /** Target Duration Ms */
    target_duration_ms: number | null | null;
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

  type ExportBlockerResponse = {
    /** Code */
    code: string;
    /** Dependency Id */
    dependency_id: string | null | null;
    /** Next Action */
    next_action: string;
    /** Shot Id */
    shot_id: string | null | null;
    /** Summary */
    summary: string;
  };

  type ExportFileResponse = {
    /** Media Type */
    media_type: string;
    /** Path */
    path: string;
    /** Sha256 */
    sha256: string;
    /** Size Bytes */
    size_bytes: number;
  };

  type ExportHistoryResponse = {
    /** Items */
    items: ExportResponse[];
    /** Total */
    total: number;
  };

  type ExportManifestResponse = {
    /** Asset Version Ids */
    asset_version_ids: string[];
    /** Coverage Basis Hash */
    coverage_basis_hash: string;
    /** Coverage Evaluation Hash */
    coverage_evaluation_hash: string;
    /** Created At */
    created_at: string;
    /** Files */
    files: ExportFileResponse[];
    /** Id */
    id: string;
    /** Input Hash */
    input_hash: string;
    /** Media Version Id */
    media_version_id: string;
    /** Narrative Structure Id */
    narrative_structure_id: string;
    /** Narrative Unit Version Ids */
    narrative_unit_version_ids: string[];
    /** Package Sha256 */
    package_sha256: string;
    /** Package Size Bytes */
    package_size_bytes: number;
    /** Schema Version */
    schema_version: number;
    /** Script Version Id */
    script_version_id: string;
    /** Shot Spec Version Ids */
    shot_spec_version_ids: string[];
  };

  type ExportPreflightResponse = {
    /** Asset Version Ids */
    asset_version_ids: string[];
    /** Blockers */
    blockers: ExportBlockerResponse[];
    /** Coverage Basis Hash */
    coverage_basis_hash: string | null;
    /** Coverage Evaluation Hash */
    coverage_evaluation_hash: string | null;
    /** Episode Id */
    episode_id: string;
    /** Input Hash */
    input_hash: string | null;
    /** Narrative Structure Id */
    narrative_structure_id: string | null;
    /** Narrative Unit Version Ids */
    narrative_unit_version_ids: string[];
    /** Readiness Evaluation Hash */
    readiness_evaluation_hash: string | null;
    /** Script Version Id */
    script_version_id: string | null;
    /** Shot Spec Version Ids */
    shot_spec_version_ids: string[];
    /** Status */
    status: "ready" | "blocked" | "unavailable";
  };

  type ExportRequest = {
    /** Expected Input Hash */
    expected_input_hash: string;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type ExportResponse = {
    /** Created At */
    created_at: string;
    /** Episode Id */
    episode_id: string;
    /** Error Code */
    error_code: string | null;
    /** Id */
    id: string;
    /** Input Hash */
    input_hash: string;
    manifest: ExportManifestResponse | null;
    /** Status */
    status: "queued" | "running" | "succeeded" | "failed";
    /** Task Id */
    task_id: string | null;
    /** Updated At */
    updated_at: string;
  };

  type ExtractionBatchResponse = {
    /** Candidate Count */
    candidate_count: number;
    /** Confirmed Script Version Id */
    confirmed_script_version_id: string | null;
    /** Created At */
    created_at: string;
    /** Extractor Version */
    extractor_version: string;
    /** Id */
    id: string;
    /** Input Hash */
    input_hash: string;
    /** Scope */
    scope: "full";
    /** Script Version Id */
    script_version_id: string;
    /** Status */
    status:
      | "queued"
      | "running"
      | "waiting_provider"
      | "succeeded"
      | "failed"
      | "cancelled"
      | "unknown";
    task: TaskResponse;
    /** Workspace Id */
    workspace_id: string;
  };

  type ExtractionCandidateResponse = {
    /** Batch Id */
    batch_id: string;
    /** Candidate Key */
    candidate_key: string;
    /** Confidence Note */
    confidence_note: string | null;
    /** Created At */
    created_at: string;
    /** Id */
    id: string;
    /** Kind */
    kind: "scene" | "dialogue" | "asset" | "shot" | "continuity";
    /** Proposal */
    proposal:
      | SceneCandidateProposal
      | DialogueCandidateProposal
      | AssetCandidateProposal
      | ShotCandidateProposal
      | ContinuityCandidateProposal;
    /** Required */
    required: boolean;
    /** Revision */
    revision: number;
    source_range: CandidateSourceRange;
    /** Status */
    status: "pending" | "accepted" | "linked" | "merged" | "ignored";
  };

  type FormatIssueResponse = {
    /** Code */
    code: string;
    /** Column Number */
    column_number: number;
    /** Details */
    details: Record<string, any>;
    /** Document Revision Id */
    document_revision_id: string;
    /** Id */
    id: string;
    /** Line Number */
    line_number: number;
    /** Next Action */
    next_action: string;
    /** Position */
    position: number;
    /** Severity */
    severity: "warning" | "blocking";
    /** Source End */
    source_end: number;
    /** Source Start */
    source_start: number;
  };

  type GenerationBlocker = {
    /** Code */
    code: string;
    /** Next Action */
    next_action: string;
    /** Summary */
    summary: string;
  };

  type GenerationConfirmationRequirement = {
    /** Code */
    code: "ACKNOWLEDGE_WARNINGS" | "CONFIRM_HIGH_COST";
    /** Warning Codes */
    warning_codes: string[];
  };

  type GenerationIntent = {
    /** First Frame */
    first_frame: string | null | null;
    /** Keyframe Notes */
    keyframe_notes: string | null | null;
    /** Last Frame */
    last_frame: string | null | null;
    /** Mode */
    mode: "keyframe_then_video" | "reference_to_video" | "text_to_video";
  };

  type GenerationPreflightRequest = {
    /** Capability Id */
    capability_id: string;
    /** Parameters */
    parameters: Record<string, any>;
    /** Shot Spec Version Id */
    shot_spec_version_id: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type GenerationPreflightResponse = {
    /** Blocking Reasons */
    blocking_reasons: GenerationBlocker[];
    /** Capability Id */
    capability_id: string;
    /** Confirmation Requirements */
    confirmation_requirements: GenerationConfirmationRequirement[];
    estimated_cost: EstimatedCostResponse | null;
    /** Expires At */
    expires_at: string;
    /** Preflight Hash */
    preflight_hash: string;
    /** Ready */
    ready: boolean;
    /** Shot Id */
    shot_id: string;
    /** Shot Spec Version Id */
    shot_spec_version_id: string;
    /** Status */
    status: "ready" | "blocked" | "unavailable";
    /** Warning Codes */
    warning_codes: string[];
  };

  type GenerationRequestResponse = {
    /** Capability Config Version */
    capability_config_version: number;
    /** Capability Id */
    capability_id: string;
    /** Created At */
    created_at: string;
    /** Episode Id */
    episode_id: string;
    /** High Cost Confirmed */
    high_cost_confirmed: boolean;
    /** Id */
    id: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Input Hash */
    input_hash: string;
    /** Parameter Snapshot */
    parameter_snapshot: Record<string, any>;
    /** Project Id */
    project_id: string;
    /** Requested By */
    requested_by: string;
    /** Shot Id */
    shot_id: string;
    /** Shot Spec Input Hash */
    shot_spec_input_hash: string;
    /** Shot Spec Version Id */
    shot_spec_version_id: string;
    /** Warning Acknowledgements */
    warning_acknowledgements: string[];
    /** Workspace Id */
    workspace_id: string;
  };

  type GenerationSubmissionRequest = {
    /** Capability Id */
    capability_id: string;
    /** High Cost Confirmed */
    high_cost_confirmed: boolean | null;
    /** Idempotency Key */
    idempotency_key: string;
    /** Parameters */
    parameters: Record<string, any>;
    /** Preflight Expires At */
    preflight_expires_at: string;
    /** Preflight Hash */
    preflight_hash: string;
    /** Shot Spec Version Id */
    shot_spec_version_id: string;
    /** Warning Acknowledgements */
    warning_acknowledgements: string[] | null;
    /** Workspace Id */
    workspace_id: string;
  };

  type GenerationSubmissionResponse = {
    initial_cost_entry: CostEntryResponse;
    /** Outbox Event Id */
    outbox_event_id: string;
    /** Replayed */
    replayed: boolean;
    request: GenerationRequestResponse;
    reservation: ReservationResponse;
    task: TaskResponse;
  };

  type GenerationTaskCancellationRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Reason */
    reason: "user_requested" | "input_changed" | "budget_changed";
    /** Workspace Id */
    workspace_id: string;
  };

  type GenerationTaskCancellationResponse = {
    release_cost_entry: CostEntryResponse;
    /** Replayed */
    replayed: boolean;
    reservation: ReservationResponse;
    task: TaskResponse;
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

  type getBatchApiV1StoryboardDraftBatchesBatchIdGetParams = {
    batch_id: string;
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

  type getCoverageApiV1EpisodesEpisodeIdCoverageGetParams = {
    episode_id: string;
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
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Error Code */
    error_code: string | null;
    /** Expected Active Order Hash */
    expected_active_order_hash: string;
    /** Expected Project Revision */
    expected_project_revision: number;
    /** Id */
    id: string;
    /** Input Hash */
    input_hash: string;
    /** Mode */
    mode: "append_new";
    /** Plan Id */
    plan_id: string;
    /** Project Id */
    project_id: string;
    /** Revision */
    revision: number;
    /** Status */
    status:
      | "pending"
      | "materializing"
      | "materialized"
      | "publishing"
      | "published"
      | "conflict"
      | "failed";
    /** Updated At */
    updated_at: string;
    /** Workspace Id */
    workspace_id: string;
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

  type listExportsApiV1EpisodesEpisodeIdStoryboardExportsGetParams = {
    episode_id: string;
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
      | "storyboard_draft"
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
    /** Lighting */
    lighting: string | null;
    /** Spatial Description */
    spatial_description: string | null;
    /** Time Weather */
    time_weather: string | null;
    /** Visual Elements */
    visual_elements: string[] | null;
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
    /** Expected Active Order Hash */
    expected_active_order_hash: string;
    /** Expected Plan Revision */
    expected_plan_revision: number;
    /** Expected Project Revision */
    expected_project_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Mode */
    mode: "append_new";
  };

  type MediaAccessRequest = {
    /** Purpose */
    purpose: "preview" | "download";
  };

  type MediaAccessResponse = {
    /** Expires At */
    expires_at: string;
    /** Method */
    method: "GET" | null;
    /** Purpose */
    purpose: "preview" | "download";
    /** Url */
    url: string;
  };

  type MediaLocationMigrationRequest = {
    /** Idempotency Key */
    idempotency_key: string;
  };

  type MediaLocationResponse = {
    /** Created At */
    created_at: string;
    /** Id */
    id: string;
    /** Media Version Id */
    media_version_id: string;
    /** Retire After */
    retire_after: string | null;
    /** Retired At */
    retired_at: string | null;
    /** Rollback Available */
    rollback_available: boolean;
    /** Status */
    status: "verified" | "active" | "retiring" | "retired" | "quarantined";
    /** Verified At */
    verified_at: string | null;
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
    /** Current Version Id */
    current_version_id: string | null;
    /** Id */
    id: string;
    /** Kind */
    kind: "image" | "video" | "audio" | "subtitle" | "delivery" | "document";
    /** Revision */
    revision: number;
    /** Source Type */
    source_type: "upload" | "generated" | "rendered";
    /** Status */
    status: "active" | "archived";
    /** Workspace Id */
    workspace_id: string;
  };

  type MediaUsageScope = {
    /** Authorized Purposes */
    authorized_purposes: string[];
    /** Channels */
    channels: string[];
    /** Regions */
    regions: string[];
    /** Rights Holder Role */
    rights_holder_role: string;
    /** Rights Types */
    rights_types: string[];
    /** Subject Id */
    subject_id: string;
    subject_type: SubjectType;
    /** Type */
    type: "media_usage";
    /** Valid From */
    valid_from: string;
    /** Valid To */
    valid_to: string;
  };

  type MediaVersionResponse = {
    /** Codec */
    codec: string | null;
    /** Container */
    container: string | null;
    /** Created At */
    created_at: string;
    /** Duration Ms */
    duration_ms: number | null;
    /** Filename */
    filename: string;
    /** Height */
    height: number | null;
    /** Id */
    id: string;
    /** Media Object Current Version Id */
    media_object_current_version_id: string | null;
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
    /** Media Object Revision */
    media_object_revision: number;
    /** Media Object Source Type */
    media_object_source_type: "upload" | "generated" | "rendered";
    /** Media Object Status */
    media_object_status: "active" | "archived";
    /** Mime Type */
    mime_type: string;
    /** Probe Attempt */
    probe_attempt: number;
    /** Probe Error Code */
    probe_error_code: string | null;
    /** Probe Error Summary */
    probe_error_summary: string | null;
    /** Probe Next Action */
    probe_next_action: string | null;
    /** Probe Status */
    probe_status: "pending" | "ready" | "failed" | "quarantined";
    /** Sha256 */
    sha256: string;
    /** Size Bytes */
    size_bytes: number;
    /** Version No */
    version_no: number;
    /** Width */
    width: number | null;
    /** Workspace Id */
    workspace_id: string;
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
    /** Expected Order Hash */
    expected_order_hash: string;
    /** Expected Spec Version Ids */
    expected_spec_version_ids: string[];
    /** Shot Ids */
    shot_ids: string[];
  };

  type MergeShotRequest = {
    /** Expected Order Hash */
    expected_order_hash: string;
    /** Expected Spec Version Ids */
    expected_spec_version_ids: string[];
    /** Idempotency Key */
    idempotency_key: string;
    /** Impact Hash */
    impact_hash: string;
    /** Shot Ids */
    shot_ids: string[];
    target: TargetShotSpecRequest;
  };

  type ModelCapabilityResponse = {
    /** Config Version */
    config_version: number;
    /** Id */
    id: string;
    /** Input Types */
    input_types: string[];
    /** Kind */
    kind: "image" | "video";
    /** Limits */
    limits: Record<string, any>;
    /** Model */
    model: string;
    /** Parameter Schema */
    parameter_schema: Record<string, any>;
    pricing: CapabilityPricingResponse | null;
    /** Provider */
    provider: string;
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
    /** Document Revision Id */
    document_revision_id: string;
    /** Id */
    id: string;
    /** Kind */
    kind:
      | "preamble"
      | "episode_marker"
      | "scene_heading"
      | "dialogue"
      | "narration"
      | "action"
      | "separator";
    /** Metadata */
    metadata: Record<string, any>;
    /** Position */
    position: number;
    /** Source End */
    source_end: number;
    /** Source Start */
    source_start: number;
    /** Text Hash */
    text_hash: string;
  };

  type NarrativeDependencyResponse = {
    /** Current Dependency Hash */
    current_dependency_hash: string;
    /** Current Script Version Id */
    current_script_version_id: string;
    /** Current Structure Id */
    current_structure_id: string;
    /** Current Structure Revision */
    current_structure_revision: number;
    /** Episode Id */
    episode_id: string;
    /** Evaluated Hash */
    evaluated_hash: string | null;
    /** Status */
    status: "fresh" | "stale";
  };

  type NarrativeImpactResponse = {
    /** Affected Shot Ids */
    affected_shot_ids: string[];
    /** Created At */
    created_at: string;
    /** Current Dependency Hash */
    current_dependency_hash: string;
    /** Current Script Version Id */
    current_script_version_id: string;
    /** Current Structure Hash */
    current_structure_hash: string;
    /** Current Unit Count */
    current_unit_count: number;
    /** Episode Id */
    episode_id: string;
    /** Episode Revision */
    episode_revision: number;
    /** Id */
    id: string;
    /** Impact Hash */
    impact_hash: string;
    /** Invalidated Scopes */
    invalidated_scopes: ("shot_readiness" | "coverage" | "export")[];
    /** Previous Dependency Hash */
    previous_dependency_hash: string | null;
    /** Previous Script Version Id */
    previous_script_version_id: string | null;
    /** Previous Structure Hash */
    previous_structure_hash: string | null;
    /** Previous Unit Count */
    previous_unit_count: number;
    /** Sequence */
    sequence: number;
    /** Trigger */
    trigger: "current_changed" | "structure_corrected";
  };

  type NarrativeReferenceInput = {
    /** Channel */
    channel: "visual" | "audio" | "both";
    /** Contribution */
    contribution: "required" | "supporting";
    /** Coverage Mode */
    coverage_mode: "full" | "partial";
    /** Role */
    role:
      | "primary"
      | "dialogue"
      | "reaction"
      | "insert"
      | "setup"
      | "payoff"
      | "transition"
      | "supporting";
    /** Segment End */
    segment_end: number | null | null;
    /** Segment Start */
    segment_start: number | null | null;
    /** Unit Version Id */
    unit_version_id: string;
  };

  type NarrativeReferenceReplaceRequest = {
    /** Expected Current Spec Version Id */
    expected_current_spec_version_id: string;
    /** Expected Evaluation Hash */
    expected_evaluation_hash: string;
    /** Expected Shot Revision */
    expected_shot_revision: number;
    /** References */
    references: NarrativeReferenceInput[];
  };

  type NarrativeReferenceReplaceResponse = {
    /** Current Spec Version Id */
    current_spec_version_id: string;
    /** Previous Spec Version Id */
    previous_spec_version_id: string;
    /** References */
    references: NarrativeReferenceResponse[];
    report: CoverageReportResponse;
    /** Shot Id */
    shot_id: string;
    /** Shot Revision */
    shot_revision: number;
  };

  type NarrativeReferenceResponse = {
    /** Channel */
    channel: "visual" | "audio" | "both";
    /** Contribution */
    contribution: "required" | "supporting";
    /** Coverage Mode */
    coverage_mode: "full" | "partial";
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Id */
    id: string;
    /** Narrative Unit Id */
    narrative_unit_id: string;
    /** Origin */
    origin: "ai" | "human" | "migrated";
    /** Role */
    role:
      | "primary"
      | "dialogue"
      | "reaction"
      | "insert"
      | "setup"
      | "payoff"
      | "transition"
      | "supporting";
    /** Segment End */
    segment_end: number | null;
    /** Segment Start */
    segment_start: number | null;
    /** Shot Id */
    shot_id: string;
    /** Shot Spec Version Id */
    shot_spec_version_id: string;
    /** Unit Version Id */
    unit_version_id: string;
  };

  type NarrativeRevisionResponse = {
    impact: NarrativeImpactResponse;
    structure: NarrativeStructureResponse;
  };

  type NarrativeSpec = {
    /** Continuity Note */
    continuity_note: string | null | null;
    /** Purpose */
    purpose: string;
  };

  type NarrativeStructureResponse = {
    /** Created At */
    created_at: string;
    /** Dependency Hash */
    dependency_hash: string;
    /** Episode Id */
    episode_id: string;
    /** Id */
    id: string;
    /** Input Hash */
    input_hash: string;
    /** Parser Version */
    parser_version: string;
    /** Revision */
    revision: number;
    /** Script Version Id */
    script_version_id: string;
    /** Structure Hash */
    structure_hash: string;
    /** Units */
    units: NarrativeUnitResponse[];
    /** Updated At */
    updated_at: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type NarrativeStructureRevisionRequest = {
    /** Expected Current Script Version Id */
    expected_current_script_version_id: string;
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** Units */
    units: NarrativeUnitRevisionItem[];
  };

  type NarrativeUnitResponse = {
    /** Created At */
    created_at: string;
    /** Exact Text */
    exact_text: string;
    /** Id */
    id: string;
    /** Kind */
    kind: "scene_heading" | "action" | "dialogue" | "narration";
    /** Origin */
    origin: "deterministic" | "manual";
    /** Position */
    position: number;
    /** Prefix Text */
    prefix_text: string;
    /** Required For Coverage */
    required_for_coverage: boolean;
    /** Source Dialogue Id */
    source_dialogue_id: string | null;
    source_range: SourceRange;
    /** Source Scene Id */
    source_scene_id: string | null;
    /** Suffix Text */
    suffix_text: string;
    /** Text Hash */
    text_hash: string;
    /** Unit Id */
    unit_id: string;
    /** Version No */
    version_no: number;
  };

  type NarrativeUnitRevisionItem = {
    /** Kind */
    kind: "scene_heading" | "action" | "dialogue" | "narration";
    /** Required For Coverage */
    required_for_coverage: boolean;
    /** Source End */
    source_end: number;
    /** Source Start */
    source_start: number;
    /** Unit Id */
    unit_id: string;
  };

  type NextAction = {
    /** Code */
    code: string;
    /** Href */
    href: string;
    /** Label */
    label: string;
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
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
  };

  type PaginatedAssetShotUsages = {
    /** Items */
    items: AssetShotUsageResponse[];
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
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
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
  };

  type PaginatedAuditEvents = {
    /** Items */
    items: AuditEventResponse[];
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
  };

  type PaginatedCandidateDecisions = {
    /** Items */
    items: CandidateDecisionEvidenceResponse[];
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
  };

  type PaginatedConsents = {
    /** Items */
    items: ConsentSummaryResponse[];
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
  };

  type PaginatedExtractionCandidates = {
    /** Items */
    items: ExtractionCandidateResponse[];
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
  };

  type PaginatedMedia = {
    /** Items */
    items: MediaVersionResponse[];
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
  };

  type PaginatedProjects = {
    /** Items */
    items: ProjectResponse[];
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
  };

  type PaginatedSchedules = {
    /** Items */
    items: ScheduleResponse[];
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
  };

  type PaginatedScriptDocuments = {
    /** Items */
    items: ScriptDocumentResponse[];
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
  };

  type PaginatedScriptSources = {
    /** Items */
    items: ScriptSourceResponse[];
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
  };

  type PaginatedScriptVersions = {
    /** Items */
    items: ScriptVersionResponse[];
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
  };

  type PaginatedTasks = {
    /** Items */
    items: TaskResponse[];
    /** Limit */
    limit: number;
    /** Offset */
    offset: number;
    /** Total */
    total: number;
  };

  type PartialFailure = {
    /** Code */
    code: string;
    /** Module */
    module: string;
    /** Summary */
    summary: string;
  };

  type pauseScheduleApiV1SchedulesScheduleIdPausePostParams = {
    schedule_id: string;
  };

  type preflightApplyApiV1StoryboardDraftBatchesBatchIdApplyPreflightPostParams =
    {
      batch_id: string;
    };

  type preflightAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePreflightPostParams =
    {
      asset_version_id: string;
    };

  type preflightExportApiV1EpisodesEpisodeIdStoryboardExportsPreflightPostParams =
    {
      episode_id: string;
    };

  type preflightGenerationApiV1ShotsShotIdGenerationPreflightPostParams = {
    shot_id: string;
  };

  type previewDocumentApiV1ProjectsProjectIdScriptImportPreviewsPostParams = {
    project_id: string;
  };

  type ProbeRetryRequest = {
    /** Idempotency Key */
    idempotency_key: string;
  };

  type ProfileUpdateRequest = {
    /** Avatar Url */
    avatar_url: string | null | null;
    /** Display Name */
    display_name: string | null | null;
  };

  type ProjectCreateRequest = {
    /** Aspect Ratio */
    aspect_ratio: "9:16" | "16:9" | "1:1" | null;
    /** Description */
    description: string | null | null;
    /** Language */
    language: string | null;
    /** Name */
    name: string;
    /** Target Duration Ms */
    target_duration_ms: number | null;
    /** Visual Style */
    visual_style: string | null | null;
    /** Workspace Id */
    workspace_id: string;
  };

  type ProjectProductionSnapshot = {
    /** Blocking Reasons */
    blocking_reasons: BlockingReason[];
    /** Completion */
    completion: number;
    /** Computed At */
    computed_at: string;
    /** Current Stage */
    current_stage:
      | "project_setup"
      | "script_import"
      | "structure_review"
      | "asset_preparation"
      | "storyboard_preparation";
    /** Episodes */
    episodes: EpisodeProductionSnapshot[];
    /** Next Actions */
    next_actions: NextAction[];
    /** Partial Failures */
    partial_failures: PartialFailure[];
    /** Project Id */
    project_id: string;
  };

  type projectProductionSnapshotApiV1ProjectsProjectIdProductionSnapshotGetParams =
    {
      project_id: string;
    };

  type ProjectResponse = {
    /** Aspect Ratio */
    aspect_ratio: "9:16" | "16:9" | "1:1";
    /** Budget Limit */
    budget_limit: string;
    /** Currency */
    currency: string;
    /** Description */
    description: string | null;
    /** Id */
    id: string;
    /** Language */
    language: string;
    /** Name */
    name: string;
    /** Revision */
    revision: number;
    /** Status */
    status: "active" | "archived";
    /** Target Duration Ms */
    target_duration_ms: number;
    /** Visual Style */
    visual_style: string | null;
    /** Workspace Id */
    workspace_id: string;
  };

  type ProjectStateRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type ProjectUpdateRequest = {
    /** Aspect Ratio */
    aspect_ratio: "9:16" | "16:9" | "1:1" | null | null;
    /** Description */
    description: string | null | null;
    /** Expected Revision */
    expected_revision: number;
    /** Language */
    language: string | null | null;
    /** Name */
    name: string | null | null;
    /** Target Duration Ms */
    target_duration_ms: number | null | null;
    /** Visual Style */
    visual_style: string | null | null;
  };

  type PropSpec = {
    /** Appearance */
    appearance: string | null;
    /** Holder Character Id */
    holder_character_id: string | null | null;
    /** Kind */
    kind: "prop";
    /** Material */
    material: string | null;
    /** Usage Context */
    usage_context: string | null;
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
    /** Dependencies */
    dependencies: Record<string, any>;
    /** Status */
    status: "ready" | "degraded" | "unavailable";
  };

  type RegisterRequest = {
    /** Display Name */
    display_name: string;
    /** Password */
    password: string;
    /** Registration Ticket */
    registration_ticket: string;
  };

  type RegistrationVerificationAccepted = {
    /** Accepted */
    accepted: true | null;
    /** Email Sent */
    email_sent: boolean;
    /** Retry After Seconds */
    retry_after_seconds: number;
  };

  type RegistrationVerificationConfirmed = {
    /** Expires In */
    expires_in: number;
    /** Registration Ticket */
    registration_ticket: string;
  };

  type RegistrationVerificationConfirmRequest = {
    /** Code */
    code: string;
    /** Email */
    email: string;
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

  type replaceReferencesApiV1ShotsShotIdNarrativeReferencesPostParams = {
    shot_id: string;
  };

  type requestExportApiV1EpisodesEpisodeIdStoryboardExportsPostParams = {
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
    /** Created At */
    created_at: string;
    /** Currency */
    currency: string;
    /** Estimated Amount */
    estimated_amount: string;
    /** Id */
    id: string;
    /** Request Id */
    request_id: string;
    /** Reserved Amount */
    reserved_amount: string;
    /** Revision */
    revision: number;
    /** Status */
    status: "active" | "settled" | "released";
    /** Workspace Id */
    workspace_id: string;
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
    /** Pending */
    pending: number | null;
    /** Status */
    status: "not_started" | "pending" | "completed" | "unavailable";
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
    /** Characters */
    characters: string[] | null;
    /** Continuity Notes */
    continuity_notes: string[] | null;
    /** Environment Details */
    environment_details: string | null | null;
    /** Episode Number */
    episode_number: number | null | null;
    /** Heading */
    heading: string;
    /** Kind */
    kind: "scene";
    /** Location */
    location: string;
    /** Production Tasks */
    production_tasks: SceneProductionTask[] | null;
    /** Props */
    props: string[] | null;
    /** Scene Number */
    scene_number: number | null | null;
    /** Story Beat */
    story_beat: string | null | null;
    /** Summary */
    summary: string;
    /** Time Of Day */
    time_of_day: string;
  };

  type SceneProductionTask = {
    /** Objective */
    objective: string;
    /** Priority */
    priority: "low" | "normal" | "high" | "blocking" | null;
    /** Task Type */
    task_type:
      | "asset_prepare"
      | "shot_breakdown"
      | "continuity_review"
      | "voice_prepare";
    /** Title */
    title: string;
  };

  type SceneResponse = {
    /** Created At */
    created_at: string;
    /** Dialogues */
    dialogues: DialogueResponse[];
    /** Heading */
    heading: string;
    /** Id */
    id: string;
    /** Location */
    location: string;
    /** Position */
    position: number;
    /** Script Version Id */
    script_version_id: string;
    /** Semantic Context */
    semantic_context: Record<string, any>;
    source_range: CandidateSourceRange;
    /** Summary */
    summary: string;
    /** Time Of Day */
    time_of_day: string;
  };

  type ScheduleConfigurationRequest = {
    /** Cron Expression */
    cron_expression: string | null | null;
    /** Effective From */
    effective_from: string;
    /** Expected Revision */
    expected_revision: number;
    /** Interval Seconds */
    interval_seconds: number | null | null;
    /** Kind */
    kind: "interval" | "cron";
    /** Max Catch Up */
    max_catch_up: number | null;
    /** Misfire Grace Seconds */
    misfire_grace_seconds: number | null;
    /** Misfire Policy */
    misfire_policy: "skip" | "run_once" | "catch_up";
    /** Timezone */
    timezone: string | null;
  };

  type ScheduleCronRuleResponse = {
    /** Expression */
    expression: string;
    /** Kind */
    kind: "cron" | null;
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
    task: TaskResponse;
    /** Trigger Kind */
    trigger_kind: "scheduled" | "manual";
  };

  type ScheduleIntervalRuleResponse = {
    /** Kind */
    kind: "interval" | null;
    /** Misfire Grace Seconds */
    misfire_grace_seconds: number;
    /** Seconds */
    seconds: number;
  };

  type ScheduleOneOffRuleResponse = {
    /** At */
    at: string;
    /** Kind */
    kind: "one_off" | null;
    /** Misfire Grace Seconds */
    misfire_grace_seconds: number;
  };

  type ScheduleResponse = {
    /** Failure Count */
    failure_count: number;
    /** Handler Name */
    handler_name:
      | "expire_upload_session"
      | "cleanup_expired_uploads"
      | "retire_media_location"
      | "unregistered";
    /** Id */
    id: string;
    /** Kind */
    kind: "one_off" | "interval" | "cron";
    /** Last Error */
    last_error: string | null;
    /** Max Catch Up */
    max_catch_up: number;
    /** Misfire Policy */
    misfire_policy: "skip" | "run_once" | "catch_up";
    /** Next Attempt At */
    next_attempt_at: string | null;
    /** Next Fire At */
    next_fire_at: string | null;
    /** Revision */
    revision: number;
    /** Rule */
    rule:
      | ScheduleOneOffRuleResponse
      | ScheduleIntervalRuleResponse
      | ScheduleCronRuleResponse
      | UnknownScheduleRuleResponse;
    /** Schedule Key */
    schedule_key: string;
    scope: ScheduleScopeResponse;
    /** Status */
    status: "active" | "paused" | "completed" | "manual_attention";
    /** Timezone */
    timezone: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type ScheduleResumeRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Max Catch Up */
    max_catch_up: number | null;
    /** Misfire Policy */
    misfire_policy: "skip" | "run_once" | "catch_up";
    /** Resume From */
    resume_from: string;
  };

  type ScheduleScopeResponse = {
    /** Usage Id */
    usage_id: string;
    /** Usage Type */
    usage_type: "upload_session" | "workspace" | "media_location";
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
    /** Blocks */
    blocks: NarrativeBlockResponse[];
    document: ScriptDocumentResponse;
    /** Issues */
    issues: FormatIssueResponse[];
    revision: DocumentRevisionResponse;
  };

  type ScriptDocumentImportRequest = {
    /** Idempotency Key */
    idempotency_key: string;
    /** Input Type */
    input_type: "text" | "media";
    /** Language */
    language: string;
    /** Media Version Id */
    media_version_id: string | null | null;
    /** Rights Declaration */
    rights_declaration: string;
    /** Text */
    text: string | null | null;
    /** Title */
    title: string;
  };

  type ScriptDocumentPreviewRequest = {
    /** Media Version Id */
    media_version_id: string;
  };

  type ScriptDocumentPreviewResponse = {
    /** Codepoint Count */
    codepoint_count: number;
    /** Media Version Id */
    media_version_id: string;
    /** Raw Hash */
    raw_hash: string;
    /** Raw Text */
    raw_text: string;
  };

  type ScriptDocumentResponse = {
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Id */
    id: string;
    /** Language */
    language: string;
    /** Project Id */
    project_id: string;
    /** Revision */
    revision: number;
    /** Rights Declaration */
    rights_declaration: string;
    /** Source Media Version Id */
    source_media_version_id: string | null;
    /** Source Type */
    source_type: "text" | "media";
    /** Status */
    status: "active" | "archived";
    /** Title */
    title: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type ScriptExtractionRequest = {
    /** Idempotency Key */
    idempotency_key: string;
    /** Scope */
    scope: "full";
  };

  type ScriptImportRequest = {
    /** Body */
    body: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Input Type */
    input_type: "text";
    /** Rights Declaration */
    rights_declaration: string;
    /** Title */
    title: string;
  };

  type ScriptImportResponse = {
    source: ScriptSourceResponse;
    version: ScriptVersionResponse;
  };

  type ScriptReference = {
    /** Confirmed Script Version Id */
    confirmed_script_version_id: string;
    /** Dialogue Ids */
    dialogue_ids: string[] | null;
    /** Scene Id */
    scene_id: string;
  };

  type ScriptSourceResponse = {
    /** Created At */
    created_at: string;
    /** Episode Id */
    episode_id: string;
    /** Id */
    id: string;
    /** Input Type */
    input_type: "text" | "media";
    /** Revision */
    revision: number;
    /** Rights Declaration */
    rights_declaration: string;
    /** Source Media Version Id */
    source_media_version_id: string | null;
    /** Status */
    status: "active" | "archived";
    /** Title */
    title: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type ScriptSourceStateRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type ScriptSummary = {
    /** Current Version Id */
    current_version_id: string | null;
    /** Extraction Batch Id */
    extraction_batch_id: string | null;
    /** Pending Required Candidates */
    pending_required_candidates: number | null;
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
  };

  type ScriptVersionDeleteResponse = {
    /** Deleted */
    deleted: true | null;
    /** Script Version Id */
    script_version_id: string;
  };

  type ScriptVersionDiffResponse = {
    /** Added Lines */
    added_lines: number;
    /** Base Version Id */
    base_version_id: string;
    /** Diff Lines */
    diff_lines: string[];
    /** Removed Lines */
    removed_lines: number;
    /** Target Version Id */
    target_version_id: string;
  };

  type ScriptVersionImpactResponse = {
    /** Affected Shot Ids */
    affected_shot_ids: string[];
    /** Current Narrative Dependency Hash */
    current_narrative_dependency_hash: string;
    /** Current Script Version Id */
    current_script_version_id: string;
    /** Invalidated Scopes */
    invalidated_scopes: ("shot_readiness" | "coverage" | "export")[];
    /** Narrative Impact Id */
    narrative_impact_id: string;
    /** Previous Narrative Dependency Hash */
    previous_narrative_dependency_hash: string | null;
    /** Previous Script Version Id */
    previous_script_version_id: string | null;
  };

  type ScriptVersionPublishRequest = {
    /** Body */
    body: string;
    /** Expected Current Version Id */
    expected_current_version_id: string | null;
  };

  type ScriptVersionPublishResponse = {
    current: CurrentScriptVersionResponse;
    version: ScriptVersionResponse;
  };

  type ScriptVersionResponse = {
    /** Body */
    body: string;
    /** Content Hash */
    content_hash: string;
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Id */
    id: string;
    /** Source Id */
    source_id: string;
    /** Status */
    status: "draft" | "published";
    /** Version No */
    version_no: number;
    /** Workspace Id */
    workspace_id: string;
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
    /** Action */
    action: string | null | null;
    /** Asset Names */
    asset_names: string[] | null;
    /** Camera Movement */
    camera_movement: string | null | null;
    /** Continuity Notes */
    continuity_notes: string[] | null;
    /** Dialogue Excerpt */
    dialogue_excerpt: string | null | null;
    /** Duration Ms */
    duration_ms: number | null | null;
    /** Framing */
    framing: string | null | null;
    /** Kind */
    kind: "shot";
    /** Purpose */
    purpose: string;
    /** Scene Candidate Key */
    scene_candidate_key: string;
    /** Shot Number */
    shot_number: number | null | null;
    /** Shot Type */
    shot_type: string | null | null;
    /** Title */
    title: string;
    /** Visual Prompt */
    visual_prompt: string | null | null;
  };

  type ShotCoverageResponse = {
    /** Position */
    position: number;
    /** Shot Id */
    shot_id: string;
    /** Spec Version Id */
    spec_version_id: string | null;
    /** Status */
    status: "linked" | "approved_invented" | "orphan";
    /** Title */
    title: string;
    /** Unit Version Ids */
    unit_version_ids: string[];
  };

  type ShotCreateRequest = {
    /** Creation Key */
    creation_key: string;
    /** Source Scene Id */
    source_scene_id: string;
    /** Source Script Version Id */
    source_script_version_id: string;
    /** Title */
    title: string;
  };

  type ShotCurrentSpecRequest = {
    /** Expected Current Spec Version Id */
    expected_current_spec_version_id: string | null;
    /** Expected Revision */
    expected_revision: number;
    /** Version Id */
    version_id: string;
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
    /** Evaluation Hash */
    evaluation_hash: string;
    /** Items */
    items: ShotReadinessResponse[];
    summary: ShotReadinessSummary;
  };

  type ShotReadinessDependencies = {
    /** Asset Evaluation Hashes */
    asset_evaluation_hashes: Record<string, any>;
    /** Asset Version Ids */
    asset_version_ids: string[];
    /** Confirmed Script Version Id */
    confirmed_script_version_id: string;
    /** Consent Ids */
    consent_ids: string[];
    /** Coverage Basis Hash */
    coverage_basis_hash: string | null;
    /** Coverage Evaluation Hash */
    coverage_evaluation_hash: string | null;
    /** Current Script Version Id */
    current_script_version_id: string | null;
    /** Dialogue Ids */
    dialogue_ids: string[];
    /** Media Version Ids */
    media_version_ids: string[];
    /** Narrative Dependency Hash */
    narrative_dependency_hash: string | null;
    /** Narrative Structure Id */
    narrative_structure_id: string | null;
    /** Narrative Structure Revision */
    narrative_structure_revision: number | null;
    /** Scene Id */
    scene_id: string;
    /** Shot Spec Version Id */
    shot_spec_version_id: string | null;
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
      | "DEPENDENCY_UNAVAILABLE"
      | "NARRATIVE_REFERENCE_INVALID"
      | "COVERAGE_UNACCOUNTED"
      | "SHOT_SOURCE_ORPHAN"
      | "COVERAGE_DEPENDENCY_UNAVAILABLE";
    /** Dependency Id */
    dependency_id: string | null | null;
    /** Dependency Type */
    dependency_type: string | null | null;
    /** Field Path */
    field_path: string | null | null;
    /** Next Action */
    next_action: string;
    /** Summary */
    summary: string;
  };

  type ShotReadinessResponse = {
    /** Blocking Reasons */
    blocking_reasons: ShotReadinessIssue[];
    evaluated_dependencies: ShotReadinessDependencies;
    /** Evaluation Hash */
    evaluation_hash: string;
    /** Next Actions */
    next_actions: string[];
    /** Ready */
    ready: boolean;
    /** Shot Id */
    shot_id: string;
    /** Status */
    status: "ready" | "blocked" | "unavailable";
    /** Warnings */
    warnings: ShotReadinessWarning[];
  };

  type ShotReadinessSummary = {
    /** Blocked */
    blocked: number;
    /** Ready */
    ready: number;
    /** Total */
    total: number;
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
    /** Next Action */
    next_action: string;
    /** Summary */
    summary: string;
  };

  type ShotReorderRequest = {
    /** Expected Order Hash */
    expected_order_hash: string;
    /** Shot Ids */
    shot_ids: string[];
  };

  type ShotResponse = {
    /** Created At */
    created_at: string;
    /** Current Spec Version Id */
    current_spec_version_id: string | null;
    /** Episode Id */
    episode_id: string;
    /** Id */
    id: string;
    /** Position */
    position: number;
    /** Revision */
    revision: number;
    /** Source Candidate Id */
    source_candidate_id: string | null;
    /** Source Draft Shot Id */
    source_draft_shot_id: string | null;
    /** Source Scene Id */
    source_scene_id: string;
    /** Source Script Version Id */
    source_script_version_id: string;
    /** Status */
    status: "active" | "archived";
    /** Title */
    title: string;
    /** Updated At */
    updated_at: string;
    /** Workspace Id */
    workspace_id: string;
  };

  type ShotSpec = {
    /** Action Beats */
    action_beats: ActionBeat[];
    audio_intent: AudioIntent | null | null;
    /** Dialogue Or Narration */
    dialogue_or_narration: DialogueOrNarration[] | null;
    /** Duration Ms */
    duration_ms: number | null;
    generation_intent: GenerationIntent;
    narrative: NarrativeSpec;
    /** Schema Version */
    schema_version: 1 | null;
    script_reference: ScriptReference;
    visual: VisualSpec;
  };

  type ShotSpecCreateRequest = {
    /** Asset References */
    asset_references: AssetReferenceRequest[] | null;
    /** Expected Current Spec Version Id */
    expected_current_spec_version_id: string | null;
    /** Narrative References */
    narrative_references: NarrativeReferenceInput[];
    spec: ShotSpec;
  };

  type ShotSpecCreateResponse = {
    shot: ShotResponse;
    version: ShotSpecVersionResponse;
  };

  type ShotSpecVersionResponse = {
    /** Asset References */
    asset_references: AssetReferenceResponse[];
    /** Content Hash */
    content_hash: string;
    /** Created At */
    created_at: string;
    /** Created By */
    created_by: string;
    /** Id */
    id: string;
    /** Input Hash */
    input_hash: string;
    /** Schema Version */
    schema_version: 1;
    /** Shot Id */
    shot_id: string;
    spec: ShotSpec;
    /** Version No */
    version_no: number;
    /** Workspace Id */
    workspace_id: string;
  };

  type ShotStateRequest = {
    /** Expected Order Hash */
    expected_order_hash: string;
    /** Expected Revision */
    expected_revision: number;
  };

  type ShotStateResponse = {
    order: ShotOrderResponse;
    shot: ShotResponse;
  };

  type ShotTransformEvidenceResponse = {
    /** Actor Id */
    actor_id: string;
    /** Created At */
    created_at: string;
    /** Id */
    id: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Impact Hash */
    impact_hash: string;
    /** Input Hash */
    input_hash: string;
    /** Operation */
    operation: "copy" | "split" | "merge";
    /** Result Shot Ids */
    result_shot_ids: string[];
    /** Source Shot Ids */
    source_shot_ids: string[];
    /** Source Spec Version Ids */
    source_spec_version_ids: string[];
  };

  type ShotTransformPreflightResponse = {
    downstream_evidence: DownstreamEvidenceResponse;
    /** Impact Hash */
    impact_hash: string;
    /** Operation */
    operation: "split" | "merge";
    /** Order Hash */
    order_hash: string;
    /** Source Shot Ids */
    source_shot_ids: string[];
    /** Source Spec Version Ids */
    source_spec_version_ids: string[];
  };

  type ShotTransformResponse = {
    order: ShotOrderResponse;
    /** Shots */
    shots: ShotResponse[];
    /** Spec Versions */
    spec_versions: ShotSpecVersionResponse[];
    transform: ShotTransformEvidenceResponse;
  };

  type ShotUpdateRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Title */
    title: string;
  };

  type SourceRange = {
    /** End */
    end: number;
    /** Start */
    start: number;
  };

  type splitEpisodeProposalApiV1EpisodePlansPlanIdSplitPostParams = {
    plan_id: string;
  };

  type SplitEpisodeProposalRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Idempotency Key */
    idempotency_key: string;
    /** New Title */
    new_title: string;
    /** Proposal Id */
    proposal_id: string;
    /** Source Offset */
    source_offset: number;
  };

  type splitPreflightApiV1ShotsShotIdSplitPreflightPostParams = {
    shot_id: string;
  };

  type SplitPreflightRequest = {
    /** Expected Order Hash */
    expected_order_hash: string;
    /** Expected Source Spec Version Id */
    expected_source_spec_version_id: string;
  };

  type splitShotApiV1ShotsShotIdSplitPostParams = {
    shot_id: string;
  };

  type SplitShotRequest = {
    /** Expected Order Hash */
    expected_order_hash: string;
    /** Expected Source Spec Version Id */
    expected_source_spec_version_id: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Impact Hash */
    impact_hash: string;
    /** Targets */
    targets: TargetShotSpecRequest[];
  };

  type startExtractionApiV1ScriptVersionsVersionIdExtractionsPostParams = {
    version_id: string;
  };

  type StoryboardSummary = {
    /** Blocked */
    blocked: number | null;
    /** Ready */
    ready: number | null;
    /** Status */
    status: "not_started" | "blocked" | "ready" | "unavailable";
    /** Total */
    total: number | null;
    /** Unavailable */
    unavailable: number | null;
  };

  type StructureConfirmationResponse = {
    /** Batch Id */
    batch_id: string;
    confirmed_version: ScriptVersionResponse;
    /** Scenes */
    scenes: SceneResponse[];
    /** Source Script Version Id */
    source_script_version_id: string;
  };

  type StyleSpec = {
    /** Kind */
    kind: "visual_style";
    /** Lighting Language */
    lighting_language: string | null;
    /** Negative Constraints */
    negative_constraints: string[] | null;
    /** Palette */
    palette: string | null;
    /** Visual Language */
    visual_language: string | null;
  };

  type SubjectIdentity = {
    kind: SubjectIdentityKind;
    /** Reference */
    reference: string;
  };

  type SubjectIdentityKind =
    | "adult"
    | "fictional_adult"
    | "organization"
    | "minor";

  type SubjectPlacement = {
    /** Placement */
    placement: string;
    /** Subject Key */
    subject_key: string;
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
    /** Asset References */
    asset_references: AssetReferenceRequest[] | null;
    /** Narrative References */
    narrative_references: NarrativeReferenceInput[];
    spec: ShotSpec;
    /** Title */
    title: string;
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
    /** Cancel Status */
    cancel_status: "none" | "requested" | "accepted" | "rejected";
    error: TaskErrorResponse | null;
    /** Id */
    id: string;
    /** Next Action */
    next_action: string | null;
    /** Progress Stage */
    progress_stage: string;
    /** Request Id */
    request_id: string;
    /** Request Type */
    request_type:
      | "extraction_batch"
      | "episode_plan"
      | "adaptation_run"
      | "storyboard_draft_batch"
      | "storyboard_export_job"
      | "generation_request"
      | "media_version"
      | "upload_session"
      | "workspace"
      | "media_location";
    /** Revision */
    revision: number;
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
    /** Task Type */
    task_type:
      | "script_extraction"
      | "episode_planning"
      | "script_adaptation"
      | "storyboard_draft"
      | "storyboard_export"
      | "image_generation"
      | "video_generation"
      | "media_probe"
      | "upload_expiration"
      | "upload_cleanup"
      | "media_location_migration"
      | "media_location_retirement";
    /** Workspace Id */
    workspace_id: string;
  };

  type TaskScopeResponse = {
    /** Episode Id */
    episode_id: string | null;
    /** Input Hash */
    input_hash: string | null;
    /** Input Version Id */
    input_version_id: string | null;
    /** Render Snapshot Id */
    render_snapshot_id: string | null;
    /** Usage Id */
    usage_id: string | null;
    /** Usage Type */
    usage_type: string | null;
  };

  type TaskSummary = {
    /** Failed */
    failed: number | null;
    /** Running */
    running: number | null;
    /** Status */
    status: "not_started" | "running" | "failed" | "succeeded" | "unavailable";
    /** Succeeded */
    succeeded: number | null;
    /** Unknown */
    unknown: number | null;
  };

  type triggerScheduleApiV1SchedulesScheduleIdTriggerPostParams = {
    schedule_id: string;
  };

  type UnitCoverageResponse = {
    /** Exact Text */
    exact_text: string;
    /** Kind */
    kind: "scene_heading" | "action" | "dialogue" | "narration";
    /** Narrative Unit Id */
    narrative_unit_id: string;
    /** Position */
    position: number;
    /** Required Channel */
    required_channel: "visual" | "audio";
    /** Required For Coverage */
    required_for_coverage: boolean;
    /** Shot Ids */
    shot_ids: string[];
    /** Status */
    status: "covered" | "approved_omitted" | "uncovered";
    /** Unit Version Id */
    unit_version_id: string;
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
    /** Expires At */
    expires_at: string;
    /** Headers */
    headers: Record<string, any>;
    /** Method */
    method: "PUT" | null;
    /** Url */
    url: string;
  };

  type UploadCompletionResponse = {
    media_object: MediaObjectResponse;
    probe_task: TaskResponse;
    version: MediaVersionResponse;
  };

  type UploadDeclaration = {
    /** Filename */
    filename: string;
    /** Idempotency Key */
    idempotency_key: string;
    /** Kind */
    kind: "image" | "video" | "audio" | "subtitle" | "delivery" | "document";
    /** Mime Type */
    mime_type: string;
    /** Sha256 */
    sha256: string;
    /** Size Bytes */
    size_bytes: number;
    /** Workspace Id */
    workspace_id: string;
  };

  type UploadInitializationResponse = {
    upload: UploadCapabilityResponse;
    upload_session: UploadSessionResponse;
  };

  type UploadSessionResponse = {
    /** Expires At */
    expires_at: string;
    /** Filename */
    filename: string;
    /** Id */
    id: string;
    /** Kind */
    kind: "image" | "video" | "audio" | "subtitle" | "delivery" | "document";
    /** Media Object Id */
    media_object_id: string | null;
    /** Mime Type */
    mime_type: string;
    /** Sha256 */
    sha256: string;
    /** Size Bytes */
    size_bytes: number;
    /** Status */
    status: "pending" | "completed" | "expired" | "failed";
    /** Workspace Id */
    workspace_id: string;
  };

  type UserResponse = {
    /** Avatar Url */
    avatar_url: string | null;
    /** Display Name */
    display_name: string;
    /** Email */
    email: string;
    /** Id */
    id: string;
  };

  type ValidationError = {
    /** Context */
    ctx: Record<string, any> | null;
    /** Input */
    input: any | null;
    /** Location */
    loc: (string | number)[];
    /** Message */
    msg: string;
    /** Error Type */
    type: string;
  };

  type VisualSpec = {
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
    /** Mood Lighting */
    mood_lighting: string;
    /** Shot Size */
    shot_size:
      | "extreme_wide"
      | "wide"
      | "full"
      | "medium"
      | "medium_close_up"
      | "close_up"
      | "extreme_close_up";
    /** Subject Placements */
    subject_placements: SubjectPlacement[] | null;
  };

  type VoiceSpec = {
    /** Allowed Usage */
    allowed_usage: string[] | null;
    /** Kind */
    kind: "voice";
    /** Language */
    language: string | null;
    /** Performance Traits */
    performance_traits: string[] | null;
    /** Source Kind */
    source_kind:
      | "synthetic_recording"
      | "human_recording"
      | "voice_clone"
      | null
      | null;
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
    /** Revision */
    revision: number;
    /** Role */
    role: "owner" | "editor" | "viewer";
    /** Status */
    status: "active" | "archived";
  };

  type WorkspaceStateRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type WorkspaceUpdateRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Name */
    name: string;
  };
}
