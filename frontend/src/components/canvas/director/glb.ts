/** The server owns format admission. This check prevents a private model from loading extra URLs. */
export function validateDirectorGLB(buffer: ArrayBuffer): void {
  const fail = () => {
    throw new Error(
      "模型必须为完整 GLB v2，资源仅可在包内或嵌入 data URI。解码器扩展需先转换。",
    );
  };
  if (buffer.byteLength < 20 || buffer.byteLength > 64 * 1024 * 1024) fail();
  const view = new DataView(buffer);
  if (
    view.getUint32(0, true) !== 0x46546c67 ||
    view.getUint32(4, true) !== 2 ||
    view.getUint32(8, true) !== buffer.byteLength
  )
    fail();
  const length = view.getUint32(12, true);
  if (
    view.getUint32(16, true) !== 0x4e4f534a ||
    length % 4 ||
    20 + length > buffer.byteLength
  )
    fail();
  let json: unknown;
  try {
    json = JSON.parse(
      new TextDecoder("utf-8", { fatal: true }).decode(
        new Uint8Array(buffer, 20, length),
      ),
    );
  } catch {
    fail();
  }
  if (!json || typeof json !== "object" || !("asset" in json)) fail();
  const nativeExtensions = new Set([
    "KHR_lights_punctual",
    "KHR_materials_unlit",
    "KHR_materials_clearcoat",
    "KHR_materials_ior",
    "KHR_materials_sheen",
    "KHR_materials_specular",
    "KHR_materials_transmission",
    "KHR_materials_volume",
    "KHR_materials_emissive_strength",
    "KHR_materials_iridescence",
    "KHR_materials_anisotropy",
    "KHR_texture_transform",
    "KHR_mesh_quantization",
    "EXT_mesh_gpu_instancing",
  ]);
  const visit = (value: unknown) => {
    if (!value || typeof value !== "object") return;
    if (Array.isArray(value)) {
      value.forEach(visit);
      return;
    }
    for (const [key, item] of Object.entries(value)) {
      if (
        key === "uri" &&
        (typeof item !== "string" ||
          !/^data:[a-z0-9.+/-]+;base64,[a-z0-9+/=\s]*$/i.test(item))
      )
        fail();
      if (
        key === "extensionsRequired" &&
        (!Array.isArray(item) ||
          item.some(
            (extension) =>
              typeof extension !== "string" || !nativeExtensions.has(extension),
          ))
      )
        fail();
      if (
        [
          "KHR_draco_mesh_compression",
          "EXT_meshopt_compression",
          "KHR_texture_basisu",
        ].includes(key)
      )
        fail();
      visit(item);
    }
  };
  visit(json);
}
