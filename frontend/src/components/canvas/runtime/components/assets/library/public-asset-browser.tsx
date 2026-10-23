"use client";

import { useDeferredValue, useState } from "react";
import useSWR from "@/components/canvas/runtime/lib/use-live-query";
import { FileImage, Music2, Video } from "lucide-react";
import * as api from "@/components/canvas/runtime/api/assetReviewBffController";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty";
import { WorkspaceCardSkeleton } from "@/components/canvas/runtime/components/feedback/workspace-card-skeleton";
import { ListPagination } from "@/components/canvas/runtime/components/layout/workspace/list-pagination";
import { AssetGrantedPreview } from "../review/asset-granted-preview";
import type { InsertAssetPayload } from "../picker/asset-picker-modal";

export function PublicAssetBrowser({
  onInsert,
  kinds,
}: {
  onInsert?: (asset: InsertAssetPayload) => void;
  kinds?: readonly string[];
}) {
  const { account } = useAccount();
  const [keyword, setKeyword] = useState("");
  const query = useDeferredValue(keyword.trim());
  const [kind, setKind] = useState(kinds?.[0] ?? "all");
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(12);
  const [selected, setSelected] = useState<CanvasContract.PublicAssetVO>();
  const [busy, setBusy] = useState(false);
  const list = useSWR(
    account
      ? ["public-assets", String(account.id), page, size, query, kind]
      : null,
    async (_key, options) =>
      unwrap(
        await api.listPublicAssets(
          {
            current: page,
            pageSize: size,
            keyword: query,
            kind: kind === "all" ? undefined : kind,
          },
          options,
        ),
      ),
    { shouldRetryOnError: false },
  );

  async function open(asset: CanvasContract.PublicAssetVO) {
    if (!asset.id) return;
    setBusy(true);
    try {
      setSelected(unwrap(await api.assetReviewPublicDetail({ id: asset.id })));
    } catch (error) {
      notifyError(error, "公共素材当前不可用");
      await list.mutate();
    } finally {
      setBusy(false);
    }
  }

  async function insert() {
    if (!selected?.id || !onInsert || busy) return;
    setBusy(true);
    try {
      const latest = unwrap(
        await api.assetReviewPublicDetail({ id: selected.id }),
      );
      if (
        latest.revision !== selected.revision ||
        !latest.fileId ||
        !latest.fileVersion ||
        !latest.id
      )
        throw new Error("公共发布或许可版本已变化，请重新选择");
      const read = unwrap(
        await api.assetReviewRead({
          scope: "PUBLIC",
          id: latest.id,
          revision: latest.revision!,
          purpose: "PREVIEW",
        }),
      );
      if (!read.url?.startsWith("/api/file/content?ticket="))
        throw new Error("公共预览授权无效");
      const reference = {
        fileId: latest.fileId,
        fileVersion: latest.fileVersion,
        publicAssetId: String(latest.id),
        publicRevision: String(latest.revision),
      };
      if (latest.kind === "image")
        onInsert({
          ...reference,
          kind: "image",
          title: latest.title ?? "",
          dataUrl: read.url,
        });
      else if (latest.kind === "video" || latest.kind === "audio")
        onInsert({
          ...reference,
          kind: latest.kind,
          title: latest.title ?? "",
          url: read.url,
        });
      else throw new Error("当前公共引用支持图片、视频和音频");
      setSelected(undefined);
    } catch (error) {
      notifyError(error, "公共素材引用未完成");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <Input
          value={keyword}
          maxLength={100}
          placeholder="搜索公共素材"
          aria-label="搜索公共素材"
          onChange={(event) => {
            setKeyword(event.target.value);
            setPage(1);
          }}
          className="min-w-0 flex-1"
        />
        <Select
          value={kind}
          onValueChange={(value) => {
            setKind(value);
            setPage(1);
          }}
        >
          <SelectTrigger className="w-auto" aria-label="公共素材类型">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {!kinds ? <SelectItem value="all">全部类型</SelectItem> : null}
              {!kinds || kinds.includes("image") ? (
                <SelectItem value="image">图片</SelectItem>
              ) : null}
              {!kinds || kinds.includes("video") ? (
                <SelectItem value="video">视频</SelectItem>
              ) : null}
              {!kinds || kinds.includes("audio") ? (
                <SelectItem value="audio">音频</SelectItem>
              ) : null}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Button
          variant="outline"
          disabled={list.isLoading}
          onClick={() => void list.mutate()}
        >
          刷新
        </Button>
      </div>
      <p className="text-sm text-muted-foreground">
        公共发布固定版本，仅供已授权的平台内预览与参考；具体模型仍需核验输入资格。
      </p>
      {list.isLoading ? (
        <WorkspaceCardSkeleton />
      ) : list.error ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>公共素材暂不可读取</EmptyTitle>
            <EmptyDescription>
              检索同步或源许可发生变化，请刷新重试。
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : !list.data?.records?.length ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>暂无公共素材</EmptyTitle>
            <EmptyDescription>
              管理员发布后将在此显示，私人素材不会自动公开。
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="grid min-w-0 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {list.data.records.map((asset) => (
            <Card key={asset.id} className="min-w-0">
              <CardContent>
                <Button
                  variant="outline"
                  className="workspace-media h-auto w-full bg-muted/30"
                  disabled={busy}
                  onClick={() => void open(asset)}
                  aria-label={`查看公共素材 ${asset.title}`}
                >
                  {asset.kind === "video" ? (
                    <Video className="text-muted-foreground" />
                  ) : asset.kind === "audio" ? (
                    <Music2 className="text-muted-foreground" />
                  ) : (
                    <FileImage className="text-muted-foreground" />
                  )}
                </Button>
              </CardContent>
              <CardHeader>
                <CardTitle className="truncate">{asset.title}</CardTitle>
                <CardDescription className="flex flex-wrap gap-2">
                  <Badge variant="outline">
                    {asset.sourceType === "PLATFORM_GENERATED"
                      ? "平台生成·免审核"
                      : "预审核通过"}
                  </Badge>
                  <span>平台内参考</span>
                </CardDescription>
              </CardHeader>
            </Card>
          ))}
        </div>
      )}
      {list.data && !list.error ? (
        <ListPagination
          label="公共素材分页"
          current={page}
          pageSize={size}
          total={Number(list.data.total ?? 0)}
          onPageChange={setPage}
          onPageSizeChange={(value) => {
            setSize(value);
            setPage(1);
          }}
        />
      ) : null}
      <Dialog
        open={!!selected}
        onOpenChange={(value) => {
          if (!value && !busy) setSelected(undefined);
        }}
      >
        <DialogContent className="dialog-standard">
          <DialogHeader>
            <DialogTitle>{selected?.title ?? "公共素材"}</DialogTitle>
            <DialogDescription>{selected?.licenseText}</DialogDescription>
          </DialogHeader>
          {selected?.id && selected.revision ? (
            <AssetGrantedPreview
              scope="PUBLIC"
              id={String(selected.id)}
              revision={String(selected.revision)}
              kind={selected.kind ?? "image"}
              title={selected.title ?? "公共素材"}
            />
          ) : null}
          <DialogFooter>
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => setSelected(undefined)}
            >
              关闭
            </Button>
            {onInsert ? (
              <Button disabled={busy} onClick={() => void insert()}>
                引用固定版本
              </Button>
            ) : null}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
