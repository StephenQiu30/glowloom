"use client";
import { useEffect, useRef, useState } from "react";
import { Canvas } from "@react-three/fiber";
import { OrbitControls } from "@react-three/drei";
import {
  Box3,
  LoadingManager,
  Mesh,
  Vector3,
  type Material,
  type Texture,
} from "three";
import { GLTFLoader, type GLTF } from "three/addons/loaders/GLTFLoader.js";
import { validateDirectorGLB } from "@/components/canvas/director/glb";

export function LibraryModelPreview({
  url,
  byteSize,
  onError,
}: {
  url: string;
  byteSize: number;
  onError: () => void;
}) {
  const [model, setModel] = useState<GLTF>();
  const report = useRef(onError);
  useEffect(() => {
    report.current = onError;
  }, [onError]);
  useEffect(() => {
    const controller = new AbortController();
    let owned: GLTF | undefined;
    void (async () => {
      if (byteSize > 64 * 1024 * 1024) throw new Error("模型超过预览容量。");
      const response = await fetch(url, {
        signal: controller.signal,
        credentials: "omit",
        redirect: "error",
      });
      if (!response.ok || !response.body) throw new Error("模型读取失败。");
      const reader = response.body.getReader(),
        chunks: Uint8Array[] = [];
      let length = 0;
      try {
        while (true) {
          const part = await reader.read();
          if (part.done) break;
          length += part.value.byteLength;
          if (length > byteSize || length > 64 * 1024 * 1024)
            throw new Error("模型实际容量无效。");
          chunks.push(part.value);
        }
      } finally {
        if (length > byteSize) await reader.cancel();
        reader.releaseLock();
      }
      if (length !== byteSize || controller.signal.aborted)
        throw new Error("模型未完整读取。");
      const data = new Uint8Array(length);
      let offset = 0;
      for (const chunk of chunks) {
        data.set(chunk, offset);
        offset += chunk.byteLength;
      }
      validateDirectorGLB(data.buffer);
      const manager = new LoadingManager();
      manager.setURLModifier((resource) => {
        if (!resource.startsWith("data:") && !resource.startsWith("blob:"))
          throw new Error("模型包含包外资源。");
        return resource;
      });
      const gltf = await new GLTFLoader(manager).parseAsync(data.buffer, "");
      owned = gltf;
      if (controller.signal.aborted) {
        dispose(gltf);
        owned = undefined;
        return;
      }
      const box = new Box3().setFromObject(gltf.scene, true),
        size = box.getSize(new Vector3());
      gltf.scene.scale.multiplyScalar(
        2 / Math.max(size.x, size.y, size.z, 0.001),
      );
      gltf.scene.updateMatrixWorld(true);
      gltf.scene.position.sub(
        new Box3().setFromObject(gltf.scene, true).getCenter(new Vector3()),
      );
      setModel(gltf);
    })().catch(() => {
      if (!controller.signal.aborted) report.current();
    });
    return () => {
      controller.abort();
      if (owned) dispose(owned);
    };
  }, [url, byteSize]);
  return (
    <div
      className="h-[50dvh] min-h-72 w-full"
      role="region"
      aria-label="3D 模型，可拖动旋转和缩放"
    >
      {!model ? (
        <p role="status">正在完整读取并解析模型…</p>
      ) : (
        <Canvas
          frameloop="demand"
          camera={{ position: [3, 2, 4], fov: 45 }}
          dpr={[1, 1.5]}
        >
          <ambientLight intensity={1.4} />
          <directionalLight position={[4, 5, 6]} intensity={2} />
          <primitive object={model.scene} dispose={null} />
          <OrbitControls makeDefault />
        </Canvas>
      )}
    </div>
  );
}
function dispose(gltf: GLTF) {
  const materials = new Set<Material>(),
    textures = new Set<Texture>();
  gltf.scene.traverse((object) => {
    if (!(object instanceof Mesh)) return;
    object.geometry.dispose();
    for (const material of Array.isArray(object.material)
      ? object.material
      : [object.material]) {
      materials.add(material);
      for (const value of Object.values(material)) {
        if (value && typeof value === "object" && "isTexture" in value)
          textures.add(value as Texture);
      }
    }
  });
  textures.forEach((texture) => {
    const image: unknown = texture.source.data;
    if (typeof ImageBitmap !== "undefined" && image instanceof ImageBitmap)
      image.close();
    texture.dispose();
  });
  materials.forEach((material) => material.dispose());
}
