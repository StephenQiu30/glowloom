"use client";
import Link from "next/link";
import { useSyncExternalStore } from "react";
import { useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ApiError } from "@/lib/request";
import {
  libraryFilterSchema,
  libraryScopeSchema,
  libraryUUID,
  defaultLibraryFilter,
} from "./library-model";
import { listLibrary } from "./library-queries";
import { LibraryBrowser } from "./library-browser";
const subscribeOrigin = () => () => {};
function readURL(parameters: URLSearchParams) {
  const kind =
    parameters.get("scope") ??
    (parameters.has("project_id") ? "project" : "personal");
  const scope = libraryScopeSchema.parse(
    kind === "project"
      ? { kind, project_id: parameters.get("project_id") }
      : { kind },
  );
  const filter = libraryFilterSchema.parse({
    ...defaultLibraryFilter(scope),
    page: Number(parameters.get("page") ?? 1),
    page_size: Number(
      parameters.get("page_size") ?? (scope.kind === "personal" ? 40 : 20),
    ),
    kind: parameters.get("kind") ?? "",
    category: parameters.get("category") ?? "",
    folder: parameters.get("folder") ?? "all",
    favorite_only: parameters.get("favorite") === "true",
    recent_only: parameters.get("recent") === "true",
    catalog_state: parameters.get("state") ?? "active",
    search: parameters.get("search") ?? "",
    order: parameters.get("order") ?? "updated_desc",
  });
  if (scope.kind === "project" && filter.favorite_only)
    throw new Error("项目素材库不支持个人收藏筛选。");
  const selected = parameters.get("asset_id");
  if (selected) libraryUUID.parse(selected);
  const view = parameters.get("view") ?? "grid";
  if (view !== "grid" && view !== "list") throw new Error("素材视图参数无效。");
  return { scope, filter, selected, view };
}
export function AssetsWorkspace() {
  const parameters = useSearchParams();
  let parsed: ReturnType<typeof readURL> | undefined;
  try {
    parsed = readURL(new URLSearchParams(parameters.toString()));
  } catch {}
  const origin = useSyncExternalStore(
    subscribeOrigin,
    () => window.location.origin,
    () => null,
  );
  const scope = parsed?.scope;
  const context = useQuery({
    queryKey: [
      "media-library-context",
      origin,
      scope?.kind,
      ...(scope?.kind === "project" ? [scope.project_id] : []),
    ],
    queryFn: ({ signal }) =>
      listLibrary(
        scope!,
        { ...defaultLibraryFilter(scope!), page_size: 1 },
        signal,
      ),
    enabled: Boolean(scope && origin),
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
  return (
    <section
      aria-label="素材工作区"
      className="mx-auto flex w-full max-w-7xl flex-col gap-6"
    >
      <header className="flex flex-col gap-2">
        <h1 className="text-xl font-semibold tracking-tight">
          {scope?.kind === "project" ? "项目素材" : "我的素材"}
        </h1>
        <p className="text-sm text-muted-foreground">
          管理个人原件与项目素材，在画布和剧本中继续创作。
        </p>
      </header>
      {!parsed ? (
        <Alert variant="destructive">
          <AlertTitle>素材库地址无效</AlertTitle>
          <AlertDescription>
            请核对范围、项目UUID、目录与分页筛选参数。
            <Button variant="outline" asChild>
              <Link href="/assets">打开个人素材库</Link>
            </Button>
          </AlertDescription>
        </Alert>
      ) : context.isError ? (
        <Alert variant="destructive">
          <AlertTitle>当前素材库不可读取</AlertTitle>
          <AlertDescription>
            <p>{context.error.message}</p>
            <Button variant="outline" onClick={() => void context.refetch()}>
              重试读取素材库身份
            </Button>
          </AlertDescription>
        </Alert>
      ) : !context.data ||
        !context.isFetchedAfterMount ||
        !context.isSuccess ||
        !origin ? (
        <p role="status">正在确认当前素材库范围…</p>
      ) : (
        <LibraryBrowser
          key={`${origin}:${context.data.current_actor_id}:${context.data.current_org_id}:${context.data.library_id}`}
          identity={{
            origin,
            actorId: context.data.current_actor_id,
            orgId: context.data.current_org_id,
            libraryId: context.data.library_id,
            scope: context.data.scope,
          }}
          initial={context.data}
          filter={parsed.filter}
          selected={parsed.selected}
          view={parsed.view}
          refreshContext={async () => {
            const result = await context.refetch({ throwOnError: true });
            if (!result.data) throw new ApiError(503, "context_unavailable");
            return result.data;
          }}
        />
      )}
    </section>
  );
}
