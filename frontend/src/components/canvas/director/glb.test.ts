import { describe, expect, it } from "vitest";
import { validateDirectorGLB } from "./glb";
function glb(json: object) {
  const text = new TextEncoder().encode(JSON.stringify(json));
  const length = Math.ceil(text.length / 4) * 4;
  const buffer = new ArrayBuffer(20 + length);
  const view = new DataView(buffer);
  view.setUint32(0, 0x46546c67, true);
  view.setUint32(4, 2, true);
  view.setUint32(8, buffer.byteLength, true);
  view.setUint32(12, length, true);
  view.setUint32(16, 0x4e4f534a, true);
  const bytes = new Uint8Array(buffer, 20);
  bytes.fill(32);
  bytes.set(text);
  return buffer;
}
describe("授权模型的封闭资源检查", () => {
  it("接受 GLB v2 包内资源和嵌入 data URI", () => {
    expect(() =>
      validateDirectorGLB(
        glb({
          asset: { version: "2.0" },
          buffers: [{ byteLength: 0 }],
          images: [{ uri: "data:image/png;base64,AA==" }],
        }),
      ),
    ).not.toThrow();
  });
  it("拒绝外部、相对地址和解码器扩展，避免模型触发额外网络请求", () => {
    for (const uri of [
      "https://example.com/image.png",
      "image.png",
      "//example.com/image.png",
    ])
      expect(() =>
        validateDirectorGLB(
          glb({ asset: { version: "2.0" }, images: [{ uri }] }),
        ),
      ).toThrow();
    expect(() =>
      validateDirectorGLB(
        glb({
          asset: { version: "2.0" },
          extensionsRequired: ["KHR_draco_mesh_compression"],
        }),
      ),
    ).toThrow();
  });
  it("拒绝损坏容器长度和超出上传边界的原件", () => {
    const bad = glb({ asset: { version: "2.0" } });
    new DataView(bad).setUint32(8, 1, true);
    expect(() => validateDirectorGLB(bad)).toThrow();
    expect(() =>
      validateDirectorGLB(new ArrayBuffer(64 * 1024 * 1024 + 1)),
    ).toThrow();
  });
});
