import { beforeEach, expect, it } from "vitest";
import {
  clearProjectCoverIntent,
  loadProjectCoverIntent,
  saveProjectCoverIntent,
  projectCoverIntentSchema,
  type ProjectCoverIntent,
} from "./project-cover-intent";

const scope = {
  origin: "http://localhost:3000",
  actorId: "d3a6a735-cbdd-4f7f-8316-fc6e08e93c34",
  orgId: "de0d143b-27bc-4f1c-90a8-edc099be4b2d",
};
const projectId = "9817c918-e49d-4dc8-b8b6-c92833b135e4";
const intent: ProjectCoverIntent = {
  ...scope,
  version: 1,
  key: "bbcc1326-63af-4027-bc30-9a5017492de2",
  projectId,
  body: { expected_revision: 7, cover_asset_id: null },
};
beforeEach(() => localStorage.clear());
it("明确清除保持 null，刷新重放原修订、正文和键，已确认才清理", () => {
  saveProjectCoverIntent(localStorage, intent);
  expect(loadProjectCoverIntent(localStorage, scope, projectId)).toEqual(
    intent,
  );
  clearProjectCoverIntent(localStorage, scope, projectId);
  expect(loadProjectCoverIntent(localStorage, scope, projectId)).toBeNull();
});
it("原设置联合正文保持逐字段，不存预览 URL 或文件内容", () => {
  const combined = {
    ...intent,
    body: {
      ...intent.body,
      name: "逆光",
      description: "草稿",
      allow_overseas_models: false,
    },
  };
  saveProjectCoverIntent(localStorage, combined);
  expect(loadProjectCoverIntent(localStorage, scope, projectId)).toEqual(
    combined,
  );
  expect(
    projectCoverIntentSchema.safeParse({
      ...intent,
      url: "https://private.invalid/x",
    }).success,
  ).toBe(false);
  expect(
    projectCoverIntentSchema.safeParse({
      ...intent,
      body: {
        ...intent.body,
        cover_asset_id: "00000000-0000-0000-0000-000000000000",
      },
    }).success,
  ).toBe(false);
});
it("主体、组织、源和项目分别隔离；损坏存储及吞掉写入必须阻断新请求", () => {
  saveProjectCoverIntent(localStorage, intent);
  for (const other of [
    { ...scope, actorId: "51751116-3c03-47d7-b47c-06bdf9d6d6d7" },
    { ...scope, orgId: "51751116-3c03-47d7-b47c-06bdf9d6d6d7" },
    { ...scope, origin: "http://127.0.0.1:3000" },
  ])
    expect(loadProjectCoverIntent(localStorage, other, projectId)).toBeNull();
  expect(
    loadProjectCoverIntent(
      localStorage,
      scope,
      "51751116-3c03-47d7-b47c-06bdf9d6d6d7",
    ),
  ).toBeNull();
  localStorage.setItem(localStorage.key(0)!, "broken");
  expect(() => loadProjectCoverIntent(localStorage, scope, projectId)).toThrow(
    /存储/,
  );
  expect(() =>
    saveProjectCoverIntent(
      { getItem: () => null, setItem: () => {}, removeItem: () => {} },
      intent,
    ),
  ).toThrow(/存储/);
});
