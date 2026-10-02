/** Synthetic triangle shared by model parser and upload contract tests. */
export function triangleDocument() {
  const binary = new Uint8Array(
    new Float32Array([0, 0, 0, 1, 0, 0, 0, 1, 0]).buffer,
  );
  const encoded = btoa(
    Array.from(binary, (value) => String.fromCharCode(value)).join(""),
  );
  return {
    asset: { version: "2.0" },
    scene: 0,
    scenes: [{ nodes: [0] }],
    nodes: [{ mesh: 0, name: "合成三角模型😀" }],
    buffers: [
      {
        byteLength: binary.byteLength,
        uri: `data:application/octet-stream;base64,${encoded}`,
      },
    ],
    bufferViews: [
      {
        buffer: 0,
        byteOffset: 0,
        byteLength: binary.byteLength,
        target: 34962,
      },
    ],
    accessors: [
      {
        bufferView: 0,
        componentType: 5126,
        count: 3,
        type: "VEC3",
        min: [0, 0, 0],
        max: [1, 1, 0],
      },
    ],
    meshes: [{ primitives: [{ attributes: { POSITION: 0 }, mode: 4 }] }],
  };
}
export function triangleJSON(doc: object = triangleDocument()) {
  return new TextEncoder().encode(JSON.stringify(doc)).buffer;
}
export function triangleGLB() {
  const doc = triangleDocument();
  const bin = new Uint8Array(
    new Float32Array([0, 0, 0, 1, 0, 0, 0, 1, 0]).buffer,
  );
  const json = new TextEncoder().encode(
    JSON.stringify({ ...doc, buffers: [{ byteLength: bin.byteLength }] }),
  );
  const padded = Math.ceil(json.length / 4) * 4;
  const buffer = new ArrayBuffer(20 + padded + 8 + bin.byteLength);
  const view = new DataView(buffer);
  view.setUint32(0, 0x46546c67, true);
  view.setUint32(4, 2, true);
  view.setUint32(8, buffer.byteLength, true);
  view.setUint32(12, padded, true);
  view.setUint32(16, 0x4e4f534a, true);
  new Uint8Array(buffer, 20, padded).fill(32);
  new Uint8Array(buffer, 20, json.length).set(json);
  view.setUint32(20 + padded, bin.byteLength, true);
  view.setUint32(24 + padded, 0x004e4942, true);
  new Uint8Array(buffer, 28 + padded, bin.byteLength).set(bin);
  return buffer;
}
