"use client";

import { useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { FileAudio, FileVideo, ImageIcon, Upload, Box } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ProjectScope } from "@/components/workbench/project-scope";
import { MediaUploadDialog } from "@/components/canvas/media-upload-dialog";
import { MediaPreviewDialog } from "@/components/canvas/media-preview-dialog";
import { CanvasNodeType } from "@/components/canvas/model";
import {
  getMediaPreview,
  listMediaAssets,
  uploadCanvasMedia,
  type MediaAsset,
} from "@/components/canvas/queries";

export function AssetsWorkspace() {
  return (
    <div className="mx-auto max-w-6xl space-y-8">
      <div>
        <h1 className="text-3xl font-medium tracking-tight">
          素材，井然有序。
        </h1>
        <p className="mt-3 text-sm text-muted-foreground">
          上传素材、查看内容，在表单和画布中继续创作。
        </p>
      </div>
      <ProjectScope>
        {(project) => <ProjectAssets key={project.id} projectId={project.id} />}
      </ProjectScope>
    </div>
  );
}

function ProjectAssets({ projectId }: { projectId: string }) {
  const parameters = useSearchParams(),
    cache = useQueryClient();
  const [search, setSearch] = useState(""),
    [kind, setKind] = useState("all");
  const [uploading, setUploading] = useState(false),
    [busy, setBusy] = useState(false);
  const [selected, setSelected] = useState<string | null>(
    parameters.get("asset_id"),
  );
  const [selection, setSelection] = useState<Set<string>>(() => new Set());
  const [notice, setNotice] = useState<string | null>(null);
  const assets = useInfiniteQuery({
    queryKey: ["media", "assets", projectId],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      listMediaAssets(projectId, pageParam, signal),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
  });
  const loaded = assets.data?.pages.flatMap((page) => page.items) ?? [];
  const filtered = loaded.filter(
    (asset) =>
      (kind === "all" || asset.kind === kind) &&
      asset.file_name.toLocaleLowerCase().includes(search.toLocaleLowerCase()),
  );
  const selectedAsset = loaded.find((asset) => asset.id === selected);
  const requested = useQuery({
    queryKey: ["media", "selected-asset", projectId, selected],
    queryFn: ({ signal }) => getMediaPreview(projectId, selected!, signal),
    enabled: !!selected && !selectedAsset,
    retry: false,
    staleTime: 0,
    gcTime: 0,
  });
  const previewAsset = selectedAsset ?? requested.data?.asset;
  function toggle(id: string) {
    setSelection((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }
  async function copyIdentities() {
    try {
      await navigator.clipboard.writeText([...selection].join("\n"));
      setNotice(`已复制 ${selection.size} 个素材编号。`);
    } catch {
      setNotice("复制失败，请检查浏览器剪贴板权限后重试。");
    }
  }
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex gap-3">
          <Input
            aria-label="搜索已加载素材"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="搜索已加载素材"
            className="w-60"
          />
          <select
            aria-label="素材类型"
            value={kind}
            onChange={(event) => setKind(event.target.value)}
            className="rounded-lg border bg-background px-3 text-sm"
          >
            {[
              ["all", "全部"],
              ["image", "图片"],
              ["video", "视频"],
              ["audio", "音频"],
              ["model", "3D 模型"],
            ].map(([key, label]) => (
              <option key={key} value={key}>
                {label}
              </option>
            ))}
          </select>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" asChild>
            <Link href={`/projects/${projectId}/canvas`}>打开画布</Link>
          </Button>
          <Button onClick={() => setUploading(true)} disabled={busy}>
            <Upload />
            上传素材
          </Button>
        </div>
      </div>
      {selection.size > 0 && (
        <div className="flex items-center gap-3 text-sm">
          <span>已选择 {selection.size} 项</span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void copyIdentities()}
          >
            复制素材编号
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setSelection(new Set())}
          >
            清空选择
          </Button>
        </div>
      )}
      {notice && (
        <p role="status" className="text-sm text-muted-foreground">
          {notice}
        </p>
      )}
      {assets.isPending ? (
        <p role="status">正在读取素材…</p>
      ) : assets.isError ? (
        <div role="alert" className="space-y-3">
          <p>素材暂时无法读取。</p>
          <Button variant="outline" onClick={() => void assets.refetch()}>
            重新读取
          </Button>
        </div>
      ) : filtered.length === 0 ? (
        <div className="rounded-xl bg-muted/40 p-12 text-center text-sm text-muted-foreground">
          {loaded.length
            ? "没有符合筛选条件的已加载素材。"
            : "上传一份素材，开始你的创作。"}
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {filtered.map((asset) => (
            <AssetTile
              key={asset.id}
              asset={asset}
              checked={selection.has(asset.id)}
              onToggle={() => toggle(asset.id)}
              onPreview={() => setSelected(asset.id)}
            />
          ))}
        </div>
      )}
      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span>
          已加载 {loaded.length} 项{assets.hasNextPage ? "，还有更多素材" : ""}
        </span>
        {assets.hasNextPage && (
          <Button
            variant="outline"
            disabled={assets.isFetchingNextPage}
            onClick={() => void assets.fetchNextPage()}
          >
            {assets.isFetchingNextPage ? "正在读取…" : "加载更多"}
          </Button>
        )}
      </div>
      {selected && requested.isError && (
        <div role="alert" className="flex items-center gap-3">
          <span>此素材暂时无法打开。</span>
          <Button variant="outline" onClick={() => void requested.refetch()}>
            重新读取
          </Button>
          <Button variant="ghost" onClick={() => setSelected(null)}>
            关闭
          </Button>
        </div>
      )}
      {previewAsset && selected && (
        <MediaPreviewDialog
          key={previewAsset.id}
          projectId={projectId}
          node={{
            id: previewAsset.id,
            type: previewAsset.kind as CanvasNodeType,
            title: previewAsset.file_name,
            position: { x: 0, y: 0 },
            width: 320,
            height: 240,
            zIndex: 0,
            assetId: previewAsset.id,
            metadata: {},
          }}
          onClose={() => setSelected(null)}
        />
      )}
      <MediaUploadDialog
        open={uploading}
        onOpenChange={setUploading}
        remainingSlots={20}
        onBusyChange={setBusy}
        upload={(file, key, options) =>
          uploadCanvasMedia(projectId, file, key, options)
        }
        onImported={async () => {
          await cache.invalidateQueries({
            queryKey: ["media", "assets", projectId],
          });
          await cache.invalidateQueries({
            queryKey: ["canvas", "media", projectId],
          });
        }}
      />
    </div>
  );
}
function AssetTile({
  asset,
  checked,
  onToggle,
  onPreview,
}: {
  asset: MediaAsset;
  checked: boolean;
  onToggle: () => void;
  onPreview: () => void;
}) {
  const Icon =
    asset.kind === "image"
      ? ImageIcon
      : asset.kind === "video"
        ? FileVideo
        : asset.kind === "audio"
          ? FileAudio
          : Box;
  return (
    <article className="overflow-hidden rounded-xl bg-muted/30">
      <div className="relative flex aspect-[4/3] items-center justify-center bg-muted/50">
        <Icon className="size-10 text-muted-foreground" aria-hidden />
        <label className="absolute top-3 left-3 flex items-center">
          <input
            type="checkbox"
            checked={checked}
            onChange={onToggle}
            aria-label={`选择 ${asset.file_name}`}
            className="size-4 accent-foreground"
          />
        </label>
        <Button
          variant="secondary"
          size="sm"
          className="absolute right-3 bottom-3"
          onClick={onPreview}
        >
          预览
        </Button>
      </div>
      <div className="space-y-2 p-4">
        <p className="truncate text-sm font-medium" title={asset.file_name}>
          {asset.file_name}
        </p>
        <p className="text-xs text-muted-foreground">
          {(asset.byte_size / (1024 * 1024)).toFixed(2)} MiB
          {asset.width && asset.height
            ? ` · ${asset.width} × ${asset.height}`
            : ""}
          {asset.duration_ms
            ? ` · ${(asset.duration_ms / 1000).toFixed(1)} 秒`
            : ""}
        </p>
      </div>
    </article>
  );
}
