"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import {
  AudioLines,
  ChevronLeft,
  ChevronRight,
  FolderOpen,
  ImageIcon,
  Search,
  Video,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
} from "@/components/ui/pagination";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  getMediaPreview,
  listMediaAssets,
  type MediaAsset,
} from "@/components/canvas/queries";
import { useReleaseMediaSource } from "@/components/canvas/nodes/media-lifecycle";
import { listProjects, PROJECTS_KEY } from "@/components/project/queries";
import { ApiError } from "@/lib/request";

const kinds = {
  image: { label: "图像", Icon: ImageIcon },
  video: { label: "视频", Icon: Video },
  audio: { label: "音频", Icon: AudioLines },
} as const;

export function AssetsWorkspace() {
  const [chosenProjectId, setChosenProjectId] = useState("");
  const projectsQuery = useInfiniteQuery({
    queryKey: [...PROJECTS_KEY, "asset-selector"],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      listProjects(
        { limit: 100, status: "active", deleted: false, cursor: pageParam },
        signal,
      ),
    getNextPageParam: (result) => result.next_cursor ?? undefined,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const projects =
    projectsQuery.data?.pages
      .flatMap((page) => page.items)
      .filter((project) => project.status === "active" && !project.is_delete) ??
    [];
  const selectedProject =
    projects.find((project) => project.id === chosenProjectId) ?? projects[0];

  return (
    <div className="flex min-w-0 flex-col gap-7">
      <div className="flex flex-wrap items-start justify-between gap-5">
        <div className="flex flex-col gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">我的资源</h1>
          <p className="text-sm leading-6 text-muted-foreground">
            找到创作素材，把灵感留在画面里。
          </p>
        </div>
        {selectedProject ? (
          <div className="flex flex-wrap items-center gap-2">
            <Select
              value={selectedProject.id}
              onValueChange={setChosenProjectId}
            >
              <SelectTrigger
                aria-label="资源所属项目"
                className="min-w-48 rounded-full border-0 bg-muted/60"
              >
                <FolderOpen aria-hidden="true" className="size-4" />
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {projects.map((project) => (
                    <SelectItem key={project.id} value={project.id}>
                      {project.name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            {projectsQuery.hasNextPage ? (
              <Button
                variant="ghost"
                size="sm"
                disabled={projectsQuery.isFetchingNextPage}
                onClick={() => void projectsQuery.fetchNextPage()}
              >
                {projectsQuery.isFetchingNextPage
                  ? "正在读取项目…"
                  : "读取更多项目"}
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
      {projectsQuery.isPending ? (
        <AssetsLoading label="正在读取项目" />
      ) : projectsQuery.error ? (
        <ServiceError
          title="项目未能加载"
          error={projectsQuery.error}
          retryLabel="重试读取项目"
          pending={projectsQuery.isFetching}
          onRetry={() => void projectsQuery.refetch()}
        />
      ) : selectedProject ? (
        <ProjectAssets
          key={selectedProject.id}
          projectId={selectedProject.id}
        />
      ) : (
        <Empty className="rounded-2xl border-0 bg-muted/30 py-20">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <FolderOpen aria-hidden="true" />
            </EmptyMedia>
            <EmptyTitle>还没有可用的项目</EmptyTitle>
            <EmptyDescription>
              创建一个项目，开始收集你的创作素材。
            </EmptyDescription>
          </EmptyHeader>
          <Button asChild>
            <Link href="/projects?create=true">新建项目</Link>
          </Button>
        </Empty>
      )}
    </div>
  );
}

function ProjectAssets({ projectId }: { projectId: string }) {
  const [kind, setKind] = useState<"all" | MediaAsset["kind"]>("all");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [selected, setSelected] = useState<MediaAsset | null>(null);
  const previewTrigger = useRef<HTMLButtonElement | null>(null);
  const assets = useQuery({
    queryKey: ["canvas", "media-library", projectId, "page", cursors[page]],
    queryFn: ({ signal }) => listMediaAssets(projectId, cursors[page], signal),
    retry: false,
    refetchOnWindowFocus: false,
  });
  const searchTerm = search.trim().toLocaleLowerCase();
  const visibleAssets =
    assets.data?.items.filter(
      (asset) =>
        (kind === "all" || asset.kind === kind) &&
        asset.file_name.toLocaleLowerCase().includes(searchTerm),
    ) ?? [];

  return (
    <div className="flex min-w-0 flex-col gap-5">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <ToggleGroup
          type="single"
          value={kind}
          aria-label="资源类型"
          className="flex-wrap gap-1"
          onValueChange={(value) => {
            if (
              value === "all" ||
              value === "image" ||
              value === "video" ||
              value === "audio"
            )
              setKind(value);
          }}
        >
          <ToggleGroupItem value="all" className="rounded-full px-4">
            全部
          </ToggleGroupItem>
          {Object.entries(kinds).map(([value, { label, Icon }]) => (
            <ToggleGroupItem
              key={value}
              value={value}
              className="rounded-full px-4"
            >
              <Icon aria-hidden="true" className="size-4" />
              {label}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <div className="relative w-full sm:w-72">
          <Search
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            aria-label="搜索当前页资源"
            placeholder="搜索当前页文件名"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            className="rounded-full border-0 bg-muted/60 pl-9 shadow-none"
          />
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        在当前页筛选资源。翻页后继续查看所选项目的素材。
      </p>
      {assets.isPending ? (
        <AssetsLoading label="正在读取资源" />
      ) : assets.error ? (
        <ServiceError
          title="资源未能加载"
          error={assets.error}
          retryLabel="重试读取资源"
          pending={assets.isFetching}
          onRetry={() => void assets.refetch()}
        />
      ) : visibleAssets.length === 0 ? (
        <Empty role="status" className="rounded-2xl border-0 bg-muted/30 py-20">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <ImageIcon aria-hidden="true" />
            </EmptyMedia>
            <EmptyTitle>
              {assets.data?.items.length
                ? "当前页没有匹配的资源"
                : "这个项目还没有资源"}
            </EmptyTitle>
            <EmptyDescription>
              {assets.data?.items.length
                ? "调整类型或文件名筛选，也可以继续查看下一页。"
                : "项目已有素材会显示在这里，媒体上传入口正在准备中。"}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <ul className="grid gap-x-5 gap-y-8 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
          {visibleAssets.map((asset) => {
            const { Icon, label } = kinds[asset.kind];
            return (
              <li key={asset.id} className="min-w-0">
                <Card className="gap-3 border-0 bg-transparent p-0 shadow-none ring-0">
                  <CardContent className="p-0">
                    <Button
                      variant="ghost"
                      aria-label={`预览 ${asset.file_name}`}
                      className="relative aspect-video h-auto w-full rounded-2xl bg-linear-to-br from-muted via-muted/70 to-muted/40 p-0 hover:bg-muted"
                      onClick={(event) => {
                        previewTrigger.current = event.currentTarget;
                        setSelected(asset);
                      }}
                    >
                      <Icon
                        aria-hidden="true"
                        className="size-12 text-muted-foreground/40"
                        strokeWidth={1.25}
                      />
                      <span className="absolute right-3 bottom-3 rounded-md bg-background/70 px-2 py-1 text-xs text-muted-foreground">
                        {label}
                      </span>
                    </Button>
                  </CardContent>
                  <CardHeader className="flex flex-col gap-2 px-0">
                    <CardTitle>
                      <h2 className="text-sm font-medium break-words">
                        {asset.file_name}
                      </h2>
                    </CardTitle>
                    <CardDescription className="flex w-full flex-wrap items-center justify-between gap-2 text-xs">
                      <Badge variant="secondary" className="font-normal">
                        {label}
                      </Badge>
                      <span>{formatSize(asset.byte_size)}</span>
                    </CardDescription>
                  </CardHeader>
                </Card>
              </li>
            );
          })}
        </ul>
      )}
      <Pagination aria-label="资源分页" className="mt-2 justify-end">
        <PaginationContent>
          <PaginationItem>
            <Button
              variant="ghost"
              size="sm"
              disabled={page === 0 || assets.isFetching}
              onClick={() => setPage((current) => current - 1)}
            >
              <ChevronLeft aria-hidden="true" className="size-4" />
              上一页
            </Button>
          </PaginationItem>
          <PaginationItem>
            <span aria-live="polite" className="px-3 text-sm tabular-nums">
              第 {page + 1} 页
            </span>
          </PaginationItem>
          <PaginationItem>
            <Button
              variant="ghost"
              size="sm"
              disabled={!assets.data?.next_cursor || assets.isFetching}
              onClick={() => {
                const cursor = assets.data?.next_cursor;
                if (!cursor) return;
                setCursors((current) => [
                  ...current.slice(0, page + 1),
                  cursor,
                ]);
                setPage((current) => current + 1);
              }}
            >
              下一页
              <ChevronRight aria-hidden="true" className="size-4" />
            </Button>
          </PaginationItem>
        </PaginationContent>
      </Pagination>
      {selected ? (
        <Dialog open onOpenChange={(open) => !open && setSelected(null)}>
          <DialogContent
            className="max-h-[90dvh] overflow-y-auto ring-0 sm:max-w-3xl"
            showCloseButton={false}
            onCloseAutoFocus={(event) => {
              event.preventDefault();
              previewTrigger.current?.focus();
            }}
          >
            <DialogHeader>
              <DialogTitle className="break-words">
                {selected.file_name}
              </DialogTitle>
              <DialogDescription>
                {kinds[selected.kind].label} · {formatSize(selected.byte_size)}
                。授权预览会自动过期，关闭窗口后停止播放。
              </DialogDescription>
            </DialogHeader>
            <AssetPreview
              key={selected.id}
              projectId={projectId}
              asset={selected}
            />
            <DialogFooter className="border-0 bg-transparent">
              <DialogClose asChild>
                <Button variant="secondary">关闭预览</Button>
              </DialogClose>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      ) : null}
    </div>
  );
}

function AssetPreview({
  projectId,
  asset,
}: {
  projectId: string;
  asset: MediaAsset;
}) {
  const [expiredLease, setExpiredLease] = useState<string | null>(null);
  const [playbackFailed, setPlaybackFailed] = useState(false);
  const preview = useQuery({
    queryKey: ["assets", "preview", projectId, asset.id],
    queryFn: async ({ signal }) => {
      const result = await getMediaPreview(projectId, asset.id, signal);
      const url = new URL(result.url);
      const expires = Date.parse(result.expires_at);
      if (
        result.asset.id !== asset.id ||
        result.asset.project_id !== projectId ||
        result.asset.kind !== asset.kind ||
        !["http:", "https:"].includes(url.protocol) ||
        url.username ||
        url.password ||
        !Number.isFinite(expires) ||
        expires <= Date.now() + 5000
      )
        throw new ApiError(502, "invalid_response");
      return result;
    },
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const expiry = preview.data?.expires_at;
  useEffect(() => {
    if (!expiry) return;
    const timer = setTimeout(
      () => setExpiredLease(expiry),
      Math.max(0, Date.parse(expiry) - Date.now() - 5000),
    );
    return () => clearTimeout(timer);
  }, [expiry]);
  const retry = () => {
    setExpiredLease(null);
    setPlaybackFailed(false);
    void preview.refetch();
  };
  if (preview.isFetching)
    return (
      <p role="status" className="py-16 text-center text-muted-foreground">
        正在获取授权预览…
      </p>
    );
  if (preview.error || playbackFailed || (expiry && expiredLease === expiry))
    return (
      <ServiceError
        title="资源暂时无法预览"
        error={preview.error}
        retryLabel="重新获取预览"
        pending={preview.isFetching}
        onRetry={retry}
      />
    );
  return preview.data ? (
    <PreviewSource
      key={preview.data.url}
      asset={asset}
      url={preview.data.url}
      onError={() => setPlaybackFailed(true)}
    />
  ) : null;
}

function PreviewSource({
  asset,
  url,
  onError,
}: {
  asset: MediaAsset;
  url: string;
  onError: () => void;
}) {
  const source = useRef<HTMLMediaElement | HTMLImageElement>(null);
  useReleaseMediaSource(source, url);
  if (asset.kind === "image")
    return (
      // eslint-disable-next-line @next/next/no-img-element -- 授权 URL 直接匿名读取，避免通过 Next 图片优化器缓存短期签名。
      <img
        ref={source as React.RefObject<HTMLImageElement>}
        src={url}
        crossOrigin="anonymous"
        alt={asset.file_name}
        onError={onError}
        className="max-h-[60dvh] w-full rounded-xl bg-muted/30 object-contain"
      />
    );
  if (asset.kind === "video")
    return (
      <video
        ref={source as React.RefObject<HTMLVideoElement>}
        src={url}
        crossOrigin="anonymous"
        aria-label={asset.file_name}
        controls
        playsInline
        preload="metadata"
        onError={onError}
        className="max-h-[60dvh] w-full rounded-xl bg-muted/30"
      />
    );
  return (
    <audio
      ref={source as React.RefObject<HTMLAudioElement>}
      src={url}
      crossOrigin="anonymous"
      aria-label={asset.file_name}
      controls
      preload="metadata"
      onError={onError}
      className="w-full"
    />
  );
}

function ServiceError({
  title,
  error,
  retryLabel,
  pending,
  onRetry,
}: {
  title: string;
  error?: Error | null;
  retryLabel: string;
  pending: boolean;
  onRetry: () => void;
}) {
  return (
    <Alert variant="destructive" className="border-0 bg-muted/40">
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>
        {error?.message ?? "授权已过期或媒体暂时不可用，请重新获取预览。"}
        {error instanceof ApiError && error.requestId ? (
          <p>请求编号：{error.requestId}</p>
        ) : null}
      </AlertDescription>
      <Button variant="ghost" disabled={pending} onClick={onRetry}>
        {retryLabel}
      </Button>
    </Alert>
  );
}

function AssetsLoading({ label }: { label: string }) {
  return (
    <div
      role="status"
      aria-label={label}
      className="grid gap-5 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4"
    >
      {[0, 1, 2, 3].map((index) => (
        <Skeleton key={index} className="aspect-video rounded-2xl" />
      ))}
    </div>
  );
}

function formatSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
}
