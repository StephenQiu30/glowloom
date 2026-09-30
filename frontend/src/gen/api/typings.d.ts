declare namespace API {
  type applyCanvasCommandsParams = {
    /** 画布UUID */
    id: string;
  };

  type createCanvasParams = {
    /** 项目UUID */
    pid: string;
  };

  type deleteCanvasParams = {
    /** 画布UUID */
    id: string;
  };

  type getCanvasParams = {
    /** 画布UUID */
    id: string;
  };

  type getMediaPreviewParams = {
    /** 项目UUID */
    pid: string;
    /** 媒体UUID */
    asset_id: string;
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
    collapsed?: boolean;
    text?: string;
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

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainViewport = {
    x?: number;
    y?: number;
    zoom?: number;
  };

  type githubComStephenQiu30LanverseBackendInternalCanvasDomainZIndex = {
    id?: string;
    z_index?: number;
  };

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

  type githubComStephenQiu30LanverseBackendInternalMediaApplicationPreview = {
    asset?: githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetSummary;
    expires_at?: string;
    url?: string;
  };

  type githubComStephenQiu30LanverseBackendInternalPlatformHttpapiProblem = {
    code?: string;
    detail?: string;
    meta?: Record<string, any>;
    request_id?: string;
    status?: number;
    title?: string;
    type?: string;
  };

  type internalCanvasAdapterHttpListResponse = {
    items?: githubComStephenQiu30LanverseBackendInternalCanvasDomainDocument[];
    next_cursor?: string;
  };

  type internalIdentityAdapterHttpLoginRequest = {
    login_name?: string;
    password?: string;
  };

  type internalIdentityAdapterHttpPasswordRequest = {
    current_password?: string;
    new_password?: string;
  };

  type internalIdentityAdapterHttpPasswordResponse = {
    must_change_password?: boolean;
    revision?: number;
  };

  type internalIdentityAdapterHttpSessionResponse = {
    must_change_password?: boolean;
    user?: internalIdentityAdapterHttpUserResponse;
  };

  type internalIdentityAdapterHttpUserResponse = {
    display_name?: string;
    id?: string;
    login_name?: string;
    must_change_password?: boolean;
    org_id?: string;
    role?: string;
  };

  type internalWorkspaceAdapterHttpListResponse = {
    items?: internalWorkspaceAdapterHttpProjectResponse[];
    next_cursor?: string;
  };

  type internalWorkspaceAdapterHttpProjectResponse = {
    aspect_ratio?: string;
    id?: string;
    name?: string;
    revision?: number;
    status?: string;
    style_type?: string;
  };

  type listCanvasesParams = {
    /** 项目UUID */
    pid: string;
  };

  type listMediaAssetsParams = {
    /** 项目UUID */
    pid: string;
    /** image/video/audio */
    kind?: string;
    /** 不透明游标 */
    cursor?: string;
    /** 1..200，默认50 */
    limit?: number;
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
  };

  type renameCanvasParams = {
    /** 画布UUID */
    id: string;
  };
}
