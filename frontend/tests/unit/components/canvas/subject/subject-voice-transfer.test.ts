import { expect, it } from "vitest";
import { requireVoiceTransfer } from "@/components/canvas/subject/subject-voice-transfer";
const id = (n: number) =>
  `11111111-1111-4111-8111-${String(n).padStart(12, "0")}`;
const job: API.TransferJob = {
  id: id(1),
  current_actor_id: id(2),
  revision: 2,
  source: { kind: "personal" },
  target: { kind: "project", project_id: id(3) },
  status: "succeeded",
  items: [
    {
      index: 0,
      source_item_id: id(4),
      target_asset_id: id(5),
      target_item_id: id(6),
      status: "succeeded",
    },
  ],
};
it("accepts only a matching actor, target project and exact successful audio transfer", () => {
  expect(requireVoiceTransfer(job, id(2), id(3), id(4))).toBe(job);
  expect(() => requireVoiceTransfer(job, id(7), id(3), id(4))).toThrow();
  expect(() => requireVoiceTransfer(job, id(2), id(7), id(4))).toThrow();
  expect(() => requireVoiceTransfer(job, id(2), id(3), id(7))).toThrow();
  expect(() =>
    requireVoiceTransfer({ ...job, items: [] }, id(2), id(3), id(4)),
  ).toThrow();
  expect(() =>
    requireVoiceTransfer(
      { ...job, items: [{ ...job.items![0], target_asset_id: undefined }] },
      id(2),
      id(3),
      id(4),
    ),
  ).toThrow();
});
it("preserves recovery states without pretending they are success and rejects stale/replaced jobs", () => {
  for (const status of [
    "queued",
    "running",
    "needs_reconciliation",
    "failed",
    "cancel_requested",
  ]) {
    expect(
      requireVoiceTransfer({ ...job, status }, id(2), id(3), id(4)).status,
    ).toBe(status);
  }
  expect(() =>
    requireVoiceTransfer({ ...job, revision: 1 }, id(2), id(3), id(4), job),
  ).toThrow();
  expect(() =>
    requireVoiceTransfer({ ...job, id: id(8) }, id(2), id(3), id(4), job),
  ).toThrow();
});
