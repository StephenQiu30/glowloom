const maximum = 64 * 1024 * 1024;
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
  "KHR_materials_dispersion",
  "KHR_texture_transform",
  "KHR_mesh_quantization",
  "EXT_mesh_gpu_instancing",
  "EXT_materials_bump",
  "EXT_texture_webp",
  "EXT_texture_avif",
]);
function fail(): never {
  throw new Error(
    "模型必须为完整 GLB v2 或自包含 glTF 2.0 JSON，资源仅可在包内或嵌入 data URI。解码器扩展需先转换。",
  );
}
function jsonDocument(bytes: Uint8Array): unknown {
  try {
    return JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes));
  } catch {
    return fail();
  }
}
function containedDocument(json: unknown) {
  if (!json || typeof json !== "object" || !("asset" in json)) fail();
  const asset = json.asset;
  if (
    !asset ||
    typeof asset !== "object" ||
    !("version" in asset) ||
    asset.version !== "2.0" ||
    ("minVersion" in asset && asset.minVersion !== "2.0")
  )
    fail();
  let entries = 0;
  const visit = (value: unknown, depth = 0) => {
    if (++entries > 65536 || depth > 64) fail();
    if (!value || typeof value !== "object") return;
    if (Array.isArray(value)) {
      value.forEach((item) => visit(item, depth + 1));
      return;
    }
    for (const [key, item] of Object.entries(value)) {
      // Three retains extras as metadata; these fields are never resources.
      if (key === "extras") continue;
      if (
        key === "uri" &&
        (typeof item !== "string" ||
          !/^data:(?:application\/(?:octet-stream|gltf-buffer)|image\/(?:png|jpeg|webp|avif));base64,[a-zA-Z0-9+/]+={0,2}$/.test(
            item,
          ))
      )
        fail();
      if (
        (key === "extensionsRequired" || key === "extensionsUsed") &&
        (!Array.isArray(item) ||
          item.some(
            (extension) =>
              typeof extension !== "string" || !nativeExtensions.has(extension),
          ))
      )
        fail();
      if (
        key === "extensions" &&
        (!item ||
          typeof item !== "object" ||
          Array.isArray(item) ||
          Object.keys(item).some(
            (extension) => !nativeExtensions.has(extension),
          ))
      )
        fail();
      visit(item, depth + 1);
    }
  };
  visit(json);
}
/** The server owns admission; this bounded check prevents extra URL loads. */
export function validateDirectorGLB(buffer: ArrayBuffer): void {
  if (buffer.byteLength < 20 || buffer.byteLength > maximum) fail();
  const view = new DataView(buffer);
  if (
    view.getUint32(0, true) !== 0x46546c67 ||
    view.getUint32(4, true) !== 2 ||
    view.getUint32(8, true) !== buffer.byteLength
  )
    fail();
  let document: unknown;
  for (let offset = 12, chunk = 0; offset < buffer.byteLength; chunk++) {
    if (buffer.byteLength - offset < 8 || chunk > 15) fail();
    const length = view.getUint32(offset, true),
      kind = view.getUint32(offset + 4, true);
    offset += 8;
    if (
      length % 4 ||
      length > buffer.byteLength - offset ||
      (chunk === 0 &&
        (kind !== 0x4e4f534a || length === 0 || length > 4 * 1024 * 1024)) ||
      (chunk !== 0 && kind === 0x4e4f534a) ||
      (kind === 0x004e4942 && chunk !== 1)
    )
      fail();
    if (chunk === 0)
      document = jsonDocument(new Uint8Array(buffer, offset, length));
    offset += length;
  }
  containedDocument(document);
}
export function validateDirectorModel(
  buffer: ArrayBuffer,
  mimeType: string,
): void {
  if (mimeType === "model/gltf-binary") return validateDirectorGLB(buffer);
  if (
    mimeType !== "model/gltf+json" ||
    buffer.byteLength < 1 ||
    buffer.byteLength > maximum
  )
    fail();
  containedDocument(jsonDocument(new Uint8Array(buffer)));
}
