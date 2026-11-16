import { expect, it } from "vitest";
import {
  requireNotificationSummary,
  mergeNotificationSummary,
  safeNotificationAction,
} from "@/components/notifications/contracts";

const base = {
  data_epoch: "00000000-0000-4000-8000-000000000001",
  mailbox_version: "9007199254740993",
  max_inbox_seq: "9007199254740994",
  unread_count: 2,
};

it("拒绝无效摘要，不能把缺失计数展示为0", () => {
  expect(() => requireNotificationSummary({})).toThrow();
  expect(() =>
    requireNotificationSummary({ ...base, unread_count: -1 }),
  ).toThrow();
  expect(requireNotificationSummary(base)).toEqual(base);
});

it("大整数版本不丢精度，迟到的旧响应不能覆盖新摘要", () => {
  const current = requireNotificationSummary(base);
  expect(
    mergeNotificationSummary(current, {
      ...base,
      mailbox_version: "9007199254740992",
      unread_count: 0,
    }),
  ).toBe(current);
  expect(
    mergeNotificationSummary(current, {
      ...base,
      mailbox_version: "9007199254740994",
      unread_count: 1,
    }).unread_count,
  ).toBe(1);
});

it.each([
  "https://outside.example",
  "//outside.example",
  "/\\outside.example",
  "javascript:alert(1)",
  "/%5coutside.example",
])("拒绝非站内通知动作 %s", (path) =>
  expect(safeNotificationAction(path)).toBeUndefined(),
);

it("允许原生站内目的地", () => {
  expect(safeNotificationAction("/assets?view=history")).toBe(
    "/assets?view=history",
  );
});
