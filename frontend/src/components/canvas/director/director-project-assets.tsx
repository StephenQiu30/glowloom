"use client";
import { useEffect, useRef, useState } from "react";
import { getMediaPreview, listMediaLibrary } from "@/api/media";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";
import { Button } from "@/components/ui/button";
import {
  directorProjectQuery,
  requireDirectorProjectPage,
} from "./director-project-media-contract";
import { DirectorProjectAssetPreview } from "./director-asset-preview";

export type DirectorAssetObjectKind =
  "actor" | "model" | "billboard" | "panorama";
export type DirectorProjectAssetsProps = {
  projectID: string;
  kind: "model" | "image";
  objectKind: DirectorAssetObjectKind;
  search: string;
  disabled?: boolean;
  guard: () => void;
  onSelect: (
    assets: API.AssetSummary[],
    kind: DirectorAssetObjectKind,
  ) => Promise<void> | void;
};
export function DirectorProjectAssets(props: DirectorProjectAssetsProps) {
  return (
    <ProjectAssets
      key={`${props.projectID}/${props.kind}/${props.search}`}
      {...props}
    />
  );
}
function ProjectAssets({
  projectID,
  kind,
  objectKind,
  search,
  disabled,
  guard,
  onSelect,
}: DirectorProjectAssetsProps) {
  const [items, setItems] = useState<API.AssetSummary[]>([]),
    [page, setPage] = useState(0),
    [hasMore, setHasMore] = useState(false),
    [loading, setLoading] = useState(true),
    [error, setError] = useState(""),
    [retry, setRetry] = useState(0),
    [selecting, setSelecting] = useState(false);
  const active = useRef<AbortController | null>(null),
    busy = useRef(false);
  const opening = useRef({
    scope: getScopeGeneration(),
    session: getSessionGeneration(),
  });
  const live = useRef(true);
  const action = useRef<AbortController | null>(null);
  const latest = useRef({ disabled, objectKind, guard, onSelect });
  useEffect(() => {
    latest.current = { disabled, objectKind, guard, onSelect };
  }, [disabled, objectKind, guard, onSelect]);
  const current = () =>
    live.current &&
    opening.current.scope === getScopeGeneration() &&
    opening.current.session === getSessionGeneration();
  async function load(index: number, controller: AbortController) {
    const result = requireDirectorProjectPage(
      await listMediaLibrary(
        directorProjectQuery(
          projectID,
          kind,
          search,
          index,
                ),
        { signal: controller.signal },
      ),
      projectID,
      kind,
      index,
    );
    if (controller.signal.aborted || !current()) return;
    setItems((previous) =>
      index === 1
        ? result.items
        : [
            ...previous,
            ...result.items.filter(
              (item) => !previous.some((old) => old.id === item.id),
            ),
          ],
    );
    setPage(index);
    setHasMore(result.hasMore);
    setError("");
  }
  useEffect(() => {
    live.current = true;
    const controller = new AbortController();
    active.current = controller;
    busy.current = true;
    const timer = setTimeout(
      () => {
        void load(1, controller)
          .catch((cause) => {
            if (!controller.signal.aborted && current())
              setError(cause instanceof Error ? cause.message : "素材读取失败");
          })
          .finally(() => {
            if (!controller.signal.aborted && current()) {
              busy.current = false;
              setLoading(false);
            }
          });
      },
      search ? 200 : 0,
    );
    return () => {
      clearTimeout(timer);
      controller.abort();
      active.current?.abort();
      action.current?.abort();
      live.current = false;
    };
    // A keyed instance owns one project/kind/query; retry restarts that same scope.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [retry]);
  async function more() {
    if (busy.current || !current()) return;
    busy.current = true;
    setLoading(true);
    const controller = new AbortController();
    active.current = controller;
    try {
      await load(page + 1, controller);
    } catch (cause) {
      if (!controller.signal.aborted && current())
        setError(cause instanceof Error ? cause.message : "素材读取失败");
    } finally {
      if (!controller.signal.aborted && current()) {
        busy.current = false;
        setLoading(false);
      }
    }
  }
  async function select(asset: API.AssetSummary) {
    if (action.current || latest.current.disabled || !current()) return;
    const controller = new AbortController();
    action.current = controller;
    setSelecting(true);
    setError("");
    const openingKind = latest.current.objectKind;
    const openingGuard = latest.current.guard;
    try {
      openingGuard();
      const receipt = await getMediaPreview(
        { pid: projectID, asset_id: asset.id! },
        { signal: controller.signal },
      );
      if (!current() || controller.signal.aborted) return;
      if (
        latest.current.disabled ||
        latest.current.objectKind !== openingKind ||
        !receipt.asset ||
        receipt.asset?.id !== asset.id ||
        receipt.asset.project_id !== projectID ||
        receipt.asset.kind !== kind ||
        receipt.asset.revision !== asset.revision
      )
        throw new Error("素材或编辑权限已变化，请重新选择");
      openingGuard();
      latest.current.guard();
      await latest.current.onSelect([receipt.asset], openingKind);
    } catch (cause) {
      if (current() && !controller.signal.aborted)
        setError(cause instanceof Error ? cause.message : "素材无法放置");
    } finally {
      if (current() && !controller.signal.aborted) {
        action.current = null;
        setSelecting(false);
      }
    }
  }
  return (
    <section aria-label="当前项目素材">
      <h4>
        我的 <span className="text-muted-foreground">· 当前项目</span>
      </h4>
      {loading && items.length === 0 && <p role="status">正在加载素材…</p>}
      {!loading && !error && items.length === 0 && (
        <p className="director-library-empty">
          {search
            ? "没有匹配的素材"
            : kind === "model"
              ? "暂无模型，可导入本地模型或从项目选择。"
              : "暂无背景图片，可从项目或画布选择。"}
        </p>
      )}
      <div className="director-library-grid">
        {items.map((asset) => (
          <div key={asset.id} className="director-library-card">
            <DirectorProjectAssetPreview
              key={`${projectID}/${asset.id}/${asset.revision}`}
              projectID={projectID}
              asset={asset}
            />
            <Button
              className="director-library-card-label"
              variant="ghost"
              disabled={disabled || selecting}
              onClick={() => void select(asset)}
            >
              <span>{asset.file_name || "未命名素材"}</span>
              <small>{kind === "model" ? "3D 模型" : "图片"}</small>
            </Button>
          </div>
        ))}
      </div>
      {error && (
        <div role="alert">
          <p>{error}</p>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              setLoading(true);
              setRetry((v) => v + 1);
            }}
          >
            重试
          </Button>
        </div>
      )}
      {hasMore && !error && (
        <Button
          size="sm"
          variant="ghost"
          disabled={loading}
          onClick={() => void more()}
        >
          {loading ? "正在加载…" : "加载更多"}
        </Button>
      )}
    </section>
  );
}
