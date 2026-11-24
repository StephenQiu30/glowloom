"use client";

import { DEFAULT_PAGE_SIZE } from "@/lib/pagination";
import { PageSizeSelect } from "@/components/ui/page-size-select";
import { useDeferredValue, useEffect, useState } from "react";
import { FileText, Plus, Search } from "lucide-react";
import dynamic from "next/dynamic";
import { listMediaLibrary } from "@/api/media";
import { ContentState } from "@/components/layout/content-state";
import { ProjectMediaPreview } from "@/components/media/project-media";
import {
  defaultProjectPickerFilter,
  projectPickerQuery,
  requireProjectPickerPage,
  type ProjectPickerAsset,
} from "@/components/media/project-picker";
import { Button } from "@/components/ui/button";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import {
  SelectGroup,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { readErrorMessage } from "@/lib/read-error";

const UploadDialog = dynamic(() =>
  import("@/components/media/upload-dialog").then(
    (module) => module.UploadDialog,
  ),
);

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
  writable,
  onReference,
}: {
  active: boolean;
  actorID: string;
  projectID: string;
  canReference: boolean;
  writable: boolean;
  onReference: (asset: API.AssetSummary) => void;
}) {
  const [kind, setKind] = useState("all");
  const [selectedID, setSelectedID] = useState("");
  const [uploadOpen, setUploadOpen] = useState(false);
  const [purpose, setPurpose] = useState("all");
  const [search, setSearch] = useState("");
  const [pageSize, setPageSize] = useState<number>(DEFAULT_PAGE_SIZE);
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
    pageSize,
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
          kind === "all" ? undefined : kind,
          query,
          deferredPage,
          defaultProjectPickerFilter,
          pageSize,
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
              kind === "all" ? undefined : kind,
              deferredPage,
              actorID,
              pageSize,
            ),
          });
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setResult({ key, error: readErrorMessage(error) });
      });
    return () => controller.abort();
  }, [
    active,
    actorID,
    projectID,
    kind,
    purpose,
    query,
    deferredPage,
    key,
    pageSize,
  ]);

  const current = result?.key === key ? result : undefined;
  const items = current?.page?.items;
  const selected =
    items?.find((asset) => asset.id === selectedID) ?? items?.[0];
  const groups = [...kinds].reverse();
  const nameOf = (asset: ProjectPickerAsset) =>
    asset.title || asset.file_name || "未命名素材";
  const sourceOf = (asset: ProjectPickerAsset) =>
    asset.sourceLabel || asset.file_name;

  return (
    <div className="space-y-4">
      {uploadOpen && writable && (
        <UploadDialog
          projectID={projectID}
          destinationLabel="当前项目"
          onClose={() => setUploadOpen(false)}
          onUploaded={(asset) => {
            setSelectedID(asset?.id ?? "");
            setSearch("");
            setKind("all");
            setPurpose("all");
            setPage(1);
            setRetry((previous) => previous + 1);
            setUploadOpen(false);
          }}
        />
      )}
      <div className="grid grid-cols-2 gap-2 lg:pl-52 xl:flex xl:flex-wrap">
        <InputGroup className="col-span-2 min-w-0 xl:max-w-64">
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
          <SelectTrigger
            aria-label="素材类型"
            className="w-full max-lg:h-11 xl:w-auto"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="all">全部类型</SelectItem>
              {kinds.map(([value, label]) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Select
          value={purpose}
          onValueChange={(value) => {
            setPurpose(value);
            setPage(1);
          }}
        >
          <SelectTrigger
            aria-label="素材用途"
            className="w-full max-lg:h-11 xl:w-auto"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {purposes.map(([value, label]) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        {writable && (
          <Button
            variant="secondary"
            size="sm"
            className="col-span-2 justify-self-end max-lg:h-11 xl:ml-auto"
            onClick={() => setUploadOpen(true)}
          >
            <Plus data-icon="inline-start" /> 添加素材
          </Button>
        )}
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
      {items?.length && selected ? (
        <div className="grid min-w-0 gap-4 xl:grid-cols-[minmax(240px,2fr)_minmax(330px,3fr)]">
          <div className="min-w-0 space-y-5">
            {groups.map(([assetKind, label]) => {
              const matches = items.filter((asset) => asset.kind === assetKind);
              if (!matches.length) return null;
              return (
                <section
                  key={assetKind}
                  className="space-y-2"
                  aria-label={label}
                >
                  <h2 className="font-medium">
                    {label}{" "}
                    <span className="ml-1 text-xs text-muted-foreground">
                      {matches.length}
                    </span>
                  </h2>
                  <ul
                    className={
                      assetKind === "document"
                        ? "space-y-2"
                        : "grid grid-cols-2 gap-2"
                    }
                  >
                    {matches.map((asset) => (
                      <li key={asset.id} className="min-w-0">
                        <Button
                          size="asset-card"
                          variant={
                            selected.id === asset.id ? "secondary" : "outline"
                          }
                          className="h-full overflow-hidden border p-2.5"
                          aria-pressed={selected.id === asset.id}
                          onClick={() => setSelectedID(asset.id ?? "")}
                        >
                          {assetKind === "document" ? (
                            <FileText className="mb-1.5" />
                          ) : (
                            <span
                              aria-hidden="true"
                              className="relative mb-2 flex aspect-video items-center justify-center overflow-hidden rounded-md bg-muted"
                            >
                              {assetKind === "audio" ? (
                                <span className="text-xs text-muted-foreground">
                                  音频
                                </span>
                              ) : (
                                <ProjectMediaPreview
                                  projectID={projectID}
                                  id={asset.id!}
                                  kind={asset.kind}
                                  compact
                                />
                              )}
                            </span>
                          )}
                          <span
                            className="truncate font-medium"
                            title={nameOf(asset)}
                          >
                            {nameOf(asset)}
                          </span>
                          <span
                            className="truncate text-xs text-muted-foreground"
                            title={sourceOf(asset)}
                          >
                            {sourceOf(asset)}
                          </span>
                        </Button>
                      </li>
                    ))}
                  </ul>
                </section>
              );
            })}
          </div>
          <section
            aria-label="素材详情"
            className="min-w-0 space-y-3 self-start rounded-lg border bg-card p-4"
          >
            <h2 className="text-base font-medium break-words">
              {nameOf(selected)}
            </h2>
            <div className="relative aspect-video max-h-72 overflow-hidden rounded-md bg-muted">
              <ProjectMediaPreview
                projectID={projectID}
                id={selected.id!}
                kind={selected.kind}
                fit="contain"
                interactive
                large
              />
            </div>
            {sourceOf(selected) && (
              <p className="text-sm break-words text-muted-foreground">
                {sourceOf(selected)}
              </p>
            )}
            <Button
              size="sm"
              className="max-lg:h-11"
              disabled={!canReference || search !== query}
              onClick={() => onReference(selected)}
            >
              加入对话
            </Button>
          </section>
        </div>
      ) : null}
      {current?.page ? (
        <div className="flex justify-between gap-2">
          <PageSizeSelect
            value={pageSize}
            label="项目素材每页数量"
            onChange={(size) => {
              setPage(1);
              setPageSize(size);
            }}
            disabled={false}
          />
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
