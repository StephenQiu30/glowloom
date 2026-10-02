declare namespace API {
  type adoptScriptVersionParams = {
    /** 项目UUID */
    pid: string;
    /** 不可变剧本版本UUID */
    vid: string;
  };

  type applyCanvasCommandsParams = {
    /** 画布UUID */
    id: string;
  };

  type archiveProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type cancelMediaDepthParams = {
    /** Depth UUID */
    job_id: string;
  };

  type cancelMediaExportParams = {
    /** Export UUID */
    job_id: string;
  };

  type cancelMediaTranscriptionParams = {
    /** Transcription UUID */
    job_id: string;
  };

  type cancelOperationBatchParams = {
    /** Batch UUID */
    id: string;
  };

  type cancelOperationParams = {
    /** Operation UUID */
    id: string;
  };

  type cancelProjectCopyParams = {
    /** 复制任务UUID */
    id: string;
  };

  type cancelScriptFileImportParams = {
    /** Project UUID */
    pid: string;
    /** Import UUID */
    id: string;
  };

  type cancelScriptSourceWriteParams = {
    /** 项目UUID */
    pid: string;
    /** 保存意图UUID */
    wid: string;
  };

  type confirmBatchQuoteParams = {
    /** Batch UUID */
    id: string;
  };

  type confirmOperationQuoteParams = {
    /** Operation UUID */
    id: string;
  };

  type confirmScriptEpisodeSplitParams = {
    /** 项目UUID */
    pid: string;
  };

  type confirmScriptEpisodeStructureParams = {
    /** 正式分集UUID */
    eid: string;
  };

  type createCanvasParams = {
    /** 项目UUID */
    pid: string;
  };

  type createFreeQuoteParams = {
    /** 项目UUID */
    pid: string;
  };

  type createGenerationQuotesParams = {
    /** 项目UUID */
    pid: string;
  };

  type createMediaDepthParams = {
    /** Project UUID */
    pid: string;
  };

  type createMediaExportParams = {
    /** Project UUID */
    pid: string;
  };

  type createMediaTranscriptionParams = {
    /** Project UUID */
    pid: string;
  };

  type createProjectCopyParams = {
    /** 源项目UUID */
    pid: string;
  };

  type createScriptFileImportParams = {
    /** Project UUID */
    pid: string;
  };

  type createScriptSourceParams = {
    /** 项目UUID */
    pid: string;
  };

  type deleteCanvasParams = {
    /** 画布UUID */
    id: string;
  };

  type deleteProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type deleteScriptSourceParams = {
    /** 项目UUID */
    pid: string;
    /** 稳定来源UUID */
    lineage: string;
  };

  type disableAdminCredentialParams = {
    /** 渠道UUID */
    id: string;
    /** 凭据UUID */
    credential_id: string;
  };

  type downloadDocumentAssetParams = {
    /** 项目UUID */
    pid: string;
    /** 文档素材UUID */
    asset_id: string;
  };

  type downloadLibraryMediaAssetParams = {
    /** 二进制素材条目UUID */
    item_id: string;
    /** personal/project；默认personal */
    scope?: string;
    /** project scope必要UUID */
    project_id?: string;
  };

  type downloadMediaDepthParams = {
    /** Depth UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type downloadMediaExportParams = {
    /** Export UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type downloadMediaExportSubtitlesParams = {
    /** Export UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type downloadMediaTranscriptionSubtitlesParams = {
    /** Transcription UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type getAdminModelParams = {
    /** 模型UUID */
    id: string;
  };

  type getAdminProviderParams = {
    /** 渠道UUID */
    id: string;
  };

  type getCanvasParams = {
    /** 画布UUID */
    id: string;
  };

  type getMediaDepthParams = {
    /** Depth UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type getMediaExportParams = {
    /** Export UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type getMediaLibraryItemParams = {
    /** 条目UUID */
    item_id: string;
    /** personal/project */
    scope?: string;
    /** project scope必要UUID */
    project_id?: string;
  };

  type getMediaPreviewParams = {
    /** 项目UUID */
    pid: string;
    /** 媒体UUID */
    asset_id: string;
  };

  type getMediaTranscriptionParams = {
    /** Transcription UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type getMediaTranscriptionResultParams = {
    /** Transcription UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type getOperationBatchParams = {
    /** Batch UUID */
    id: string;
    /** 项目UUID */
    project_id: string;
  };

  type getOperationParams = {
    /** Operation UUID */
    id: string;
    /** 项目UUID */
    project_id: string;
  };

  type getProjectCopyParams = {
    /** 复制任务UUID */
    id: string;
  };

  type getProjectModelDefaultsParams = {
    /** 项目UUID */
    pid: string;
  };

  type getProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type getScriptEpisodeSourceTextParams = {
    /** 正式分集UUID */
    eid: string;
    /** 包含起点，Unicode scalar */
    from: number;
    /** 不包含终点，最多65536scalar */
    to: number;
  };

  type getScriptEpisodeStructureParams = {
    /** 正式分集UUID */
    eid: string;
  };

  type getScriptEpisodeStructureVersionParams = {
    /** 正式分集UUID */
    eid: string;
    /** 结构版本号 */
    version: number;
  };

  type getScriptFileImportParams = {
    /** Project UUID */
    pid: string;
    /** Import UUID */
    id: string;
  };

  type getScriptSourceParams = {
    /** 项目UUID */
    pid: string;
    /** 稳定来源lineageUUID */
    lineage: string;
    /** 历史版本UUID */
    version_id?: string;
  };

  type getScriptSourceSnapshotParams = {
    /** 项目UUID */
    pid: string;
    /** 来源快照UUID */
    sid: string;
  };

  type getScriptSourceWriteParams = {
    /** 项目UUID */
    pid: string;
    /** 保存意图UUID */
    wid: string;
  };

  type getScriptSplitConfirmationParams = {
    /** 项目UUID */
    pid: string;
    /** 不可变剧本版本UUID */
    vid: string;
    /** 确认UUID */
    cid: string;
  };

  type getScriptVersionSourceTextParams = {
    /** 项目UUID */
    pid: string;
    /** 不可变剧本版本UUID */
    vid: string;
    /** 包含起点，Unicode scalar */
    from: number;
    /** 不含终点，最多65536scalar */
    to: number;
  };

  type getScriptWorkspaceParams = {
    /** 项目UUID */
    pid: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasApplicationCommandResult =
    {
      ok?: boolean;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasApplicationCommandsInput =
    {
      commands?: githubComStephenQiu30LanverseBackendInternalCanvasDomainCommand[];
      expected_revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasApplicationCreateInput =
    {
      name?: string;
      scope?: Record<string, any>;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasApplicationDeleteInput =
    {
      expected_revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasApplicationDeleteResult =
    {
      deleted?: boolean;
      id?: string;
      revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasApplicationRenameInput =
    {
      expected_revision?: number;
      name?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasApplicationResult = {
    edges?: githubComStephenQiu30LanverseBackendInternalCanvasDomainEdge[];
    id?: string;
    name?: string;
    nodes?: githubComStephenQiu30LanverseBackendInternalCanvasDomainNode[];
    project_id?: string;
    results?: githubComStephenQiu30LanverseBackendInternalCanvasApplicationCommandResult[];
    revision?: number;
    scope?: Record<string, any>;
    viewport?: githubComStephenQiu30LanverseBackendInternalCanvasDomainViewport;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainBatchReferenceColumn =
    {
      id?: string;
      label?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainBatchTableConfig =
    {
      concurrency?: number;
      global_prompt?: string;
      mode?: string;
      model_profile_id?: string | null;
      operation?: string;
      output_count?: number;
      params?: Record<string, any>;
      reference_columns?: githubComStephenQiu30LanverseBackendInternalCanvasDomainBatchReferenceColumn[];
      rows?: githubComStephenQiu30LanverseBackendInternalCanvasDomainBatchTableRow[];
      version?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainBatchTableRow = {
    enabled?: boolean;
    id?: string;
    input_node_ids?: (string | null)[];
    prompt?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainCommand = {
    config?: githubComStephenQiu30LanverseBackendInternalCanvasDomainNodeConfig;
    edges?: githubComStephenQiu30LanverseBackendInternalCanvasDomainEdge[];
    id?: string;
    ids?: string[];
    moves?: githubComStephenQiu30LanverseBackendInternalCanvasDomainMove[];
    names?: githubComStephenQiu30LanverseBackendInternalCanvasDomainName[];
    nodes?: githubComStephenQiu30LanverseBackendInternalCanvasDomainNode[];
    parents?: githubComStephenQiu30LanverseBackendInternalCanvasDomainParent[];
    sizes?: githubComStephenQiu30LanverseBackendInternalCanvasDomainSize[];
    type?: string;
    viewport?: githubComStephenQiu30LanverseBackendInternalCanvasDomainViewport;
    z_indices?: githubComStephenQiu30LanverseBackendInternalCanvasDomainZIndex[];
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorBoneFrame =
    {
      easing?: string;
      id?: string;
      rotation?: number[];
      time?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorBoneTrack =
    {
      bone?: string;
      keyframes?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorBoneFrame[];
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorCamera =
    {
      aperture?: number;
      far?: number;
      focal_length?: number;
      focus_distance?: number;
      follow_anchor?: number[];
      follow_object_id?: string;
      fov?: number;
      id?: string;
      keyframes?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorKeyframe[];
      look_at_mode?: string;
      look_at_object_id?: string;
      name?: string;
      near?: number;
      target?: number[];
      transform?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorTransform;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorConfig =
    {
      active_shot_id?: string;
      aspect_ratio?: string;
      background?: string;
      cameras?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorCamera[];
      cover?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorCover;
      environment_intensity?: number;
      grid_snap?: boolean;
      grid_visible?: boolean;
      ground?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorGround;
      id?: string;
      labels_visible?: boolean;
      lights?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorLight[];
      objects?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorObject[];
      panorama?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorPanorama;
      panorama_radius?: number;
      panorama_rotation?: number;
      shots?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorShot[];
      stage_transform?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorStageTransform;
      title?: string;
      version?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorCover = {
    asset_id?: string;
    shot_id?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorGround =
    {
      height?: number;
      opacity?: number;
      visible?: boolean;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorKeyframe =
    {
      easing?: string;
      id?: string;
      time?: number;
      transform?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorTransform;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorLight = {
    angle?: number;
    cast_shadow?: boolean;
    color?: string;
    id?: string;
    intensity?: number;
    name?: string;
    penumbra?: number;
    transform?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorTransform;
    type?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorMotionClip =
    {
      duration?: number;
      id?: string;
      loop?: boolean;
      name?: string;
      playback_rate?: number;
      source_animation?: string;
      start?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorObject =
    {
      active_motion_clip_id?: string;
      asset_id?: string;
      bone_overrides?: Record<string, any>;
      bone_tracks?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorBoneTrack[];
      builtin_actor?: string;
      cast_shadow?: boolean;
      color?: string;
      id?: string;
      keyframes?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorKeyframe[];
      kind?: string;
      motion_clips?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorMotionClip[];
      name?: string;
      pose?: string;
      primitive?: string;
      receive_shadow?: boolean;
      rig?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorRig;
      source_node_id?: string;
      transform?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorTransform;
      uniform_scale?: number;
      visible?: boolean;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorPanorama =
    {
      asset_id?: string;
      name?: string;
      rotation?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorRig = {
    animation_names?: string[];
    bone_map?: Record<string, any>;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorScreenshot =
    {
      asset_id?: string;
      created_at?: string;
      id?: string;
      name?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorShot = {
    camera_id?: string;
    camera_move?: string;
    duration?: number;
    fps?: number;
    id?: string;
    name?: string;
    prompt?: string;
    screenshots?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorScreenshot[];
    shot_size?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorStageTransform =
    {
      position?: number[];
      rotation?: number[];
      scale?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorTransform =
    {
      position?: number[];
      rotation?: number[];
      scale?: number[];
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainDocument = {
    edges?: githubComStephenQiu30LanverseBackendInternalCanvasDomainEdge[];
    id?: string;
    name?: string;
    nodes?: githubComStephenQiu30LanverseBackendInternalCanvasDomainNode[];
    project_id?: string;
    revision?: number;
    scope?: Record<string, any>;
    viewport?: githubComStephenQiu30LanverseBackendInternalCanvasDomainViewport;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainEdge = {
    binding?: Record<string, any>;
    edge_type?: string;
    id?: string;
    role?: string;
    source_node_id?: string;
    target_node_id?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainGenerationConfig =
    {
      capability?: string;
      inputs?: githubComStephenQiu30LanverseBackendInternalCanvasDomainGenerationReference[];
      mode?: string;
      model_profile_id?: string | null;
      output_count?: number;
      params?: Record<string, any>;
      prompt?: string;
      version?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainGenerationReference =
    {
      media_asset_id?: string;
      role?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainMove = {
    id?: string;
    x?: number;
    y?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainName = {
    id?: string;
    title?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainNode = {
    config?: githubComStephenQiu30LanverseBackendInternalCanvasDomainNodeConfig;
    height?: number;
    id?: string;
    last_operation_id?: string;
    node_action?: string;
    node_type?: string;
    parent_id?: string;
    ref_id?: string;
    ref_type?: string;
    title?: string;
    width?: number;
    x?: number;
    y?: number;
    z_index?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainNodeConfig = {
    batch_table?: githubComStephenQiu30LanverseBackendInternalCanvasDomainBatchTableConfig;
    collapsed?: boolean;
    director?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDirectorConfig;
    generation?: githubComStephenQiu30LanverseBackendInternalCanvasDomainGenerationConfig;
    text?: string;
    timeline?: githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineConfig;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainParent = {
    id?: string;
    parent_id?: string | null;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainSize = {
    height?: number;
    id?: string;
    width?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainSubtitleStyle = {
    color?: string;
    font_size?: number;
    position?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineClip = {
    asset_id?: string | null;
    crop?: githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineCrop | null;
    duration_ms?: number;
    fade_in_ms?: number;
    fade_out_ms?: number;
    id?: string;
    kind?: string;
    node_id?: string | null;
    source_duration_ms?: number | null;
    source_start_ms?: number;
    start_ms?: number;
    text?: string;
    title?: string;
    track_id?: string;
    volume?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineConfig =
    {
      aspect_ratio?: string;
      clips?: githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineClip[];
      fps?: number;
      snapping?: boolean;
      subtitle_style?: githubComStephenQiu30LanverseBackendInternalCanvasDomainSubtitleStyle;
      tracks?: githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineTrack[];
      version?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineCrop = {
    height?: number;
    width?: number;
    x?: number;
    y?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainTimelineTrack = {
    id?: string;
    kind?: string;
    label?: string;
    locked?: boolean;
    muted?: boolean;
    visible?: boolean;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainViewport = {
    x?: number;
    y?: number;
    zoom?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainZIndex = {
    id?: string;
    z_index?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationAdminModelDetail =
    {
      model?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedModel;
      prices?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationAdminPrice[];
      versions?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationAdminModelVersion[];
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationAdminModelVersion =
    {
      create_time?: string;
      expected_max_ms?: number;
      id?: string;
      limits?: Record<string, any>;
      moderation?: string;
      modes?: string[];
      param_schema?: Record<string, any>[];
      provider_model_id?: string;
      queue?: string;
      supports_callback?: boolean;
      supports_cancel?: boolean;
      supports_query?: boolean;
      version_no?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationAdminPrice =
    {
      create_time?: string;
      currency?: string;
      effective_from?: string;
      fx_rate_to_cny?: string | null;
      id?: string;
      rule?: Record<string, any>;
      unit?: string;
      version_no?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationCapabilitySummary =
    {
      id?: string;
      input_roles?: string[];
      key?: string;
      modes?: string[];
      output_type?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationChangedModelStatus =
    {
      id?: string;
      revision?: number;
      status?: githubComStephenQiu30LanverseBackendInternalCatalogDomainModelStatus;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedModel =
    {
      capability?: string;
      create_time?: string;
      current_version_id?: string;
      display_name?: string;
      id?: string;
      model_key?: string;
      provider_id?: string;
      revision?: number;
      status?: githubComStephenQiu30LanverseBackendInternalCatalogDomainModelStatus;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedProvider =
    {
      adapter_key?: string;
      concurrency_limit?: number;
      create_time?: string;
      id?: string;
      key?: string;
      name?: string;
      rate_limit_per_min?: number;
      region?: githubComStephenQiu30LanverseBackendInternalCatalogDomainRegion;
      revision?: number;
      status?: githubComStephenQiu30LanverseBackendInternalCatalogDomainProviderStatus;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationCredentialField =
    {
      name?: string;
      required?: boolean;
      type?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationCredentialTestAccepted =
    {
      accepted?: boolean;
      credential_id?: string;
      event_id?: string;
      provider_id?: string;
      test_id?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationProviderCredentialSummary =
    {
      create_time?: string;
      id?: string;
      label?: string;
      last4?: string;
      last_test_result?: githubComStephenQiu30LanverseBackendInternalCatalogDomainTestResult;
      last_tested_at?: string;
      provider_id?: string;
      status?: githubComStephenQiu30LanverseBackendInternalCatalogDomainCredentialStatus;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationProviderDetail =
    {
      credential?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationProviderCredentialSummary;
      credential_schema?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationCredentialField[];
      provider?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedProvider;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationProviderListItem =
    {
      credential?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationProviderCredentialSummary;
      model_count?: number;
      provider?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedProvider;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationPublishedModelVersion =
    {
      create_time?: string;
      id?: string;
      model_id?: string;
      revision?: number;
      version_no?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationPublishedPriceRule =
    {
      create_time?: string;
      id?: string;
      model_id?: string;
      revision?: number;
      version_no?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogApplicationSavedCredential =
    {
      create_time?: string;
      id?: string;
      label?: string;
      last4?: string;
      last_test_result?: githubComStephenQiu30LanverseBackendInternalCatalogDomainTestResult;
      provider_id?: string;
      status?: githubComStephenQiu30LanverseBackendInternalCatalogDomainCredentialStatus;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalCatalogDomainCredentialStatus =
    "active" | "disabled";

  type githubComStephenQiu30LanverseBackendInternalCatalogDomainModelStatus =
    "active" | "disabled";

  type githubComStephenQiu30LanverseBackendInternalCatalogDomainModeration =
    "provider" | "platform" | "both";

  type githubComStephenQiu30LanverseBackendInternalCatalogDomainPriceUnit =
    | "per_image"
    | "per_second"
    | "per_request"
    | "per_1k_tokens"
    | "per_1k_chars";

  type githubComStephenQiu30LanverseBackendInternalCatalogDomainProviderStatus =
    "active" | "disabled";

  type githubComStephenQiu30LanverseBackendInternalCatalogDomainRegion =
    "domestic" | "overseas";

  type githubComStephenQiu30LanverseBackendInternalCatalogDomainTestResult =
    "ok" | "auth_failed" | "unreachable" | "timeout" | "unsupported";

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetPage = {
    items?: githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetSummary[];
    next_cursor?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetSummary =
    {
      byte_size?: number;
      duration_ms?: number;
      file_name?: string;
      height?: number;
      id?: string;
      kind?: string;
      mime_type?: string;
      project_id?: string;
      revision?: number;
      width?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryAssetSummary =
    {
      byte_size?: number;
      duration_ms?: number | null;
      file_name?: string;
      height?: number | null;
      id?: string;
      kind?: string;
      mime_type?: string;
      revision?: number;
      width?: number | null;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryCommand =
    {
      action?: string;
      expected_folder_revision?: number;
      expected_item_revision?: number;
      expected_revision?: number;
      folder?: githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryFolderInput | null;
      folder_id?: string | null;
      item_id?: string | null;
      items?: githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryItemRevision[];
      metadata?: githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryMetadata | null;
      scope?: githubComStephenQiu30LanverseBackendInternalMediaDomainLibraryScope;
      target_folder_id?: string | null;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryFolderInput =
    {
      name?: string;
      parent_id?: string | null;
      style?: string;
      theme?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryItemChange =
    {
      asset_id?: string | null;
      catalog_state?: string;
      folder_id?: string | null;
      id?: string;
      kind?: string;
      revision?: number;
      trashed_at?: string | null;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryItemDetail =
    {
      asset_id?: string | null;
      catalog_state?: string;
      category?: string;
      created_at?: string;
      favorite?: boolean;
      folder_id?: string | null;
      id?: string;
      kind?: string;
      media?: githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryAssetSummary | null;
      note?: string;
      plain_text?: string | null;
      position?: number;
      revision?: number;
      source_label?: string;
      tags?: string[];
      title?: string;
      trashed_at?: string | null;
      updated_at?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryItemRevision =
    {
      id?: string;
      revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryItemSummary =
    {
      asset_id?: string | null;
      catalog_state?: string;
      category?: string;
      created_at?: string;
      favorite?: boolean;
      folder_id?: string | null;
      id?: string;
      kind?: string;
      media?: githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryAssetSummary | null;
      note?: string;
      position?: number;
      revision?: number;
      source_label?: string;
      tags?: string[];
      title?: string;
      trashed_at?: string | null;
      updated_at?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryMediaPreview =
    {
      asset?: githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryAssetSummary;
      expires_at?: string;
      renditions?: githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryRenditionPreview[];
      url?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryMetadata =
    {
      category?: string;
      favorite?: boolean;
      folder_id?: string | null;
      note?: string;
      plain_text?: string | null;
      source_label?: string;
      tags?: string[];
      title?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryPage =
    {
      category_counts?: Record<string, any>;
      current_actor_id?: string;
      current_org_id?: string;
      folder_counts?: Record<string, any>;
      folders?: githubComStephenQiu30LanverseBackendInternalMediaDomainLibraryFolder[];
      items?: githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryItemSummary[];
      library_id?: string;
      page?: number;
      page_size?: number;
      revision?: number;
      scope?: githubComStephenQiu30LanverseBackendInternalMediaDomainLibraryScope;
      total?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryReceipt =
    {
      current_actor_id?: string;
      current_org_id?: string;
      folder?: githubComStephenQiu30LanverseBackendInternalMediaDomainLibraryFolder | null;
      items?: githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryItemChange[];
      library_id?: string;
      project_revision?: number | null;
      revision?: number;
      scope?: githubComStephenQiu30LanverseBackendInternalMediaDomainLibraryScope;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryRenditionPreview =
    {
      expires_at?: string;
      height?: number | null;
      kind?: string;
      url?: string;
      width?: number | null;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationPersonalUploadResult =
    {
      asset?: githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryAssetSummary;
      duplicate_of?: string | null;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationPreview = {
    asset?: githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetSummary;
    expires_at?: string;
    url?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationUploadResult =
    {
      asset?: githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetSummary;
      duplicate_of?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalMediaDomainLibraryFolder = {
    created_at?: string;
    id?: string;
    library_id?: string;
    library_kind?: githubComStephenQiu30LanverseBackendInternalMediaDomainLibraryKind;
    name?: string;
    parent_id?: string | null;
    position?: number;
    revision?: number;
    style?: string;
    theme?: string;
    updated_at?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalMediaDomainLibraryKind =
    "project" | "personal";

  type githubComStephenQiu30LanverseBackendInternalMediaDomainLibraryScope = {
    kind?: githubComStephenQiu30LanverseBackendInternalMediaDomainLibraryKind;
    project_id?: string | null;
  };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationControlInput =
    {
      project_id?: string;
      revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationCreateInput =
    {
      canvas_id?: string;
      node_id?: string;
      output_kind?: "video" | "audio";
      revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationDepthCreateInput =
    {
      canvas_id?: string;
      node_id?: string;
      revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationDepthPage =
    {
      current_actor_id?: string;
      current_org_id?: string;
      items?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainDepthJob[];
      next_cursor?: string | null;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationDepthPreview =
    {
      asset?: githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetSummary;
      expires_at?: string;
      job_id?: string;
      revision?: number;
      sha256?: string;
      url?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationExportPage =
    {
      items?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainExportJob[];
      next_cursor?: string | null;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationExportPreview =
    {
      asset?: githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetSummary;
      expires_at?: string;
      job_id?: string;
      revision?: number;
      sha256?: string;
      url?: string;
      waveform?: githubComStephenQiu30LanverseBackendInternalMediatoolApplicationExportWaveformPreview;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationExportWaveformPreview =
    {
      expires_at?: string;
      height?: number;
      url?: string;
      width?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationReviewInput =
    {
      local_review_confirmed?: boolean;
      project_id?: string;
      revision?: number;
      sha256?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationTranscriptionCreateInput =
    {
      canvas_id?: string;
      language?: string;
      node_id?: string;
      revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationTranscriptionPage =
    {
      items?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainTranscriptionJob[];
      next_cursor?: string | null;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolApplicationTranscriptionResult =
    {
      draft?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainTranscript;
      job_id?: string;
      revision?: number;
      sha256?: string;
      source_asset_id?: string;
      source_asset_revision?: number;
      source_sha256?: string;
      srt?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainDepthJob = {
    asset_id?: string | null;
    attempt?: number;
    cancellation_requested?: boolean;
    created_at?: string;
    execution_unconfirmed?: boolean;
    failure_code?: string | null;
    id?: string;
    needs_reconciliation?: boolean;
    profile_id?: string;
    project_id?: string;
    reconciliation_requested?: boolean;
    retryable?: boolean;
    revision?: number;
    sha256?: string | null;
    source?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainSource;
    source_asset_id?: string;
    source_asset_revision?: number;
    source_sha256?: string;
    stage?: string;
    status?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainDepthStatus;
    updated_at?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainDepthStatus =
    | "queued"
    | "running"
    | "review_required"
    | "succeeded"
    | "failed"
    | "cancel_requested"
    | "cancelled";

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainExportJob = {
    asset_id?: string | null;
    attempt?: number;
    created_at?: string;
    failure_code?: string | null;
    id?: string;
    output_kind?: "video" | "audio";
    progress?: number;
    project_id?: string;
    revision?: number;
    sha256?: string | null;
    source?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainSource;
    stage?: string;
    status?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainStatus;
    updated_at?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainOutputKind =
    "video" | "audio";

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainSource = {
    canvas_id?: string;
    node_id?: string;
    revision?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainStatus =
    | "queued"
    | "running"
    | "review_required"
    | "succeeded"
    | "failed"
    | "cancel_requested"
    | "cancelled";

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainSubtitleSegment =
    {
      end_ms?: number;
      start_ms?: number;
      text?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainTranscript = {
    duration_ms?: number;
    language?: string;
    segments?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainSubtitleSegment[];
    version?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainTranscriptionJob =
    {
      attempt?: number;
      created_at?: string;
      failure_code?: string | null;
      id?: string;
      language?: string;
      progress?: number;
      project_id?: string;
      result_sha256?: string | null;
      revision?: number;
      source?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainTranscriptionSource;
      stage?: string;
      status?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainTranscriptionStatus;
      updated_at?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainTranscriptionSource =
    {
      canvas_id?: string;
      node_id?: string;
      revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalMediatoolDomainTranscriptionStatus =
    | "queued"
    | "running"
    | "succeeded"
    | "failed"
    | "cancel_requested"
    | "cancelled";

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationBatchDetail =
    {
      cancel_requested_at?: string | null;
      failed_count?: number;
      id?: string;
      items?: githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskSummary[];
      kind?: string;
      paused_reason?: string | null;
      project_id?: string;
      quote_total_micros?: number;
      status?: githubComStephenQiu30LanverseBackendInternalOperationDomainBatchStatus;
      succeeded_count?: number;
      total_count?: number;
      unknown_count?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationBatchFreeQuoteItemResult =
    {
      error_code?: string;
      final_prompt?: string;
      mode?: string;
      model_key?: string;
      operation_id?: string;
      prompt_preparation?: githubComStephenQiu30LanverseBackendInternalOperationDomainPromptPreparation;
      quote_detail?: number[];
      quote_micros?: number;
      region?: string;
      reused_from_id?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationCreateBatchFreeQuoteResult =
    {
      available_micros?: number;
      batch_id?: string;
      confirmable?: boolean;
      expires_at?: string;
      items?: githubComStephenQiu30LanverseBackendInternalOperationApplicationBatchFreeQuoteItemResult[];
      total_micros?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskDetail =
    {
      batch_id?: string | null;
      cancel_requested?: boolean;
      capability?: string;
      create_time?: string;
      events?: githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskEvent[];
      failure_code?: string | null;
      finished_at?: string | null;
      id?: string;
      inputs?: githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskInput[];
      mode?: string;
      model_key?: string;
      model_name?: string;
      origin?: string;
      output_count?: number;
      outputs?: githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskOutput[];
      params?: Record<string, any>;
      project_id?: string;
      prompt_preparation?: githubComStephenQiu30LanverseBackendInternalOperationDomainPromptPreparation;
      quote_detail?: Record<string, any>;
      quote_expires_at?: string | null;
      quote_micros?: number | null;
      retryable?: boolean | null;
      reused_from_id?: string | null;
      settled_micros?: number | null;
      source?: githubComStephenQiu30LanverseBackendInternalOperationDomainCanvasSource | null;
      started_at?: string | null;
      status?: githubComStephenQiu30LanverseBackendInternalOperationDomainStatus;
      target_id?: string | null;
      target_type?: string;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskEvent =
    {
      create_time?: string;
      from_status?: string | null;
      id?: string;
      reason?: string | null;
      to_status?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskInput =
    {
      mask_asset_id?: string | null;
      media_asset_id?: string | null;
      role?: string;
      sequence?: number;
      text?: string | null;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskOutput =
    {
      create_time?: string;
      id?: string;
      kind?: string;
      media_asset_id?: string | null;
      moderation_status?: string;
      sequence?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskSummary =
    {
      batch_id?: string | null;
      cancel_requested?: boolean;
      capability?: string;
      create_time?: string;
      failure_code?: string | null;
      finished_at?: string | null;
      id?: string;
      mode?: string;
      model_key?: string;
      model_name?: string;
      origin?: string;
      project_id?: string;
      quote_micros?: number | null;
      retryable?: boolean | null;
      settled_micros?: number | null;
      source?: githubComStephenQiu30LanverseBackendInternalOperationDomainCanvasSource | null;
      started_at?: string | null;
      status?: githubComStephenQiu30LanverseBackendInternalOperationDomainStatus;
      target_id?: string | null;
      target_type?: string;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationApplicationWorkflowControlResult =
    {
      accepted?: boolean;
      action?: string;
      event_id?: string;
      request_id?: string;
      target_id?: string;
      target_type?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationDomainBatchStatus =
    "quoted" | "confirmed" | "running" | "finished" | "cancelled" | "expired";

  type githubComStephenQiu30LanverseBackendInternalOperationDomainCanvasSource =
    {
      canvas_id?: string;
      node_id?: string;
      revision?: number;
      row_id?: string | null;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationDomainPromptPreparation =
    {
      content_sha256?: string;
      customization_id?: string;
      customization_revision?: number;
      operation?: string;
      policy?: string;
      request_sha256?: string;
      template_id?: string;
      template_version?: number;
      user_prompt_sha256?: string;
      version?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalOperationDomainStatus =
    | "draft"
    | "quoted"
    | "confirmed"
    | "submitting"
    | "submitted"
    | "succeeded"
    | "ingesting"
    | "completed"
    | "failed"
    | "unknown"
    | "reconciling"
    | "manual"
    | "cancelling"
    | "cancelled"
    | "expired";

  type githubComStephenQiu30LanverseBackendInternalPlatformHttpapiProblem = {
    code?: string;
    detail?: string;
    meta?: Record<string, any>;
    request_id?: string;
    status?: number;
    title?: string;
    type?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalPromptApplicationOutlineOptions =
    {
      chapter_count: "3" | "5" | "8" | "10";
      chapter_length: "短" | "中" | "长";
      character_scale: "2 个" | "3-4 个" | "5-6 个";
      perspective: "第三人称" | "第一人称" | "多视角";
      structure: "单线推进" | "双线并行" | "群像多线" | "反转嵌套";
      tone: "平稳叙事" | "轻松喜剧" | "紧张悬疑" | "热血成长" | "甜宠治愈";
      word_count: "500" | "800" | "1200" | "2000";
    };

  type githubComStephenQiu30LanverseBackendInternalPromptApplicationPreference =
    {
      customization?: githubComStephenQiu30LanverseBackendInternalPromptDomainCustomization;
      definition?: githubComStephenQiu30LanverseBackendInternalPromptDomainDefinition;
      outdated?: boolean;
    };

  type githubComStephenQiu30LanverseBackendInternalPromptApplicationTemplateRequest =
    {
      expected_customization_revision: number;
      expected_template_id: string;
      operation:
        | "chapter_assets_extract"
        | "character_extract"
        | "character_turnaround"
        | "storyboard_plan"
        | "storyboard_repair"
        | "storyboard_first_frame"
        | "storyboard_video"
        | "short_drama_outline"
        | "skill_draft";
      outline?: githubComStephenQiu30LanverseBackendInternalPromptApplicationOutlineOptions;
    };

  type githubComStephenQiu30LanverseBackendInternalPromptDomainCustomization = {
    base_template_id?: string;
    content?: string;
    id?: string;
    mode?: githubComStephenQiu30LanverseBackendInternalPromptDomainMode;
    operation?: string;
    revision?: number;
    update_time?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalPromptDomainDefinition = {
    category?: string;
    content?: string;
    description?: string;
    label?: string;
    operation?: string;
    output_contract?: string;
    output_type?: string;
    schema_key?: string;
    template_id?: string;
    template_version?: number;
    variables?: githubComStephenQiu30LanverseBackendInternalPromptDomainVariable[];
  };

  type githubComStephenQiu30LanverseBackendInternalPromptDomainMode =
    "inherit" | "append" | "rewrite";

  type githubComStephenQiu30LanverseBackendInternalPromptDomainVariable = {
    label?: string;
    placeholder?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationAdoptReceipt =
    {
      changed?: boolean;
      duplicate?: boolean;
      episode_mappings?: githubComStephenQiu30LanverseBackendInternalScriptApplicationEpisodeMapping[];
      previous_version_id?: string;
      project_revision?: number;
      script_revision?: number;
      split_set_id?: string;
      version_id?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationConfirmationPage =
    {
      items?: githubComStephenQiu30LanverseBackendInternalScriptApplicationConfirmationSummary[];
      next_revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationConfirmationSummary =
    {
      candidate_set_id?: string;
      created_at?: string;
      episode_count?: number;
      formal_set_id?: string;
      id?: string;
      revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationEpisodeMapping =
    {
      episode_id?: string;
      inherit_status?: string;
      previous_episode_id?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationEpisodeText =
    {
      content_hash?: string;
      episode_id?: string;
      script_version_id?: string;
      span_end?: number;
      span_start?: number;
      text?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationEpisodeView =
    {
      candidate?: githubComStephenQiu30LanverseBackendInternalScriptDomainSplitSet;
      episodes?: githubComStephenQiu30LanverseBackendInternalScriptDomainEpisode[];
      head?: githubComStephenQiu30LanverseBackendInternalScriptApplicationVersionHead;
      version_id?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationExtractionWarning =
    {
      code?: string;
      count?: number;
      paragraph?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationImportFile =
    {
      asset_id?: string;
      attempt?: number;
      failure_code?: string;
      file_name?: string;
      position?: number;
      source_id?: string;
      source_lineage_id?: string;
      status?: string;
      warnings?: githubComStephenQiu30LanverseBackendInternalScriptApplicationExtractionWarning[];
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationImportJob =
    {
      active_io?: boolean;
      attempt?: number;
      can_control?: boolean;
      cancellation_requested?: boolean;
      created_at?: string;
      expected_script_revision?: number;
      failure_code?: string;
      files?: githubComStephenQiu30LanverseBackendInternalScriptApplicationImportFile[];
      id?: string;
      latest_script_revision?: number;
      latest_version_id?: string;
      needs_reconciliation?: boolean;
      project_id?: string;
      reconciliation_requested?: boolean;
      retryable?: boolean;
      revision?: number;
      stage?: string;
      status?: string;
      updated_at?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationImportPage =
    {
      current_actor_id?: string;
      current_org_id?: string;
      items?: githubComStephenQiu30LanverseBackendInternalScriptApplicationImportJob[];
      next_after?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceChange =
    {
      new_source_id?: string;
      old_source_id?: string;
      source_lineage_id?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceControlReceipt =
    {
      accepted?: boolean;
      action?: string;
      intent_id?: string;
      revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceDetail =
    {
      char_count?: number;
      content_hash?: string;
      document?: githubComStephenQiu30LanverseBackendInternalScriptDomainRichDocument;
      id?: string;
      media_asset_id?: string;
      origin?: string;
      plain_text?: string;
      position?: number;
      previous_source_id?: string;
      provenance?: githubComStephenQiu30LanverseBackendInternalScriptDomainSourceProvenance;
      rich_sha256?: string;
      source_kind?: string;
      source_lineage_id?: string;
      source_revision?: number;
      source_span?: githubComStephenQiu30LanverseBackendInternalScriptDomainSourceSpan;
      status?: string;
      title?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceHistoryPage =
    {
      items?: githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceSummary[];
      next_revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationSourcePage =
    {
      items?: githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceSummary[];
      next_position?: number;
      version_id?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceReceipt =
    {
      changed?: boolean;
      duplicate?: boolean;
      project_revision?: number;
      script_revision?: number;
      source_mappings?: githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceChange[];
      split_set_id?: string;
      version_id?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceSnapshotDetail =
    {
      char_count?: number;
      content_hash?: string;
      document?: githubComStephenQiu30LanverseBackendInternalScriptDomainRichDocument;
      id?: string;
      media_asset_id?: string;
      origin?: string;
      original_html?: string;
      plain_text?: string;
      position?: number;
      previous_source_id?: string;
      provenance?: githubComStephenQiu30LanverseBackendInternalScriptDomainSourceProvenance;
      rich_sha256?: string;
      source_kind?: string;
      source_lineage_id?: string;
      source_revision?: number;
      status?: string;
      title?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceSummary =
    {
      char_count?: number;
      content_hash?: string;
      id?: string;
      media_asset_id?: string;
      origin?: string;
      position?: number;
      previous_source_id?: string;
      rich_sha256?: string;
      source_kind?: string;
      source_lineage_id?: string;
      source_revision?: number;
      status?: string;
      title?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceWriteIntent =
    {
      action?: string;
      active_io?: boolean;
      base_version_id?: string;
      can_control?: boolean;
      cancellation_requested?: boolean;
      confirmed_object_count?: number;
      created_at?: string;
      expected_script_revision?: number;
      id?: string;
      needs_reconciliation?: boolean;
      object_count?: number;
      revision?: number;
      status?: string;
      updated_at?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceWritePage =
    {
      current_actor_id?: string;
      current_org_id?: string;
      items?: githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceWriteIntent[];
      next_after?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationSplitReceipt =
    {
      confirmation_id?: string;
      episode_mappings?: githubComStephenQiu30LanverseBackendInternalScriptApplicationEpisodeMapping[];
      episodes?: githubComStephenQiu30LanverseBackendInternalScriptDomainEpisode[];
      project_revision?: number;
      renamed_episode_ids?: string[];
      script_revision?: number;
      split_revision?: number;
      split_set_id?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationStructurePage =
    {
      items?: githubComStephenQiu30LanverseBackendInternalScriptApplicationStructureSummary[];
      next_version_no?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationStructureReceipt =
    {
      episode_revision?: number;
      project_revision?: number;
      review_status?: string;
      script_revision?: number;
      structure_id?: string;
      version_no?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationStructureSummary =
    {
      created_at?: string;
      episode_id?: string;
      id?: string;
      source_hash?: string;
      version_no?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationVersionHead =
    {
      candidate_split_set_id?: string;
      confirmed_split_set_id?: string;
      split_revision?: number;
      version_id?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationVersionPage =
    {
      items?: githubComStephenQiu30LanverseBackendInternalScriptApplicationVersionSummary[];
      next_version_no?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationVersionSummary =
    {
      char_count?: number;
      content_hash?: string;
      created_at?: string;
      document_sha256?: string;
      id?: string;
      source_count?: number;
      source_manifest_sha256?: string;
      version_no?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationVersionText =
    {
      content_hash?: string;
      script_version_id?: string;
      span_end?: number;
      span_start?: number;
      text?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptApplicationWorkspaceView =
    {
      current_actor_id?: string;
      current_org_id?: string;
      state?: githubComStephenQiu30LanverseBackendInternalScriptDomainProjectState;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainEpisode = {
    confirmed_structure_id?: string;
    current_structure_id?: string;
    id?: string;
    inherit_status?: string;
    is_delete?: boolean;
    org_id?: string;
    previous_episode_id?: string;
    project_id?: string;
    revision?: number;
    script_version_id?: string;
    seq_no?: number;
    span_end?: number;
    span_start?: number;
    split_set_id?: string;
    title?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainEpisodeBoundary =
    {
      seq_no?: number;
      source_lineage_id?: string;
      span_end?: number;
      span_start?: number;
      title?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainEpisodeStructure =
    {
      actor_id?: string;
      created_at?: string;
      document?: githubComStephenQiu30LanverseBackendInternalScriptDomainStructureDocument;
      episode_id?: string;
      id?: string;
      org_id?: string;
      project_id?: string;
      source_hash?: string;
      version_no?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainMarkAttrs = {
    color?: string;
    href?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainProjectState = {
    adopted_version_id?: string;
    draft_version_id?: string;
    org_id?: string;
    project_id?: string;
    revision?: number;
    updated_at?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainRichAttrs = {
    language?: string;
    level?: number;
    start?: number;
    text_align?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainRichDocument = {
    attrs?: githubComStephenQiu30LanverseBackendInternalScriptDomainRichAttrs;
    content?: githubComStephenQiu30LanverseBackendInternalScriptDomainRichDocument[];
    marks?: githubComStephenQiu30LanverseBackendInternalScriptDomainRichMark[];
    text?: string;
    type?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainRichMark = {
    attrs?: githubComStephenQiu30LanverseBackendInternalScriptDomainMarkAttrs;
    type?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainScalarSpan = {
    end?: number;
    start?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainSourceExtractionWarning =
    {
      code?: string;
      count?: number;
      paragraph?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainSourceOriginMapping =
    {
      decoded_end?: number;
      decoded_start?: number;
      paragraph?: number;
      part?: string;
      span_end?: number;
      span_start?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainSourceProvenance =
    {
      encoding?: string;
      external_id?: string;
      external_position?: number;
      mapping?: githubComStephenQiu30LanverseBackendInternalScriptDomainSourceOriginMapping[];
      warnings?: githubComStephenQiu30LanverseBackendInternalScriptDomainSourceExtractionWarning[];
    };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainSourceSpan = {
    end?: number;
    position?: number;
    source_id?: string;
    source_lineage_id?: string;
    start?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainSplitConfirmation =
    {
      actor_id?: string;
      candidate_set_id?: string;
      created_at?: string;
      episodes?: githubComStephenQiu30LanverseBackendInternalScriptDomainEpisode[];
      formal_set_id?: string;
      id?: string;
      org_id?: string;
      preface?: githubComStephenQiu30LanverseBackendInternalScriptDomainScalarSpan;
      project_id?: string;
      revision?: number;
      version_id?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainSplitSet = {
    boundaries?: githubComStephenQiu30LanverseBackendInternalScriptDomainEpisodeBoundary[];
    created_at?: string;
    id?: string;
    kind?: string;
    org_id?: string;
    origin?: string;
    preface?: githubComStephenQiu30LanverseBackendInternalScriptDomainScalarSpan;
    project_id?: string;
    version_id?: string;
    warnings?: string[];
  };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainStructureDocument =
    {
      scenes?: githubComStephenQiu30LanverseBackendInternalScriptDomainStructureScene[];
      unassigned_lines?: githubComStephenQiu30LanverseBackendInternalScriptDomainUnassignedLine[];
    };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainStructureItem = {
    character_id?: string;
    content?: string;
    emotion?: string;
    kind?: string;
    line_key?: string;
    span_end?: number;
    span_start?: number;
    speaker_text?: string;
    type?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainStructureScene =
    {
      heading?: string;
      items?: githubComStephenQiu30LanverseBackendInternalScriptDomainStructureItem[];
      location_text?: string;
      scene_key?: string;
      seq_no?: number;
      span_end?: number;
      span_start?: number;
      time_of_day?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalScriptDomainUnassignedLine =
    {
      content?: string;
      line_key?: string;
      span_end?: number;
      span_start?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalWorkspaceApplicationCreatedProject =
    {
      allow_overseas_models?: boolean;
      aspect_ratio?: string;
      cover_asset_id?: string;
      create_time?: string;
      description?: string;
      id?: string;
      name?: string;
      org_id?: string;
      resolution?: string;
      revision?: number;
      status?: string;
      style_preset_id?: string;
      style_subtype?: string;
      style_type?: string;
      update_time?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalWorkspaceApplicationModelDefaults =
    {
      default_models?: Record<string, any>;
      project_id?: string;
      revision?: number;
    };

  type githubComStephenQiu30LanverseBackendInternalWorkspaceApplicationStylePresetSummary =
    {
      id?: string;
      name?: string;
      style_subtype?: string;
      style_type?: string;
    };

  type githubComStephenQiu30LanverseBackendInternalWorkspaceDomainFolderCover =
    {
      asset_id?: string;
      project_id?: string;
    };

  type importScriptSourcesParams = {
    /** 项目UUID */
    pid: string;
  };

  type internalCanvasAdapterHttpListResponse = {
    items?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDocument[];
    next_cursor?: string;
  };

  type internalCatalogAdapterHttpAdminModelPage = {
    items?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedModel[];
    next_cursor?: string | null;
  };

  type internalCatalogAdapterHttpCreateModelRequest = {
    capability?: string;
    display_name?: string;
    model_key?: string;
    provider_id?: string;
  };

  type internalCatalogAdapterHttpCreateProviderRequest = {
    adapter_key?: string;
    concurrency_limit?: number;
    key?: string;
    name?: string;
    rate_limit_per_min?: number;
    region?: githubComStephenQiu30LanverseBackendInternalCatalogDomainRegion;
  };

  type internalCatalogAdapterHttpModelPage = {
    items?: internalCatalogAdapterHttpModelResponse[];
    next_cursor?: string | null;
  };

  type internalCatalogAdapterHttpModelPrice = {
    currency?: string;
    effective_from?: string;
    fx_rate_to_cny?: string | null;
    id?: string;
    rule?: Record<string, any>;
    unit?: string;
    version_no?: number;
  };

  type internalCatalogAdapterHttpModelResponse = {
    capability?: string;
    credential_present?: boolean;
    credential_test_result?: string | null;
    current_price?: internalCatalogAdapterHttpModelPrice | null;
    current_version?: internalCatalogAdapterHttpModelVersion | null;
    display_name?: string;
    id?: string;
    input_roles?: string[];
    key?: string;
    provider_id?: string;
    provider_name?: string;
    provider_status?: string;
    region?: string;
    status?: string;
  };

  type internalCatalogAdapterHttpModelVersion = {
    id?: string;
    limits?: Record<string, any>;
    modes?: string[];
    param_schema?: Record<string, any>[];
    version_no?: number;
  };

  type internalCatalogAdapterHttpProviderPage = {
    items?: githubComStephenQiu30LanverseBackendInternalCatalogApplicationProviderListItem[];
    next_cursor?: string | null;
  };

  type internalCatalogAdapterHttpPublishPriceRequest = {
    currency?: string;
    effective_from?: string;
    expected_revision?: number;
    fx_rate_to_cny?: string;
    rule?: Record<string, any>;
    unit?: githubComStephenQiu30LanverseBackendInternalCatalogDomainPriceUnit;
    version_no?: number;
  };

  type internalCatalogAdapterHttpPublishVersionRequest = {
    expected_max_ms?: number;
    expected_revision?: number;
    limits?: Record<string, any>;
    moderation?: githubComStephenQiu30LanverseBackendInternalCatalogDomainModeration;
    modes?: string[];
    param_schema?: Record<string, any>[];
    provider_model_id?: string;
    queue?: string;
    supports_callback?: boolean;
    supports_cancel?: boolean;
    supports_query?: boolean;
    version_no?: number;
  };

  type internalCatalogAdapterHttpSetCredentialRequest = {
    label?: string;
    secret?: Record<string, any>;
  };

  type internalCatalogAdapterHttpSetModelStatusRequest = {
    expected_revision?: number;
    status?: githubComStephenQiu30LanverseBackendInternalCatalogDomainModelStatus;
  };

  type internalCatalogAdapterHttpUpdateProviderRequest = {
    concurrency_limit?: number;
    expected_revision?: number;
    rate_limit_per_min?: number;
    status?: githubComStephenQiu30LanverseBackendInternalCatalogDomainProviderStatus;
  };

  type internalMediatoolAdapterHttpDepthJobResponse = {
    asset_id?: string | null;
    attempt?: number;
    cancellation_requested?: boolean;
    created_at?: string;
    execution_unconfirmed?: boolean;
    failure_code?: string | null;
    id?: string;
    needs_reconciliation?: boolean;
    profile_id?: string;
    project_id?: string;
    reconciliation_requested?: boolean;
    retryable?: boolean;
    revision?: number;
    sha256?: string | null;
    source?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainSource;
    source_asset_id?: string;
    source_asset_revision?: number;
    source_sha256?: string;
    stage?: string;
    status?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainDepthStatus;
    updated_at?: string;
  };

  type internalMediatoolAdapterHttpExportJobResponse = {
    asset_id?: string | null;
    attempt?: number;
    created_at?: string;
    failure_code?: string | null;
    id?: string;
    output_kind?: "video" | "audio";
    progress?: number;
    project_id?: string;
    revision?: number;
    sha256?: string | null;
    source?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainSource;
    stage?: string;
    status?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainStatus;
    updated_at?: string;
  };

  type internalMediatoolAdapterHttpTranscriptionJobResponse = {
    attempt?: number;
    created_at?: string;
    failure_code?: string | null;
    id?: string;
    language?: string;
    progress?: number;
    project_id?: string;
    result_sha256?: string | null;
    revision?: number;
    source?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainTranscriptionSource;
    stage?: string;
    status?: githubComStephenQiu30LanverseBackendInternalMediatoolDomainTranscriptionStatus;
    updated_at?: string;
  };

  type internalOperationAdapterHttpConfirmBatchItem = {
    operation_id?: string;
    reasons?: string[];
    status?: githubComStephenQiu30LanverseBackendInternalOperationDomainStatus;
  };

  type internalOperationAdapterHttpConfirmBatchRequest = {
    exclude_operation_ids?: string[];
    project_id?: string;
  };

  type internalOperationAdapterHttpConfirmBatchResponse = {
    available_micros?: number;
    budget_revision?: number;
    confirmed_count?: number;
    items?: internalOperationAdapterHttpConfirmBatchItem[];
    quote_total_micros?: number;
  };

  type internalOperationAdapterHttpConfirmResponse = {
    available_micros?: number;
    budget_revision?: number;
    reservation_id?: string;
  };

  type internalOperationAdapterHttpProjectRequest = {
    project_id?: string;
  };

  type internalOperationAdapterHttpQuoteItemRequest = {
    capability?: string;
    force_regenerate?: boolean;
    media_inputs?: internalOperationAdapterHttpQuoteMediaInput[];
    mode?: string;
    model_key?: string;
    output_count?: number;
    params?: Record<string, any>;
    prompt?: string;
    prompt_template?: githubComStephenQiu30LanverseBackendInternalPromptApplicationTemplateRequest;
    source?: githubComStephenQiu30LanverseBackendInternalOperationDomainCanvasSource | null;
  };

  type internalOperationAdapterHttpQuoteMediaInput = {
    media_asset_id?: string;
    role?: string;
  };

  type internalOperationAdapterHttpQuoteResponse = {
    available_micros?: number;
    confirmable?: boolean;
    expires_at?: string;
    final_prompt?: string;
    operation_id?: string;
    prompt_preparation?: githubComStephenQiu30LanverseBackendInternalOperationDomainPromptPreparation;
    quote_detail?: Record<string, any>;
    quote_micros?: number;
    reused_from_id?: string | null;
  };

  type internalOperationAdapterHttpQuotesRequest = {
    items?: internalOperationAdapterHttpQuoteItemRequest[];
  };

  type internalOperationAdapterHttpTaskPageResponse = {
    items?: githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskSummary[];
    next_cursor?: string | null;
  };

  type internalPromptAdapterHttpPreferencesResponse = {
    items?: githubComStephenQiu30LanverseBackendInternalPromptApplicationPreference[];
  };

  type internalPromptAdapterHttpSaveRequest = {
    base_template_id: string;
    content?: string;
    expected_revision: number;
    mode: githubComStephenQiu30LanverseBackendInternalPromptDomainMode;
  };

  type internalScriptAdapterHttpAdoptVersionRequest = {
    ack_invalidate?: boolean;
    expected_revision?: number;
    expected_split_revision?: number;
  };

  type internalScriptAdapterHttpFileImportControlRequest = {
    expected_revision?: number;
  };

  type internalScriptAdapterHttpFileImportRequest = {
    asset_ids?: string[];
    base_version_id?: string;
    expected_revision?: number;
    rights_confirmed?: boolean;
  };

  type internalScriptAdapterHttpSourceControlRequest = {
    expected_revision?: number;
  };

  type internalScriptAdapterHttpSourceDeleteRequest = {
    base_version_id?: string;
    expected_revision?: number;
  };

  type internalScriptAdapterHttpSourceImportItem = {
    document?: githubComStephenQiu30LanverseBackendInternalScriptDomainRichDocument;
    original_html?: string;
    provenance?: githubComStephenQiu30LanverseBackendInternalScriptDomainSourceProvenance;
    source_kind?: string;
    status?: string;
    title?: string;
  };

  type internalScriptAdapterHttpSourceImportRequest = {
    base_version_id?: string;
    expected_revision?: number;
    rights_confirmed?: boolean;
    sources?: internalScriptAdapterHttpSourceImportItem[];
  };

  type internalScriptAdapterHttpSourceReorderRequest = {
    base_version_id?: string;
    expected_revision?: number;
    source_lineage_ids?: string[];
  };

  type internalScriptAdapterHttpSourceWriteRequest = {
    base_version_id?: string;
    document?: githubComStephenQiu30LanverseBackendInternalScriptDomainRichDocument;
    expected_revision?: number;
    original_html?: string;
    provenance?: githubComStephenQiu30LanverseBackendInternalScriptDomainSourceProvenance;
    rights_confirmed?: boolean;
    source_kind?: string;
    status?: string;
    title?: string;
  };

  type internalScriptAdapterHttpSplitReviewRequest = {
    ack_invalidate?: boolean;
    boundaries?: githubComStephenQiu30LanverseBackendInternalScriptDomainEpisodeBoundary[];
    candidate_set_id?: string;
    expected_revision?: number;
    expected_split_revision?: number;
    preface?: githubComStephenQiu30LanverseBackendInternalScriptDomainScalarSpan;
    version_id?: string;
  };

  type internalScriptAdapterHttpStructureConfirmRequest = {
    ack_invalidate?: boolean;
    base_structure_version_no?: number;
    expected_episode_revision?: number;
    expected_revision?: number;
  };

  type internalScriptAdapterHttpStructureDetail = {
    episode?: githubComStephenQiu30LanverseBackendInternalScriptDomainEpisode;
    structure?: githubComStephenQiu30LanverseBackendInternalScriptDomainEpisodeStructure;
  };

  type internalScriptAdapterHttpStructureSaveRequest = {
    base_structure_version_no?: number;
    document?: githubComStephenQiu30LanverseBackendInternalScriptDomainStructureDocument;
    expected_episode_revision?: number;
    expected_revision?: number;
  };

  type internalWorkspaceAdapterHttpCreateProjectRequest = {
    aspect_ratio: "9:16" | "16:9";
    description?: string;
    name: string;
    style_preset_id?: string;
    style_subtype?: "anime_jp" | "guofeng_xianxia" | "cartoon_3d" | "manhwa";
    style_type: "realistic" | "stylized";
  };

  type internalWorkspaceAdapterHttpListResponse = {
    items?: internalWorkspaceAdapterHttpProjectResponse[];
    next_cursor?: string | null;
  };

  type internalWorkspaceAdapterHttpModelDefaultsRequest = {
    default_models: Record<string, any>;
    expected_revision: number;
  };

  type internalWorkspaceAdapterHttpProjectCopyListResponse = {
    copies?: internalWorkspaceAdapterHttpProjectCopyResponse[];
    current_actor_id?: string;
    current_org_id?: string;
    next_cursor?: string | null;
  };

  type internalWorkspaceAdapterHttpProjectCopyPlacementRequest = {
    expected_folder_revision: number;
    expected_placement_revision: number;
    folder_id?: string | null;
  };

  type internalWorkspaceAdapterHttpProjectCopyRequest = {
    expected_revision: number;
    placement?: internalWorkspaceAdapterHttpProjectCopyPlacementRequest;
    target_name?: string;
  };

  type internalWorkspaceAdapterHttpProjectCopyResponse = {
    assets?: number;
    attempt?: number;
    cancellation_requested?: boolean;
    completed_assets?: number;
    completed_documents?: number;
    completed_renditions?: number;
    documents?: number;
    execution_unconfirmed?: boolean;
    failure_code?: string;
    id?: string;
    needs_reconciliation?: boolean;
    reconciliation_requested?: boolean;
    renditions?: number;
    retryable?: boolean;
    revision?: number;
    script?: internalWorkspaceAdapterHttpProjectCopyScriptProgress;
    source_project_id?: string;
    source_revision?: number;
    stage?:
      "media" | "script" | "canvases" | "finalizing" | "cleanup" | "complete";
    status?:
      | "queued"
      | "running"
      | "failed"
      | "cancel_requested"
      | "cancelled"
      | "succeeded";
    target_name?: string;
    target_project_id?: string;
  };

  type internalWorkspaceAdapterHttpProjectCopyScriptCounts = {
    action_lines?: number;
    dialogue_lines?: number;
    episodes?: number;
    objects?: number;
    project_states?: number;
    scenes?: number;
    sources?: number;
    split_confirmations?: number;
    split_sets?: number;
    structures?: number;
    version_heads?: number;
    version_sources?: number;
    versions?: number;
  };

  type internalWorkspaceAdapterHttpProjectCopyScriptProgress = {
    completed_counts?: internalWorkspaceAdapterHttpProjectCopyScriptCounts;
    counts?: internalWorkspaceAdapterHttpProjectCopyScriptCounts;
  };

  type internalWorkspaceAdapterHttpProjectDetailResponse = {
    allow_overseas_models?: boolean;
    archived_at?: string | null;
    aspect_ratio?: string;
    cover_asset_id?: string | null;
    cover_unavailable?: boolean;
    create_time?: string;
    default_models?: Record<string, any>;
    delete_time?: string | null;
    description?: string;
    id?: string;
    is_delete?: boolean;
    name?: string;
    purge_after?: string | null;
    resolution?: string;
    revision?: number;
    status?: string;
    style_preset_id?: string | null;
    style_subtype?: string;
    style_type?: string;
    update_time?: string;
  };

  type internalWorkspaceAdapterHttpProjectFolderChangeResponse = {
    folder?: internalWorkspaceAdapterHttpProjectFolderResponse;
    placement?: internalWorkspaceAdapterHttpProjectFolderPlacementResponse;
    recycled_project_ids?: string[];
  };

  type internalWorkspaceAdapterHttpProjectFolderCreateRequest = {
    cover?: githubComStephenQiu30LanverseBackendInternalWorkspaceDomainFolderCover;
    name?: string;
  };

  type internalWorkspaceAdapterHttpProjectFolderListResponse = {
    current_actor_id?: string;
    current_org_id?: string;
    items?: internalWorkspaceAdapterHttpProjectFolderResponse[];
    next_cursor?: string | null;
  };

  type internalWorkspaceAdapterHttpProjectFolderMoveRequest = {
    expected_folder_revision?: number;
    expected_placement_revision?: number;
    expected_project_revision?: number;
    folder_id?: string | null;
  };

  type internalWorkspaceAdapterHttpProjectFolderPlacementResponse = {
    folder_id?: string | null;
    project_id?: string;
    revision?: number;
  };

  type internalWorkspaceAdapterHttpProjectFolderResponse = {
    cover?: githubComStephenQiu30LanverseBackendInternalWorkspaceDomainFolderCover | null;
    cover_unavailable?: boolean;
    create_time?: string;
    delete_time?: string | null;
    id?: string;
    is_delete?: boolean;
    name?: string;
    project_count?: number | null;
    revision?: number;
    update_time?: string;
  };

  type internalWorkspaceAdapterHttpProjectFolderUpdateRequest = {
    cover?: githubComStephenQiu30LanverseBackendInternalWorkspaceDomainFolderCover | null;
    expected_revision?: number;
    name?: string;
  };

  type internalWorkspaceAdapterHttpProjectResponse = {
    archived_at?: string | null;
    aspect_ratio?: string;
    cover_asset_id?: string | null;
    cover_unavailable?: boolean;
    delete_time?: string | null;
    folder_id?: string | null;
    id?: string;
    is_delete?: boolean;
    name?: string;
    placement_revision?: number;
    purge_after?: string | null;
    revision?: number;
    status?: string;
    style_type?: string;
  };

  type internalWorkspaceAdapterHttpProjectTransitionRequest = {
    expected_revision: number;
  };

  type internalWorkspaceAdapterHttpProjectUpdateRequest = {
    allow_overseas_models?: boolean;
    cover_asset_id?: string | null;
    description?: string;
    expected_revision: number;
    name?: string;
    style_preset_id?: string;
  };

  type internalWorkspaceAdapterHttpStylePresetListResponse = {
    items?: githubComStephenQiu30LanverseBackendInternalWorkspaceApplicationStylePresetSummary[];
    next_cursor?: string | null;
  };

  type listAdminModelsParams = {
    /** 1..200 */
    limit?: number;
    /** 分页游标 */
    cursor?: string;
  };

  type listAdminProvidersParams = {
    /** 1..200 */
    limit?: number;
    /** 绑定管理员与分页的不透明游标 */
    cursor?: string;
  };

  type listCanvasesParams = {
    /** 项目UUID */
    pid: string;
  };

  type listMediaAssetsParams = {
    /** 项目UUID */
    pid: string;
    /** image/video/audio/model/document；默认仅可渲染媒体 */
    kind?: string;
    /** 不透明游标 */
    cursor?: string;
    /** 1..200，默认50 */
    limit?: number;
  };

  type listMediaDepthsParams = {
    /** Project UUID */
    pid: string;
    /** Canvas UUID */
    canvas_id?: string;
    /** Timeline node UUID */
    node_id?: string;
    /** 1..200, default 50 */
    limit?: number;
    /** Project and source bound cursor */
    cursor?: string;
  };

  type listMediaExportsParams = {
    /** Project UUID */
    pid: string;
    /** Canvas UUID */
    canvas_id?: string;
    /** Timeline node UUID */
    node_id?: string;
    /** 1..200, default 50 */
    limit?: number;
    /** Project and source bound cursor */
    cursor?: string;
  };

  type listMediaLibraryParams = {
    /** personal/project；默认personal */
    scope?: string;
    /** project scope必要UUID */
    project_id?: string;
    /** 页码1..100000；默认1 */
    page?: number;
    /** 每页1..120；个人默认40/项目默认20 */
    page_size?: number;
    /** text/image/video/audio/document/model */
    kind?: string;
    /** character/environment/prop/material/other */
    category?: string;
    /** 当前库目录UUID */
    folder_id?: string;
    /** 只看未分类根目录 */
    root_only?: boolean;
    /** 个人收藏 */
    favorite_only?: boolean;
    /** 最近30日 */
    recent_only?: boolean;
    /** active/trashed；默认active */
    catalog_state?: string;
    /** 完整库文字搜索 */
    search?: string;
    /** updated_desc/updated_asc/name_asc */
    order?: string;
  };

  type listMediaTranscriptionsParams = {
    /** Project UUID */
    pid: string;
    /** Canvas UUID */
    canvas_id?: string;
    /** Timeline node UUID */
    node_id?: string;
    /** 1..200, default 50 */
    limit?: number;
    /** Project and source bound cursor */
    cursor?: string;
  };

  type listProjectCopiesParams = {
    /** 源项目UUID */
    pid: string;
    /** 1至100，默认50 */
    limit?: number;
    /** 上一页next_cursor */
    cursor?: string;
  };

  type listProjectFoldersParams = {
    /** 分页大小，上限200 */
    limit?: number;
    /** 当前主体目录分页游标 */
    cursor?: string;
  };

  type listProjectModelsParams = {
    /** 项目UUID */
    project_id: string;
    /** 能力 */
    capability?: string;
    /** 生成模式 */
    mode?: string;
    /** 1..200，默认50 */
    limit?: number;
    /** 绑定项目与筛选的不透明游标 */
    cursor?: string;
  };

  type listProjectOperationsParams = {
    /** 项目UUID */
    pid: string;
    /** 任务状态 */
    status?: string;
    /** 能力 */
    capability?: string;
    /** 模型 */
    model_key?: string;
    /** 来源 */
    origin?: string;
    /** 冻结来源画布UUID */
    canvas_id?: string;
    /** 冻结来源节点UUID，需canvas_id */
    node_id?: string;
    /** 冻结来源行UUID，需node_id */
    row_id?: string;
    /** 1..200，默认50 */
    limit?: number;
    /** 项目与筛选绑定游标 */
    cursor?: string;
  };

  type listProjectsParams = {
    /** 分页大小，上限200 */
    limit?: number;
    /** 不透明游标 */
    cursor?: string;
    /** 名称检索 */
    q?: string;
    /** active或archived */
    status?: string;
    /** 只查询回收中的项目 */
    deleted?: boolean;
    /** 个人目录UUID，root表示未分类；未指定列出全部 */
    folder_id?: string;
  };

  type listScriptEpisodesParams = {
    /** 项目UUID */
    pid: string;
    /** 剧本版本UUID */
    version_id: string;
  };

  type listScriptEpisodeStructureVersionsParams = {
    /** 正式分集UUID */
    eid: string;
    /** 严格早于结构版本号 */
    before_version_no?: number;
    /** 1..100 */
    limit?: number;
  };

  type listScriptFileImportsParams = {
    /** Project UUID */
    pid: string;
    /** Offset */
    after?: number;
    /** Page size */
    limit?: number;
  };

  type listScriptSourceHistoryParams = {
    /** 项目UUID */
    pid: string;
    /** 稳定来源UUID */
    lineage: string;
    /** 严格早于来源修订 */
    before_revision?: number;
    /** 1..100 */
    limit?: number;
  };

  type listScriptSourcesParams = {
    /** 项目UUID */
    pid: string;
    /** 不可变历史版本UUID */
    version_id?: string;
    /** 下一个来源位置，须与返回version_id绑定 */
    after?: number;
    /** 1..100 */
    limit?: number;
  };

  type listScriptSourceWritesParams = {
    /** 项目UUID */
    pid: string;
    /** 分页位置 */
    after?: number;
    /** 1..100 */
    limit?: number;
  };

  type listScriptSplitConfirmationsParams = {
    /** 项目UUID */
    pid: string;
    /** 不可变剧本版本UUID */
    vid: string;
    /** 严格早于确认修订 */
    before_revision?: number;
    /** 1..100 */
    limit?: number;
  };

  type listScriptVersionsParams = {
    /** 项目UUID */
    pid: string;
    /** 严格早于版本号 */
    before_version_no?: number;
    /** 1..100 */
    limit?: number;
  };

  type listStylePresetsParams = {
    /** 风格类型 */
    style_type?: "realistic" | "stylized";
    /** 子风格 */
    style_subtype?: "anime_jp" | "guofeng_xianxia" | "cartoon_3d" | "manhwa";
    /** 每页数量（1～200，未传或0默认50） */
    limit?: number;
    /** 与账号、组织及筛选绑定的游标 */
    cursor?: string;
  };

  type moveProjectToFolderParams = {
    /** 项目UUID */
    pid: string;
  };

  type previewLibraryMediaAssetParams = {
    /** 二进制素材条目UUID */
    item_id: string;
    /** personal/project；默认personal */
    scope?: string;
    /** project scope必要UUID */
    project_id?: string;
  };

  type previewMediaDepthParams = {
    /** Depth UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type previewMediaExportParams = {
    /** Export UUID */
    job_id: string;
    /** Project UUID */
    project_id: string;
  };

  type publishAdminModelPriceParams = {
    /** 模型UUID */
    id: string;
  };

  type publishAdminModelVersionParams = {
    /** 模型UUID */
    id: string;
  };

  type reconcileMediaDepthParams = {
    /** Depth UUID */
    job_id: string;
  };

  type reconcileProjectCopyParams = {
    /** 复制任务UUID */
    id: string;
  };

  type reconcileScriptFileImportParams = {
    /** Project UUID */
    pid: string;
    /** Import UUID */
    id: string;
  };

  type reconcileScriptSourceWriteParams = {
    /** 项目UUID */
    pid: string;
    /** 保存意图UUID */
    wid: string;
  };

  type recycleProjectFolderParams = {
    /** 目录UUID */
    folder_id: string;
  };

  type renameCanvasParams = {
    /** 画布UUID */
    id: string;
  };

  type reorderScriptSourcesParams = {
    /** 项目UUID */
    pid: string;
  };

  type resplitScriptEpisodesParams = {
    /** 项目UUID */
    pid: string;
  };

  type restoreProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type resumeOperationBatchParams = {
    /** Batch UUID */
    id: string;
  };

  type retryMediaDepthParams = {
    /** Depth UUID */
    job_id: string;
  };

  type retryMediaExportParams = {
    /** Export UUID */
    job_id: string;
  };

  type retryMediaTranscriptionParams = {
    /** Transcription UUID */
    job_id: string;
  };

  type retryProjectCopyParams = {
    /** 复制任务UUID */
    id: string;
  };

  type retryScriptFileImportParams = {
    /** Project UUID */
    pid: string;
    /** Import UUID */
    id: string;
  };

  type reviewMediaDepthParams = {
    /** Depth UUID */
    job_id: string;
  };

  type reviewMediaExportParams = {
    /** Export UUID */
    job_id: string;
  };

  type saveProjectModelDefaultsParams = {
    /** 项目UUID */
    pid: string;
  };

  type savePromptPreferenceParams = {
    /** 只读目录中的操作标识 */
    operation: string;
  };

  type saveScriptEpisodeStructureParams = {
    /** 正式分集UUID */
    eid: string;
  };

  type setAdminCredentialParams = {
    /** 渠道UUID */
    id: string;
  };

  type setAdminModelStatusParams = {
    /** 模型UUID */
    id: string;
  };

  type testAdminCredentialParams = {
    /** 渠道UUID */
    id: string;
    /** 凭据UUID */
    credential_id: string;
  };

  type unarchiveProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type updateAdminProviderParams = {
    /** 渠道UUID */
    id: string;
  };

  type updateProjectFolderParams = {
    /** 目录UUID */
    folder_id: string;
  };

  type updateProjectParams = {
    /** 项目UUID */
    pid: string;
  };

  type updateScriptSourceParams = {
    /** 项目UUID */
    pid: string;
    /** 稳定来源UUID */
    lineage: string;
  };

  type uploadMediaAssetParams = {
    /** 项目UUID */
    pid: string;
  };
}
