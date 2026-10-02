"use client";
import { useEffect, useRef, useState } from "react";
import { Canvas } from "@react-three/fiber";
import { OrbitControls } from "@react-three/drei";
import { Box3, Vector3 } from "three";
import type { GLTF } from "three/addons/loaders/GLTFLoader.js";
import {
  parseDirectorModel,
  readModelOriginal,
  disposeDirectorModel,
} from "@/components/canvas/director/model-file";

export function LibraryModelPreview({
  url,
  file,
  byteSize,
  mimeType,
  onError,
}: {
  byteSize: number;
  mimeType: string;
  onError: () => void;
} & ({ url: string; file?: never } | { file: File; url?: never })) {
  const source = file ?? url;
  const [loaded, setLoaded] = useState<{
    source: string | File;
    byteSize: number;
    mimeType: string;
    model: GLTF;
  }>();
  const model =
    loaded?.source === source &&
    loaded.byteSize === byteSize &&
    loaded.mimeType === mimeType
      ? loaded.model
      : undefined;
  const report = useRef(onError);
  useEffect(() => {
    report.current = onError;
  }, [onError]);
  useEffect(() => {
    const controller = new AbortController();
    let owned: GLTF | undefined;
    void (async () => {
      if (
        !Number.isSafeInteger(byteSize) ||
        byteSize < 1 ||
        byteSize > 64 * 1024 * 1024
      )
        throw new Error("模型超过预览容量或容量无效。");
      const data = file
        ? await file.arrayBuffer()
        : await readModelOriginal(
            await fetch(url, {
              signal: controller.signal,
              credentials: "omit",
              redirect: "error",
            }),
            byteSize,
            controller.signal,
          );
      if (data.byteLength !== byteSize) throw new Error("模型未完整读取。");
      if (controller.signal.aborted) return;
      const gltf = await parseDirectorModel(data, mimeType);
      owned = gltf;
      if (controller.signal.aborted) {
        disposeDirectorModel(gltf);
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
      setLoaded({ source, byteSize, mimeType, model: gltf });
    })().catch(() => {
      if (!controller.signal.aborted) report.current();
    });
    return () => {
      controller.abort();
      if (owned) disposeDirectorModel(owned);
    };
  }, [url, file, source, byteSize, mimeType]);
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
