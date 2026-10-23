"use client";

import { useDeferredValue, useEffect, useState } from "react";
import useSWR from "swr";
import { Check, ChevronsUpDown } from "lucide-react";
import {
  getAssetLibrary,
  listAssetLibraries,
} from "@/components/canvas/runtime/api/assetBffController";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { unwrap } from "@/components/canvas/runtime/lib/request";

export function assetLibraryTarget(value: string) {
  return value === "ALL" || value === "GENERAL"
    ? { libraryScope: "GENERAL" as const }
    : { libraryScope: "PROJECT" as const, projectId: value };
}

export function useAssetLibrary(projectId?: string | number) {
  const { account } = useAccount();
  return useSWR(
    account && projectId
      ? ["asset-library", String(account.id), projectId]
      : null,
    () => getAssetLibrary({ projectId: projectId! }).then(unwrap),
    { shouldRetryOnError: false },
  );
}

export function AssetLibrarySelect({
  value,
  onChange,
  includeAll = false,
  includeGeneral = true,
  enabledOnly = true,
  disabled = false,
  label = "所属素材库",
  prefix,
}: {
  value: string;
  onChange: (value: string, library?: CanvasContract.AssetLibraryVO) => void;
  includeAll?: boolean;
  includeGeneral?: boolean;
  enabledOnly?: boolean;
  disabled?: boolean;
  label?: string;
  prefix?: string;
}) {
  const { account } = useAccount();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState({ search: "", page: 1 });
  const deferredQuery = useDeferredValue(query);
  const keyword = deferredQuery.search.trim();
  const page = deferredQuery.page;
  const detail = useAssetLibrary(
    value && value !== "ALL" && value !== "GENERAL" ? value : undefined,
  );
  const list = useSWR(
    open && account
      ? ["asset-libraries", String(account.id), keyword, page, enabledOnly]
      : null,
    () =>
      listAssetLibraries({
        current: String(page),
        pageSize: "20",
        keyword,
        enabledOnly,
      }).then(unwrap),
    { shouldRetryOnError: false },
  );
  useEffect(() => {
    if (list.error) notifyError(list.error, "项目列表读取失败");
  }, [list.error]);
  const loading = query !== deferredQuery || list.isLoading;
  const total = Number(list.data?.total ?? 0);
  const pages = Math.max(1, Math.ceil(total / 20));
  const title =
    value === "ALL"
      ? "全部素材"
      : value === "GENERAL"
        ? "通用素材"
        : (detail.data?.projectPath ??
          (detail.isLoading ? "正在读取项目…" : "选择项目"));
  function choose(next: string, library?: CanvasContract.AssetLibraryVO) {
    onChange(next, library);
    setOpen(false);
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          disabled={disabled}
          aria-label={label}
          className="max-w-full min-w-0 justify-between gap-2"
        >
          {prefix && (
            <span className="shrink-0 text-muted-foreground">{prefix}</span>
          )}
          <span className="min-w-0 flex-1 truncate text-left" title={title}>
            {title}
          </span>
          <ChevronsUpDown className="size-4 shrink-0" aria-hidden="true" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        collisionPadding={16}
        className="max-h-[min(32rem,var(--radix-popover-content-available-height))] w-100 max-w-[calc(100vw-2rem)] gap-2 overflow-hidden p-2 [@media(max-height:600px)]:gap-1"
      >
        <Input
          aria-label="搜索项目素材库"
          placeholder="搜索项目名称…"
          className="h-10 shrink-0"
          maxLength={100}
          value={query.search}
          onChange={(event) =>
            setQuery({ search: event.target.value, page: 1 })
          }
        />
        <div
          className="shrink-0 space-y-1 [@media(max-height:600px)]:grid [@media(max-height:600px)]:grid-cols-2 [@media(max-height:600px)]:gap-1 [@media(max-height:600px)]:space-y-0"
          aria-label="素材范围"
        >
          {includeAll && (
            <Button
              type="button"
              variant={value === "ALL" ? "secondary" : "ghost"}
              className="h-10 w-full justify-between"
              aria-pressed={value === "ALL"}
              onClick={() => choose("ALL")}
            >
              全部素材
              {value === "ALL" && (
                <Check className="size-4" aria-hidden="true" />
              )}
            </Button>
          )}
          {includeGeneral && (
            <Button
              type="button"
              variant={value === "GENERAL" ? "secondary" : "ghost"}
              className="h-10 w-full justify-between"
              aria-pressed={value === "GENERAL"}
              onClick={() => choose("GENERAL")}
            >
              通用素材
              {value === "GENERAL" && (
                <Check className="size-4" aria-hidden="true" />
              )}
            </Button>
          )}
        </div>
        <p className="shrink-0 px-2 pt-2 text-xs text-muted-foreground [@media(max-height:600px)]:pt-0">
          {enabledOnly ? "项目素材库" : "选择已有项目"}
        </p>
        <div
          key={`${keyword}:${page}`}
          className="max-h-60 min-h-0 space-y-1 overflow-x-hidden overflow-y-auto overscroll-contain"
          aria-label="可选项目素材库"
          aria-busy={loading}
        >
          {loading ? (
            <p role="status" className="p-2 text-sm">
              正在读取项目…
            </p>
          ) : list.error ? (
            <Button
              type="button"
              variant="ghost"
              onClick={() => void list.mutate()}
            >
              重新读取
            </Button>
          ) : !list.data?.records?.length ? (
            <p role="status" className="p-2 text-sm text-muted-foreground">
              {page > 1
                ? "当前页暂无项目，请返回上一页"
                : keyword
                  ? "没有匹配的项目"
                  : enabledOnly
                    ? "尚未创建项目素材库"
                    : "请先在项目空间创建项目"}
            </p>
          ) : (
            list.data.records.map((library) => (
              <Button
                key={library.projectId}
                type="button"
                variant={
                  value === String(library.projectId) ? "secondary" : "ghost"
                }
                className="h-10 w-full min-w-0 justify-between text-left"
                aria-pressed={value === String(library.projectId)}
                title={library.projectPath}
                onClick={() => choose(String(library.projectId), library)}
              >
                <span className="min-w-0 flex-1 truncate">
                  {library.projectPath}
                </span>
                {value === String(library.projectId) && (
                  <Check className="size-4 shrink-0" aria-hidden="true" />
                )}
              </Button>
            ))
          )}
        </div>
        <div
          className="flex min-w-0 shrink-0 items-center gap-2"
          aria-label="项目素材库分页"
        >
          <span
            className="min-w-0 flex-1 truncate text-xs text-muted-foreground"
            role="status"
          >
            {loading
              ? "正在读取…"
              : list.error
                ? "读取失败"
                : `共${total}个项目 · ${page > pages ? `第${page}页` : `${page}/${pages}`}`}
          </span>
          <div className="flex shrink-0 items-center gap-1">
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="h-9 px-2"
              disabled={page === 1 || loading}
              onClick={() =>
                setQuery((current) => ({ ...current, page: current.page - 1 }))
              }
            >
              上一页
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="h-9 px-2"
              disabled={page >= pages || loading || !!list.error}
              onClick={() =>
                setQuery((current) => ({ ...current, page: current.page + 1 }))
              }
            >
              下一页
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
