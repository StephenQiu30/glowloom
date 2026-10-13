declare namespace MediaAPI {
  type getMediaLibraryItemParams = {
    /** 条目UUID */
    item_id: string;
    /** personal/project */
    scope?: string;
    /** project scope必要UUID */
    project_id?: string;
  };

  type LibraryAssetSummary = {
    byte_size?: number;
    duration_ms?: number;
    file_name?: string;
    height?: number;
    id?: string;
    kind?: string;
    mime_type?: string;
    revision?: number;
    width?: number;
  };

  type LibraryCommand = {
    action?: string;
    expected_folder_revision?: number;
    expected_item_revision?: number;
    expected_revision?: number;
    folder?: LibraryFolderInput;
    folder_id?: string;
    item_id?: string;
    items?: LibraryItemRevision[];
    metadata?: LibraryMetadata;
    scope?: LibraryScope;
    target_folder_id?: string;
  };

  type LibraryFolder = {
    created_at?: string;
    id?: string;
    library_id?: string;
    library_kind?: LibraryKind;
    name?: string;
    parent_id?: string;
    position?: number;
    revision?: number;
    style?: string;
    theme?: string;
    updated_at?: string;
  };

  type LibraryFolderInput = {
    name?: string;
    parent_id?: string;
    style?: string;
    theme?: string;
  };

  type LibraryItemChange = {
    asset_id?: string;
    catalog_state?: string;
    folder_id?: string;
    id?: string;
    kind?: string;
    revision?: number;
    trashed_at?: string;
  };

  type LibraryItemDetail = {
    asset_id?: string;
    catalog_state?: string;
    category?: string;
    created_at?: string;
    favorite?: boolean;
    folder_id?: string;
    id?: string;
    kind?: string;
    media?: LibraryAssetSummary;
    note?: string;
    plain_text?: string;
    position?: number;
    revision?: number;
    source_label?: string;
    tags?: string[];
    title?: string;
    trashed_at?: string;
    updated_at?: string;
  };

  type LibraryItemRevision = {
    id?: string;
    revision?: number;
  };

  type LibraryItemSummary = {
    asset_id?: string;
    catalog_state?: string;
    category?: string;
    created_at?: string;
    favorite?: boolean;
    folder_id?: string;
    id?: string;
    kind?: string;
    media?: LibraryAssetSummary;
    note?: string;
    position?: number;
    revision?: number;
    source_label?: string;
    tags?: string[];
    title?: string;
    trashed_at?: string;
    updated_at?: string;
  };

  type LibraryKind = "project" | "personal";

  type LibraryMediaPreview = {
    asset?: LibraryAssetSummary;
    expires_at?: string;
    renditions?: LibraryRenditionPreview[];
    url?: string;
  };

  type LibraryMetadata = {
    category?: string;
    favorite?: boolean;
    folder_id?: string;
    note?: string;
    plain_text?: string;
    source_label?: string;
    tags?: string[];
    title?: string;
  };

  type LibraryPage = {
    category_counts?: Record<string, any>;
    current_actor_id?: string;
    current_org_id?: string;
    folder_counts?: Record<string, any>;
    folders?: LibraryFolder[];
    items?: LibraryItemSummary[];
    library_id?: string;
    page?: number;
    page_size?: number;
    revision?: number;
    scope?: LibraryScope;
    total?: number;
  };

  type LibraryReceipt = {
    current_actor_id?: string;
    current_org_id?: string;
    folder?: LibraryFolder;
    items?: LibraryItemChange[];
    library_id?: string;
    project_revision?: number;
    revision?: number;
    scope?: LibraryScope;
  };

  type LibraryRenditionPreview = {
    expires_at?: string;
    height?: number;
    kind?: string;
    url?: string;
    width?: number;
  };

  type LibraryScope = {
    kind?: LibraryKind;
    project_id?: string;
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

  type PersonalUploadResult = {
    asset?: LibraryAssetSummary;
    duplicate_of?: string;
  };

  type previewLibraryMediaAssetParams = {
    /** 二进制素材条目UUID */
    item_id: string;
    /** personal/project；默认personal */
    scope?: string;
    /** project scope必要UUID */
    project_id?: string;
  };

  type Problem = {
    code?: string;
    detail?: string;
    meta?: Record<string, any>;
    request_id?: string;
    status?: number;
    title?: string;
    type?: string;
  };
}
