import { expect, it } from "vitest";
import {
  readAssetLocation,
  writeAssetLocation,
} from "@/app/assets/_components/asset-location";

it("round-trips media filters, dates and order without losing the current query", () => {
  const query = new URLSearchParams(
    "kind=video&q=山谷&from=2026-10-01&through=2026-10-28&order=asc&favorite=1&process_types=frame_interpolation&resolutions=1080P&ratios=16%3A9&generation_modes=lip_sync",
  );
  const restored = readAssetLocation(query);
  expect(restored.kind).toBe("video");
  expect(restored.filters).toMatchObject({
    search: "山谷",
    from: "2026-10-01",
    through: "2026-10-28",
    favorite: true,
    processTypes: ["frame_interpolation"],
    generationModes: ["lip_sync"],
  });
  expect(
    readAssetLocation(new URLSearchParams(writeAssetLocation(restored))),
  ).toEqual(restored);
});

it("restores legacy canvas and rejects invalid or type-incompatible URL conditions", () => {
  const restored = readAssetLocation(
    new URLSearchParams(
      "view=canvas&collection=legacy&sort=created&favorite=1&resolutions=8K&from=2026-02-30",
    ),
  );
  expect(restored).toMatchObject({
    view: "canvas",
    collection: "legacy",
    filters: { sort: "created", favorite: true, resolutions: [], from: "" },
  });
  expect(
    readAssetLocation(
      new URLSearchParams(
        "kind=audio&resolutions=8K&generation_modes=tts,bogus,tts",
      ),
    ).filters,
  ).toMatchObject({ resolutions: [], generationModes: ["tts"] });
});

it("round-trips reference-library conditions separately from generated media", () => {
  const query = new URLSearchParams(
    "view=reference&reference_kind=model&reference_order=name_asc&reference_recent=1&reference_folder=folder&q=山谷&favorite=1",
  );
  const restored = readAssetLocation(query);
  expect(restored.reference).toMatchObject({
    kind: "model",
    order: "name_asc",
    recent: true,
    folder: "folder",
    favorite: true,
    search: "山谷",
  });
  expect(
    readAssetLocation(new URLSearchParams(writeAssetLocation(restored)))
      .reference,
  ).toEqual(restored.reference);
  expect(
    readAssetLocation(
      new URLSearchParams("view=reference&favorite=1&reference_trash=1"),
    ).reference?.favorite,
  ).toBe(false);
});
