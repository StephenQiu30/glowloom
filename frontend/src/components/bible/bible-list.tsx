"use client";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { bibleScopeKey, listBible } from "./bible-queries";
import { kindLabels, type BibleIdentity, type BibleKind } from "./bible-model";
export function BibleList({
  identity,
  kind,
  selected,
  locked,
  onSelect,
}: {
  identity: BibleIdentity;
  kind: BibleKind;
  selected?: string;
  locked: boolean;
  onSelect: (id: string) => void;
}) {
  const page = useInfiniteQuery({
    queryKey: [...bibleScopeKey(identity), "list", kind],
    queryFn: ({ signal, pageParam }) =>
      listBible(
        identity.projectId,
        kind,
        { cursor: pageParam },
        signal,
        identity,
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (result) => result.next_cursor,
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
  const summaries = page.data?.pages.flatMap((result) => result.entries) ?? [],
    unique =
      new Set(summaries.map((item) => item.head.id)).size === summaries.length;
  return (
    <section
      aria-label={`${kindLabels[kind]}列表`}
      className="min-w-0 space-y-3"
    >
      <h2 className="text-base font-medium">{kindLabels[kind]}</h2>
      {page.isPending && <p role="status">正在读取当前授权设定…</p>}
      {page.isError && (
        <div>
          <p role="alert">当前设定列表无法读取。</p>
          <Button
            variant="outline"
            disabled={locked}
            onClick={() => void page.refetch()}
          >
            重试{kindLabels[kind]}列表
          </Button>
        </div>
      )}
      {!unique && (
        <p role="alert">分页返回了重复设定身份，当前列表需要重新核验。</p>
      )}
      {page.isSuccess && unique && (
        <ul className="space-y-2">
          {summaries.map((item) => (
            <li key={item.head.id}>
              <Button
                variant={selected === item.head.id ? "secondary" : "ghost"}
                className="h-auto w-full justify-start text-left whitespace-normal"
                disabled={locked}
                onClick={() => onSelect(item.head.id)}
              >
                <span className="min-w-0 wrap-anywhere whitespace-pre-wrap">
                  <span className="block">{item.name}</span>
                  <span className="block text-xs text-muted-foreground">
                    rev{item.head.revision} ·{" "}
                    {item.head.deleted
                      ? "已回收 · "
                      : item.head.redirect_id
                        ? "已合并重定向 · "
                        : ""}
                    {item.head.confirmed_version_id ===
                    item.head.current_version_id
                      ? "当前版本已确认"
                      : item.head.confirmed_version_id
                        ? "已确认旧版本，当前待审核"
                        : "尚未确认"}
                  </span>
                  <span className="block text-xs text-muted-foreground">
                    {item.head.id}
                  </span>
                </span>
              </Button>
            </li>
          ))}
        </ul>
      )}
      {page.isSuccess && !summaries.length && (
        <p>当前项目没有{kindLabels[kind]}，可创建完整手工设定。</p>
      )}
      {page.hasNextPage && (
        <Button
          variant="outline"
          disabled={locked || page.isFetchingNextPage || !unique}
          onClick={() => void page.fetchNextPage()}
        >
          读取更多{kindLabels[kind]}
        </Button>
      )}
    </section>
  );
}
