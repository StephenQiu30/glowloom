"use client";

import { useDeferredValue, useEffect, useState } from "react";
import { Search } from "lucide-react";
import { listMediaLibrary } from "@/api/media";
import { ContentState } from "@/components/layout/content-state";
import { ProjectMediaPreview } from "@/components/media/project-media";
import {
  defaultProjectPickerFilter,
  projectPickerQuery,
  requireProjectPickerPage,
} from "@/components/media/project-picker";
import { Button } from "@/components/ui/button";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { readErrorMessage } from "@/lib/read-error";

const kinds = [
  ["image", "图片"],
  ["video", "视频"],
  ["audio", "音频"],
  ["document", "文档"],
] as const;
const purposes = [
  ["all", "全部用途"],
  ["character", "角色"],
  ["environment", "场景"],
  ["prop", "道具"],
  ["material", "素材"],
  ["other", "其他"],
] as const;

export function CanvasStoryboardMaterials({
  active,
  actorID,
  projectID,
  canReference,
  onReference,
}: {
  active: boolean;
  actorID: string;
  projectID: string;
  canReference: boolean;
  onReference: (asset: API.AssetSummary) => void;
}) {
  const [kind, setKind] = useState("image");
  const [purpose, setPurpose] = useState("all");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const [retry, setRetry] = useState(0);
  const query = useDeferredValue(search);
  const deferredPage = search === query ? page : 1;
  const key = JSON.stringify([
    actorID,
    projectID,
    kind,
    purpose,
    query,
    deferredPage,
    retry,
  ]);
  const [result, setResult] = useState<{
    key: string;
    page?: ReturnType<typeof requireProjectPickerPage>;
    error?: string;
  }>();
  useEffect(() => {
    if (!active) return;
    const controller = new AbortController();
    void listMediaLibrary(
      {
        ...projectPickerQuery(
          projectID,
          kind,
          query,
          deferredPage,
          defaultProjectPickerFilter,
        ),
        category: purpose === "all" ? undefined : purpose,
      },
      { signal: controller.signal },
    )
      .then((value) => {
        if (!controller.signal.aborted)
          setResult({
            key,
            page: requireProjectPickerPage(
              value,
              projectID,
              kind,
              deferredPage,
              actorID,
            ),
          });
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setResult({ key, error: readErrorMessage(error) });
      });
    return () => controller.abort();
  }, [active, actorID, projectID, kind, purpose, query, deferredPage, key]);

  const current = result?.key === key ? result : undefined;
  return (
    <div className="space-y-4 p-3 sm:p-4">
      <div className="grid grid-cols-2 gap-2">
        <InputGroup className="col-span-2">
          <InputGroupAddon>
            <Search />
          </InputGroupAddon>
          <InputGroupInput
            aria-label="搜索项目素材"
            placeholder="搜索项目素材"
            value={search}
            onChange={(event) => {
              setSearch(event.target.value);
              setPage(1);
            }}
          />
        </InputGroup>
        <Select
          value={kind}
          onValueChange={(value) => {
            setKind(value);
            setPage(1);
          }}
        >
          <SelectTrigger aria-label="素材类型" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {kinds.map(([value, label]) => (
              <SelectItem key={value} value={value}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={purpose}
          onValueChange={(value) => {
            setPurpose(value);
            setPage(1);
          }}
        >
          <SelectTrigger aria-label="素材用途" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {purposes.map(([value, label]) => (
              <SelectItem key={value} value={value}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {!current ? (
        <ContentState state="loading" title="正在读取项目素材…" />
      ) : null}
      {current?.error ? (
        <ContentState
          state="error"
          title={current.error}
          actions={
            <Button
              variant="outline"
              onClick={() => setRetry((previous) => previous + 1)}
            >
              重新读取
            </Button>
          }
        />
      ) : null}
      {current?.page && !current.page.items.length ? (
        <ContentState title="没有符合条件的项目素材" />
      ) : null}
      {current?.page?.items.length ? (
        <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3">
          {current.page.items.map((asset) => (
            <li
              key={asset.id}
              className="min-w-0 overflow-hidden rounded-lg border"
            >
              <div className="relative aspect-video overflow-hidden bg-muted">
                <ProjectMediaPreview
                  projectID={projectID}
                  id={asset.id!}
                  kind={asset.kind}
                  fit="contain"
                  interactive
                />
              </div>
              <div className="space-y-1 p-2">
                <p
                  className="truncate font-medium"
                  title={asset.title || asset.file_name}
                >
                  {asset.title || asset.file_name || "未命名素材"}
                </p>
                {asset.sourceLabel || asset.file_name ? (
                  <p
                    className="truncate text-xs text-muted-foreground"
                    title={asset.sourceLabel || asset.file_name}
                  >
                    {asset.sourceLabel || asset.file_name}
                  </p>
                ) : null}
                <Button
                  variant="ghost"
                  size="sm"
                  className="w-full justify-start px-0 max-md:min-h-11"
                  disabled={!canReference || search !== query}
                  onClick={() => onReference(asset)}
                >
                  加入对话
                </Button>
              </div>
            </li>
          ))}
        </ul>
      ) : null}
      {current?.page && (page > 1 || current.page.hasMore) ? (
        <div className="flex justify-between gap-2">
          <Button
            variant="outline"
            disabled={page === 1}
            onClick={() => setPage((previous) => previous - 1)}
          >
            上一页
          </Button>
          <Button
            variant="outline"
            disabled={!current.page.hasMore}
            onClick={() => setPage((previous) => previous + 1)}
          >
            下一页
          </Button>
        </div>
      ) : null}
    </div>
  );
}
