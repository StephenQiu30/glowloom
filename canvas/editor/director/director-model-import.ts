import type { AnimationClip, Object3D } from "three";

// Match the existing project media upload limit, including the converted GLB.
export const DIRECTOR_MODEL_MAX_BYTES = 64 * 1024 * 1024;
export const DIRECTOR_MODEL_ACCEPT = ".glb,.gltf,.fbx,.obj";
export type DirectorModelImport = {
  file: File;
  meshes: number;
  triangles: number;
  animations: string[];
};

function validateSize(size: number) {
  if (!size) throw new Error("不能导入空文件");
  if (size > DIRECTOR_MODEL_MAX_BYTES)
    throw new Error("模型文件及转换结果不能超过 64 MiB");
}

function waitForImport<T>(
  pending: Promise<T>,
  signal?: AbortSignal,
): Promise<T> {
  if (!signal) return pending;
  return new Promise((resolve, reject) => {
    const abort = () =>
      reject(signal.reason ?? new DOMException("Import aborted", "AbortError"));
    if (signal.aborted) abort();
    else signal.addEventListener("abort", abort, { once: true });
    pending
      .then(resolve, reject)
      .finally(() => signal.removeEventListener("abort", abort));
  });
}

function validateURIs(value: unknown) {
  if (!value || typeof value !== "object") return;
  for (const [key, child] of Object.entries(value)) {
    if (
      key === "uri" &&
      (typeof child !== "string" || !/^data:[^,]*,/i.test(child))
    )
      throw new Error("模型包含外部引用，请将贴图和缓冲数据嵌入文件后重试");
    validateURIs(child);
  }
}

function validateGLTF(bytes: ArrayBuffer, binary: boolean) {
  let json: string;
  if (binary) {
    const view = new DataView(bytes);
    if (
      bytes.byteLength < 20 ||
      view.getUint32(0, true) !== 0x46546c67 ||
      view.getUint32(4, true) !== 2 ||
      view.getUint32(8, true) !== bytes.byteLength ||
      view.getUint32(16, true) !== 0x4e4f534a
    )
      throw new Error("GLB 文件头无效，只支持 glTF 2.0");
    const size = view.getUint32(12, true);
    if (!size || size % 4 || size > bytes.byteLength - 20)
      throw new Error("GLB JSON 数据不完整");
    json = new TextDecoder().decode(new Uint8Array(bytes, 20, size));
  } else json = new TextDecoder().decode(bytes);
  let document: { asset?: { version?: string }; buffers?: { uri?: string }[] };
  try {
    document = JSON.parse(json);
  } catch {
    throw new Error("glTF JSON 无法解析");
  }
  if (!document || document.asset?.version !== "2.0")
    throw new Error("只支持 glTF 2.0 模型");
  validateURIs(document);
  if (!binary && document.buffers?.some((buffer) => !buffer.uri))
    throw new Error("glTF 缓冲数据必须内嵌，不能使用外部引用");
}

/** Local conversion only. Upload and document writes remain owned by the caller. */
export async function convertDirectorModel(
  file: File,
  signal?: AbortSignal,
): Promise<DirectorModelImport> {
  signal?.throwIfAborted();
  validateSize(file.size);
  const format = file.name.split(".").at(-1)?.toLowerCase();
  if (!["glb", "gltf", "fbx", "obj"].includes(format ?? ""))
    throw new Error("请选择 GLB、glTF、FBX 或 OBJ 模型");
  const bytes = await file.arrayBuffer();
  signal?.throwIfAborted();
  validateSize(bytes.byteLength);
  if (format === "glb" || format === "gltf")
    validateGLTF(bytes, format === "glb");
  const text =
    format === "obj" || format === "fbx" ? new TextDecoder().decode(bytes) : "";
  if (format === "obj" && /^\s*(mtllib|call)\s+/im.test(text))
    throw new Error("OBJ 包含外部引用，请移除 MTL 或嵌套文件引用后重试");
  // FBXLoader creates blob URLs for embedded binary textures. Reject any blob URL
  // supplied by the file itself before permitting those loader-owned resources.
  if (format === "fbx" && /blob:/i.test(text))
    throw new Error("FBX 包含未知的 blob 外部引用");

  const THREE = await import("three");
  signal?.throwIfAborted();
  const manager = new THREE.LoadingManager();
  const abortLoads = () => manager.abort();
  signal?.addEventListener("abort", abortLoads, { once: true });
  const urls = new Set<string>();
  let resourceFailure = "",
    started = false;
  let finishResources: () => void = () => {};
  const resources = new Promise<void>((resolve) => {
    finishResources = resolve;
  });
  manager.onStart = () => {
    started = true;
  };
  manager.onLoad = () => finishResources();
  manager.onError = () => {
    resourceFailure = "模型贴图读取失败，请检查内嵌资源";
  };
  manager.setURLModifier((url) => {
    signal?.throwIfAborted();
    if (/^data:[^,]*,/i.test(url)) return url;
    if (url.startsWith("blob:")) {
      urls.add(url);
      return url;
    }
    resourceFailure = "模型包含外部引用，请使用自包含模型文件";
    throw new Error(resourceFailure);
  });
  function dispose(model: Object3D) {
    const disposed = new Set<unknown>();
    model.traverse((node) => {
      if (!(node instanceof THREE.Mesh)) return;
      if (!disposed.has(node.geometry)) {
        disposed.add(node.geometry);
        node.geometry.dispose();
      }
      const materials = Array.isArray(node.material)
        ? node.material
        : [node.material];
      for (const material of materials) {
        if (disposed.has(material)) continue;
        disposed.add(material);
        for (const value of Object.values(material)) {
          if (value instanceof THREE.Texture && !disposed.has(value)) {
            disposed.add(value);
            const image = value.source?.data;
            if (
              typeof ImageBitmap !== "undefined" &&
              image instanceof ImageBitmap &&
              !disposed.has(image)
            ) {
              disposed.add(image);
              image.close();
            }
            value.dispose();
          }
        }
        material.dispose();
      }
      if (node instanceof THREE.SkinnedMesh && !disposed.has(node.skeleton)) {
        disposed.add(node.skeleton);
        node.skeleton.dispose();
      }
    });
  }
  let root: Object3D | undefined;
  let animations: AnimationClip[] = [];
  try {
    if (format === "glb" || format === "gltf") {
      const { GLTFLoader } = await import("three/addons/loaders/GLTFLoader.js");
      signal?.throwIfAborted();
      const loader = new GLTFLoader(manager).register((parser) => ({
        name: "director_import_requirements",
        beforeRoot: () => {
          for (const extension of parser.json.extensionsRequired ?? [])
            if (!parser.extensions[extension] && !parser.plugins[extension])
              throw new Error(`模型使用了尚不支持的必需扩展：${extension}`);
          return null;
        },
      }));
      const parsed = await waitForImport(
        loader.parseAsync(bytes, "").then((parsed) => {
          if (signal?.aborted) {
            dispose(parsed.scene);
            signal.throwIfAborted();
          }
          return parsed;
        }),
        signal,
      );
      root = parsed.scene;
      animations = parsed.animations;
    } else if (format === "fbx") {
      const { FBXLoader } = await import("three/addons/loaders/FBXLoader.js");
      signal?.throwIfAborted();
      root = new FBXLoader(manager).parse(bytes, "");
      animations = root.animations;
      // FBX scene units are centimetres in the director import contract.
      const wrapper = new THREE.Group();
      wrapper.scale.setScalar(0.01);
      wrapper.add(root);
      root = wrapper;
    } else {
      const { OBJLoader } = await import("three/addons/loaders/OBJLoader.js");
      signal?.throwIfAborted();
      root = new OBJLoader(manager).parse(text);
    }
    if (started) await waitForImport(resources, signal);
    signal?.throwIfAborted();
    if (resourceFailure) throw new Error(resourceFailure);
    root.updateMatrixWorld(true);
    let meshes = 0,
      triangles = 0;
    root.traverse((node) => {
      if (!(node instanceof THREE.Mesh)) return;
      const materials = Array.isArray(node.material)
        ? node.material
        : [node.material];
      for (const material of materials)
        for (const value of Object.values(material))
          if (value instanceof THREE.Texture && !value.source?.data)
            throw new Error("模型贴图未完整读取，请使用支持的内嵌图片");
      const position = node.geometry.getAttribute("position");
      if (!position || !position.count) return;
      for (let index = 0; index < position.count; index++) {
        if (
          ![
            position.getX(index),
            position.getY(index),
            position.getZ(index),
          ].every(Number.isFinite)
        )
          throw new Error("模型包含无效的顶点坐标");
      }
      meshes++;
      triangles += Math.floor(
        (node.geometry.index?.count ?? position.count) / 3,
      );
    });
    if (!meshes || !triangles) throw new Error("模型中没有可用的三角网格");
    const bounds = new THREE.Box3().setFromObject(root);
    if (
      bounds.isEmpty() ||
      ![...bounds.min, ...bounds.max].every(Number.isFinite)
    )
      throw new Error("模型尺寸无效");
    // Existing GLB files retain their original supported glTF metadata.
    let output = bytes;
    if (format !== "glb") {
      const { GLTFExporter } =
        await import("three/addons/exporters/GLTFExporter.js");
      signal?.throwIfAborted();
      const exported = await waitForImport(
        new GLTFExporter().parseAsync(root, {
          binary: true,
          animations,
          onlyVisible: false,
        }),
        signal,
      );
      if (!(exported instanceof ArrayBuffer))
        throw new Error("模型转换未生成 GLB");
      output = exported;
    }
    signal?.throwIfAborted();
    validateSize(output.byteLength);
    validateGLTF(output, true);
    return {
      file: new File(
        [output],
        `${file.name.replace(/\.[^.]+$/, "").slice(0, 120) || "模型"}.glb`,
        { type: "model/gltf-binary" },
      ),
      meshes,
      triangles,
      animations: animations.map((clip) => clip.name),
    };
  } finally {
    // Parsing/exporting owns these temporary resources, never the viewport cache.
    signal?.removeEventListener("abort", abortLoads);
    if (root) dispose(root);
    for (const url of urls) URL.revokeObjectURL(url);
  }
}
