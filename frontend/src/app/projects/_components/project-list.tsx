"use client";
import { LoadingState } from "@/components/layout/loading-state";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Grid2X2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { listProjects } from "@/api/projects";
import {
  canvasHref,
  projectErrorMessage,
  requireProjectList,
  type Project,
} from "@/components/projects/project";
export function ProjectList({ query, view }: { query: string; view: string }) {
  const [page, setPage] = useState<{
    items: Project[];
    next_cursor: string | null;
  } | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [retry, setRetry] = useState(0);
  const active = useRef<AbortController | null>(null);
  const paging = useRef(false);
  useEffect(() => {
    const controller = new AbortController();
    active.current = controller;
    listProjects({ q: query, limit: 50 }, { signal: controller.signal })
      .then(requireProjectList)
      .then((result) => {
        if (!controller.signal.aborted) setPage(result);
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted) setError(projectErrorMessage(e));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => {
      active.current?.abort();
    };
  }, [query, retry]);

  async function loadMore() {
    if (!page?.next_cursor || loading || paging.current) return;
    paging.current = true;
    const controller = new AbortController();
    active.current = controller;
    setLoading(true);
    setError("");
    try {
      const next = requireProjectList(
        await listProjects(
          { q: query, limit: 50, cursor: page.next_cursor },
          { signal: controller.signal },
        ),
      );
      if (!controller.signal.aborted)
        setPage({
          items: [
            ...page.items,
            ...next.items.filter(
              (p) => !page.items.some((previous) => previous.id === p.id),
            ),
          ],
          next_cursor: next.next_cursor,
        });
    } catch (e) {
      if (!controller.signal.aborted) setError(projectErrorMessage(e));
    } finally {
      paging.current = false;
      if (!controller.signal.aborted) setLoading(false);
    }
  }
  return (
    <div aria-busy={loading}>
      {loading && !page && (
        <LoadingState className="py-10 text-muted-foreground">
          正在读取画布…
        </LoadingState>
      )}
      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>
            <p>{error}</p>
            <Button
              variant="outline"
              className="mt-2"
              disabled={loading}
              onClick={() => {
                if (page?.next_cursor) {
                  void loadMore();
                } else {
                  setPage(null);
                  setError("");
                  setLoading(true);
                  setRetry((n) => n + 1);
                }
              }}
            >
              重试读取
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {page?.items.length === 0 && (
        <div className="rounded-2xl border border-dashed p-12 text-center">
          <h3 className="font-medium">
            {query ? "没有匹配的画布" : "还没有画布"}
          </h3>
          <p className="mt-2 text-sm text-muted-foreground">
            {query ? "试试其他名称。" : "创建第一个空白画布，进入你的画布。"}
          </p>
        </div>
      )}
      <div className={view === "grid" ? "canvas-grid" : "space-y-3"}>
        {page?.items.map((project) => (
          <article className="canvas-tile" key={project.id}>
            {project.canvas_id ? (
              <Link
                href={canvasHref(project)}
                aria-label={`打开画布 · ${project.name}`}
              >
                <div className="canvas-tile-preview">
                  <Grid2X2 size={40} strokeWidth={1} />
                </div>
                <h3>{project.name}</h3>
                <p>
                  {project.aspect_ratio} ·{" "}
                  {new Date(project.update_time).toLocaleDateString("zh-CN")}{" "}
                  更新
                </p>
              </Link>
            ) : (
              <>
                <h3>{project.name}</h3>
                <p>暂无默认画布</p>
              </>
            )}
          </article>
        ))}
      </div>
      {page?.next_cursor && (
        <Button
          className="mt-5"
          variant="outline"
          disabled={loading}
          onClick={() => void loadMore()}
        >
          {loading ? "正在读取…" : "加载更多"}
        </Button>
      )}
    </div>
  );
}
