declare namespace API {
  type applicationCommandResult = {
    ok?: boolean;
  };

  type applicationCommandsInput = {
    commands?: domainCommand[];
    expected_revision?: number;
  };

  type applicationCreateInput = {
    name?: string;
    scope?: Record<string, any>;
  };

  type applicationResult = {
    edges?: domainEdge[];
    id?: string;
    name?: string;
    nodes?: domainNode[];
    project_id?: string;
    results?: applicationCommandResult[];
    revision?: number;
    scope?: Record<string, any>;
    viewport?: domainViewport;
  };

  type applyCanvasCommandsParams = {
    /** 画布UUID */
    id: string;
  };

  type createCanvasParams = {
    /** 项目UUID */
    pid: string;
  };

  type domainCommand = {
    config?: domainTextConfig;
    edge?: domainEdge;
    id?: string;
    ids?: string[];
    moves?: domainMove[];
    nodes?: domainNode[];
    type?: string;
    viewport?: domainViewport;
  };

  type domainDocument = {
    edges?: domainEdge[];
    id?: string;
    name?: string;
    nodes?: domainNode[];
    project_id?: string;
    revision?: number;
    scope?: Record<string, any>;
    viewport?: domainViewport;
  };

  type domainEdge = {
    binding?: Record<string, any>;
    edge_type?: string;
    id?: string;
    role?: string;
    source_node_id?: string;
    target_node_id?: string;
  };

  type domainMove = {
    id?: string;
    x?: number;
    y?: number;
  };

  type domainNode = {
    config?: domainTextConfig;
    height?: number;
    id?: string;
    last_operation_id?: string;
    node_action?: string;
    node_type?: string;
    parent_id?: string;
    ref_id?: string;
    ref_type?: string;
    width?: number;
    x?: number;
    y?: number;
    z_index?: number;
  };

  type domainTextConfig = {
    text?: string;
  };

  type domainViewport = {
    x?: number;
    y?: number;
    zoom?: number;
  };

  type getCanvasParams = {
    /** 画布UUID */
    id: string;
  };

  type httpapiProblem = {
    code?: string;
    detail?: string;
    meta?: Record<string, any>;
    request_id?: string;
    status?: number;
    title?: string;
    type?: string;
  };

  type httpLoginRequest = {
    login_name?: string;
    password?: string;
  };

  type httpPasswordRequest = {
    current_password?: string;
    new_password?: string;
  };

  type httpPasswordResponse = {
    must_change_password?: boolean;
    revision?: number;
  };

  type httpProjectResponse = {
    aspect_ratio?: string;
    id?: string;
    name?: string;
    revision?: number;
    status?: string;
    style_type?: string;
  };

  type httpSessionResponse = {
    must_change_password?: boolean;
    user?: httpUserResponse;
  };

  type httpUserResponse = {
    display_name?: string;
    id?: string;
    login_name?: string;
    must_change_password?: boolean;
    org_id?: string;
    role?: string;
  };

  type internalCanvasAdapterHttpListResponse = {
    items?: domainDocument[];
    next_cursor?: string;
  };

  type internalWorkspaceAdapterHttpListResponse = {
    items?: httpProjectResponse[];
    next_cursor?: string;
  };

  type listCanvasesParams = {
    /** 项目UUID */
    pid: string;
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
}
