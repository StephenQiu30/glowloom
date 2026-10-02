"use client";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  bibleScopeKey,
  getBibleHistory,
  getBibleSnapshot,
} from "./bible-queries";
import { BibleVersionBody } from "./bible-detail";
import type { BibleIdentity, BibleKind } from "./bible-model";

export function BibleHistoryDialog({
  identity,
  kind,
  id,
  version,
  onVersion,
  onClose,
  onCloseAutoFocus,
}: {
  identity: BibleIdentity;
  kind: BibleKind;
  id: string;
  version?: string;
  onVersion: (version?: string) => void;
  onClose: () => void;
  onCloseAutoFocus?: (event: Event) => void;
}) {
  const history = useInfiniteQuery({
    queryKey: [...bibleScopeKey(identity), "history", kind, id],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      getBibleHistory(identity, kind, id, pageParam, signal),
    getNextPageParam: (page) => page.next_cursor,
    staleTime: 0,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const snapshot = useQuery({
    queryKey: [...bibleScopeKey(identity), "snapshot", kind, id, version],
    queryFn: ({ signal }) =>
      getBibleSnapshot(identity, kind, id, version!, signal),
    enabled: Boolean(version),
    staleTime: 0,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        data-bible-dialog
        onCloseAutoFocus={onCloseAutoFocus}
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-5xl"
      >
        <DialogHeader>
          <DialogTitle>不可变设定版本历史</DialogTitle>
          <DialogDescription>
            读取原稳定身份的完整版本，确认指针改变不会覆盖旧版本。历史正文仅按当前授权读取。
          </DialogDescription>
        </DialogHeader>
        {history.isPending && <p role="status">正在读取原设定历史…</p>}
        {history.isError && (
          <div>
            <p role="alert">历史版本无法读取。</p>
            <Button variant="outline" onClick={() => void history.refetch()}>
              重试设定历史
            </Button>
          </div>
        )}
        <ul className="space-y-2">
          {history.isSuccess &&
            history.isFetchedAfterMount &&
            history.data.pages
              .flatMap((page) => page.versions)
              .map((item) => (
                <li key={item.id}>
                  <Button
                    variant={version === item.id ? "secondary" : "outline"}
                    onClick={() => onVersion(item.id)}
                  >
                    查看 v{item.number} · {item.created_at}
                  </Button>
                  <p className="text-xs break-all">
                    {item.id} · SHA {item.content_sha256} ·{" "}
                    {item.origin === "manual" ? "手工" : "正式结果采纳"}
                  </p>
                </li>
              ))}
        </ul>
        {history.hasNextPage && (
          <Button
            variant="outline"
            disabled={history.isFetchingNextPage}
            onClick={() => void history.fetchNextPage()}
          >
            读取更多不可变版本
          </Button>
        )}
        {version && (
          <section aria-label="选中不变版本" className="space-y-4">
            <Button variant="ghost" onClick={() => onVersion(undefined)}>
              关闭历史正文
            </Button>
            {snapshot.isPending && (
              <p role="status">正在核验身份并读取选中完整版本…</p>
            )}
            {snapshot.isError && (
              <div>
                <p role="alert">选中历史版本无法读取。</p>
                <Button
                  variant="outline"
                  onClick={() => void snapshot.refetch()}
                >
                  重试不变版本正文
                </Button>
              </div>
            )}
            {snapshot.isSuccess && snapshot.isFetchedAfterMount && (
              <BibleVersionBody
                key={snapshot.data.id}
                identity={identity}
                version={snapshot.data}
              />
            )}
          </section>
        )}
      </DialogContent>
    </Dialog>
  );
}
