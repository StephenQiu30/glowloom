"use client";

import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useFrame } from "@react-three/fiber";
import { Html } from "@react-three/drei";
import {
  AnimationMixer,
  Box3,
  Vector3,
  Bone,
  DoubleSide,
  LoadingManager,
  Mesh,
  MeshDepthMaterial,
  MeshNormalMaterial,
  MeshStandardMaterial,
  Quaternion,
  SRGBColorSpace,
  TextureLoader,
  type Material,
  type Texture,
} from "three";
import { GLTFLoader, type GLTF } from "three/addons/loaders/GLTFLoader.js";
import { getMediaPreview } from "../queries";
import { resolveDirectorBoneRotation } from "./animation-semantics";
import { validateDirectorGLB } from "./glb";
import type { DirectorHumanoidBone, DirectorObject } from "./model";
import { directorPoseBoneDeltas } from "./scene";

function useAssetLease(
  projectId: string,
  assetId: string,
  kind: "model" | "image",
) {
  const query = useQuery({
    queryKey: ["canvas", "director-asset", projectId, assetId, kind],
    queryFn: async ({ signal }) => {
      const result = await getMediaPreview(projectId, assetId, signal);
      const url = new URL(result.url);
      if (
        result.asset.id !== assetId ||
        result.asset.project_id !== projectId ||
        result.asset.kind !== kind ||
        !["https:", "http:"].includes(url.protocol) ||
        url.username ||
        url.password ||
        !Number.isFinite(Date.parse(result.expires_at)) ||
        Date.parse(result.expires_at) <= Date.now() + 5000
      )
        throw new Error("项目资产预览身份或授权无效。");
      return result;
    },
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  const { data, refetch } = query;
  useEffect(() => {
    if (!data) return;
    const timer = setTimeout(
      () => {
        void refetch();
      },
      Math.max(1000, Date.parse(data.expires_at) - Date.now() - 5000),
    );
    return () => clearTimeout(timer);
  }, [data, refetch]);
  return query;
}
function disposeModel(gltf: GLTF) {
  const textures = new Set<Texture>(),
    materials = new Set<Material>();
  gltf.scene.traverse((item) => {
    if (!(item instanceof Mesh)) return;
    item.geometry.dispose();
    for (const material of Array.isArray(item.material)
      ? item.material
      : [item.material]) {
      materials.add(material);
      Object.values(material).forEach((value) => {
        if (value && typeof value === "object" && "isTexture" in value)
          textures.add(value as Texture);
      });
    }
  });
  textures.forEach((texture) => {
    if (
      typeof ImageBitmap !== "undefined" &&
      texture.image instanceof ImageBitmap
    )
      texture.image.close();
    texture.dispose();
  });
  materials.forEach((material) => material.dispose());
}
async function readBounded(response: Response, signal: AbortSignal) {
  if (!response.ok || !response.body) throw new Error("模型原件读取失败。");
  const reader = response.body.getReader(),
    parts: Uint8Array[] = [];
  let length = 0;
  try {
    while (true) {
      if (signal.aborted) throw new DOMException("Aborted", "AbortError");
      const result = await reader.read();
      if (result.done) break;
      length += result.value.byteLength;
      if (length > 64 * 1024 * 1024) throw new Error("模型原件超过 64 MiB。");
      parts.push(result.value);
    }
  } finally {
    await reader.cancel();
    reader.releaseLock();
  }
  const data = new Uint8Array(length);
  let offset = 0;
  parts.forEach((part) => {
    data.set(part, offset);
    offset += part.byteLength;
  });
  return data.buffer;
}
export function AuthorizedModel({
  projectId,
  assetId,
  object,
  time = 0,
  mode = "beauty",
  onMetadata,
  onStatus,
  onLoaded,
}: {
  projectId: string;
  assetId: string;
  object?: DirectorObject;
  time?: number;
  mode?: "beauty" | "clay" | "depth" | "normal" | "pose";
  onStatus?: (ready: boolean) => void;
  onLoaded?: (model: import("three").Object3D) => void;
  onMetadata?: (value: {
    boneNames: string[];
    animations: { name: string; duration: number }[];
  }) => void;
}) {
  const lease = useAssetLease(projectId, assetId, "model");
  const metadata = useRef(onMetadata),
    status = useRef(onStatus),
    modelLoaded = useRef(onLoaded);
  useEffect(() => {
    metadata.current = onMetadata;
    status.current = onStatus;
    modelLoaded.current = onLoaded;
  }, [onMetadata, onStatus, onLoaded]);
  const [loaded, setLoaded] = useState<{
    identity: string;
    gltf: GLTF;
    mixer: AnimationMixer;
    bones: Map<string, { bone: Bone; rest: Quaternion }>;
  }>();
  const [error, setError] = useState("");
  useEffect(() => {
    if (!lease.data) return;
    status.current?.(false);
    const controller = new AbortController();
    let current: GLTF | undefined;
    void (async () => {
      const data = await readBounded(
        await fetch(lease.data.url, {
          signal: controller.signal,
          credentials: "omit",
          redirect: "error",
        }),
        controller.signal,
      );
      validateDirectorGLB(data);
      const manager = new LoadingManager();
      manager.setURLModifier((url) => {
        if (!url.startsWith("data:") && !url.startsWith("blob:"))
          throw new Error("模型引用包外资源。");
        return url;
      });
      const gltf = await new GLTFLoader(manager).parseAsync(data, "");
      current = gltf;
      if (controller.signal.aborted) {
        disposeModel(gltf);
        current = undefined;
        return;
      }
      const bones = new Map<string, { bone: Bone; rest: Quaternion }>();
      gltf.scene.traverse((item) => {
        if (item instanceof Bone)
          bones.set(item.name, { bone: item, rest: item.quaternion.clone() });
      });
      gltf.scene.updateMatrixWorld(true);
      const bounds = new Box3().setFromObject(gltf.scene, true),
        size = bounds.getSize(new Vector3());
      gltf.scene.scale.multiplyScalar(
        2 / Math.max(size.x, size.y, size.z, 0.001),
      );
      gltf.scene.updateMatrixWorld(true);
      const centered = new Box3().setFromObject(gltf.scene, true),
        center = centered.getCenter(new Vector3());
      gltf.scene.position.sub(center);
      gltf.scene.position.y -= centered.min.y - center.y;
      setError("");
      setLoaded({
        identity: lease.data.url,
        gltf,
        mixer: new AnimationMixer(gltf.scene),
        bones,
      });
      status.current?.(true);
      modelLoaded.current?.(gltf.scene);
      metadata.current?.({
        boneNames: [...bones.keys()],
        animations: gltf.animations.map((animation) => ({
          name: animation.name,
          duration: animation.duration,
        })),
      });
    })().catch((reason: unknown) => {
      if (!controller.signal.aborted)
        setError(reason instanceof Error ? reason.message : "模型加载失败。");
    });
    return () => {
      status.current?.(false);
      controller.abort();
      if (current) disposeModel(current);
    };
  }, [lease.data]);
  useEffect(() => {
    if (!loaded) return;
    const replacements: { item: Mesh; original: Material | Material[] }[] = [];
    loaded.gltf.scene.traverse((item) => {
      if (!(item instanceof Mesh)) return;
      item.castShadow = object?.castShadow ?? true;
      item.receiveShadow = object?.receiveShadow ?? true;
      if (mode === "beauty" && object?.kind !== "actor") return;
      replacements.push({ item, original: item.material });
      item.material =
        mode === "normal"
          ? new MeshNormalMaterial()
          : mode === "depth"
            ? new MeshDepthMaterial()
            : new MeshStandardMaterial({
                color:
                  mode === "beauty"
                    ? (object?.color ?? "#eeeeee")
                    : mode === "pose"
                      ? "#eeeeee"
                      : "#aab2ba",
                roughness: 0.8,
              });
    });
    return () =>
      replacements.forEach(({ item, original }) => {
        (item.material as Material).dispose();
        item.material = original;
      });
  }, [
    loaded,
    mode,
    object?.castShadow,
    object?.receiveShadow,
    object?.kind,
    object?.color,
  ]);
  useFrame(() => {
    if (!loaded || !object) return;
    const motion = object.motionClips?.find(
      (clip) => clip.id === object.activeMotionClipId,
    );
    if (motion) {
      const animation = loaded.gltf.animations.find(
        (clip) => clip.name === motion.sourceAnimation,
      );
      if (animation) {
        const action = loaded.mixer.clipAction(animation);
        action.play();
        const elapsed = Math.max(0, time - motion.start) * motion.playbackRate;
        loaded.mixer.setTime(
          motion.loop
            ? elapsed % animation.duration
            : Math.min(elapsed, animation.duration),
        );
      }
    } else loaded.mixer.stopAllAction();
    if (!motion)
      loaded.bones.forEach(({ bone, rest }) => bone.quaternion.copy(rest));
    const poses = motion
      ? {}
      : directorPoseBoneDeltas(object.pose ?? "neutral");
    for (const [boneName, name] of Object.entries(object.rig?.boneMap ?? {})) {
      const target = name ? loaded.bones.get(name) : undefined;
      if (!target) continue;
      const bone = boneName as DirectorHumanoidBone;
      const rotation = resolveDirectorBoneRotation({
        motion: motion ? target.bone.quaternion.toArray() : null,
        rest: motion ? null : target.rest.toArray(),
        poseDelta: motion ? null : (poses[bone] ?? null),
        override: object.boneOverrides?.[bone] ?? null,
        keyframes: object.boneTracks?.find((item) => item.bone === bone)
          ?.keyframes,
        time,
      });
      if (rotation) target.bone.quaternion.fromArray(rotation);
      else if (!motion) {
        target.bone.quaternion.copy(target.rest);
        if (poses[bone])
          target.bone.quaternion.multiply(new Quaternion(...poses[bone]));
      }
    }
  });
  if (lease.error || error)
    return (
      <Html center>
        <p
          role="alert"
          className="max-w-60 rounded bg-black/80 p-2 text-xs text-white"
        >
          {lease.error?.message ?? error}
        </p>
      </Html>
    );
  return loaded && loaded.identity === lease.data?.url ? (
    <primitive object={loaded.gltf.scene} dispose={null} />
  ) : (
    <Html center>正在载入项目模型…</Html>
  );
}
export function AuthorizedImage({
  projectId,
  assetId,
  panorama = false,
  radius = 60,
  onStatus,
}: {
  projectId: string;
  assetId: string;
  panorama?: boolean;
  radius?: number;
  onStatus?: (ready: boolean) => void;
}) {
  const lease = useAssetLease(projectId, assetId, "image");
  const status = useRef(onStatus);
  useEffect(() => {
    status.current = onStatus;
  }, [onStatus]);
  const [loadedImage, setLoadedImage] = useState<{
    identity: string;
    texture: Texture;
  }>();
  const texture =
    loadedImage?.identity === lease.data?.url
      ? loadedImage?.texture
      : undefined;
  const [error, setError] = useState("");
  useEffect(() => {
    if (!lease.data) return;
    status.current?.(false);
    let cancelled = false;
    const loader = new TextureLoader();
    loader.setCrossOrigin("anonymous");
    const owned = loader.load(
      lease.data.url,
      (value) => {
        if (cancelled) return;
        value.colorSpace = SRGBColorSpace;
        setError("");
        setLoadedImage({ identity: lease.data.url, texture: value });
        status.current?.(true);
      },
      undefined,
      () => {
        if (!cancelled) setError("图片纹理读取失败。");
      },
    );
    return () => {
      cancelled = true;
      status.current?.(false);
      if (owned.image instanceof HTMLImageElement) {
        owned.image.onload = null;
        owned.image.onerror = null;
        owned.image.removeAttribute("src");
      }
      owned.dispose();
    };
  }, [lease.data]);
  if (lease.error || error)
    return (
      <Html center>
        <p role="alert">{lease.error?.message ?? error}</p>
      </Html>
    );
  if (!texture) return null;
  const image = texture.image as { width: number; height: number };
  return (
    <mesh>
      {panorama ? (
        <sphereGeometry args={[radius, 48, 24]} />
      ) : (
        <planeGeometry args={[(2 * image.width) / image.height, 2]} />
      )}
      <meshBasicMaterial
        map={texture}
        side={DoubleSide}
        transparent={!panorama}
        depthWrite={panorama}
      />
    </mesh>
  );
}
