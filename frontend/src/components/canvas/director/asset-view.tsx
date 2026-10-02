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
import type { GLTF } from "three/addons/loaders/GLTFLoader.js";
import { getMediaPreview } from "../queries";
import { resolveDirectorBoneRotation } from "./animation-semantics";
import {
  parseDirectorModel,
  readModelOriginal,
  disposeDirectorModel,
} from "./model-file";
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
  const authorized =
    lease.isSuccess && lease.isFetchedAfterMount && !lease.isFetching
      ? lease.data
      : undefined;
  const authorizationId = authorized
    ? `${lease.dataUpdatedAt}:${authorized.url}`
    : "";
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
    authorization: NonNullable<typeof authorized>;
    gltf: GLTF;
    mixer: AnimationMixer;
    bones: Map<string, { bone: Bone; rest: Quaternion }>;
  }>();
  const [error, setError] = useState("");
  const restoreMaterials = useRef<(() => void) | undefined>(undefined);
  useEffect(() => {
    if (
      !loaded ||
      loaded.identity !== authorizationId ||
      loaded.authorization !== authorized
    )
      return;
    const replacements: {
      item: Mesh;
      original: Material | Material[];
      replacement: Material;
    }[] = [];
    loaded.gltf.scene.traverse((item) => {
      if (!(item instanceof Mesh)) return;
      item.castShadow = object?.castShadow ?? true;
      item.receiveShadow = object?.receiveShadow ?? true;
      if (mode === "beauty" && object?.kind !== "actor") return;
      const replacement =
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
      replacements.push({ item, original: item.material, replacement });
      item.material = replacement;
    });
    let restored = false;
    const restore = () => {
      if (restored) return;
      restored = true;
      replacements.forEach(({ item, original, replacement }) => {
        replacement.dispose();
        item.material = original;
      });
    };
    restoreMaterials.current = restore;
    return () => {
      restore();
      if (restoreMaterials.current === restore)
        restoreMaterials.current = undefined;
    };
  }, [
    loaded,
    authorized,
    authorizationId,
    mode,
    object?.castShadow,
    object?.receiveShadow,
    object?.kind,
    object?.color,
  ]);
  useEffect(() => {
    if (!authorized) return;
    status.current?.(false);
    const controller = new AbortController();
    let current: GLTF | undefined;
    void (async () => {
      const data = await readModelOriginal(
        await fetch(authorized.url, {
          signal: controller.signal,
          credentials: "omit",
          redirect: "error",
        }),
        authorized.asset.byte_size,
        controller.signal,
      );
      if (controller.signal.aborted) return;
      const gltf = await parseDirectorModel(data, authorized.asset.mime_type);
      current = gltf;
      if (controller.signal.aborted) {
        disposeDirectorModel(gltf);
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
        identity: authorizationId,
        authorization: authorized,
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
      if (current) {
        restoreMaterials.current?.();
        disposeDirectorModel(current);
      }
    };
  }, [authorized, authorizationId]);
  useFrame(() => {
    if (
      !loaded ||
      loaded.identity !== authorizationId ||
      loaded.authorization !== authorized ||
      !object
    )
      return;
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
  return loaded &&
    authorized &&
    loaded.identity === authorizationId &&
    loaded.authorization === authorized ? (
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
