"use client";
import { Suspense, useCallback } from "react";
import { Canvas } from "@react-three/fiber";
import { Bounds, OrbitControls, Html, useBounds } from "@react-three/drei";
import type { Object3D } from "three";
import { AuthorizedModel } from "./asset-view";
export function ModelAssetPreview({
  projectId,
  assetId,
}: {
  projectId: string;
  assetId: string;
}) {
  return (
    <div className="h-[60dvh] min-h-72 w-full" aria-label="3D 模型预览">
      <Canvas
        frameloop="demand"
        camera={{ position: [4, 3, 6], fov: 45 }}
        dpr={[1, 1.5]}
      >
        <ambientLight intensity={1.4} />
        <directionalLight position={[4, 5, 6]} intensity={2} />
        <Suspense fallback={<Html center>正在载入模型…</Html>}>
          <Bounds fit clip observe margin={1.5}>
            <FittedModel projectId={projectId} assetId={assetId} />
          </Bounds>
        </Suspense>
        <OrbitControls makeDefault />
      </Canvas>
    </div>
  );
}

function FittedModel({
  projectId,
  assetId,
}: {
  projectId: string;
  assetId: string;
}) {
  const bounds = useBounds();
  const loaded = useCallback(
    (model: Object3D) => {
      bounds.refresh(model).clip().fit();
    },
    [bounds],
  );
  return (
    <AuthorizedModel
      projectId={projectId}
      assetId={assetId}
      onLoaded={loaded}
    />
  );
}
