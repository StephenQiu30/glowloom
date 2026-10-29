import { describe, expect, it } from "vitest";
import {
  directorTransform,
  editDirector,
  emptyDirectorConfig,
} from "@/components/canvas/director/director-model";
import {
  activeDirectorScene,
  migrateDirectorDocument,
  replaceDirectorScene,
  switchDirectorWorkspace,
  validateDirectorDocument,
} from "@/components/canvas/director/director-workspaces";
const populated = () =>
  editDirector(emptyDirectorConfig(), { type: "add-object", primitive: "box" });
describe("independent director workspaces", () => {
  it("migrates a legacy document deterministically across repeated reads", () => {
    const source = populated();
    expect(migrateDirectorDocument(source)).toEqual(
      migrateDirectorDocument(source),
    );
  });
  it("preserves hidden legacy animation while keeping the static workspace active", () => {
    const source = populated();
    source.objects[0].keyframes = [
      {
        id: crypto.randomUUID(),
        time: 2,
        transform: directorTransform([6, 0, 0]),
        easing: "linear",
      },
    ];
    const document = migrateDirectorDocument(source);
    expect(document.version).toBe(2);
    expect(document.mode).toBe("static");
    expect(document.workspaces.dynamic.objects[0].keyframes).toEqual(
      source.objects[0].keyframes,
    );
    expect(document.workspaces.static.objects[0].keyframes).toEqual([]);
    expect(source.version).toBe(1);
    expect(
      validateDirectorDocument(JSON.parse(JSON.stringify(document))),
    ).toEqual(document);
  });
  it("switches without copying and edits only the active workspace", () => {
    const document = migrateDirectorDocument(populated());
    const dynamic = switchDirectorWorkspace(document, "dynamic", false, 0);
    expect(activeDirectorScene(dynamic).objects).toEqual([]);
    const edited = replaceDirectorScene(
      dynamic,
      editDirector(activeDirectorScene(dynamic), {
        type: "add-object",
        primitive: "sphere",
      }),
    );
    expect(edited.workspaces.static).toEqual(document.workspaces.static);
    expect(edited.workspaces.dynamic.objects[0].primitive).toBe("sphere");
  });
  it("syncs layout while preserving dynamic animation and rejecting missing referenced objects", () => {
    let document = migrateDirectorDocument(populated());
    document = switchDirectorWorkspace(document, "dynamic", true, 0);
    document.workspaces.dynamic.objects[0].keyframes = [
      {
        id: crypto.randomUUID(),
        time: 1,
        transform: directorTransform([3, 0, 0]),
        easing: "linear",
      },
    ];
    document = switchDirectorWorkspace(document, "static", false, 0);
    document.workspaces.static.objects[0].transform.position[0] = 9;
    const synced = switchDirectorWorkspace(document, "dynamic", true, 0);
    expect(synced.workspaces.dynamic.objects[0].transform.position[0]).toBe(9);
    expect(synced.workspaces.dynamic.objects[0].keyframes).toHaveLength(1);
    document.workspaces.static.objects = [];
    expect(() => switchDirectorWorkspace(document, "dynamic", true, 0)).toThrow(
      "时间轴",
    );
  });
  it("freezes the requested frame and strips animation only from the static snapshot", () => {
    const document = switchDirectorWorkspace(
      migrateDirectorDocument(populated()),
      "dynamic",
      true,
      0,
    );
    document.workspaces.dynamic.objects[0].keyframes = [
      {
        id: crypto.randomUUID(),
        time: 0,
        transform: directorTransform([0, 0, 0]),
        easing: "linear",
      },
      {
        id: crypto.randomUUID(),
        time: 4,
        transform: directorTransform([8, 0, 0]),
        easing: "linear",
      },
    ];
    const frozen = switchDirectorWorkspace(document, "static", true, 2);
    expect(frozen.workspaces.static.objects[0].transform.position[0]).toBe(4);
    expect(frozen.workspaces.static.objects[0].keyframes).toEqual([]);
    expect(frozen.workspaces.dynamic).toEqual(document.workspaces.dynamic);
  });
  it("rejects malformed inactive scenes, unknown versions and mixed legacy fields", () => {
    const document = migrateDirectorDocument(populated());
    document.workspaces.dynamic.cameras = [];
    expect(() => validateDirectorDocument(document)).toThrow();
    expect(() =>
      validateDirectorDocument({ ...populated(), version: 99 }),
    ).toThrow();
    expect(() =>
      validateDirectorDocument({
        ...migrateDirectorDocument(populated()),
        objects: [],
      }),
    ).toThrow();
  });
});

it("rejects synchronization that would remove an animated camera without a shot", () => {
  let document = switchDirectorWorkspace(
    migrateDirectorDocument(populated()),
    "dynamic",
    true,
    0,
  );
  let dynamic = editDirector(activeDirectorScene(document), {
    type: "add-camera",
  });
  dynamic = editDirector(dynamic, {
    type: "keyframe",
    selection: { kind: "camera", id: dynamic.cameras.at(-1)!.id },
    time: 1,
    transform: directorTransform([1, 2, 3]),
  });
  document = replaceDirectorScene(document, dynamic);
  document = switchDirectorWorkspace(document, "static", false, 0);
  expect(() => switchDirectorWorkspace(document, "dynamic", true, 0)).toThrow(
    "缺失的机位",
  );
});
