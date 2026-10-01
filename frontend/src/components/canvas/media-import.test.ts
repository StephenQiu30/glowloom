import { expect, it } from "vitest";
import { applyCommands } from "./document";
import {
  mediaAssetsToCanvasNodes,
  validateCanvasMediaFiles,
} from "./media-import";
import type { MediaAsset } from "./queries";

const mib = 1024 * 1024;
const project = "bd60f9a4-7d27-4c61-acdd-0e49ac5b1bb4";
function file(name: string, type: string, size = 1) {
  const value = new File(["x"], name, { type });
  Object.defineProperty(value, "size", { value: size });
  return value;
}
const asset: MediaAsset = {
  id: "2f4136fb-c16e-49d2-a3ac-3d4c5a179884",
  project_id: project,
  kind: "image",
  file_name: "参考.png",
  mime_type: "image/png",
  byte_size: 100,
  width: 1920,
  height: 1080,
  revision: 1,
};
it("接受约定格式的大小上界，空 MIME 仍按扩展初筛", () => {
  for (const [name, mime, size] of [
    ["a.JPG", "image/jpeg", 20 * mib],
    ["a.png", "image/png", 20 * mib],
    ["a.webp", "", 20 * mib],
    ["a.mp4", "video/mp4", 500 * mib],
    ["a.mov", "video/quicktime", 500 * mib],
    ["a.mp3", "audio/mpeg", 100 * mib],
    ["a.wav", "audio/wav", 100 * mib],
    ["a.m4a", "audio/mp4", 100 * mib],
  ] as const)
    expect(validateCanvasMediaFiles([file(name, mime, size)]).valid).toBe(true);
});
it("拒绝错误类型、MIME冲突、超限及空文件，不凭扩展声称内容已校验", () => {
  for (const value of [
    file("a.zip", "application/zip"),
    file("a.png", "video/mp4"),
    file("a.png", "image/png", 20 * mib + 1),
    file("a.mp4", "video/mp4", 500 * mib + 1),
    file("a.wav", "audio/wav", 100 * mib + 1),
    file("a.png", "image/png", 0),
  ]) {
    expect(validateCanvasMediaFiles([value]).valid).toBe(false);
  }
});
it("整批最多20个、总计700MiB，且不能超过剩余画布节点", () => {
  expect(validateCanvasMediaFiles([]).valid).toBe(false);
  expect(
    validateCanvasMediaFiles(
      Array.from({ length: 21 }, () => file("a.png", "image/png")),
    ).valid,
  ).toBe(false);
  expect(validateCanvasMediaFiles([file("a.png", "image/png")], 0).valid).toBe(
    false,
  );
  expect(
    validateCanvasMediaFiles([
      file("a.mp4", "video/mp4", 500 * mib),
      file("b.mp4", "video/mp4", 200 * mib),
    ]).valid,
  ).toBe(true);
  expect(
    validateCanvasMediaFiles([
      file("a.mp4", "video/mp4", 500 * mib),
      file("b.mp4", "video/mp4", 200 * mib + 1),
    ]).valid,
  ).toBe(false);
});
it("宫格工具显式允许25个派生文件，普通上传仍限制20个", () => {
  const files = Array.from({ length: 25 }, () => file("宫格.png", "image/png"));
  expect(validateCanvasMediaFiles(files, 100, 25).valid).toBe(true);
  expect(validateCanvasMediaFiles(files, 24, 25).valid).toBe(false);
  expect(validateCanvasMediaFiles(files).valid).toBe(false);
});
it("资产映射只写正式身份，生成新UUID、Unicode标题和有界尺寸", () => {
  const source = {
    ...asset,
    file_name: "😀".repeat(140),
    url: "https://private.invalid/signed",
  };
  const nodes = mediaAssetsToCanvasNodes([source], { x: 10, y: 20 });
  expect(nodes[0].id).toMatch(/^[0-9a-f-]{36}$/);
  expect(nodes[0].id).not.toBe(asset.id);
  expect(nodes[0].position).toEqual({ x: 10, y: 20 });
  expect(Array.from(nodes[0].title)).toHaveLength(128);
  expect(nodes[0].width).toBeLessThanOrEqual(480);
  expect(nodes[0].height).toBeLessThanOrEqual(420);
  expect(nodes[0].assetId).toBe(asset.id);
  expect(JSON.stringify(nodes)).not.toContain("private.invalid");
  expect(nodes[0].media).toBeUndefined();
});
it("图片、视频和音频都能通过正式AddNodes校验，尺寸缺失使用有界默认值", () => {
  const assets = [
    asset,
    {
      ...asset,
      id: crypto.randomUUID(),
      kind: "video" as const,
      width: 1,
      height: 100000,
    },
    {
      ...asset,
      id: crypto.randomUUID(),
      kind: "audio" as const,
      width: undefined,
      height: undefined,
    },
  ];
  const nodes = mediaAssetsToCanvasNodes(assets, { x: 0, y: 0 });
  expect(nodes.map((node) => node.type)).toEqual(["image", "video", "audio"]);
  expect(new Set(nodes.map((node) => JSON.stringify(node.position))).size).toBe(
    3,
  );
  expect(() =>
    applyCommands(
      {
        id: crypto.randomUUID(),
        projectId: project,
        name: "画布",
        revision: 1,
        nodes: [],
        connections: [],
        scope: {},
        viewport: { x: 0, y: 0, k: 1 },
      },
      [{ type: "AddNodes", nodes }],
    ),
  ).not.toThrow();
});
it("拒绝跨项目或坏资产身份、非法尺寸和坐标，同一正式资产可有不同节点引用", () => {
  const repeated = mediaAssetsToCanvasNodes([asset, asset], { x: 0, y: 0 });
  expect(repeated[0].id).not.toBe(repeated[1].id);
  for (const assets of [
    [{ ...asset, id: "bad" }],
    [{ ...asset, project_id: "bad" }],
    [
      asset,
      { ...asset, id: crypto.randomUUID(), project_id: crypto.randomUUID() },
    ],
    [{ ...asset, width: NaN }],
  ]) {
    expect(() => mediaAssetsToCanvasNodes(assets, { x: 0, y: 0 })).toThrow();
  }
  expect(() =>
    mediaAssetsToCanvasNodes([asset], { x: Infinity, y: 0 }),
  ).toThrow();
  expect(() =>
    mediaAssetsToCanvasNodes([asset], { x: 1000001, y: 0 }),
  ).toThrow();
});
it("GLB 原件按 64MiB 初筛并只持久 model 资产身份", () => {
  expect(
    validateCanvasMediaFiles([file("actor.glb", "model/gltf-binary", 64 * mib)])
      .valid,
  ).toBe(true);
  expect(
    validateCanvasMediaFiles([
      file("actor.glb", "model/gltf-binary", 64 * mib + 1),
    ]).valid,
  ).toBe(false);
  expect(
    validateCanvasMediaFiles([file("actor.gltf", "model/gltf+json")]).valid,
  ).toBe(false);
  const node = mediaAssetsToCanvasNodes(
    [
      {
        ...asset,
        kind: "model",
        mime_type: "model/gltf-binary",
        file_name: "actor.glb",
        width: undefined,
        height: undefined,
      },
    ],
    { x: 0, y: 0 },
  )[0];
  expect(node.type).toBe("model");
  expect(node.assetId).toBe(asset.id);
  expect(node.media).toBeUndefined();
});

it("静音WebM只作原件初筛，真实VP8/VP9及MP4规范化交给服务端", () => {
  expect(
    validateCanvasMediaFiles([file("白膜.webm", "video/webm", 500 * mib)])
      .valid,
  ).toBe(true);
  expect(
    validateCanvasMediaFiles([file("白膜.webm", "video/webm", 500 * mib + 1)])
      .valid,
  ).toBe(false);
  expect(validateCanvasMediaFiles([file("白膜.mp4", "video/webm")]).valid).toBe(
    false,
  );
});
