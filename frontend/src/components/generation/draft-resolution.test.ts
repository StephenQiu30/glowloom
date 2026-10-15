import { expect, it } from "vitest";
import { reconcileLabels } from "./draft-resolution";
import { emptyPromptDraft } from "./prompt-document";
it("explicit overwrite renumbers only local collisions above both high water marks", () => {
  const server = emptyPromptDraft(),
    local = emptyPromptDraft();
  server.labelState = {
    highWater: { image: 3, video: 0, audio: 0, subject: 0 },
    allocations: { old: { labelKind: "image", ordinal: 3 } },
  };
  local.labelState = {
    highWater: { image: 3, video: 0, audio: 0, subject: 0 },
    allocations: { new: { labelKind: "image", ordinal: 3 } },
  };
  local.references = [
    {
      id: "new",
      bindingGeneration: 1,
      order: 0,
      mediaKind: "image",
      role: "reference_image",
      source: { kind: "media", sourceId: "asset", sourceVersion: "1" },
    },
  ];
  local.document.content[0].content = [
    { type: "reference", attrs: { referenceId: "new" } },
  ];
  const result = reconcileLabels(local, server);
  expect(result.draft.labelState.allocations.old.ordinal).toBe(3);
  expect(result.draft.labelState.allocations.new.ordinal).toBe(4);
  expect(result.changes).toEqual(["图片3 → 图片4"]);
  expect(local.labelState.allocations.new.ordinal).toBe(3);
  const adopted = reconcileLabels(server, server, local);
  expect(adopted.draft.references).toEqual([]);
  expect(adopted.draft.labelState.allocations.old.ordinal).toBe(3);
  expect(adopted.draft.labelState.highWater.image).toBe(3);
});
