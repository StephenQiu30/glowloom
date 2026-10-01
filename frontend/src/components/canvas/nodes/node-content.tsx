"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { CanvasNodeType, type CanvasNodeData } from "../model";
import { getMediaPreview } from "../queries";
import { bindCanvasVideoHoverPreview } from "../engine/video-hover-preview";
import { useMediaLifecycle, useReleaseMediaSource } from "./media-lifecycle";

type Props = {
  node: CanvasNodeData;
  projectId: string;
  active: boolean;
  playable?: boolean;
  inMotion?: boolean;
  onPreview?: () => void;
};
export function NodeContent(props: Props) {
  const { node } = props;
  if (node.type === "text")
    return (
      <p className="line-clamp-8 px-5 py-4 text-sm leading-7 break-words whitespace-pre-wrap">
        {node.metadata?.content || "选择后编辑文字"}
      </p>
    );
  if (node.type === "group")
    return (
      <div className="p-5 text-xs text-muted-foreground">
        {node.metadata?.frame?.collapsed ? "分组已折叠" : "拖入节点加入分组"}
      </div>
    );
  if (node.type === "model")
    return (
      <div className="space-y-3 p-5 text-xs text-muted-foreground">
        <p>3D 模型 · GLB</p>
        <Button variant="outline" size="sm" onClick={props.onPreview}>
          预览模型
        </Button>
      </div>
    );
  if (node.generation)
    return (
      <div className="space-y-3 p-5 text-xs text-muted-foreground">
        <p>{node.generation.capability}</p>
        <p className="line-clamp-4">
          {node.generation.prompt || "选择节点后设置生成参数"}
        </p>
      </div>
    );
  if (node.director)
    if (node.director.cover)
      return (
        <div className="flex h-full flex-col">
          <div className="min-h-0 flex-1">
            <MediaNodeContent
              key={
                props.projectId +
                ":" +
                node.id +
                ":" +
                node.director.cover.assetId
              }
              {...props}
              node={{
                ...node,
                type: CanvasNodeType.Image,
                assetId: node.director.cover.assetId,
              }}
              onPreview={undefined}
            />
          </div>
          <p className="shrink-0 px-3 py-1 text-xs text-muted-foreground">
            {node.director.shots.length} 分镜 · 导演台封面
          </p>
        </div>
      );
  if (node.director)
    return (
      <div className="space-y-3 p-5 text-xs text-muted-foreground">
        <p>
          {node.director.objects.length} 对象 · {node.director.shots.length}{" "}
          分镜
        </p>
        <p>选择节点后打开导演台</p>
      </div>
    );
  if (node.batchTable || node.timeline)
    return (
      <div className="space-y-3 p-5 text-xs text-muted-foreground">
        <p>
          {node.batchTable
            ? `${node.batchTable.rows.length} 行 · ${node.batchTable.referenceColumns.length} 参考列`
            : `${node.timeline!.tracks.length} 轨道 · ${node.timeline!.clips.length} 片段`}
        </p>
        <p>选择节点后打开编辑器</p>
      </div>
    );
  return (
    <MediaNodeContent
      key={props.projectId + ":" + node.id + ":" + node.assetId}
      {...props}
    />
  );
}

function MediaNodeContent({
  node,
  projectId,
  active,
  playable = false,
  inMotion = false,
  onPreview,
}: Props) {
  const root = useRef<HTMLDivElement>(null);
  const eligible = useMediaLifecycle(root, active, inMotion);
  const wantsSource = eligible && (node.type === "image" || playable);
  const client = useQueryClient();
  // A node owns cancellation of its own pending lease, including duplicate references.
  const queryKey = useMemo(
    () => ["canvas", "media-preview", projectId, node.assetId, node.id],
    [projectId, node.assetId, node.id],
  );
  const [failed, setFailed] = useState(false);
  const [recovering, setRecovering] = useState(false);
  const automaticUsed = useRef(false);
  const preview = useQuery({
    queryKey,
    queryFn: async ({ signal }) => {
      try {
        return await getMediaPreview(projectId, node.assetId!, signal);
      } catch (error) {
        if (!signal.aborted) setFailed(true);
        throw error;
      }
    },
    enabled: Boolean(node.assetId && wantsSource && !failed),
    retry: false,
    retryOnMount: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    staleTime: Infinity,
    gcTime: 60_000,
  });
  const refetch = preview.refetch;
  useEffect(() => {
    if (!eligible) void client.cancelQueries({ queryKey, exact: true });
    return () => {
      void client.cancelQueries({ queryKey, exact: true });
    };
  }, [client, queryKey, eligible]);
  const refresh = useCallback(
    (manual: boolean) => {
      if (!manual && automaticUsed.current) {
        setFailed(true);
        return;
      }
      automaticUsed.current = !manual;
      setRecovering(true);
      setFailed(false);
      void refetch().then((result) => {
        setRecovering(false);
        const expires = Date.parse(result.data?.expires_at ?? "");
        setFailed(
          Boolean(result.error) ||
            !Number.isFinite(expires) ||
            expires <= Date.now() + 5000,
        );
      });
    },
    [refetch],
  );
  useEffect(() => {
    if (!wantsSource || failed || recovering || !preview.data) return;
    const remaining = Date.parse(preview.data.expires_at) - Date.now() - 5000;
    const timer = setTimeout(
      () => refresh(false),
      Number.isFinite(remaining) ? Math.max(0, remaining) : 0,
    );
    return () => clearTimeout(timer);
  }, [preview.data, wantsSource, failed, recovering, refresh]);
  useEffect(() => {
    const element = root.current;
    if (
      !element ||
      !eligible ||
      playable ||
      node.type !== "video" ||
      failed ||
      !node.assetId
    )
      return;
    return bindCanvasVideoHoverPreview(
      element,
      async (signal) => {
        const existing =
          client.getQueryData<Awaited<ReturnType<typeof getMediaPreview>>>(
            queryKey,
          );
        if (existing && Date.parse(existing.expires_at) > Date.now() + 5000)
          return existing.url;
        const value = await getMediaPreview(projectId, node.assetId!, signal);
        if (signal.aborted) return "";
        client.setQueryData(queryKey, value);
        return value.url;
      },
      { onError: () => setFailed(true) },
    );
  }, [
    client,
    queryKey,
    eligible,
    playable,
    failed,
    node.type,
    node.assetId,
    projectId,
  ]);

  const label =
    node.type === "image"
      ? "图片"
      : node.type === "video"
        ? "悬停预览视频，单选后播放"
        : "单选后播放音频";
  return (
    <div
      ref={root}
      className="relative h-full w-full overflow-hidden"
      onDoubleClick={
        node.type === "image" && onPreview
          ? (event) => {
              event.stopPropagation();
              onPreview();
            }
          : undefined
      }
    >
      {!eligible ? (
        <Placeholder label={label} />
      ) : failed || preview.error ? (
        <div role="alert" className="p-4 text-xs">
          <p>媒体暂时无法读取或播放。</p>
          <Button size="sm" variant="ghost" onClick={() => refresh(true)}>
            重新读取媒体
          </Button>
        </div>
      ) : !wantsSource ? (
        <Placeholder label={label} />
      ) : recovering || !preview.data ? (
        <p role="status" className="p-5 text-xs text-muted-foreground">
          正在读取媒体…
        </p>
      ) : (
        <MediaSource
          key={preview.data.url}
          kind={node.type}
          title={node.title}
          url={preview.data.url}
          onError={() => refresh(false)}
        />
      )}
    </div>
  );
}
function Placeholder({ label }: { label: string }) {
  return (
    <div className="flex h-full items-center justify-center p-5 text-xs text-muted-foreground">
      {label}
    </div>
  );
}
function MediaSource({
  kind,
  title,
  url,
  onError,
}: {
  kind: string;
  title: string;
  url: string;
  onError: () => void;
}) {
  const ref = useRef<HTMLMediaElement | HTMLImageElement>(null);
  useReleaseMediaSource(ref, url);
  if (kind === "image")
    return (
      // eslint-disable-next-line @next/next/no-img-element -- 授权URL需直接匿名请求并在卸载时清理，不经Next优化器缓存。
      <img
        ref={ref as React.RefObject<HTMLImageElement>}
        crossOrigin="anonymous"
        src={url}
        alt={title}
        className="h-full w-full object-contain"
        draggable={false}
        onError={onError}
      />
    );
  if (kind === "video")
    return (
      <video
        ref={ref as React.RefObject<HTMLVideoElement>}
        crossOrigin="anonymous"
        src={url}
        aria-label={title}
        controls
        preload="metadata"
        playsInline
        className="h-full w-full object-contain"
        data-canvas-no-zoom
        onError={onError}
      />
    );
  return (
    <div className="flex h-full items-center justify-center p-4">
      <audio
        ref={ref as React.RefObject<HTMLAudioElement>}
        crossOrigin="anonymous"
        src={url}
        aria-label={title}
        controls
        preload="metadata"
        className="w-full"
        data-canvas-no-zoom
        onError={onError}
      />
    </div>
  );
}
