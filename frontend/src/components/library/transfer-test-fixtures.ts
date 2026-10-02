import type {
  LibraryIdentity,
  LibraryPage,
} from "@/components/media/library-model";
import type {
  ProjectDetail,
  ProjectListedSummary,
} from "@/components/project/queries";
import type { TransferInput, TransferJob } from "./transfer-model";
export const transferIDs = {
  actor: "11111111-1111-4111-8111-111111111111",
  org: "22222222-2222-4222-8222-222222222222",
  project: "33333333-3333-4333-8333-333333333333",
  item: "44444444-4444-4444-8444-444444444444",
  target: "55555555-5555-4555-8555-555555555555",
  job: "66666666-6666-4666-8666-666666666666",
  folder: "77777777-7777-4777-8777-777777777777",
};
export const transferIdentity: LibraryIdentity = {
  origin: window.location.origin,
  actorId: transferIDs.actor,
  orgId: transferIDs.org,
  libraryId: transferIDs.item,
  scope: { kind: "personal" },
};
export const transferInput: TransferInput = {
  source: transferIdentity.scope,
  target: { kind: "project", project_id: transferIDs.project },
  items: [{ id: transferIDs.item, revision: 0 }],
  expected_source_revision: 0,
  expected_target_revision: 0,
  expected_project_revision: 1,
  target_folder_id: null,
  expected_folder_revision: 0,
};
const timestamp = "2026-10-02T00:00:00Z";
export const transferJob: TransferJob = {
  id: transferIDs.job,
  current_actor_id: transferIDs.actor,
  current_org_id: transferIDs.org,
  source: transferInput.source,
  target: transferInput.target,
  target_folder_id: null,
  status: "queued",
  stage: "frozen",
  attempt: 1,
  revision: 1,
  needs_reconciliation: false,
  cancellation_requested: false,
  execution_unconfirmed: false,
  items: [
    {
      index: 0,
      source_item_id: transferIDs.item,
      target_item_id: transferIDs.target,
      target_asset_id: null,
      status: "queued",
      failure_code: null,
    },
  ],
  created_at: timestamp,
  updated_at: timestamp,
};
export const transferLibrary: LibraryPage = {
  current_actor_id: transferIDs.actor,
  current_org_id: transferIDs.org,
  library_id: transferIDs.item,
  scope: transferIdentity.scope,
  revision: 0,
  page: 1,
  page_size: 1,
  total: 0,
  items: [],
  folders: [],
  category_counts: {},
  folder_counts: {},
};
export const transferProject: ProjectDetail = {
  id: transferIDs.project,
  name: "真实目标项目",
  description: "",
  status: "active",
  is_delete: false,
  revision: 1,
  cover_asset_id: null,
  cover_unavailable: false,
  aspect_ratio: "16:9",
  style_type: "realistic",
  style_preset_id: null,
  resolution: "1080p",
  allow_overseas_models: false,
  default_models: {},
  archived_at: null,
  delete_time: null,
  purge_after: null,
  create_time: timestamp,
  update_time: timestamp,
};
export const transferProjectSummary: ProjectListedSummary = {
  ...transferProject,
  folder_id: null,
  placement_revision: 0,
};
