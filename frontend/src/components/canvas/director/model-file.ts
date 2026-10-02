import { BufferGeometry, LoadingManager, Material, Texture } from "three";
import { GLTFLoader, type GLTF } from "three/addons/loaders/GLTFLoader.js";
import { validateDirectorModel } from "./glb";

/** Parse the exact original container, with no decoders or remote resource base. */
export async function parseDirectorModel(
  buffer: ArrayBuffer,
  mimeType: string,
) {
  validateDirectorModel(buffer, mimeType);
  const manager = new LoadingManager();
  manager.setURLModifier((resource) => {
    if (!resource.startsWith("data:") && !resource.startsWith("blob:"))
      throw new Error("模型包含包外资源。");
    return resource;
  });
  const source =
    mimeType === "model/gltf+json"
      ? new TextDecoder("utf-8", { fatal: true }).decode(new Uint8Array(buffer))
      : buffer;
  return new GLTFLoader(manager).parseAsync(source, "");
}

/** Both private previews consume exact, bounded originals and release readers. */
export async function readModelOriginal(
  response: Response,
  byteSize: number,
  signal: AbortSignal,
) {
  if (
    !Number.isSafeInteger(byteSize) ||
    byteSize < 1 ||
    byteSize > 64 * 1024 * 1024
  )
    throw new Error("模型原件容量无效。");
  if (!response.ok || !response.body) throw new Error("模型原件读取失败。");
  const reader = response.body.getReader(),
    parts: Uint8Array[] = [];
  let length = 0;
  try {
    while (true) {
      if (signal.aborted) throw new DOMException("Aborted", "AbortError");
      const part = await reader.read();
      if (part.done) break;
      length += part.value.byteLength;
      if (length > byteSize) throw new Error("模型实际容量无效。");
      parts.push(part.value);
    }
  } finally {
    await reader.cancel();
    reader.releaseLock();
  }
  if (length !== byteSize) throw new Error("模型未完整读取。");
  const buffer = new Uint8Array(length);
  let offset = 0;
  for (const part of parts) {
    buffer.set(part, offset);
    offset += part.byteLength;
  }
  return buffer.buffer;
}

/** A model owns all parsed scenes, including shared mesh/line/point resources. */
export function disposeDirectorModel(gltf: GLTF) {
  const geometries = new Set<BufferGeometry>(),
    materials = new Set<Material>(),
    textures = new Set<Texture>();
  for (const scene of gltf.scenes ?? [gltf.scene])
    scene.traverse((object) => {
      if ("geometry" in object && object.geometry instanceof BufferGeometry)
        geometries.add(object.geometry);
      if (!("material" in object)) return;
      for (const material of Array.isArray(object.material)
        ? object.material
        : [object.material]) {
        if (!(material instanceof Material)) continue;
        materials.add(material);
        for (const value of Object.values(material))
          if (value instanceof Texture) textures.add(value);
      }
    });
  const bitmaps = new Set<ImageBitmap>();
  for (const texture of textures) {
    const image: unknown = texture.source.data;
    if (typeof ImageBitmap !== "undefined" && image instanceof ImageBitmap)
      bitmaps.add(image);
    texture.dispose();
  }
  bitmaps.forEach((image) => image.close());
  materials.forEach((material) => material.dispose());
  geometries.forEach((geometry) => geometry.dispose());
}
