declare namespace CanvasContract {
  type AccountVO = {
    id?: number | string;
    userAccount?: string;
    userName?: string;
    userAvatar?: string;
    userProfile?: string;
    userEmail?: string;
    userPhone?: string;
    userRole?: string;
    revision?: number | string;
    actions?: string[];
    createdAt?: string;
    updatedAt?: string;
    userCoverFileId?: string;
  };

  type AssetBatchDeleteEntry = {
    assetId: number | string;
    expectedRevision: number | string;
  };

  type AssetBatchDeleteRequest = {
    commandId: string;
    assets: AssetBatchDeleteEntry[];
  };

  type AssetBatchDeleteVO = {
    assetIds?: (number | string)[];
  };

  type AssetDeleteRequest = {
    commandId: string;
    assetId: number | string;
    expectedRevision?: number | string;
  };

  type AssetDownloadRequest = {
    assetId: number | string;
  };

  type AssetDownloadVO = {
    url?: string;
    headUrl?: string;
    expiresSeconds?: number;
  };

  type AssetLibraryCreateRequest = {
    commandId: string;
    projectId: number | string;
    expectedRevision: number | string;
  };

  type AssetLibraryPageVO = {
    records?: AssetLibraryVO[];
    total?: number | string;
    current?: number | string;
    pageSize?: number | string;
  };

  type AssetLibraryVO = {
    projectId?: number | string;
    title?: string;
    projectPath?: string;
    revision?: number | string;
    enabled?: boolean;
  };

  type AssetMoveRequest = {
    commandId: string;
    libraryScope: string;
    projectId?: number | string;
    assets: AssetBatchDeleteEntry[];
  };

  type AssetMoveVO = {
    assetIds?: (number | string)[];
    libraryScope?: string;
    projectId?: number | string;
  };

  type AssetPageVO = {
    records?: AssetVO[];
    total?: number | string;
    current?: number | string;
    size?: number | string;
  };

  type AssetReadRequest = {
    scope: string;
    id: number | string;
    revision: number | string;
    purpose: string;
  };

  type BaseResponseCreditPriceSimulationVO = {
    code?: number;
    data?: CreditPriceSimulationVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseModelPricingContextVO = {
    code?: number;
    data?: ModelPricingContextVO;
    message?: string;
    requestId?: string;
  };

  type CreditPriceConfigureRequest = {
    commandId: string;
    expectedPriceIds: (number | string)[];
    content: CreditPriceContent;
  };

  type CreditPriceSimulationRequest = {
    content: CreditPriceContent;
    specification: Record<string, unknown>;
  };

  type CreditPriceSimulationVO = {
    content?: CreditPriceContent;
    estimatedPoints?: string;
    maximumPoints?: string;
  };

  type ModelPricingContextVO = {
    connection?: ModelConnectionVO;
    capabilities?: Record<string, unknown>;
  };

  type ModelPricingRule = {
    capabilityVersion: string;
    tiers: Tier[];
  };

  type Tier = {
    profileId: string;
    sizes: string[];
    resolutions: string[];
    ratios: string[];
    minDuration?: number;
    maxDuration?: number;
    maxReferenceImages?: number;
    rates: Record<string, unknown>;
  };

  type assetReviewCommandParams = {
    commandId: string;
  };

  type assetReviewCurrentParams = {
    assetId: number | string;
  };

  type AssetReviewDecisionRequest = {
    commandId: string;
    operationId: number | string;
    expectedRevision: number | string;
    decision: string;
    reason: string;
    viewedOriginal: boolean;
  };

  type assetReviewDetailParams = {
    operationId: number | string;
  };

  type AssetReviewPageVO = {
    records?: AssetReviewVO[];
    total?: number | string;
    current?: number | string;
    size?: number | string;
  };

  type assetReviewPublicCommandParams = {
    commandId: string;
  };

  type assetReviewPublicDetailParams = {
    id: number | string;
  };

  type AssetReviewSubmitRequest = {
    commandId: string;
    assetId: number | string;
    expectedRevision: number | string;
    statement: string;
    intendedUse: string;
    contentAccessGranted: boolean;
    arkAuthorized: boolean;
    publicAuthorized: boolean;
  };

  type AssetReviewVO = {
    assetId?: number | string;
    assetRevision?: number | string;
    operationId?: number | string;
    revision?: number | string;
    title?: string;
    kind?: string;
    sourceType?: string;
    policy?: string;
    status?: string;
    stage?: string;
    basis?: string;
    statement?: string;
    intendedUse?: string;
    reason?: string;
    arkAuthorized?: boolean;
    publicAuthorized?: boolean;
    reviewerId?: number | string;
    updateTime?: string;
  };

  type AssetReviewWithdrawRequest = {
    commandId: string;
    operationId: number | string;
    expectedRevision: number | string;
  };

  type AssetSaveRequest = {
    commandId: string;
    assetId?: number | string;
    expectedRevision?: number | string;
    kind: string;
    title: string;
    tags?: string[];
    source?: string;
    note?: string;
    content?: string;
    fileId?: string;
    fileVersion?: string;
    libraryScope?: string;
    projectId?: number | string;
  };

  type AssetSearchQueryRequest = {
    current?: number;
    pageSize?: number;
    sortField?: string;
    sortOrder?: string;
    keyword?: string;
    matchMode?: string;
    libraryScope?: string;
    projectId?: string;
    kind?: string;
    referenceOnly?: boolean;
    reviewStatus?: string;
  };

  type AssetVO = {
    id?: number | string;
    kind?: string;
    title?: string;
    source?: string;
    note?: string;
    content?: string;
    fileId?: string;
    fileVersion?: string;
    mediaJson?: string;
    revision?: number | string;
    createTime?: string;
    updateTime?: string;
    deleted?: boolean;
    projectId?: number | string;
    projectPath?: string;
    review?: AssetReviewVO;
  };

  type AssetZipExportRequest = {
    commandId: string;
    assetIds?: (number | string)[];
    files?: FileReference[];
    texts?: TextEntry[];
    archiveName?: string;
  };

  type AssetZipExportVO = {
    jobId?: string;
    commandId?: string;
    status?: string;
    parts?: Part[];
    expiresAt?: string;
    syncRevision?: number | string;
  };

  type AssetZipImportRequest = {
    commandId: string;
    sourceFileId: string;
    sourceVersion: string;
    libraryScope?: string;
    projectId?: number | string;
  };

  type AssetZipImportVO = {
    jobId?: string;
    commandId?: string;
    status?: string;
    sourceArchiveName?: string;
    total?: number | string;
    expandedBytes?: number | string;
    saved?: number | string;
    failed?: number | string;
    skipped?: number | string;
    current?: number | string;
    pageSize?: number | string;
    items?: Item[];
    expiresAt?: string;
    errorCode?: string;
    syncRevision?: number | string;
    projectId?: number | string;
  };

  type AssetZipJobCommand = {
    jobId: string;
    commandId: string;
  };

  type AuthorityCommandRequest = {
    commandId: string;
    revision: number | string;
    operation: string;
    id?: number | string;
    key?: string;
    name?: string;
    enabled?: number;
    userId?: number | string;
    groupId?: number | string;
    ids?: (number | string)[];
    serviceName?: string;
    actionKey?: string;
  };

  type AuthorityItemVO = {
    id?: number | string;
    key?: string;
    name?: string;
    enabled?: number;
    revision?: number | string;
    userId?: number | string;
    groupId?: number | string;
    permissionId?: number | string;
    serviceName?: string;
    actionKey?: string;
  };

  type AuthorityOperationVO = {
    serviceName?: string;
    actionKey?: string;
  };

  type authorityPageParams = {
    kind: string;
    current?: number | string;
    pageSize?: number | string;
    ownerId?: number | string;
    actionKey?: string;
    serviceName?: string;
  };

  type AuthorityPageVO = {
    records?: AuthorityItemVO[];
    total?: number | string;
    current?: number | string;
    pageSize?: number | string;
    revision?: number | string;
    actions?: string[];
    operations?: AuthorityOperationVO[];
  };

  type BaseResponseAssetBatchDeleteVO = {
    code?: number;
    data?: AssetBatchDeleteVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseAssetDownloadVO = {
    code?: number;
    data?: AssetDownloadVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseAssetLibraryPageVO = {
    code?: number;
    data?: AssetLibraryPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseAssetLibraryVO = {
    code?: number;
    data?: AssetLibraryVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseAssetMoveVO = {
    code?: number;
    data?: AssetMoveVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseAssetPageVO = {
    code?: number;
    data?: AssetPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseAssetReviewPageVO = {
    code?: number;
    data?: AssetReviewPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseAssetReviewVO = {
    code?: number;
    data?: AssetReviewVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseAssetVO = {
    code?: number;
    data?: AssetVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseAssetZipExportVO = {
    code?: number;
    data?: AssetZipExportVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseAssetZipImportVO = {
    code?: number;
    data?: AssetZipImportVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseAuthorityPageVO = {
    code?: number;
    data?: AuthorityPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseBoolean = {
    code?: number;
    data?: boolean;
    message?: string;
    requestId?: string;
  };

  type BaseResponseBrowserSessionVO = {
    code?: number;
    data?: BrowserSessionVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCanvasCommandResultVO = {
    code?: number;
    data?: CanvasCommandResultVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCanvasCommandStatusVO = {
    code?: number;
    data?: CanvasCommandStatusVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCanvasGenerationVO = {
    code?: number;
    data?: CanvasGenerationVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCanvasPageVO = {
    code?: number;
    data?: CanvasPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCanvasSnapshotPageVO = {
    code?: number;
    data?: CanvasSnapshotPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCanvasSnapshotSliceVO = {
    code?: number;
    data?: CanvasSnapshotSliceVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCanvasSnapshotVO = {
    code?: number;
    data?: CanvasSnapshotVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCanvasTemplatePageVO = {
    code?: number;
    data?: CanvasTemplatePageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCanvasTemplateVO = {
    code?: number;
    data?: CanvasTemplateVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCanvasVO = {
    code?: number;
    data?: CanvasVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCaptchaChallengeVO = {
    code?: number;
    data?: CaptchaChallengeVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCaptchaTicketVO = {
    code?: number;
    data?: CaptchaTicketVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCreditAccountVO = {
    code?: number;
    data?: CreditAccountVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCreditActivityVO = {
    code?: number;
    data?: CreditActivityVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCreditAdminAccountPageVO = {
    code?: number;
    data?: CreditAdminAccountPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCreditBatchPageVO = {
    code?: number;
    data?: CreditBatchPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCreditConsumptionVO = {
    code?: number;
    data?: CreditConsumptionVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCreditGrantBatchVO = {
    code?: number;
    data?: CreditGrantBatchVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCreditLedgerPageVO = {
    code?: number;
    data?: CreditLedgerPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCreditLedgerVO = {
    code?: number;
    data?: CreditLedgerVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCreditPriceVO = {
    code?: number;
    data?: CreditPriceVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseCreditQuoteVO = {
    code?: number;
    data?: CreditQuoteVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseFilePreviewVO = {
    code?: number;
    data?: FilePreviewVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseFileSignedUrlVO = {
    code?: number;
    data?: FileSignedUrlVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseFileTransferVO = {
    code?: number;
    data?: FileTransferVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseFileUploadChoiceVO = {
    code?: number;
    data?: FileUploadChoiceVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseGenerationOptionsVO = {
    code?: number;
    data?: GenerationOptionsVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseGenerationStatisticsVO = {
    code?: number;
    data?: GenerationStatisticsVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseGenerationTaskVO = {
    code?: number;
    data?: GenerationTaskVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseListAssetZipExportVO = {
    code?: number;
    data?: AssetZipExportVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseListAssetZipImportVO = {
    code?: number;
    data?: AssetZipImportVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseListCanvasGenerationVO = {
    code?: number;
    data?: CanvasGenerationVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseListCreditPriceVO = {
    code?: number;
    data?: CreditPriceVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseListCreditProjectVO = {
    code?: number;
    data?: CreditProjectVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseListFilePreviewVO = {
    code?: number;
    data?: FilePreviewVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseListFileTransferVO = {
    code?: number;
    data?: FileTransferVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseListGenerationTaskVO = {
    code?: number;
    data?: GenerationTaskVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseListModelExecutionVO = {
    code?: number;
    data?: ModelExecutionVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseListModelManagedPresetVO = {
    code?: number;
    data?: ModelManagedPresetVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseListModelPresetVO = {
    code?: number;
    data?: ModelPresetVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseListModelPricePresetVO = {
    code?: number;
    data?: ModelPricePresetVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseListProjectCreditDirectoryVO = {
    code?: number;
    data?: ProjectCreditDirectoryVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseListPublicAssetVO = {
    code?: number;
    data?: PublicAssetVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseListWorkflowRunVO = {
    code?: number;
    data?: WorkflowRunVO[];
    message?: string;
    requestId?: string;
  };

  type BaseResponseLogEntryPageVO = {
    code?: number;
    data?: LogEntryPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseLogTaskPageVO = {
    code?: number;
    data?: LogTaskPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseLogTaskVO = {
    code?: number;
    data?: LogTaskVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseLong = {
    code?: number;
    data?: number | string;
    message?: string;
    requestId?: string;
  };

  type BaseResponseModelConnectionVO = {
    code?: number;
    data?: ModelConnectionVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseModelManagedPresetVO = {
    code?: number;
    data?: ModelManagedPresetVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseModelMeteringCatalogVO = {
    code?: number;
    data?: ModelMeteringCatalogVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseModelStatisticsVO = {
    code?: number;
    data?: ModelStatisticsVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseNotificationPageVO = {
    code?: number;
    data?: NotificationPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseNotificationSummaryVO = {
    code?: number;
    data?: NotificationSummaryVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseNotificationVO = {
    code?: number;
    data?: NotificationVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponsePageModelConnectionVO = {
    code?: number;
    data?: PageModelConnectionVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponsePageProjectVO = {
    code?: number;
    data?: PageProjectVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponsePlatformAuthorizationVO = {
    code?: number;
    data?: PlatformAuthorizationVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseProjectCanvasOperationStatusVO = {
    code?: number;
    data?: ProjectCanvasOperationStatusVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseProjectCreationStatusVO = {
    code?: number;
    data?: ProjectCreationStatusVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseProjectCreditDirectoryVO = {
    code?: number;
    data?: ProjectCreditDirectoryVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseProjectVO = {
    code?: number;
    data?: ProjectVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseProjectWorkspacePageVO = {
    code?: number;
    data?: ProjectWorkspacePageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseProjectWorkspaceVO = {
    code?: number;
    data?: ProjectWorkspaceVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponsePublicAssetPageVO = {
    code?: number;
    data?: PublicAssetPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponsePublicAssetVO = {
    code?: number;
    data?: PublicAssetVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseSearchVisibilityVO = {
    code?: number;
    data?: SearchVisibilityVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseString = {
    code?: number;
    data?: string;
    message?: string;
    requestId?: string;
  };

  type BaseResponseTeamCommandResultVO = {
    code?: number;
    data?: TeamCommandResultVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseTeamInvitationVO = {
    code?: number;
    data?: TeamInvitationVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseTeamMemberPageVO = {
    code?: number;
    data?: TeamMemberPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseTeamPageVO = {
    code?: number;
    data?: TeamPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseTeamVO = {
    code?: number;
    data?: TeamVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseUserAdminVO = {
    code?: number;
    data?: UserAdminVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseUserBatchVO = {
    code?: number;
    data?: UserBatchVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseUserHistoryPageVO = {
    code?: number;
    data?: UserHistoryPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseUserPageVO = {
    code?: number;
    data?: UserPageVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseUserStatisticsVO = {
    code?: number;
    data?: UserStatisticsVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseWorkflowOptionsVO = {
    code?: number;
    data?: WorkflowOptionsVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseWorkflowRunVO = {
    code?: number;
    data?: WorkflowRunVO;
    message?: string;
    requestId?: string;
  };

  type BaseResponseWorkflowSubmissionVO = {
    code?: number;
    data?: WorkflowSubmissionVO;
    message?: string;
    requestId?: string;
  };

  type BrowserSessionVO = {
    csrfToken?: string;
    account?: AccountVO;
  };

  type CanvasAccessVO = {
    read: boolean;
    edit: boolean;
    manage: boolean;
  };

  type CanvasAddRequest = {
    commandId: string;
    title: string;
  };

  type CanvasAgentConfigurationDTO = {
    profile: string;
    goal: string;
    maxRounds: number;
    maxTools: number;
    parallelism: number;
  };

  type CanvasBindingDTO = {
    kind: string;
    referenceKey?: string;
    selection?: CanvasOutputSelectionDTO;
  };

  type CanvasCheckpointRequest = {
    commandId: string;
    canvasId: number | string;
    expectedRevision: number | string;
    expectedProjectBindingRevision: number | string;
    expectedEpoch: string;
    expectedCommitSeq: string;
  };

  type CanvasCommandResultVO = {
    resultVersion: number;
    commandId: string;
    operation: string;
    canvasId: number | string;
    revision: number | string;
    projectBindingRevision: number | string;
    graphHash: string;
    snapshotId?: number | string;
    sourceRevision?: number | string;
    deleted?: boolean;
    epoch?: string;
    commitSeq?: string;
    effect?: string;
    updateId?: string;
    payloadHash?: string;
  };

  type CanvasCommandStatusVO = {
    commandId: string;
    operation: string;
    status: string;
    result: CanvasCommandResultVO;
    stage?: string;
    failureCode?: string;
  };

  type CanvasConnectionDTO = {
    id: string;
    fromNodeId: string;
    toNodeId: string;
  };

  type CanvasDeleteRequest = {
    id: number | string;
    commandId: string;
    expectedRevision: number | string;
    expectedProjectBindingRevision: number | string;
    expectedEpoch: string;
    expectedCommitSeq: string;
  };

  type CanvasDocumentConnectionDTO = {
    id: string;
    fromNodeId: string;
    fromPortId: string;
    toNodeId: string;
    toPortId: string;
    purpose: string;
    order: number;
    binding: CanvasBindingDTO;
  };

  type CanvasDocumentGraphDTO = {
    schemaVersion: number;
    nodes: CanvasDocumentNodeDTO[];
    connections: CanvasDocumentConnectionDTO[];
    backgroundMode?: string;
    showImageInfo?: boolean;
    viewport?: { x?: number; y?: number; zoom?: number };
  };

  type CanvasDocumentNodeDTO = {
    id: string;
    type: string;
    nodeTypeVersion: number;
    title: string;
    position: { x: number; y: number };
    width: number;
    height: number;
    parentId?: string;
    collapsed?: boolean;
    textDocument?: Record<string, unknown>;
    parameters?: CanvasNodeParametersDTO;
    fileReferences?: CanvasFileReferenceDTO[];
    zIndex: number;
  };

  type CanvasEditRequest = {
    commandId: string;
    canvasId: number | string;
    expectedRevision: number | string;
    expectedProjectBindingRevision: number | string;
    expectedEpoch: string;
    expectedCommitSeq: string;
    title: string;
    coverFileId?: string;
    coverFileVersion?: string;
  };

  type CanvasFileReferenceDTO = {
    referenceKey: string;
    fileId: string;
    version: string;
    purpose: string;
    publicAssetId?: number | string;
    publicRevision?: number | string;
  };

  type CanvasGenerateRequest = {
    commandId: string;
    canvasId: number | string;
    expectedRevision: number | string;
    expectedProjectBindingRevision: number | string;
    expectedEpoch: string;
    expectedCommitSeq: string;
    expectedGraphHash: string;
    quoteId?: number | string;
    nodeId: string;
  };

  type CanvasGenerationRefreshRequest = {
    canvasId: number | string;
    commandId: string;
  };

  type CanvasGenerationVO = {
    sourceId?: number | string;
    commandId?: string;
    nodeId?: string;
    canvasRevision?: number | string;
    projectBindingRevision?: number | string;
    inputHash?: string;
    current?: boolean;
    frozenNode?: CanvasDocumentNodeDTO;
    taskId?: number | string;
    state?: string;
    failureKind?: string;
    pending?: boolean;
    outputs?: CanvasFileReferenceDTO[];
    textOutput?: string;
    syncRevision?: number | string;
    taskSyncRevision?: number | string;
    failureDiagnostic?: GenerationFailureDiagnosticVO;
    billing?: GenerationBillingVO;
  };

  type CanvasGraphDTO = {
    nodes: CanvasNodeDTO[];
    connections: CanvasConnectionDTO[];
    chatSessions: Record<string, unknown>[];
    activeChatId?: string;
    backgroundMode: string;
    showImageInfo?: boolean;
    viewport: Record<string, unknown>;
  };

  type CanvasModelSelectionDTO = {
    connectionId: number | string;
    modelId: string;
  };

  type CanvasNodeDTO = {
    id: string;
    type: string;
    title: string;
    position: { x: number; y: number };
    width: number;
    height: number;
    metadata?: Record<string, unknown>;
  };

  type CanvasNodeParametersDTO = {
    mode?: string;
    agent?: CanvasAgentConfigurationDTO;
    modelSelection?: CanvasModelSelectionDTO;
    modelParameters?: Record<string, unknown>;
    modelParameterSchemaId?: string;
    values?: Record<string, unknown>;
  };

  type CanvasOutputSelectionDTO = {
    mode: string;
    index?: number;
    /** 已注册结构化结果的顶层字段；为空时读取原摘要 */
    field?: string;
  };

  type CanvasPageVO = {
    records: CanvasSummaryVO[];
    total: number | string;
    current: number | string;
    size: number | string;
  };

  type CanvasQueryRequest = {
    current?: number;
    pageSize?: number;
    keyword?: string;
    projectId?: number | string;
    sortField?: string;
    sortOrder?: string;
  };

  type CanvasRestoreRequest = {
    commandId: string;
    canvasId: number | string;
    expectedRevision: number | string;
    expectedProjectBindingRevision: number | string;
    expectedEpoch: string;
    expectedCommitSeq: string;
    snapshotId: number | string;
  };

  type CanvasSearchStatusRequest = {
    canvasId?: number | string;
    projectId?: number | string;
  };

  type CanvasSnapshotPageVO = {
    records: CanvasSnapshotSummaryVO[];
    total: number | string;
    current: number | string;
    size: number | string;
  };

  type CanvasSnapshotQueryRequest = {
    canvasId: number | string;
    current?: number;
    pageSize?: number;
  };

  type CanvasSnapshotScanCloseRequest = {
    cursor: string;
  };

  type CanvasSnapshotScanRequest = {
    canvasId?: number | string;
    pageSize?: number;
    includeTotal?: boolean;
    cursor?: string;
  };

  type CanvasSnapshotSliceVO = {
    records: CanvasSnapshotSummaryVO[];
    startCursor: string;
    nextCursor?: string;
    closeCursor: string;
    hasMore: boolean;
    total?: number | string;
    totalRelation: string;
    indexGeneration: string;
    expiresAt: string;
    size: number;
  };

  type CanvasSnapshotSummaryVO = {
    snapshotId: number | string;
    canvasId: number | string;
    sourceRevision: number | string;
    projectBindingRevision: number | string;
    title: string;
    graphHash: string;
    reason:
      | "INITIAL"
      | "AUTO_SAVE"
      | "BEFORE_CHANGE"
      | "CHECKPOINT"
      | "BEFORE_RESTORE"
      | "RESTORE_RESULT"
      | "ATTACH_BASELINE"
      | "MIGRATION_BASELINE";
    restoredFromSnapshotId?: number | string;
    createdAt: string;
  };

  type CanvasSnapshotVO = {
    snapshotId: number | string;
    canvasId: number | string;
    sourceRevision: number | string;
    projectBindingRevision: number | string;
    title: string;
    schemaVersion: number;
    graphHash: string;
    reason:
      | "INITIAL"
      | "AUTO_SAVE"
      | "BEFORE_CHANGE"
      | "CHECKPOINT"
      | "BEFORE_RESTORE"
      | "RESTORE_RESULT"
      | "ATTACH_BASELINE"
      | "MIGRATION_BASELINE";
    restoredFromSnapshotId?: number | string;
    createdAt: string;
    graph: CanvasDocumentGraphDTO;
  };

  type CanvasSummaryVO = {
    canvasId: number | string;
    title: string;
    coverFileId?: string;
    coverFileVersion?: string;
    projectId?: number | string;
    revision: number | string;
    projectBindingRevision: number | string;
    graphHash: string;
    createdAt: string;
    updatedAt: string;
    access: CanvasAccessVO;
  };

  type CanvasTemplateApplyRequest = {
    commandId: string;
    templateId: number | string;
    title: string;
  };

  type CanvasTemplatePageVO = {
    current: number | string;
    size: number | string;
    total: number | string;
    records: CanvasTemplateVO[];
  };

  type CanvasTemplateQueryRequest = {
    current?: number;
    pageSize?: number;
    keyword?: string;
  };

  type CanvasTemplateSaveRequest = {
    commandId: string;
    canvasId: number | string;
    expectedRevision: number | string;
    expectedProjectBindingRevision: number | string;
    expectedEpoch: string;
    expectedCommitSeq: string;
    title: string;
    groupId?: string;
  };

  type CanvasTemplateVO = {
    id: number | string;
    title: string;
    nodeCount: number;
    sourceCanvasId: number | string;
    sourceRevision: number | string;
    createdAt: string;
  };

  type CanvasVO = {
    canvasId: number | string;
    title: string;
    coverFileId?: string;
    coverFileVersion?: string;
    projectId?: number | string;
    revision: number | string;
    projectBindingRevision: number | string;
    epoch: string;
    commitSeq: string;
    schemaVersion: number;
    graph: CanvasDocumentGraphDTO;
    graphHash: string;
    createdAt: string;
    updatedAt: string;
    access: CanvasAccessVO;
  };

  type CaptchaChallengeRequest = {
    scene: "LOGIN" | "REGISTER";
  };

  type CaptchaChallengeVO = {
    challengeId?: string;
    backgroundImage?: string;
    templateImage?: string;
    backgroundWidth?: number;
    backgroundHeight?: number;
    templateWidth?: number;
    templateHeight?: number;
    expiresAt?: number | string;
  };

  type CaptchaTicketVO = {
    captchaToken?: string;
    expiresAt?: number | string;
  };

  type CaptchaVerifyRequest = {
    scene: "LOGIN" | "REGISTER";
    challengeId: string;
    position: number;
  };

  type CommonErrorResponse = {
    code?: number;
    data?: Record<string, unknown>;
    message?: string;
    requestId?: string;
  };

  type ConsumptionDay = {
    date?: string;
    points?: string;
  };

  type ConsumptionMember = {
    id?: string;
    points?: string;
    tasks?: number | string;
  };

  type ConsumptionModel = {
    id?: string;
    modelId?: string;
    modelName?: string;
    priceId?: string;
    points?: string;
    tasks?: number | string;
  };

  type CreditAccountVO = {
    id?: string;
    userId?: string;
    availablePoints?: string;
    reservedPoints?: string;
    consumedPoints?: string;
    version?: number | string;
  };

  type CreditActivityCoverageVO = {
    startDate?: string;
    endDate?: string;
  };

  type CreditActivityDayVO = {
    date?: string;
    points?: string;
  };

  type CreditActivityThresholdsVO = {
    low?: string;
    high?: string;
    strategyVersion?: string;
  };

  type CreditActivityVO = {
    state?: "NOT_ENABLED" | "NO_COVERAGE" | "PARTIAL" | "READY";
    startDate?: string;
    endDate?: string;
    timezone?: string;
    coverageRanges?: CreditActivityCoverageVO[];
    asOf?: string;
    revision?: string;
    days?: CreditActivityDayVO[];
    thresholds?: CreditActivityThresholdsVO;
  };

  type CreditAdminAccountPageVO = {
    records?: CreditAdminAccountVO[];
    total?: number | string;
    current?: number | string;
    pageSize?: number | string;
  };

  type CreditAdminAccountVO = {
    userId?: string;
    userName?: string;
    userAccount?: string;
    availablePoints?: string;
    reservedPoints?: string;
    consumedPoints?: string;
  };

  type CreditBatchPageVO = {
    records?: CreditGrantBatchVO[];
    total?: number | string;
    current?: number | string;
    pageSize?: number | string;
  };

  type CreditConsumptionVO = {
    asOf?: string;
    revision?: string;
    chargingEnabled?: boolean;
    startDate?: string;
    endDate?: string;
    points?: string;
    tasks?: number | string;
    unattributedEntries?: number | string;
    trend?: ConsumptionDay[];
    members?: ConsumptionMember[];
    memberTotal?: number | string;
    models?: ConsumptionModel[];
    modelTotal?: number | string;
    page?: number;
    pageSize?: number;
  };

  type CreditGrantBatchVO = {
    id?: string;
    operatorId?: string;
    commandId?: string;
    payloadHash?: string;
    sourceProjectId?: string;
    sourceProjectName?: string;
    reason?: string;
    note?: string;
    recipientCount?: number;
    succeededCount?: number;
    failedCount?: number;
    pendingCount?: number;
    totalPoints?: number | string;
    state?: string;
    expiresAt?: string;
    createTime?: string;
    items?: CreditGrantItemVO[];
  };

  type CreditGrantExecuteRequest = {
    batchId: number | string;
    payloadHash: string;
    retryFailed?: boolean;
  };

  type CreditGrantItemVO = {
    id?: string;
    userId?: string;
    userName?: string;
    userAccount?: string;
    points?: number | string;
    state?: string;
    failureReason?: string;
  };

  type CreditGrantPreviewRequest = {
    commandId: string;
    sourceProjectId?: number | string;
    reason: string;
    note?: string;
    items: CreditGrantRecipientRequest[];
  };

  type CreditGrantRecipientRequest = {
    userId: number | string;
    points: number;
  };

  type CreditLedgerPageVO = {
    records?: CreditLedgerVO[];
    total?: number | string;
    current?: number | string;
    pageSize?: number | string;
  };

  type CreditLedgerVO = {
    id?: string;
    entryType?: string;
    points?: string;
    availableAfter?: string;
    accountVersion?: number | string;
    batchId?: string;
    sourceProjectId?: string;
    sourceProjectName?: string;
    reason?: string;
    note?: string;
    createTime?: string;
    reservationId?: string;
    reservedAfter?: string;
    consumedAfter?: string;
    taskProjectId?: string;
    settlementReason?: string;
  };

  type CreditPriceContent = {
    connectionId: number | string;
    modelId: string;
    connectionRevision: number | string;
    providerModelId: string;
    catalogVersion: string;
    profileId: string;
    specification: Record<string, unknown>;
    currency: string;
    fxVersion: string;
    fxCnyPerCurrency: string;
    evidenceUrl: string;
    upperBoundEvidence: string;
    violationPolicy: string;
    disclosure: string;
    meters: Meter[];
    pricingRule?: ModelPricingRule;
  };

  type CreditPricePublishRequest = {
    commandId: string;
    expectedPriceId?: number | string;
    content: CreditPriceContent;
  };

  type CreditPriceRetireRequest = {
    priceId: number | string;
  };

  type CreditPriceVO = {
    id?: string;
    content?: CreditPriceContent;
    active?: boolean;
    createdAt?: string;
  };

  type CreditProjectVO = {
    id?: string;
    title?: string;
  };

  type CreditQueryRequest = {
    current?: number;
    pageSize?: number;
    sortField?: string;
    sortOrder?: string;
    keyword?: string;
    status?: string;
    kind?: string;
    days?: number;
    sourceProjectId?: number | string;
  };

  type CreditQuoteVO = {
    id?: string;
    priceVersion?: string;
    inputHash?: string;
    estimatedPoints?: string;
    maximumPoints?: string;
    disclosure?: string;
    expiresAt?: string;
    connectionRevision?: string;
    meterKeys?: string[];
  };

  type Day = {
    date?: string;
    total?: number | string;
  };

  type Defaults = {
    size?: string;
    resolution?: string;
    duration?: number;
    ratio?: string;
    quality?: string;
    n?: number;
  };

  type diagnoseParams = {
    subjectId: number | string;
    serviceName: string;
    actionKey: string;
  };

  type FileCanvasReadRequest = {
    canvasId: number | string;
    bindingRevision: number | string;
    snapshotId?: number | string;
    nodeId: string;
    referenceKey: string;
    fileId: string;
    version: string;
  };

  type FilePartVO = {
    number?: number;
    size?: number | string;
    etag?: string;
  };

  type FilePreviewRequest = {
    fileId: string;
    version: string;
    variant: string;
  };

  type FilePreviewVO = {
    status?: string;
    url?: string;
    expiresIn?: number;
    syncRevision?: number | string;
  };

  type FileReference = {
    fileId: string;
    version: string;
    publicAssetId?: number | string;
    publicRevision?: number | string;
  };

  type FileSignedUrlVO = {
    url?: string;
    headUrl?: string;
    expiresSeconds?: number;
  };

  type FileTransferCommand = {
    fileId: string;
    partNumber?: number;
  };

  type FileTransferInitRequest = {
    commandId: string;
    biz: string;
    fileName: string;
    size: number | string;
  };

  type FileTransferVO = {
    registrationId?: number | string;
    fileId?: string;
    version?: string;
    status?: string;
    fileName?: string;
    biz?: string;
    size?: number | string;
    partSize?: number | string;
    expireTime?: string;
    contentType?: string;
    mediaJson?: string;
    parts?: FilePartVO[];
    syncRevision?: number | string;
  };

  type FileUploadChoiceRequest = {
    fileName: string;
    size: number | string;
  };

  type FileUploadChoiceVO = {
    biz?: string;
    label?: string;
    confirmationRequired?: boolean;
  };

  type GenerationBillingVO = {
    quoteId?: string;
    reservationId?: string;
    state?: string;
    reservedPoints?: string;
    chargedPoints?: string;
  };

  type GenerationFailureDiagnosticVO = {
    /** 失败执行阶段 */
    stage?:
      | "input_validation"
      | "provider_submit"
      | "provider_query"
      | "provider_control"
      | "archive";
    /** 供应商 HTTP 状态 */
    httpStatus?: number;
    /** 白名单内的供应商错误代码 */
    providerCode?: string;
    /** 受控供应商请求编号 */
    providerRequestId?: string;
  };

  type GenerationOptionsVO = {
    parameters?: Parameter[];
    billingEnabled?: boolean;
  };

  type GenerationOutputVO = {
    index?: number;
    modality?: string;
    fileId?: string;
    fileVersion?: string;
    contentType?: string;
    fileSize?: number | string;
    mediaJson?: string;
    metadata?: Record<string, unknown>;
  };

  type GenerationReadRequest = {
    taskId: number | string;
    outputIndex?: number;
  };

  type GenerationRefreshRequest = {
    taskId: number | string;
  };

  type GenerationStatisticsVO = {
    sampledAt?: string;
    startDate?: string;
    days?: number;
    total?: number | string;
    succeeded?: number | string;
    failed?: number | string;
    cancelled?: number | string;
    successRate?: number;
    queued?: number | string;
    processing?: number | string;
    attention?: number | string;
    trend?: Day[];
    models?: Model[];
  };

  type GenerationSubmitRequest = {
    commandId: string;
    quoteId?: number | string;
    connectionId: number | string;
    modelId: string;
    modality: string;
    prompt: string;
    parameters: Record<string, unknown>;
    referenceImages?: ReferenceImage[];
    referenceMedia?: ReferenceMedia[];
  };

  type GenerationTaskVO = {
    id?: number | string;
    commandId?: string;
    modality?: string;
    modelName?: string;
    prompt?: string;
    parameters?: Record<string, unknown>;
    state?:
      | "PREPARING_FUNDS"
      | "QUEUED"
      | "SUBMITTING"
      | "SUBMISSION_UNKNOWN"
      | "WAITING"
      | "QUERYING"
      | "OUTPUT_READY"
      | "ARCHIVE_FAILED"
      | "DISPATCH_FAILED"
      | "FAILED"
      | "SUCCEEDED"
      | "CANCELLED";
    failureKind?: string;
    fileId?: string;
    fileVersion?: string;
    contentType?: string;
    fileSize?: number | string;
    mediaJson?: string;
    createTime?: string;
    updateTime?: string;
    textOutput?: string;
    outputs?: GenerationOutputVO[];
    providerResult?: Record<string, unknown>;
    syncRevision?: number | string;
    billing?: GenerationBillingVO;
    failureDiagnostic?: GenerationFailureDiagnosticVO;
  };

  type getAssetBatchDeleteCommandParams = {
    commandId: string;
  };

  type getAssetCommandParams = {
    commandId: string;
  };

  type getAssetLibraryCommandParams = {
    commandId: string;
  };

  type getAssetLibraryParams = {
    projectId: number | string;
  };

  type getAssetMoveCommandParams = {
    commandId: string;
  };

  type getAssetSearchStatusParams = {
    libraryScope?: string;
    projectId?: number | string;
  };

  type getAssetZipExportParams = {
    jobId?: string;
    commandId?: string;
  };

  type getAssetZipImportParams = {
    jobId?: string;
    commandId?: string;
    current?: number | string;
  };

  type getCanvasCommandParams = {
    commandId: string;
  };

  type getCanvasDocumentParams = {
    canvasId: number | string;
  };

  type getCanvasParams = {
    projectId: number | string;
  };

  type getCanvasSnapshotParams = {
    canvasId: number | string;
    snapshotId: number | string;
  };

  type getCreditConsumptionParams = {
    days?: number;
    page?: number;
    pageSize?: number;
  };

  type getCreditGrantCommandParams = {
    commandId: string;
  };

  type getCreditGrantParams = {
    id: number | string;
  };

  type getFileTransferParams = {
    fileId: string;
  };

  type getGenerationCommandParams = {
    commandId: string;
  };

  type getGenerationOptionsParams = {
    modality: string;
  };

  type getGenerationStatisticsParams = {
    days?: number;
  };

  type getGenerationTaskParams = {
    taskId: number | string;
  };

  type getLogTaskParams = {
    id: number | string;
  };

  type getModelConnectionCommandParams = {
    commandId: string;
  };

  type getModelConnectionParams = {
    id: number | string;
  };

  type getModelPricingContextParams = {
    connectionId: number | string;
  };

  type getMyAssetParams = {
    assetId: number | string;
  };

  type getMyCreditLedgerParams = {
    id: number | string;
  };

  type getNotificationVOByIdParams = {
    id: number | string;
  };

  type getPersonalProjectCommandParams = {
    commandId: string;
  };

  type getPersonalProjectCreationParams = {
    commandId: string;
  };

  type getPersonalProjectWorkspaceParams = {
    projectId: number | string;
  };

  type getProjectCanvasOperationParams = {
    commandId: string;
  };

  type getReceiptParams = {
    commandId: string;
  };

  type getTeamCommandParams = {
    commandId: string;
  };

  type getTeamInvitationParams = {
    invitationId: number | string;
  };

  type getTeamParams = {
    teamId: number | string;
  };

  type getUserStatisticsParams = {
    days?: number;
  };

  type getWorkflowCommandParams = {
    commandId: string;
  };

  type getWorkflowRunParams = {
    runId: number | string;
  };

  type Item = {
    ordinal?: number;
    fileName?: string;
    relativePath?: string;
    kind?: string;
    size?: number | string;
    status?: string;
    reason?: string;
    assetId?: number | string;
  };

  type listActiveModelCreditPricesParams = {
    connectionId: number | string;
    modelId: string;
    revision: number | string;
  };

  type listAssetLibrariesParams = {
    current?: number | string;
    pageSize?: number | string;
    keyword?: string;
    enabledOnly?: boolean;
  };

  type listCanvasGenerationsParams = {
    canvasId: number | string;
  };

  type listCreditProjectMembersParams = {
    projectId: number | string;
  };

  type listCreditProjectsParams = {
    keyword?: string;
  };

  type listFileTransfersParams = {
    beforeId?: number | string;
  };

  type listGenerationHistoryParams = {
    modality: string;
    beforeId?: number | string;
  };

  type listModelCreditPricesParams = {
    connectionId: number | string;
  };

  type listMyAssetsParams = {
    current?: number | string;
    pageSize?: number | string;
    keyword?: string;
    kind?: string;
    libraryScope?: string;
    projectId?: number | string;
    referenceOnly?: boolean;
    reviewStatus?: string;
  };

  type listTeamMembersParams = {
    teamId: number | string;
    current: number | string;
    pageSize: number | string;
  };

  type listWorkflowHistoryParams = {
    canvasId: number | string;
  };

  type localGetCanvasCommandParams = {
    commandId: string;
  };

  type localGetCanvasParams = {
    canvasId: number | string;
  };

  type localGetCanvasSnapshotParams = {
    canvasId: number | string;
    snapshotId: number | string;
  };

  type LogCenterQuery = {
    from: string;
    until: string;
    pageSize: number;
    cursor?: string;
    modality?: string;
    state?: string;
    taskId?: string;
    requestId?: string;
    service?: string;
    level?: string;
  };

  type LogEntryPageVO = {
    records?: LogEntryVO[];
    nextCursor?: string;
  };

  type LogEntryVO = {
    id?: string;
    timestamp?: string;
    service?: string;
    level?: string;
    event?: string;
    summary?: string;
    taskId?: string;
    requestId?: string;
    operationId?: string;
    attemptId?: string;
    commandId?: string;
    factId?: string;
  };

  type LogTaskPageVO = {
    records?: LogTaskVO[];
    nextCursor?: string;
  };

  type LogTaskVO = {
    id?: string;
    modality?: string;
    state?: string;
    modelName?: string;
    createdAt?: string;
    attemptId?: string;
    operationId?: string;
    commandId?: string;
    failureKind?: string;
    claimedAt?: string;
    dispatchedAt?: string;
    registeredAt?: string;
  };

  type Meter = {
    key: string;
    unitPrice: string;
    unitSize: string;
    minimumUnits: string;
    stepUnits: string;
    estimatedUnits: string;
    maximumUnits: string;
  };

  type MeteringCostReference = {
    condition?: string;
    billable?: string;
    unit?: string;
    unitSize?: string;
    unitPrice?: string;
  };

  type MeteringItem = {
    key?: string;
    label?: string;
    unit?: string;
    unitSize?: string;
    supplierUnitPrice?: string;
  };

  type MeteringProfile = {
    id?: string;
    providerId?: string;
    providerModelId?: string;
    modality?: string;
    currency?: string;
    basis?: string;
    description?: string;
    conditions?: Record<string, unknown>;
    meters?: MeteringItem[];
    references?: MeteringCostReference[];
    evidenceUrl?: string;
    verification?: string;
  };

  type Model = {
    modelId?: string;
    modelName?: string;
    modality?: string;
    total?: number | string;
    succeeded?: number | string;
    failed?: number | string;
  };

  type ModelBindingRequest = {
    id?: string;
    name: string;
    providerModelId: string;
    modality: string;
    executionEnabled: boolean;
    presetId: number | string;
    presetRevision: number | string;
  };

  type ModelBindingVO = {
    id?: string;
    name?: string;
    providerModelId?: string;
    modality?: string;
    executionEnabled?: boolean;
    presetId?: number | string;
    presetRevision?: number | string;
    configuration?: ModelPresetContent;
    capabilityVersion?: string;
  };

  type ModelConnectionDeleteRequest = {
    commandId: string;
    id: number | string;
    expectedRevision: number | string;
  };

  type ModelConnectionQueryRequest = {
    current?: number;
    pageSize?: number;
    sortField?: string;
    sortOrder?: string;
    keyword?: string;
    modality?: string;
    enabled?: boolean;
  };

  type ModelConnectionSaveRequest = {
    commandId: string;
    id?: number | string;
    expectedRevision: number | string;
    presetId: string;
    name: string;
    baseUrl?: string;
    enabled: boolean;
    apiKey?: string;
    models: ModelBindingRequest[];
  };

  type ModelConnectionVO = {
    id?: number | string;
    revision?: number | string;
    presetId?: string;
    presetVersion?: number;
    name?: string;
    baseUrl?: string;
    enabled?: boolean;
    deleted?: boolean;
    hasCredential?: boolean;
    credentialVersion?: number | string;
    executionAvailable?: boolean;
    verification?: string;
    models?: ModelBindingVO[];
    updateTime?: string;
    verificationScopeId?: number | string;
    verifiedAt?: string;
    verificationFailure?: string;
  };

  type ModelExecutionVO = {
    connectionId?: number | string;
    modelId?: string;
    channelName?: string;
    modelName?: string;
    modality?: string;
    imageSizes?: string[];
    providerModelId?: string;
    nativeCapabilities?: ModelNativeCapabilitiesVO;
  };

  type ModelManagedPresetVO = {
    id?: number | string;
    providerId?: string;
    revision?: number | string;
    builtIn?: boolean;
    deleted?: boolean;
    content?: ModelPresetContent;
    supportedModels?: ModelPresetBindingVO[];
  };

  type ModelMeteringCatalogVO = {
    version?: string;
    verifiedAt?: string;
    profiles?: MeteringProfile[];
  };

  type ModelNativeCapabilitiesVO = {
    version?: string;
    operations?: string[];
    inputModalities?: string[];
    outputFormats?: string[];
    resolutions?: string[];
    ratios?: string[];
    minDuration?: number;
    maxDuration?: number;
    maxImages?: number;
    maxVideos?: number;
    maxAudios?: number;
    audioOnlyReference?: boolean;
    protocols?: string[];
    documentationUrl?: string;
  };

  type ModelPresetBindingVO = {
    name?: string;
    providerModelId?: string;
    modality?: string;
    imageSizes?: string[];
    presetId?: number | string;
    presetRevision?: number | string;
  };

  type ModelPresetCommand = {
    commandId: string;
    id?: number | string;
    expectedRevision: number | string;
    action: "SAVE" | "DELETE";
    providerId: string;
    content?: ModelPresetContent;
    reason: string;
  };

  type ModelPresetContent = {
    name: string;
    description?: string;
    providerModelId: string;
    modality: string;
    allowedSizes: string[];
    defaults: Defaults;
    sort?: number;
  };

  type ModelPresetVO = {
    id?: string;
    version?: number;
    name?: string;
    baseUrl?: string;
    configurationAvailable?: boolean;
    executionAvailable?: boolean;
    modalities?: string[];
    status?: string;
    models?: ModelPresetBindingVO[];
    supportedModels?: ModelPresetBindingVO[];
  };

  type ModelPricePresetVO = {
    id?: string;
    name?: string;
    providerId?: string;
    providerModelId?: string;
    catalogVersion?: string;
    profileId?: string;
    specification?: Record<string, unknown>;
    currency?: string;
    fxVersion?: string;
    fxCnyPerCurrency?: string;
    meters?: Meter[];
    evidenceUrl?: string;
    upperBoundEvidence?: string;
    violationPolicy?: string;
    disclosure?: string;
    ready?: boolean;
    estimatedPoints?: string;
    maximumPoints?: string;
  };

  type ModelStatisticsVO = {
    sampledAt?: string;
    total?: number | string;
    enabled?: number | string;
    disabled?: number | string;
  };

  type ModelVerifyRequest = {
    connectionId: number | string;
    expectedRevision: number | string;
    commandId: string;
  };

  type NotificationPageVO = {
    items?: NotificationVO[];
    nextCursor?: string;
    maxInboxSeq?: number | string;
    mailboxVersion?: number | string;
    dataEpoch?: string;
    unreadCount?: number | string;
  };

  type NotificationQueryRequest = {
    current?: number;
    pageSize?: number;
    sortField?: string;
    sortOrder?: string;
    cursor?: string;
    category?: string;
    unreadOnly?: boolean;
  };

  type NotificationReadAllRequest = {
    throughSeq: number | string;
  };

  type NotificationReadRequest = {
    id: number | string;
  };

  type NotificationSummaryVO = {
    unreadCount?: number | string;
    categoryCounts?: Record<string, unknown>;
    maxInboxSeq?: number | string;
    mailboxVersion?: number | string;
    dataEpoch?: string;
  };

  type NotificationVO = {
    id?: number | string;
    inboxSeq?: number | string;
    category?: string;
    title?: string;
    body?: string;
    read?: boolean;
    occurredAt?: string;
    projectionState?: string;
    actionType?: string;
    subjectId?: number | string;
    teamId?: number | string;
  };

  type operationHistoryParams = {
    current?: number | string;
    pageSize?: number | string;
  };

  type OrderItem = {
    column?: string;
    asc?: boolean;
  };

  type PageModelConnectionVO = {
    records?: ModelConnectionVO[];
    total?: number | string;
    size?: number | string;
    current?: number | string;
    orders?: OrderItem[];
    optimizeCountSql?: PageModelConnectionVO;
    searchCount?: PageModelConnectionVO;
    optimizeJoinOfCountSql?: boolean;
    maxLimit?: number | string;
    countId?: string;
    pages?: number | string;
  };

  type PageProjectVO = {
    records: ProjectVO[];
    total: number | string;
    size: number | string;
    current: number | string;
    orders?: OrderItem[];
    optimizeCountSql?: PageProjectVO;
    searchCount?: PageProjectVO;
    optimizeJoinOfCountSql?: boolean;
    maxLimit?: number | string;
    countId?: string;
    pages: number | string;
  };

  type PageRequest = {
    current?: number;
    pageSize?: number;
    sortField?: string;
    sortOrder?: string;
  };

  type Parameter = {
    key?: string;
    label?: string;
    values?: string[];
    defaultValue?: string;
    numeric?: boolean;
  };

  type Part = {
    jobId?: string;
    status?: string;
    archiveName?: string;
    size?: number | string;
    url?: string;
    expiresAt?: string;
  };

  type PlatformAuthorizationVO = {
    allowed?: boolean;
    reason?: string;
    deniedPermissions?: string[];
    revision?: number | string;
  };

  type ProjectAddRequest = {
    commandId: string;
    title: string;
  };

  type ProjectBreadcrumbVO = {
    projectId?: number | string;
    title?: string;
  };

  type ProjectCanvasAttachRequest = {
    commandId: string;
    projectId: number | string;
    sourceCanvasId: number | string;
    expectedRevision: number | string;
    expectedProjectBindingRevision: number | string;
  };

  type ProjectCanvasCreateRequest = {
    commandId: string;
    projectId: number | string;
    title: string;
  };

  type ProjectCanvasOperationStatusVO = {
    commandId: string;
    projectId: number | string;
    purpose:
      "PROJECT_CREATE" | "METADATA_CREATE" | "CANVAS_CREATE" | "CANVAS_ATTACH";
    status: "PROCESSING" | "COMPLETED" | "ABORTED";
    stage:
      | "CREATING"
      | "PREPARED"
      | "CANVAS_READY"
      | "ATTACHING"
      | "COMPLETED"
      | "ABORTED";
    failureCode?: string;
    result?: ProjectCreationResultVO;
  };

  type ProjectCreateRequest = {
    commandId: string;
    title: string;
    sourceCanvasId?: number | string;
    expectedRevision?: number | string;
    expectedProjectBindingRevision?: number | string;
    createMode?: string;
    parentProjectId?: number | string;
  };

  type ProjectCreationResultVO = {
    projectId: number | string;
    title: string;
    revision: number | string;
    workspaceState: "INITIALIZING" | "READY";
    canvasId?: number | string;
    createdAt: string;
    updatedAt: string;
  };

  type ProjectCreationStatusVO = {
    commandId: string;
    projectId: number | string;
    status: "PROCESSING" | "COMPLETED" | "ABORTED";
    stage:
      | "CREATING"
      | "PREPARED"
      | "CANVAS_READY"
      | "ATTACHING"
      | "COMPLETED"
      | "ABORTED";
    failureCode?: string;
    result?: ProjectCreationResultVO;
  };

  type ProjectCreditDirectoryVO = {
    id?: string;
    title?: string;
    members?: ProjectCreditMemberVO[];
  };

  type ProjectCreditMemberVO = {
    userId?: string;
    role?: string;
    userName?: string;
    userAccount?: string;
    membershipVersion?: number | string;
  };

  type ProjectDeleteRequest = {
    id: number | string;
    commandId: string;
    expectedRevision: number | string;
  };

  type ProjectEditRequest = {
    commandId: string;
    projectId: number | string;
    expectedRevision: number | string;
    title: string;
  };

  type ProjectGraphCommandRequest = {
    commandId: string;
    projectId: number | string;
    expectedRevision: number | string;
    graph: CanvasGraphDTO;
  };

  type ProjectQueryRequest = {
    current?: number;
    pageSize?: number;
    sortField?: string;
    sortOrder?: string;
    keyword?: string;
    parentProjectId?: number | string;
  };

  type ProjectVO = {
    id: number | string;
    title: string;
    revision: number | string;
    graph: CanvasGraphDTO;
    createdAt: string;
    updatedAt: string;
    deleted: boolean;
  };

  type ProjectWorkspacePageVO = {
    records: ProjectWorkspaceVO[];
    total: number | string;
    current: number | string;
    size: number | string;
  };

  type ProjectWorkspaceVO = {
    projectId: number | string;
    title: string;
    revision: number | string;
    workspaceState: "INITIALIZING" | "READY";
    createdAt: string;
    updatedAt: string;
    parentProjectId?: number | string;
    rootProjectId?: number | string;
    childrenRevision?: number | string;
    breadcrumbs?: ProjectBreadcrumbVO[];
    canCreateChild?: boolean;
    coverFileId?: string;
    coverFileVersion?: string;
  };

  type PublicAssetPageVO = {
    records?: PublicAssetVO[];
    total?: number | string;
    current?: number | string;
    size?: number | string;
  };

  type PublicAssetPublishRequest = {
    commandId: string;
    assetId: number | string;
    expectedRevision: number | string;
    licenseText: string;
  };

  type PublicAssetStateRequest = {
    commandId: string;
    publicAssetId: number | string;
    expectedRevision: number | string;
    state: string;
  };

  type PublicAssetVO = {
    id?: number | string;
    revision?: number | string;
    title?: string;
    kind?: string;
    sourceType?: string;
    reviewBasis?: string;
    state?: string;
    licenseText?: string;
    downloadAllowed?: boolean;
    redistributionAllowed?: boolean;
    updateTime?: string;
    fileId?: string;
    fileVersion?: string;
    content?: string;
  };

  type ReferenceImage = {
    fileId: string;
    version: string;
  };

  type ReferenceMedia = {
    fileId: string;
    version: string;
    mediaType: string;
    role: string;
    publicAssetId?: number | string;
    publicRevision?: number | string;
  };

  type SearchScopeVO = {
    resourceType?: string;
    ownerId?: string;
    scopeRevision?: string;
    permissionRevision?: string;
    emptyScope?: boolean;
    scopeKey?: string;
  };

  type SearchVisibilityVO = {
    state?: string;
    scope?: SearchScopeVO;
    generation?: string;
    appliedRevision?: string;
    visibleRevision?: string;
  };

  type TeamCommandRequest = {
    commandId: string;
    operation:
      | "CREATE"
      | "RENAME"
      | "INVITE"
      | "ACCEPT"
      | "DECLINE"
      | "REVOKE"
      | "ROLE"
      | "REMOVE"
      | "LEAVE"
      | "REQUEST_TRANSFER"
      | "ACCEPT_TRANSFER"
      | "CANCEL_TRANSFER";
    teamId?: number | string;
    revision?: number | string;
    memberUserId?: number | string;
    name?: string;
    role?: "ADMIN" | "EDITOR" | "VIEWER";
    invitationId?: number | string;
    invitationRevision?: number | string;
  };

  type TeamCommandResultVO = {
    teamId?: number | string;
    revision?: number | string;
    commandId?: string;
  };

  type TeamInvitationVO = {
    invitationId?: number | string;
    teamId?: number | string;
    inviteeUserId?: number | string;
    inviterUserId?: number | string;
    role?: string;
    state?: string;
    invitationRevision?: number | string;
    teamRevision?: number | string;
    teamName?: string;
    inviterName?: string;
  };

  type TeamMemberPageVO = {
    records?: TeamMemberVO[];
    total?: number | string;
    current?: number | string;
    pageSize?: number | string;
    revision?: number | string;
  };

  type TeamMemberVO = {
    userId?: number | string;
    userName?: string;
    role?: string;
    state?: string;
    membershipVersion?: number | string;
    currentInvitationId?: number | string;
    invitationRevision?: number | string;
  };

  type TeamPageVO = {
    records?: TeamVO[];
    total?: number | string;
    current?: number | string;
    pageSize?: number | string;
  };

  type TeamQueryRequest = {
    current: number | string;
    pageSize: number | string;
    invitations: boolean;
    search?: string;
  };

  type TeamVO = {
    id?: number | string;
    name?: string;
    ownerUserId?: number | string;
    pendingOwnerId?: number | string;
    revision?: number | string;
    role?: string;
    membershipState?: string;
    membershipVersion?: number | string;
    currentInvitationId?: number | string;
    invitationRevision?: number | string;
  };

  type TextEntry = {
    fileName: string;
    content: string;
  };

  type UserAddRequest = {
    commandId?: string;
    userAccount: string;
    userPassword: string;
    userName?: string;
    userAvatar?: string;
    userProfile?: string;
    userRole?: string;
    userEmail?: string;
    userPhone?: string;
  };

  type UserAdminVO = {
    revision?: number | string;
    id?: number | string;
    userAccount?: string;
    userName?: string;
    userAvatar?: string;
    userProfile?: string;
    userRole?: string;
    userEmail?: string;
    userPhone?: string;
    createdAt?: string;
    updatedAt?: string;
    lastLoginTime?: string;
    lastLoginRegion?: string;
  };

  type UserBatchItemRequest = {
    id: number | string;
    revision: number | string;
    commandId: string;
  };

  type UserBatchItemVO = {
    id?: number | string;
    code?: number;
    message?: string;
  };

  type UserBatchRequest = {
    operation: string;
    items: UserBatchItemRequest[];
  };

  type UserBatchVO = {
    items?: UserBatchItemVO[];
  };

  type UserDeleteRequest = {
    id: number | string;
    revision: number | string;
    commandId?: string;
  };

  type userDetailParams = {
    id: number | string;
  };

  type UserEditRequest = {
    revision: number | string;
    commandId?: string;
    userName?: string;
    userAvatar?: string;
    userCoverFileId?: string;
    userProfile?: string;
    userEmail?: string;
    userPhone?: string;
  };

  type UserHistoryPageVO = {
    records?: UserHistoryVO[];
    total?: number | string;
    current?: number | string;
    size?: number | string;
  };

  type UserHistoryVO = {
    id?: number | string;
    action?: string;
    result?: string;
    requestId?: string;
    createdAt?: string;
  };

  type UserLoginRequest = {
    captchaToken?: string;
    userAccount: string;
    userPassword: string;
  };

  type UserPageVO = {
    records?: UserAdminVO[];
    total?: number | string;
    current?: number | string;
    size?: number | string;
  };

  type UserQueryRequest = {
    current?: number;
    pageSize?: number;
    sortField?: string;
    sortOrder?: string;
    id?: number | string;
    notId?: number | string;
    userAccount?: string;
    userName?: string;
    userRole?: string;
    userEmail?: string;
    userPhone?: string;
    searchText?: string;
  };

  type UserRegisterRequest = {
    captchaToken?: string;
    commandId?: string;
    userAccount: string;
    userPassword: string;
    checkPassword: string;
  };

  type UserStatisticsVO = {
    sampledAt?: string;
    startDate?: string;
    days?: number;
    total?: number | string;
    administrators?: number | string;
    creators?: number | string;
    banned?: number | string;
    loginUsers?: number | string;
    trend?: Day[];
  };

  type UserUpdatePasswordRequest = {
    oldPassword: string;
    newPassword: string;
    checkPassword: string;
  };

  type UserUpdateRequest = {
    revision: number | string;
    commandId?: string;
    id: number | string;
    userName?: string;
    userAvatar?: string;
    userProfile?: string;
    userRole?: string;
    userEmail?: string;
    userPhone?: string;
  };

  type WorkflowAgentOutputFieldVO = {
    key?: string;
    label?: string;
    format?: string;
  };

  type WorkflowAgentProfileVO = {
    id?: string;
    label?: string;
    description?: string;
    fields?: WorkflowAgentOutputFieldVO[];
  };

  type WorkflowCommandCloseRequest = {
    commandId: string;
  };

  type WorkflowOptionsVO = {
    enabled?: boolean;
    acceptNewRuns?: boolean;
    generationEnabled?: boolean;
    maxNodes?: number;
    maxEdges?: number;
    maxConcurrency?: number;
    nodeTypes?: string[];
    agentProfiles?: WorkflowAgentProfileVO[];
  };

  type WorkflowRunFileVO = {
    fileId?: string;
    version?: string;
    mediaType?: string;
    purpose?: string;
    sha256?: string;
  };

  type WorkflowRunNodeVO = {
    nodeRunId?: string;
    canvasNodeId?: string;
    nodeType?: string;
    executionKind?: string;
    state?: string;
    taskId?: string;
    failureKind?: string;
    textOutput?: string;
    inputHash?: string;
    outputs?: WorkflowRunFileVO[];
    title?: string;
    structuredOutput?: Record<string, unknown>;
  };

  type WorkflowRunVO = {
    runId?: string;
    commandId?: string;
    canvasId?: string;
    graphRevision?: number | string;
    lifecycle?: string;
    resolution?: string;
    runSeq?: number | string;
    controlVersion?: number | string;
    stopRequested?: boolean;
    gateApplied?: boolean;
    nodes?: WorkflowRunNodeVO[];
    planHash?: string;
  };

  type WorkflowStopRequest = {
    runId: number | string;
    commandId: string;
  };

  type WorkflowSubmissionVO = {
    commandId?: string;
    state?: string;
    run?: WorkflowRunVO;
  };

  type WorkflowSubmitRequest = {
    commandId: string;
    canvasId: number | string;
    expectedRevision: number | string;
    expectedProjectBindingRevision: number | string;
    nodeIds: string[];
    expectedEpoch: string;
    expectedCommitSeq: string;
    expectedGraphHash: string;
  };

  type WorkspaceDeleteRequest = {
    commandId: string;
    projectId: number | string;
    expectedRevision: number | string;
  };

  type WorkspaceEditRequest = {
    commandId: string;
    projectId: number | string;
    expectedRevision: number | string;
    title: string;
    coverFileId?: string;
    coverFileVersion?: string;
  };
}
