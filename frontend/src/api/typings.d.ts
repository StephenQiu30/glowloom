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

  type ApiResponseAssetDeletePreflightResponse_ = {
    data: AssetDeletePreflightResponse;
  };

  type ApiResponseAssetDeleteResponse_ = {
    data: AssetDeleteResponse;
  };

  type ApiResponseAssetReadinessResponse_ = {
    data: AssetReadinessResponse;
  };

  type ApiResponseAssetResponse_ = {
    data: AssetResponse;
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

  type ApiResponseConsentDetailResponse_ = {
    data: ConsentDetailResponse;
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

  type ApiResponseMediaAccessResponse_ = {
    data: MediaAccessResponse;
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

  type ApiResponsePaginatedAssets_ = {
    data: PaginatedAssets;
  };

  type ApiResponsePaginatedAssetVersions_ = {
    data: PaginatedAssetVersions;
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

  type ApiResponseUploadCompletionResponse_ = {
    data: UploadCompletionResponse;
  };

  type ApiResponseUploadInitializationResponse_ = {
    data: UploadInitializationResponse;
  };

  type ApiResponseWorkspaceResponse_ = {
    data: WorkspaceResponse;
  };

  type appendAssetVersionApiV1AssetsAssetIdVersionsPostParams = {
    asset_id: string;
  };

  type AppendVersionRequest = {
    /** Workspace Id */
    workspace_id: string;
    /** Kind */
    kind: "image" | "video" | "audio" | "subtitle" | "delivery";
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

  type AssetCurrentVersionRequest = {
    /** Version Id */
    version_id: string;
    /** Expected Current Version Id */
    expected_current_version_id: string | null;
    /** Expected Revision */
    expected_revision: number;
  };

  type AssetDeleteBlocker = {
    /** Code */
    code: string;
    /** Summary */
    summary: string;
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
    /** Current Version Id */
    current_version_id: string | null;
    /** Revision */
    revision: number;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
    /** Warnings */
    warnings: "duplicate_name"[] | null;
  };

  type AssetStateRequest = {
    /** Expected Revision */
    expected_revision: number;
  };

  type AssetUpdateRequest = {
    /** Expected Revision */
    expected_revision: number;
    /** Name */
    name: string | null | null;
    /** Aliases */
    aliases: string[] | null | null;
    /** Tags */
    tags: string[] | null | null;
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
    source_type: "manual" | "candidate" | null;
    /** Source Id */
    source_id: string | null | null;
    /** Expected Current Version Id */
    expected_current_version_id: string | null;
    /** Set As Current */
    set_as_current: boolean | null;
  };

  type AssetVersionCreateResponse = {
    asset: AssetResponse;
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
    source_type: "manual" | "candidate";
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

  type getAssetApiV1AssetsAssetIdGetParams = {
    asset_id: string;
  };

  type getAssetReadinessApiV1AssetVersionsVersionIdReadinessGetParams = {
    version_id: string;
    purpose: string;
    channel: string;
    region: string;
  };

  type getAssetVersionApiV1AssetVersionsVersionIdGetParams = {
    version_id: string;
  };

  type getConsentApiV1ConsentsConsentIdGetParams = {
    consent_id: string;
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

  type getMediaApiV1MediaVersionIdGetParams = {
    version_id: string;
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

  type listAssetVersionsApiV1AssetsAssetIdVersionsGetParams = {
    asset_id: string;
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
    kind: "image" | "video" | "audio" | "subtitle" | "delivery" | null | null;
    source_type: "upload" | "generated" | "rendered" | null | null;
    include_archived: boolean | null;
    created_from: string | null | null;
    created_to: string | null | null;
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
    task_type: "script_extraction" | "media_probe" | null | null;
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

  type MediaObjectResponse = {
    /** Id */
    id: string;
    /** Workspace Id */
    workspace_id: string;
    /** Kind */
    kind: "image" | "video" | "audio" | "subtitle" | "delivery";
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

  type restoreAssetApiV1AssetsAssetIdRestorePostParams = {
    asset_id: string;
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

  type retryProbeApiV1MediaVersionIdProbeRetryPostParams = {
    version_id: string;
  };

  type ReviewSummary = {
    /** Status */
    status: "not_started" | null;
    /** Pending */
    pending: number | null;
  };

  type reviseConsentApiV1ConsentsConsentIdRevisionsPostParams = {
    consent_id: string;
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

  type setCurrentAssetVersionApiV1AssetsAssetIdCurrentVersionPostParams = {
    asset_id: string;
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

  type SubjectType =
    | "SCRIPT_VERSION"
    | "ASSET_VERSION"
    | "SHOT_SPEC_VERSION"
    | "CANDIDATE"
    | "MEDIA_VERSION"
    | "TIMELINE_VERSION"
    | "DELIVERY";

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
    task_type: "script_extraction" | "media_probe";
    /** Request Type */
    request_type: "extraction_batch" | "media_version";
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

  type updateAssetApiV1AssetsAssetIdPatchParams = {
    asset_id: string;
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
    kind: "image" | "video" | "audio" | "subtitle" | "delivery";
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
    kind: "image" | "video" | "audio" | "subtitle" | "delivery";
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
