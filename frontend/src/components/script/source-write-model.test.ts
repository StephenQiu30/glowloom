import { expect, it } from "vitest";
import {
  readSourceWritePage,
  sourceWriteActions,
  readSourceControlReceipt,
} from "./source-write-model";
const scope = {
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
};
const item = {
  id: "33333333-3333-4333-8333-333333333333",
  revision: 4,
  action: "update" as const,
  status: "pending" as const,
  expected_script_revision: 7,
  object_count: 5,
  confirmed_object_count: 2,
  cancellation_requested: false,
  needs_reconciliation: false,
  active_io: true,
  can_control: true,
  created_at: "2026-10-02T00:00:00Z",
  updated_at: "2026-10-02T00:00:00Z",
};
it("安全journal绑定当前scope、唯一ID与真实对象计数，不接受私有正文或外actor", () => {
  const page = {
    current_actor_id: scope.actorId,
    current_org_id: scope.orgId,
    items: [item],
  };
  expect(readSourceWritePage(page, scope, 0, 25).items).toEqual([item]);
  expect(() =>
    readSourceWritePage({ ...page, current_actor_id: item.id }, scope, 0, 25),
  ).toThrow();
  expect(() =>
    readSourceWritePage({ ...page, items: [item, item] }, scope, 0, 25),
  ).toThrow();
  expect(() =>
    readSourceWritePage(
      { ...page, items: [{ ...item, confirmed_object_count: 6 }] },
      scope,
      0,
      25,
    ),
  ).toThrow();
  expect(() =>
    readSourceWritePage(
      { ...page, items: [{ ...item, object_key: "private" }] },
      scope,
      0,
      25,
    ),
  ).toThrow();
});
it("activeIO不假禁恢复，cancel标志足以显式reconcile；202只是原命令受理", () => {
  expect(sourceWriteActions(item)).toEqual({ cancel: true, reconcile: false });
  expect(sourceWriteActions({ ...item, cancellation_requested: true })).toEqual(
    { cancel: false, reconcile: true },
  );
  expect(
    sourceWriteActions({
      ...item,
      status: "cancelled" as const,
      can_control: false,
    }),
  ).toEqual({ cancel: false, reconcile: false });
  const command = {
    intentId: item.id,
    action: "cancel" as const,
    body: { expected_revision: 4 },
  };
  expect(
    readSourceControlReceipt(
      { intent_id: item.id, action: "cancel", revision: 5, accepted: true },
      command,
    ).accepted,
  ).toBe(true);
  expect(() =>
    readSourceControlReceipt(
      {
        intent_id: scope.actorId,
        action: "cancel",
        revision: 5,
        accepted: true,
      },
      command,
    ),
  ).toThrow();
});
