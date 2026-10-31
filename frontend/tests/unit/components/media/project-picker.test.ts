import { describe, expect, it } from "vitest";
import {
  defaultProjectPickerFilter,
  projectPickerQuery,
  requireProjectPickerPage,
} from "@/components/media/project-picker";
const id = "00000000-0000-4000-8000-000000000001";
const assetID = "00000000-0000-4000-8000-000000000002";
const response = {
  current_actor_id: id,
  current_org_id: id,
  library_id: id,
  revision: 1,
  scope: { kind: "project", project_id: id },
  page: 1,
  page_size: 40,
  total: 45,
  items: [
    {
      id: assetID,
      asset_id: assetID,
      kind: "image",
      catalog_state: "active",
      media: { id: assetID, kind: "image", revision: 3, file_name: "真实图片" },
    },
  ],
};
describe("project picker formal query", () => {
  it("combines server predicates; ignores image-only filters for audio", () => {
    const filters = {
      from: "2026-10-01",
      through: "2026-10-02",
      origin: "upload" as const,
      resolutions: ["1K", "2K"],
      ratios: ["16:9", "1:1"],
    };
    expect(projectPickerQuery(id, "image", " 海边 ", 2, filters)).toMatchObject(
      {
        scope: "project",
        project_id: id,
        kind: "image",
        page: 2,
        page_size: 40,
        search: "海边",
        origin: "upload",
        resolutions: "1K,2K",
        ratios: "16:9,1:1",
        created_from: new Date("2026-10-01T00:00:00").toISOString(),
        created_before: new Date("2026-10-03T00:00:00").toISOString(),
      },
    );
    expect(projectPickerQuery(id, "audio", "", 1, filters)).toMatchObject({
      resolutions: undefined,
      ratios: undefined,
    });
  });
  it("rejects impossible dates and another actor receipt", () => {
    expect(() =>
      projectPickerQuery(id, "image", "", 1, {
        ...defaultProjectPickerFilter,
        from: "2026-02-30",
      }),
    ).toThrow();
    expect(() =>
      requireProjectPickerPage(response, id, "image", 1, assetID),
    ).toThrow();
  });
  it("uses media revision and server total, not catalog revision or loaded count", () => {
    expect(requireProjectPickerPage(response, id, "image", 1)).toEqual({
      items: [
        {
          id: assetID,
          kind: "image",
          revision: 3,
          file_name: "真实图片",
          project_id: id,
        },
      ],
      total: 45,
      page: 1,
      hasMore: true,
    });
  });
  it.each([
    { ...response, scope: { kind: "personal" } },
    { ...response, page: 2 },
    { ...response, total: -1 },
    {
      ...response,
      items: [
        { ...response.items[0], media: { ...response.items[0].media, id } },
      ],
    },
    {
      ...response,
      items: [
        {
          ...response.items[0],
          media: { ...response.items[0].media, revision: 0 },
        },
      ],
    },
  ])("rejects mixed ownership and malformed identity", (bad) =>
    expect(() => requireProjectPickerPage(bad, id, "image", 1)).toThrow(),
  );
});
