"use client";
import { useId, useState } from "react";
import dynamic from "next/dynamic";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { getProject } from "@/components/project/queries";
import { ApiError } from "@/lib/request";
import { getCanvas, listCanvases, listProjects } from "./queries";
import { CanvasNodeType } from "./model";
import { depthUUID, depthStatusLabels } from "./media-depth-model";
import { getDepth, listDepths, MEDIA_DEPTHS_KEY } from "./media-depth-queries";
import { DepthFailure } from "./media-depth-failure";
const MediaDepthPanel = dynamic(
  () => import("./media-depth-panel").then((module) => module.MediaDepthPanel),
  { ssr: false, loading: () => <p role="status">载入视频深度工具…</p> },
);
export function DepthWorkspace() {
  const parameters = useSearchParams(),
    router = useRouter(),
    session = useId(),
    [locked, setLocked] = useState(false);
  const projectId = parameters.get("project_id") ?? "",
    canvasId = parameters.get("canvas_id") ?? "",
    nodeId = parameters.get("node_id") ?? "",
    jobId = parameters.get("job_id") ?? "";
  const invalid = [projectId, canvasId, nodeId, jobId].some(
    (value) => value && !depthUUID.safeParse(value).success,
  );
  const projects = useInfiniteQuery({
    queryKey: ["depth-workspace", "projects", session],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) => listProjects(pageParam, signal),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
  });
  const project = useQuery({
    queryKey: ["depth-workspace", "project", projectId, session],
    queryFn: ({ signal }) => getProject(projectId, signal),
    enabled: Boolean(projectId) && !invalid,
    retry: false,
  });
  const recovered = useQuery({
    queryKey: [...MEDIA_DEPTHS_KEY, "restore", projectId, jobId, session],
    queryFn: ({ signal }) => getDepth(projectId, jobId, signal),
    enabled: Boolean(projectId && jobId) && !invalid,
    retry: false,
  });
  const chosenCanvas = recovered.data?.source.canvas_id ?? canvasId,
    chosenNode = recovered.data?.source.node_id ?? nodeId;
  const canvases = useQuery({
    queryKey: ["depth-workspace", "canvases", projectId, session],
    queryFn: ({ signal }) => listCanvases(projectId, signal),
    enabled: Boolean(projectId) && !invalid,
    retry: false,
  });
  const document = useQuery({
    queryKey: ["depth-workspace", "source", projectId, chosenCanvas, session],
    queryFn: async ({ signal }) => {
      const value = await getCanvas(chosenCanvas, signal);
      if (value.projectId !== projectId)
        throw new ApiError(502, "invalid_response");
      return value;
    },
    enabled: Boolean(projectId && chosenCanvas) && !invalid,
    retry: false,
    staleTime: 0,
  });
  const jobs = useInfiniteQuery({
    queryKey: [...MEDIA_DEPTHS_KEY, "workspace", projectId, session],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      listDepths(projectId, undefined, undefined, pageParam, signal),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    enabled: Boolean(projectId) && !invalid,
    retry: false,
    refetchInterval: 5000,
  });
  const videos =
    document.data?.nodes.filter(
      (node) => node.type === CanvasNodeType.Video && node.assetId,
    ) ?? [];
  const selectedVideo = videos.find((node) => node.id === chosenNode);
  function choose(values: Record<string, string>) {
    if (locked) return;
    const next = new URLSearchParams(parameters);
    for (const key of ["canvas_id", "node_id", "job_id"]) next.delete(key);
    for (const [key, value] of Object.entries(values))
      if (value) next.set(key, value);
    router.replace(`/depths?${next}`, { scroll: false });
  }
  function selectedJob(id: string) {
    const next = new URLSearchParams(parameters);
    next.set("project_id", projectId);
    next.set("canvas_id", chosenCanvas);
    next.set("node_id", chosenNode);
    next.set("job_id", id);
    router.replace(`/depths?${next}`, { scroll: false });
  }
  return (
    <div className="max-w-5xl min-w-0 space-y-8">
      <div>
        <h1 className="text-3xl font-medium tracking-tight">视频深度</h1>
        <p className="mt-3 text-sm text-muted-foreground">
          从正式视频原件生成相对深度，查看任务并审核真实结果。
        </p>
      </div>
      {invalid ? (
        <DepthFailure
          title="深度任务链接无效"
          error={new Error("链接参数必须为有效 UUID。")}
        />
      ) : null}
      <div className="flex flex-wrap items-center gap-3">
        <Label htmlFor="depth-project">当前项目</Label>
        <select
          id="depth-project"
          className="h-10 max-w-full rounded-lg border bg-background px-3"
          value={projectId}
          disabled={locked || projects.isPending}
          onChange={(event) => choose({ project_id: event.target.value })}
        >
          <option value="">请选择项目</option>
          {projectId &&
          !projects.data?.pages.some((page) =>
            page.items.some((item) => item.id === projectId),
          ) ? (
            <option value={projectId}>{project.data?.name ?? projectId}</option>
          ) : null}
          {projects.data?.pages
            .flatMap((page) => page.items)
            .map((item) => (
              <option key={item.id} value={item.id}>
                {item.name}
              </option>
            ))}
        </select>
        {projects.hasNextPage ? (
          <Button
            variant="ghost"
            disabled={locked || projects.isFetchingNextPage}
            onClick={() => void projects.fetchNextPage()}
          >
            加载更多项目
          </Button>
        ) : null}
      </div>
      {projects.error ? (
        <DepthFailure
          title="项目列表读取失败"
          error={projects.error}
          retry={() => void projects.refetch()}
        />
      ) : null}
      {project.error ? (
        <DepthFailure
          title="当前项目读取失败"
          error={project.error}
          retry={() => void project.refetch()}
        />
      ) : null}
      {projectId && !invalid ? (
        <>
          <div className="flex flex-wrap items-center gap-3">
            <Label htmlFor="depth-canvas">来源画布</Label>
            <select
              id="depth-canvas"
              className="h-10 max-w-full rounded-lg border bg-background px-3"
              value={chosenCanvas}
              disabled={locked || canvases.isPending}
              onChange={(event) =>
                choose({ project_id: projectId, canvas_id: event.target.value })
              }
            >
              <option value="">请选择画布</option>
              {chosenCanvas &&
              !canvases.data?.items.some((item) => item.id === chosenCanvas) ? (
                <option value={chosenCanvas}>
                  {document.data?.name ?? chosenCanvas}
                </option>
              ) : null}
              {canvases.data?.items.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
            {chosenCanvas ? (
              <>
                <Label htmlFor="depth-video">正式视频</Label>
                <select
                  id="depth-video"
                  className="h-10 max-w-full rounded-lg border bg-background px-3"
                  value={chosenNode}
                  disabled={locked || document.isPending}
                  onChange={(event) =>
                    choose({
                      project_id: projectId,
                      canvas_id: chosenCanvas,
                      node_id: event.target.value,
                    })
                  }
                >
                  <option value="">请选择视频节点</option>
                  {chosenNode &&
                  !videos.some((item) => item.id === chosenNode) ? (
                    <option value={chosenNode}>{chosenNode}（冻结来源）</option>
                  ) : null}
                  {videos.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.title}
                    </option>
                  ))}
                </select>
              </>
            ) : null}
          </div>
          {canvases.error ? (
            <DepthFailure
              title="画布列表读取失败"
              error={canvases.error}
              retry={() => void canvases.refetch()}
            />
          ) : null}
          {document.error ? (
            <DepthFailure
              title="来源画布读取失败"
              error={document.error}
              retry={() => void document.refetch()}
            />
          ) : null}
          {recovered.error ? (
            <DepthFailure
              title="原深度任务读取失败"
              error={recovered.error}
              retry={() => void recovered.refetch()}
            />
          ) : null}
          {chosenCanvas && chosenNode && (!jobId || recovered.data) ? (
            <MediaDepthPanel
              key={`${projectId}:${chosenCanvas}:${chosenNode}`}
              projectId={projectId}
              canvasId={chosenCanvas}
              nodeId={chosenNode}
              jobId={jobId || undefined}
              onJobSelected={selectedJob}
              onLockChange={setLocked}
              onPrepareSource={
                selectedVideo && project.data?.status === "active"
                  ? async () => {
                      const [fresh, latestProject] = await Promise.all([
                        getCanvas(chosenCanvas),
                        getProject(projectId),
                      ]);
                      const source = fresh.nodes.find(
                        (node) =>
                          node.id === chosenNode &&
                          node.type === CanvasNodeType.Video &&
                          node.assetId === selectedVideo.assetId,
                      );
                      if (
                        fresh.projectId !== projectId ||
                        latestProject.status !== "active" ||
                        !source
                      )
                        throw new ApiError(409, "media_depth_conflict");
                      return {
                        canvas_id: fresh.id,
                        node_id: source.id,
                        revision: fresh.revision,
                      };
                    }
                  : undefined
              }
            />
          ) : null}
          {jobs.error ? (
            <DepthFailure
              title="项目深度任务读取失败"
              error={jobs.error}
              retry={() => void jobs.refetch()}
            />
          ) : null}
          <section className="space-y-3" aria-label="项目深度任务">
            <h2 className="font-medium">项目全部深度任务</h2>
            {jobs.isPending ? (
              <p role="status">正在读取深度任务…</p>
            ) : jobs.data?.pages.every((page) => page.items.length === 0) ? (
              <p className="text-sm text-muted-foreground">
                此项目尚无深度任务。先选择画布内已保存的视频节点。
              </p>
            ) : (
              <ul className="space-y-2">
                {jobs.data?.pages
                  .flatMap((page) => page.items)
                  .map((item) => (
                    <li key={item.id}>
                      <Link
                        className="flex flex-wrap justify-between gap-2 rounded-lg bg-muted/40 p-3 text-sm"
                        aria-disabled={locked}
                        href={`/depths?project_id=${projectId}&job_id=${item.id}`}
                      >
                        <span className="break-all">{item.id}</span>
                        <span>{depthStatusLabels[item.status]}</span>
                      </Link>
                    </li>
                  ))}
              </ul>
            )}
            {jobs.hasNextPage ? (
              <Button
                variant="ghost"
                disabled={locked || jobs.isFetchingNextPage}
                onClick={() => void jobs.fetchNextPage()}
              >
                加载更多深度任务
              </Button>
            ) : null}
          </section>
        </>
      ) : (
        <p className="text-muted-foreground">请选择项目以查看深度任务。</p>
      )}
    </div>
  );
}
