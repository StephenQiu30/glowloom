import { describe, expect, it } from "vitest";
import { assertCanvasSubjectSourcePreview } from "@/components/canvas/lib/canvas-subject-connection-preview";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import { setSubjectConfig } from "@/components/canvas/subject/subject-node-state";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";

function sources(types: string[]) {
  return types.map((type) => {
    const node = createCanvasNode(type, { x: 0, y: 0 });
    node.serviceData = {
      id: node.id,
      node_type: type,
      media_ref: { asset_id: crypto.randomUUID() },
    };
    return node;
  });
}
function target() {
  return createCanvasNode("subject", { x: 0, y: 0 });
}
function localTarget(
  references = 0,
  voice = false,
  kind: "image" | "video" = "image",
) {
  return setSubjectConfig(target(), {
    version: 1,
    local: {
      description: "保留说明",
      primary: { asset_id: crypto.randomUUID(), asset_revision: 1, kind },
      references: Array.from({ length: references }, () => ({
        asset_id: crypto.randomUUID(),
        asset_revision: 1,
        kind: "image",
      })),
      ...(voice
        ? {
            voice: {
              name: "已有音色",
              media: {
                asset_id: crypto.randomUUID(),
                asset_revision: 1,
                kind: "audio",
              },
            },
          }
        : {}),
    },
  });
}
describe("subject connection structural preview", () => {
  it("permits a complete image composition while marking unknown audio duration for resolution", () => {
    expect(
      assertCanvasSubjectSourcePreview(
        sources(["audio", "image", "image", "image", "image"]),
      ),
    ).toEqual({
      requiresMediaValidation: true,
      requiresSubjectValidation: false,
    });
    expect(assertCanvasSubjectSourcePreview(sources(["image"]))).toEqual({
      requiresMediaValidation: false,
      requiresSubjectValidation: false,
    });
  });
  it.each([
    ["video", "audio"],
    ["image", "video"],
    ["video", "image"],
    ["image", "audio", "audio"],
    ["image", "image", "image", "image", "image"],
    ["audio"],
    ["text"],
    [],
  ])("rejects definitely invalid new-target combination %j", (...types) => {
    expect(() => assertCanvasSubjectSourcePreview(sources(types))).toThrow();
  });
  it("rejects duplicates and loading/failed sources before opening an enabled candidate", () => {
    const [source] = sources(["image"]);
    expect(() => assertCanvasSubjectSourcePreview([source, source])).toThrow();
    const [other] = sources(["image"]);
    other.serviceData!.media_ref = source.serviceData!.media_ref;
    expect(() => assertCanvasSubjectSourcePreview([source, other])).toThrow();
    for (const status of ["loading", "error"] as const) {
      source.metadata = { ...source.metadata, status };
      expect(() => assertCanvasSubjectSourcePreview([source])).toThrow();
    }
  });
  it("uses existing target capacity and does not mutate its configuration", () => {
    const full = localTarget(3),
      before = structuredClone(full);
    expect(() =>
      assertCanvasSubjectSourcePreview(sources(["image"]), { target: full }),
    ).toThrow();
    expect(
      assertCanvasSubjectSourcePreview(sources(["audio"]), { target: full })
        .requiresMediaValidation,
    ).toBe(true);
    expect(() =>
      assertCanvasSubjectSourcePreview(sources(["audio"]), {
        target: localTarget(0, true),
      }),
    ).toThrow();
    expect(() =>
      assertCanvasSubjectSourcePreview(sources(["image"]), {
        target: localTarget(0, false, "video"),
      }),
    ).toThrow();
    expect(full).toEqual(before);
  });
  it("rejects a reused existing asset and a source that is the target", () => {
    const t = localTarget(),
      [source] = sources(["image"]);
    source.serviceData!.media_ref!.asset_id =
      t.serviceData!.config!.subject!.local!.primary!.asset_id;
    expect(() =>
      assertCanvasSubjectSourcePreview([source], { target: t }),
    ).toThrow();
    source.id = t.id;
    expect(() =>
      assertCanvasSubjectSourcePreview([source], { target: t }),
    ).toThrow();
  });
  it("defers unknown pinned capacity and validates an exact snapshot when supplied", () => {
    const subject: UnifiedSubject = {
      id: crypto.randomUUID(),
      version_id: crypto.randomUUID(),
      owner_scope_id: crypto.randomUUID(),
      revision: 1,
      name: "固定主体",
      description: "",
      primary: {
        binding_id: crypto.randomUUID(),
        asset_id: crypto.randomUUID(),
        asset_revision: 1,
        kind: "image",
        file_name: "a.png",
        mime_type: "image/png",
      },
      references: [],
      favorite: false,
      created_at: "2026-10-29T00:00:00Z",
      updated_at: "2026-10-29T00:00:00Z",
    };
    const t = setSubjectConfig(target(), {
      version: 1,
      subject_id: subject.id,
      version_id: subject.version_id,
    });
    expect(
      assertCanvasSubjectSourcePreview(sources(["image"]), { target: t }),
    ).toEqual({
      requiresMediaValidation: false,
      requiresSubjectValidation: true,
    });
    expect(
      assertCanvasSubjectSourcePreview(sources(["audio"]), { target: t })
        .requiresSubjectValidation,
    ).toBe(true);
    expect(() =>
      assertCanvasSubjectSourcePreview(sources(["video"]), { target: t }),
    ).toThrow();
    expect(
      assertCanvasSubjectSourcePreview(sources(["image"]), {
        target: t,
        pinnedSubject: subject,
      }).requiresMediaValidation,
    ).toBe(false);
    expect(() =>
      assertCanvasSubjectSourcePreview(sources(["image"]), {
        target: t,
        pinnedSubject: { ...subject, version_id: crypto.randomUUID() },
      }),
    ).toThrow();
  });
  it.each([
    ["audio", 1999],
    ["audio", 5001],
    ["video", 15500],
    ["video", 0],
  ])("rejects known invalid %s duration %i", (type, duration) => {
    const selected = sources(type === "audio" ? ["image", type] : [type]);
    selected.at(-1)!.metadata = { durationMs: duration };
    expect(() => assertCanvasSubjectSourcePreview(selected)).toThrow();
  });
  it.each([
    ["audio", 2000],
    ["audio", 5000],
    ["video", 15499],
  ])("accepts known %s duration boundary %i", (type, duration) => {
    const selected = sources(type === "audio" ? ["image", type] : [type]);
    selected.at(-1)!.metadata = { durationMs: duration };
    expect(assertCanvasSubjectSourcePreview(selected)).toEqual({
      requiresMediaValidation: false,
      requiresSubjectValidation: false,
    });
  });
  it("uses authoritative resolved duration instead of stale display metadata", () => {
    const selected = sources(["video"]),
      source = selected[0],
      id = source.serviceData!.media_ref!.asset_id!;
    source.metadata = { durationMs: 60000 };
    const assets = new Map<string, API.AssetSummary>([
      [id, { id, kind: "video", revision: 1, duration_ms: 10000 }],
    ]);
    expect(assertCanvasSubjectSourcePreview(selected, { assets })).toEqual({
      requiresMediaValidation: false,
      requiresSubjectValidation: false,
    });
    assets.get(id)!.duration_ms = 16000;
    expect(() =>
      assertCanvasSubjectSourcePreview(selected, { assets }),
    ).toThrow();
  });
});
