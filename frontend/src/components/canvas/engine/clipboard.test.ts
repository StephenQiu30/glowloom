import { afterEach, expect, it, vi } from "vitest";
import { createNode } from "../document";
import { CanvasNodeType } from "../model";
import * as clipboard from "./clipboard";
import { animatedGIFFile } from "../../media/gif-test-fixtures";

const projectId = "cdad8ff4-c265-4c1d-ab65-9413f8dc051b";
const otherProject = "9590a20d-1893-40c1-a8ae-a501d2186cfb";
afterEach(() => vi.unstubAllGlobals());

it("剪贴板同时有PNG和GIF时优先完整GIF原件，不转换或丢掉动画帧", async () => {
  const original = animatedGIFFile();
  const getType = vi.fn(async () => original);
  const files = await clipboard.readCanvasClipboardImages([
    { types: ["image/png", "image/gif"], getType } as unknown as ClipboardItem,
  ]);
  expect(getType).toHaveBeenCalledExactlyOnceWith("image/gif");
  expect(files).toHaveLength(1);
  expect(files[0].type).toBe("image/gif");
  expect(files[0].name).toMatch(/\.gif$/);
  expect(await files[0].arrayBuffer()).toEqual(await original.arrayBuffer());
});

it("剪贴板GIF读取失败不得退回PNG，非图片仍留给图及纯文本粘贴", async () => {
  const getType = vi.fn().mockRejectedValue(new Error("permission denied"));
  await expect(
    clipboard.readCanvasClipboardImages([
      {
        types: ["image/png", "image/gif"],
        getType,
      } as unknown as ClipboardItem,
    ]),
  ).rejects.toThrow("读取剪贴板图片失败");
  expect(getType).toHaveBeenCalledExactlyOnceWith("image/gif");
  expect(
    await clipboard.readCanvasClipboardImages([
      { types: ["text/plain"], getType } as unknown as ClipboardItem,
    ]),
  ).toEqual([]);
});
function graph() {
  const group = createNode(CanvasNodeType.Frame, { x: 20, y: 30 });
  const text = createNode(CanvasNodeType.Text, { x: 70, y: 80 });
  text.parentId = group.id;
  text.metadata = { content: "一段文字", locked: true };
  const image = createNode(CanvasNodeType.Image, { x: 120, y: 130 });
  image.assetId = "95368d09-c39a-4fe1-a1b9-56765a85af8d";
  image.media = {
    url: "https://private.invalid/temporary?secret=never-copy",
    name: "runtime",
    mimeType: "image/png",
  };
  return {
    nodes: [group, text, image],
    connections: [
      { id: crypto.randomUUID(), fromNodeId: text.id, toNodeId: image.id },
    ],
  };
}
it("复制投影不携带临时媒体地址或非配置metadata", () => {
  const source = graph();
  const copied = clipboard.copySelection(
    source.nodes,
    source.connections,
    new Set(source.nodes.map((n) => n.id)),
  );
  expect(copied.nodes[2].media).toBeUndefined();
  expect(copied.nodes[1].metadata).toEqual({ content: "一段文字" });
  expect(source.nodes[2].media).toBeDefined();
});
it("版本化图经系统剪贴板在新实例读取并重建UUID和内部关系", async () => {
  let value = "";
  vi.stubGlobal("navigator", {
    clipboard: {
      writeText: vi.fn(async (text: string) => {
        value = text;
      }),
      readText: vi.fn(async () => value),
    },
  });
  const source = graph();
  await clipboard.writeCanvasClipboard(source, projectId);
  expect(value).not.toContain("private.invalid");
  expect(value).not.toContain("locked");
  const restored = await clipboard.readCanvasClipboard(projectId);
  const pasted = clipboard.pasteSelection(restored);
  expect(
    pasted.nodes.every((n) => !source.nodes.some((old) => old.id === n.id)),
  ).toBe(true);
  expect(pasted.nodes[1].parentId).toBe(pasted.nodes[0].id);
  expect(pasted.connections[0].fromNodeId).toBe(pasted.nodes[1].id);
  expect(pasted.connections[0].id).not.toBe(source.connections[0].id);
  expect(pasted.nodes[2].assetId).toBe(source.nodes[2].assetId);
  expect(pasted.nodes[2].media).toBeUndefined();
});
it("跨项目拒绝图粘贴，原生纯文本可作为text节点", () => {
  const encoded = clipboard.encodeCanvasClipboard(graph(), projectId);
  expect(() => clipboard.decodeCanvasClipboard(encoded, otherProject)).toThrow(
    /项目/,
  );
  const decoded = clipboard.decodeCanvasClipboard(
    "原生文字\nhttps://example.com",
    projectId,
    { x: 10, y: 20 },
  );
  expect(decoded.nodes).toHaveLength(1);
  expect(decoded.nodes[0].type).toBe(CanvasNodeType.Text);
  expect(decoded.nodes[0].metadata?.content).toBe(
    "原生文字\nhttps://example.com",
  );
  expect(decoded.nodes[0].position).toEqual({ x: 10, y: 20 });
});
it("拒绝未知版本字段、任意URL、坏UUID和无效父关系", () => {
  const encoded = clipboard.encodeCanvasClipboard(graph(), projectId);
  const original = JSON.parse(encoded);
  const corrupt = [
    (value: typeof original) => {
      value.version = 2;
    },
    (value: typeof original) => {
      value.untrusted = "not allowed";
    },
    (value: typeof original) => {
      value.nodes[2].url = "blob:bad";
    },
    (value: typeof original) => {
      value.nodes[1].config.url = "https://bad.invalid";
    },
    (value: typeof original) => {
      value.nodes[0].id = "invalid";
    },
    (value: typeof original) => {
      value.nodes[1].parentId = value.nodes[1].id;
    },
    (value: typeof original) => {
      value.nodes[0].parentId = value.nodes[1].id;
    },
    (value: typeof original) => {
      value.connections[0].toNodeId = crypto.randomUUID();
    },
    (value: typeof original) => {
      value.nodes[0].width = 100001;
    },
    (value: typeof original) => {
      value.nodes[1].position.x = 1000001;
    },
    (value: typeof original) => {
      value.nodes[1].assetId = crypto.randomUUID();
    },
  ];
  for (const mutate of corrupt) {
    const value = structuredClone(original);
    mutate(value);
    expect(() =>
      clipboard.decodeCanvasClipboard(JSON.stringify(value), projectId),
    ).toThrow(/剪贴板/);
  }
});
it("限制图JSON、文本、节点及连线容量，并拒绝重复身份", () => {
  const original = JSON.parse(
    clipboard.encodeCanvasClipboard(graph(), projectId),
  );
  expect(() =>
    clipboard.decodeCanvasClipboard("x".repeat(1048577), projectId),
  ).toThrow();
  expect(() =>
    clipboard.decodeCanvasClipboard("x".repeat(10001), projectId),
  ).toThrow();
  for (const [field, count] of [
    ["nodes", 2001],
    ["connections", 4001],
  ] as const) {
    const value = {
      ...original,
      [field]: Array.from({ length: count }, () => original[field][0]),
    };
    expect(() =>
      clipboard.decodeCanvasClipboard(JSON.stringify(value), projectId),
    ).toThrow();
  }
  original.nodes.push(original.nodes[0]);
  expect(() =>
    clipboard.decodeCanvasClipboard(JSON.stringify(original), projectId),
  ).toThrow();
});
it("父环、重复边和节点未知动作不能通过投影，复制单独子节点会解除外部父关系", () => {
  const source = graph();
  const child = clipboard.copySelection(
    source.nodes,
    source.connections,
    new Set([source.nodes[1].id]),
  );
  expect(child.nodes[0].parentId).toBeUndefined();
  expect(() => clipboard.encodeCanvasClipboard(child, projectId)).not.toThrow();
  const original = JSON.parse(
    clipboard.encodeCanvasClipboard(source, projectId),
  );
  const secondGroup = {
    ...original.nodes[0],
    id: crypto.randomUUID(),
    parentId: original.nodes[0].id,
  };
  original.nodes[0].parentId = secondGroup.id;
  original.nodes.push(secondGroup);
  expect(() =>
    clipboard.decodeCanvasClipboard(JSON.stringify(original), projectId),
  ).toThrow();
  const duplicatedEdges = JSON.parse(
    clipboard.encodeCanvasClipboard(source, projectId),
  );
  duplicatedEdges.connections.push(duplicatedEdges.connections[0]);
  expect(() =>
    clipboard.decodeCanvasClipboard(JSON.stringify(duplicatedEdges), projectId),
  ).toThrow();
  const unknownAction = JSON.parse(
    clipboard.encodeCanvasClipboard(source, projectId),
  );
  unknownAction.nodes[0].node_action = "generate";
  expect(() =>
    clipboard.decodeCanvasClipboard(JSON.stringify(unknownAction), projectId),
  ).toThrow();
});
it("UTF8字节、Unicode字符和畸形图有独立容量边界", () => {
  const source = graph();
  source.nodes[1].metadata = { content: "😀".repeat(10000) };
  expect(() =>
    clipboard.encodeCanvasClipboard(source, projectId),
  ).not.toThrow();
  expect(() =>
    clipboard.decodeCanvasClipboard("😀".repeat(10001), projectId),
  ).toThrow();
  expect(() =>
    clipboard.decodeCanvasClipboard("字".repeat(349526), projectId),
  ).toThrow();
  expect(() =>
    clipboard.decodeCanvasClipboard(
      '{"format":"lanverse.canvas", broken',
      projectId,
    ),
  ).toThrow();
  source.nodes[1].title = "😀".repeat(128);
  expect(() =>
    clipboard.encodeCanvasClipboard(
      clipboard.pasteSelection(source),
      projectId,
    ),
  ).not.toThrow();
});
it("原生剪贴板权限失败显式失败，不返回页面内缓存", async () => {
  vi.stubGlobal("navigator", {
    clipboard: {
      writeText: vi.fn(async () => {
        throw new Error("denied with private details");
      }),
      readText: vi.fn(async () => {
        throw new Error("private details");
      }),
    },
  });
  await expect(
    clipboard.writeCanvasClipboard(graph(), projectId),
  ).rejects.toThrow(/剪贴板.*权限/);
  await expect(clipboard.readCanvasClipboard(projectId)).rejects.toThrow(
    /剪贴板.*权限/,
  );
  vi.stubGlobal("navigator", {});
  await expect(clipboard.readCanvasClipboard(projectId)).rejects.toThrow(
    /剪贴板.*不可用/,
  );
});
it("导演、生成与模型资源往返并重映射引用，运行地址不入剪贴板", () => {
  const model = createNode(
    CanvasNodeType.Model,
    { x: 0, y: 0 },
    crypto.randomUUID(),
    { assetId: crypto.randomUUID() },
  );
  const director = createNode(CanvasNodeType.Director, { x: 400, y: 0 });
  director.director!.objects.push({
    ...director.director!.objects[0],
    id: crypto.randomUUID(),
    kind: "model",
    assetId: model.assetId,
    sourceNodeId: model.id,
    builtinActor: undefined,
  });
  const generation = createNode(CanvasNodeType.Generation, { x: 800, y: 0 });
  generation.generation!.prompt = "场景描述";
  const encoded = clipboard.encodeCanvasClipboard(
    { nodes: [model, director, generation], connections: [] },
    projectId,
  );
  const decoded = clipboard.decodeCanvasClipboard(encoded, projectId);
  const pasted = clipboard.pasteSelection(decoded);
  expect(decoded.nodes[1].director).toEqual(
    JSON.parse(JSON.stringify(director.director)),
  );
  expect(pasted.nodes[1].director!.objects[1].sourceNodeId).toBe(
    pasted.nodes[0].id,
  );
  expect(pasted.nodes[1].director!.id).not.toBe(director.director!.id);
  expect(pasted.nodes[2].generation!.prompt).toBe("场景描述");
});
