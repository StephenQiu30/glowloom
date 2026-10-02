import type { PurgeInput, PurgeJob, PurgeReview } from "./library-purge-model";
import {
  transferIdentity,
  transferIDs,
} from "@/components/library/transfer-test-fixtures";
export const purgeTestIdentity = transferIdentity;
export const purgeTestInput: PurgeInput = {
  scope: transferIdentity.scope,
  items: [{ id: transferIDs.item, revision: 2 }],
  expected_revision: 4,
  expected_project_revision: 0,
  permanent_delete_confirmed: true,
};
export const purgeTestReview: PurgeReview = {
  mode: "selected",
  revision: 4,
  projectRevision: 0,
  items: [{ id: transferIDs.item, revision: 2, title: "回收素材" }],
};
export const purgeTestJob: PurgeJob = {
  id: transferIDs.job,
  scope: transferIdentity.scope,
  current_actor_id: transferIDs.actor,
  current_org_id: transferIDs.org,
  status: "queued",
  stage: "frozen",
  revision: 1,
  attempt: 1,
  cancellation_requested: false,
  needs_reconciliation: false,
  execution_unconfirmed: false,
  items: [
    {
      index: 0,
      item_id: transferIDs.item,
      asset_id: null,
      status: "queued",
      failure_code: null,
    },
  ],
  created_at: "2026-10-02T00:00:00Z",
  updated_at: "2026-10-02T00:00:00Z",
};
