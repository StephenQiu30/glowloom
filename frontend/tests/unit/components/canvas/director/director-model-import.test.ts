import { describe, expect, it, vi } from "vitest";
import {
  convertDirectorModel,
  DIRECTOR_MODEL_MAX_BYTES,
} from "@/components/canvas/director/director-model-import";

function file(name: string, content: string | Uint8Array) {
  const bytes =
    typeof content === "string" ? new TextEncoder().encode(content) : content;
  const value = new File([bytes as BlobPart], name);
  Object.defineProperty(value, "arrayBuffer", {
    value: async () => {
      const buffer = new ArrayBuffer(bytes.byteLength);
      new Uint8Array(buffer).set(bytes);
      return buffer;
    },
  });
  return value;
}
const triangle = "v 0 0 0\nv 1 0 0\nv 0 1 0\nf 1 2 3\n";

function readBytes(blob: Blob): Promise<ArrayBuffer> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as ArrayBuffer);
    reader.onerror = () => reject(reader.error);
    reader.readAsArrayBuffer(blob);
  });
}

const triangleFBX = `; FBX 7.4.0 project file
FBXHeaderExtension:  {
\tFBXVersion: 7400
}
Objects:  {
\tGeometry: 1, "Geometry::Triangle", "Mesh" {
\t\tVertices: *9 {
\t\t\ta: 0,0,0,100,0,0,0,100,0
\t\t}
\t\tPolygonVertexIndex: *3 {
\t\t\ta: 0,1,-3
\t\t}
\t}
\tModel: 2, "Model::Triangle", "Mesh" {
\t\tVersion: 232
\t}
}
Connections:  {
\tC: "OO",1,2
\tC: "OO",2,0
}
`;

describe("local director model import", () => {
  it("converts an actual embedded glTF and preserves valid GLB bytes on reimport", async () => {
    const bytes = new Uint8Array(
      new Float32Array([0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 1, 0, 0, 0, 1, 0, 0])
        .buffer,
    );
    const document = {
      asset: { version: "2.0" },
      scene: 0,
      scenes: [{ nodes: [0] }],
      nodes: [{ name: "Triangle", mesh: 0 }],
      meshes: [{ primitives: [{ attributes: { POSITION: 0 } }] }],
      animations: [
        {
          name: "Move",
          channels: [{ sampler: 0, target: { node: 0, path: "translation" } }],
          samplers: [{ input: 1, output: 2 }],
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
        {
          bufferView: 1,
          componentType: 5126,
          count: 2,
          type: "SCALAR",
          min: [0],
          max: [1],
        },
        { bufferView: 2, componentType: 5126, count: 2, type: "VEC3" },
      ],
      bufferViews: [
        { buffer: 0, byteLength: 36 },
        { buffer: 0, byteOffset: 36, byteLength: 8 },
        { buffer: 0, byteOffset: 44, byteLength: 24 },
      ],
      buffers: [
        {
          byteLength: bytes.byteLength,
          uri: `data:application/octet-stream;base64,${btoa(String.fromCharCode(...bytes))}`,
        },
      ],
    };
    const result = await convertDirectorModel(
      file("triangle.gltf", JSON.stringify(document)),
    );
    expect(result.triangles).toBe(1);
    expect(result.animations).toEqual(["Move"]);
    const output = new Uint8Array(await readBytes(result.file));
    const again = await convertDirectorModel(file("triangle.glb", output));
    expect(again.animations).toEqual(["Move"]);
    expect(new Uint8Array(await readBytes(again.file))).toEqual(output);
  });
  it("converts an actual FBX mesh from centimetres to metres", async () => {
    const result = await convertDirectorModel(
      file("triangle.fbx", triangleFBX),
    );
    expect(result.meshes).toBe(1);
    const { GLTFLoader } = await import("three/addons/loaders/GLTFLoader.js");
    const { Box3 } = await import("three");
    const decoded = await new GLTFLoader().parseAsync(
      await readBytes(result.file),
      "",
    );
    const bounds = new Box3().setFromObject(decoded.scene);
    expect(bounds.max.x).toBeCloseTo(1);
    expect(bounds.max.y).toBeCloseTo(1);
  });
  it("rejects a FBX external texture before any request", async () => {
    const source = triangleFBX
      .replace(
        "Objects:  {",
        `Objects:  {
\tVideo: 3, "Video::External", "Clip" {
\t\tFilename: "https://example.com/texture.png"
\t\tRelativeFilename: "https://example.com/texture.png"
\t}
\tTexture: 4, "Texture::External", "" {
\t\tFileName: "https://example.com/texture.png"
\t}`,
      )
      .replace("Connections:  {", 'Connections:  {\n\tC: "OO",3,4');
    const fetch = vi.spyOn(globalThis, "fetch");
    try {
      await expect(
        convertDirectorModel(file("external.fbx", source)),
      ).rejects.toThrow("外部引用");
      expect(fetch).not.toHaveBeenCalled();
    } finally {
      fetch.mockRestore();
    }
  });
  it("refuses unknown required glTF extensions instead of silently dropping them", async () => {
    const warning = vi.spyOn(console, "warn").mockImplementation(() => {});
    try {
      await expect(
        convertDirectorModel(
          file(
            "unsupported.gltf",
            JSON.stringify({
              asset: { version: "2.0" },
              extensionsUsed: ["VENDOR_unknown"],
              extensionsRequired: ["VENDOR_unknown"],
            }),
          ),
        ),
      ).rejects.toThrow("必需扩展");
    } finally {
      warning.mockRestore();
    }
  });
  it("converts an actual OBJ mesh to a GLB without network access", async () => {
    const fetch = vi.spyOn(globalThis, "fetch");
    try {
      const result = await convertDirectorModel(file("triangle.OBJ", triangle));
      expect(result.file.name).toBe("triangle.glb");
      expect(result.file.type).toBe("model/gltf-binary");
      expect(result.meshes).toBe(1);
      expect(result.triangles).toBe(1);
      expect(result.file.size).toBeGreaterThan(100);
      expect(fetch).not.toHaveBeenCalled();
    } finally {
      fetch.mockRestore();
    }
  });
  it.each(["mtllib external.mtl\n", "call nested.obj\n"])(
    "rejects OBJ external references before parsing: %s",
    async (reference) => {
      await expect(
        convertDirectorModel(file("bad.obj", reference + triangle)),
      ).rejects.toThrow("外部引用");
    },
  );
  it.each(["https://example.com/mesh.bin", "mesh.bin", "blob:other-resource"])(
    "rejects a non-embedded glTF buffer: %s",
    async (uri) => {
      await expect(
        convertDirectorModel(
          file(
            "bad.gltf",
            JSON.stringify({
              asset: { version: "2.0" },
              buffers: [{ uri, byteLength: 12 }],
            }),
          ),
        ),
      ).rejects.toThrow("外部引用");
    },
  );
  it("rejects empty, oversize and unsupported inputs before reading", async () => {
    await expect(convertDirectorModel(file("empty.obj", ""))).rejects.toThrow(
      "空文件",
    );
    const large = file("large.obj", triangle);
    Object.defineProperty(large, "size", {
      value: DIRECTOR_MODEL_MAX_BYTES + 1,
    });
    await expect(convertDirectorModel(large)).rejects.toThrow("64");
    await expect(
      convertDirectorModel(file("mesh.zip", triangle)),
    ).rejects.toThrow("GLB");
  });
  it("rejects broken GLB and texture-only OBJ", async () => {
    await expect(
      convertDirectorModel(file("bad.glb", triangle)),
    ).rejects.toThrow("GLB");
    await expect(
      convertDirectorModel(file("empty.obj", "# no mesh\n")),
    ).rejects.toThrow("网格");
  });
  it("rejects untrusted FBX blob references", async () => {
    await expect(
      convertDirectorModel(
        file("bad.fbx", "Kaydara FBX Binary  \0\x1a\0 blob:untrusted"),
      ),
    ).rejects.toThrow("外部引用");
  });
  it("honors an aborted operation before parsing", async () => {
    const controller = new AbortController();
    controller.abort();
    await expect(
      convertDirectorModel(file("mesh.obj", triangle), controller.signal),
    ).rejects.toMatchObject({ name: "AbortError" });
  });
});
