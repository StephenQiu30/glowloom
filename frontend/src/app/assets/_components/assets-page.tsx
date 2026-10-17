"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { FieldSet } from "@/components/ui/field";
import Link from "next/link";
import { cn } from "cn";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  Upload,
  Trash2,
  Heart,
  FolderPlus,
  Folder,
  RotateCw,
  Download,
  ArrowLeft,
  CheckSquare,
} from "lucide-react";
import { BasicLayout } from "@/components/layout/basic-layout";
import { useSession } from "@/components/auth/session";
import { RequestError } from "@/lib/request";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/components/ui/card";
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  SelectGroup,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  listMediaLibrary,
  applyMediaLibraryCommand,
  downloadLibraryMediaAsset,
} from "@/api/media";
import {
  requireLibraryPage,
  requireLibraryReceipt,
  mediaError,
  type LibraryPage,
  type LibraryItem,
} from "@/components/media/library";
import { DownloadButton } from "@/components/media/download-button";
import { AssetPreview } from "@/components/media/asset-preview";
import { UploadDialog } from "@/components/media/upload-dialog";
import { GenerationComposer } from "@/components/generation/generation-composer";
const kinds = [
  { id: "image", label: "图片" },
  { id: "video", label: "视频" },
  { id: "audio", label: "音频" },
  { id: "document", label: "文档" },
  { id: "model", label: "3D" },
];
type Filter = {
  kind: string;
  search: string;
  favorite: boolean;
  trash: boolean;
  folder: string;
  order: string;
};
function Library({
  filter,
  onFolder,
  refresh,
  onRefresh,
  onLock,
}: {
  filter: Filter;
  onFolder: (id: string) => void;
  refresh: number;
  onRefresh: () => void;
  onLock: (locked: boolean) => void;
}) {
  const session = useSession();
  const router = useRouter();
  const [page, setPage] = useState<LibraryPage | null>(null);
  const [pageIndex, setPageIndex] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const [selected, setSelected] = useState<string[]>([]);
  const [batch, setBatch] = useState(false);
  const [detail, setDetail] = useState<LibraryItem | null>(null);
  const [folderName, setFolderName] = useState("");
  const [folderOpen, setFolderOpen] = useState(false);
  const [confirm, setConfirm] = useState<
    "recycle_items" | "restore_items" | null
  >(null);
  const [saving, setSaving] = useState(false);
  const [writeError, setWriteError] = useState("");
  const [pendingOperation, setPendingOperation] = useState<{
    body: API.LibraryCommand;
    key: string;
    base: LibraryPage;
  } | null>(null);
  const busy = useRef(false);
  useEffect(() => {
    const controller = new AbortController();
    listMediaLibrary(
      {
        scope: "personal",
        kind: filter.kind,
        search: filter.search,
        favorite_only: filter.favorite,
        catalog_state: filter.trash ? "trashed" : "active",
        folder_id: filter.folder || undefined,
        order: filter.order,
        page: pageIndex,
        page_size: 40,
      },
      { signal: controller.signal },
    )
      .then((v) => requireLibraryPage(v, session.actor.id))
      .then((result) => {
        if (!controller.signal.aborted) {
          setPage(result);
          setError("");
        }
      })
      .catch((e) => {
        if (!controller.signal.aborted) {
          if (e instanceof RequestError && e.status === 401)
            router.replace("/login");
          else setError(mediaError(e));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [filter, session.actor.id, router, pageIndex, retry, refresh]);
  function reload() {
    setLoading(true);
    setError("");
    setSelected([]);
    setDetail(null);
    setRetry((n) => n + 1);
  }
  async function command(body?: API.LibraryCommand) {
    if (!page || busy.current) return;
    const op =
      pendingOperation ??
      (body ? { body, key: crypto.randomUUID(), base: page } : null);
    if (!op) return;
    busy.current = true;
    onLock(true);
    setPendingOperation(op);
    setSaving(true);
    setWriteError("");
    try {
      requireLibraryReceipt(
        await applyMediaLibraryCommand(op.body, {
          headers: { "Idempotency-Key": op.key },
        }),
        op.base,
      );
      setPendingOperation(null);
      onLock(false);
      setConfirm(null);
      setFolderOpen(false);
      setFolderName("");
      reload();
      onRefresh();
    } catch (e) {
      setWriteError(mediaError(e));
      if (e instanceof RequestError && e.status >= 400 && e.status < 500) {
        setPendingOperation(null);
        onLock(false);
      }
    } finally {
      busy.current = false;
      setSaving(false);
    }
  }
  function favorite(item: LibraryItem) {
    if (!page) return;
    void command({
      scope: { kind: "personal" },
      expected_revision: page.revision,
      action: "update_item",
      item_id: item.id,
      expected_item_revision: item.revision,
      metadata: {
        title: item.title,
        category: item.category,
        tags: item.tags,
        source_label: item.source_label,
        note: item.note,
        folder_id: item.folder_id ?? undefined,
        favorite: !item.favorite,
      },
    });
  }
  const selection = page?.items.filter((i) => selected.includes(i.id)) ?? [];
  return (
    <>
      <div className="asset-actions">
        <Button
          variant="ghost"
          disabled={saving || !!pendingOperation}
          onClick={() => {
            setBatch(!batch);
            setSelected([]);
          }}
        >
          <CheckSquare />
          {batch ? "退出批量" : "批量操作"}
        </Button>
        <Button
          variant="ghost"
          disabled={saving || !!pendingOperation || !page}
          onClick={() => setFolderOpen(true)}
        >
          <FolderPlus />
          新建文件夹
        </Button>
        {filter.folder && (
          <Button
            variant="ghost"
            disabled={saving || !!pendingOperation}
            onClick={() => onFolder("")}
          >
            <ArrowLeft />
            全部资产
          </Button>
        )}
        <Button
          variant="ghost"
          className="ml-auto"
          disabled={loading || saving || !!pendingOperation}
          onClick={reload}
        >
          <RotateCw />
          刷新
        </Button>
      </div>
      {writeError && (
        <FeedbackToast
          message={writeError}
          action={
            saving
              ? undefined
              : {
                  label: pendingOperation ? "重试原操作" : "刷新资产版本",
                  onClick: () => {
                    if (pendingOperation) void command();
                    else {
                      setWriteError("");
                      reload();
                    }
                  },
                }
          }
        />
      )}
      {batch && (
        <div className="mb-5 flex items-center gap-4 rounded-lg border p-3">
          <Checkbox
            aria-label="选择本页全部资产"
            checked={
              !!page?.items.length && selection.length === page.items.length
            }
            disabled={saving}
            onCheckedChange={(v) =>
              setSelected(
                v === true ? (page?.items.map((i) => i.id) ?? []) : [],
              )
            }
          />
          <span className="text-sm">已选择 {selection.length} 项</span>
          <Button
            disabled={!selection.length || saving || !!pendingOperation}
            variant={filter.trash ? "outline" : "destructive"}
            onClick={() =>
              setConfirm(filter.trash ? "restore_items" : "recycle_items")
            }
          >
            {filter.trash ? <RotateCw /> : <Trash2 />}
            {filter.trash ? "恢复" : "移入回收站"}
          </Button>
        </div>
      )}
      {page && !filter.trash && !filter.folder && page.folders.length > 0 && (
        <div className="mb-6 flex flex-wrap gap-3">
          {page.folders.map((f) => (
            <Button
              variant="outline"
              key={f.id}
              disabled={saving || !!pendingOperation}
              onClick={() => onFolder(f.id!)}
            >
              <Folder />
              {f.name}
            </Button>
          ))}
        </div>
      )}
      {loading ? (
        <div className="asset-grid" role="status" aria-label="正在读取资产">
          {[0, 1, 2, 3].map((n) => (
            <Skeleton key={n} className="aspect-video rounded-xl" />
          ))}
        </div>
      ) : error ? (
        <Empty className="min-h-80">
          <FeedbackToast message={error} />
          <EmptyHeader>
            <EmptyTitle>无法读取资产</EmptyTitle>
            <EmptyDescription>请重试读取。</EmptyDescription>
          </EmptyHeader>
          <Button variant="outline" onClick={reload}>
            重试读取资产
          </Button>
        </Empty>
      ) : page?.items.length === 0 ? (
        <Empty className="min-h-80">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Folder />
            </EmptyMedia>
            <EmptyTitle>
              {filter.search
                ? "没有匹配的资产"
                : filter.trash
                  ? "回收站为空"
                  : "这里还没有资产"}
            </EmptyTitle>
            <EmptyDescription>
              {filter.search
                ? "试试其他关键词，或调整筛选条件。"
                : filter.trash
                  ? "移入回收站的资产会显示在这里。"
                  : "上传素材，开始积累你的创作灵感。"}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="asset-grid">
          {page?.items.map((item) => (
            <Card key={item.id} variant="media" className="asset-card">
              <CardContent>
                <Button
                  variant="ghost"
                  className="asset-cover h-auto w-full p-0"
                  aria-label={`查看 ${item.title}`}
                  onClick={() => setDetail(item)}
                >
                  <AssetPreview item={item} />
                </Button>
              </CardContent>
              {batch && (
                <Checkbox
                  aria-label={`选择 ${item.title}`}
                  className="asset-selection"
                  checked={selected.includes(item.id)}
                  disabled={saving}
                  onCheckedChange={(v) =>
                    setSelected((ids) =>
                      v === true
                        ? [...ids, item.id]
                        : ids.filter((id) => id !== item.id),
                    )
                  }
                />
              )}
              <CardHeader>
                <div className="flex items-center gap-2">
                  <CardTitle className="min-w-0 flex-1 truncate">
                    {item.title}
                  </CardTitle>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`${item.favorite ? "取消收藏" : "收藏"} ${item.title}`}
                    aria-pressed={item.favorite}
                    disabled={saving || !!pendingOperation || filter.trash}
                    onClick={() => favorite(item)}
                  >
                    <Heart className={cn(item.favorite && "fill-current")} />
                  </Button>
                </div>
                <CardDescription>
                  {kinds.find((k) => k.id === item.kind)?.label ?? "素材"}
                  {item.media?.width
                    ? ` · ${item.media.width} × ${item.media.height}`
                    : ""}{" "}
                  · {item.source_label || "个人资产"}
                </CardDescription>
              </CardHeader>
            </Card>
          ))}
        </div>
      )}
      {page && page.total > page.page_size && (
        <div className="mt-8 flex justify-center gap-5">
          <Button
            variant="outline"
            disabled={pageIndex === 1 || loading}
            onClick={() => {
              setPageIndex((n) => n - 1);
              setSelected([]);
              setLoading(true);
            }}
          >
            上一页
          </Button>
          <span className="self-center text-sm">
            {pageIndex} / {Math.ceil(page.total / page.page_size)}
          </span>
          <Button
            variant="outline"
            disabled={pageIndex * page.page_size >= page.total || loading}
            onClick={() => {
              setPageIndex((n) => n + 1);
              setSelected([]);
              setLoading(true);
            }}
          >
            下一页
          </Button>
        </div>
      )}
      <Dialog
        open={!!detail}
        onOpenChange={(v) => {
          if (!v) setDetail(null);
        }}
      >
        <DialogContent className="sm:max-w-4xl">
          <DialogTitle>{detail?.title}</DialogTitle>
          <DialogDescription>
            个人资产 · {detail?.media?.file_name ?? detail?.kind}
          </DialogDescription>
          {detail && (
            <>
              <div className="relative flex aspect-video items-center justify-center overflow-hidden rounded-xl bg-background">
                <AssetPreview key={detail.id} item={detail} large />
              </div>
              <div className="flex items-center gap-3">
                <span className="mr-auto text-xs text-muted-foreground">
                  {detail.media
                    ? `${(detail.media.byte_size ?? 0) / 1024 / 1024 < 0.01 ? "< 0.01" : ((detail.media.byte_size ?? 0) / 1024 / 1024).toFixed(2)} MB`
                    : ""}
                </span>
                <Button
                  variant="outline"
                  disabled={saving || !!pendingOperation || filter.trash}
                  onClick={() => favorite(detail)}
                >
                  <Heart />
                  {detail.favorite ? "取消收藏" : "收藏"}
                </Button>
                {!filter.trash && (
                  <DownloadButton
                    filename={detail.media?.file_name || detail.title}
                    load={() =>
                      downloadLibraryMediaAsset({
                        item_id: detail.id,
                        scope: "personal",
                      })
                    }
                  >
                    <Download />
                    下载原件
                  </DownloadButton>
                )}
              </div>
            </>
          )}
        </DialogContent>
      </Dialog>
      <Dialog
        open={folderOpen}
        onOpenChange={(v) => {
          if (!saving && !pendingOperation) setFolderOpen(v);
        }}
      >
        <DialogContent>
          <DialogTitle>新建文件夹</DialogTitle>
          <DialogDescription>把相关素材整理到一起。</DialogDescription>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              if (page && folderName.trim())
                void command({
                  scope: { kind: "personal" },
                  expected_revision: page.revision,
                  action: "create_folder",
                  folder: {
                    name: folderName.trim(),
                    parent_id: undefined,
                    style: "",
                    theme: "",
                  },
                });
            }}
          >
            <Input
              aria-label="文件夹名称"
              value={folderName}
              maxLength={40}
              disabled={saving || !!pendingOperation}
              onChange={(e) => setFolderName(e.target.value)}
            />
            <DialogFooter className="mt-5">
              <Button
                type="submit"
                disabled={saving || !!pendingOperation || !folderName.trim()}
              >
                创建文件夹
              </Button>
            </DialogFooter>
          </form>
          {writeError && <></>}
          {pendingOperation && !saving && (
            <Button onClick={() => void command()}>重试原操作</Button>
          )}
        </DialogContent>
      </Dialog>
      <Dialog
        open={!!confirm}
        onOpenChange={(v) => {
          if (!v && !saving && !pendingOperation) setConfirm(null);
        }}
      >
        <DialogContent>
          <DialogTitle>
            {confirm === "restore_items" ? "恢复所选资产？" : "移入回收站？"}
          </DialogTitle>
          <DialogDescription>
            将处理已选择的 {selection.length} 项资产。
            {confirm === "restore_items"
              ? "恢复后可在资产列表中继续使用。"
              : "移入回收站后可以恢复。"}
          </DialogDescription>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={saving || !!pendingOperation}
              onClick={() => setConfirm(null)}
            >
              取消
            </Button>
            <Button
              disabled={saving}
              onClick={() => {
                if (page && confirm)
                  void command({
                    scope: { kind: "personal" },
                    expected_revision: page.revision,
                    action: confirm,
                    items: selection.map((i) => ({
                      id: i.id,
                      revision: i.revision,
                    })),
                  });
              }}
            >
              {saving ? "正在处理…" : pendingOperation ? "重试原操作" : "确认"}
            </Button>
          </DialogFooter>
          {writeError && <></>}
        </DialogContent>
      </Dialog>
    </>
  );
}
export function AssetsPage() {
  const [filter, setFilter] = useState<Filter>({
    kind: "image",
    search: "",
    favorite: false,
    trash: false,
    folder: "",
    order: "updated_desc",
  });
  const [search, setSearch] = useState("");
  const [upload, setUpload] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const [locked, setLocked] = useState(false);
  return (
    <BasicLayout
      screen="assets"
      search={{
        value: search,
        onValueChange: setSearch,
        onSearch: (value) => setFilter((f) => ({ ...f, search: value })),
        label: "搜索资产",
        disabled: locked,
      }}
    >
      <nav className="workspace-tabs asset-tabs">
        <Link href="/assets" aria-current="page">
          生成历史
        </Link>
        <Link href="/projects">画布</Link>
      </nav>
      <FieldSet className="workspace-toolbar asset-filters" disabled={locked}>
        <Tabs
          value={filter.kind}
          onValueChange={(kind) => setFilter((f) => ({ ...f, kind }))}
        >
          <TabsList variant="navigation">
            {kinds.map((k) => (
              <TabsTrigger key={k.id} value={k.id}>
                {k.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <Button
          variant="ghost"
          aria-pressed={filter.favorite}
          onClick={() => setFilter((f) => ({ ...f, favorite: !f.favorite }))}
        >
          <Heart />
          收藏
        </Button>
        <Select
          value={filter.order}
          onValueChange={(order) => setFilter((f) => ({ ...f, order }))}
        >
          <SelectTrigger aria-label="资产排序" className="w-auto">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="updated_desc">最近修改</SelectItem>
              <SelectItem value="updated_asc">最早修改</SelectItem>
              <SelectItem value="name_asc">名称顺序</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        <Button
          className="ml-auto"
          variant="outline"
          onClick={() => setUpload(true)}
        >
          <Upload />
          上传素材
        </Button>

        <Button
          variant="ghost"
          className="ml-auto"
          aria-pressed={filter.trash}
          onClick={() =>
            setFilter((f) => ({
              ...f,
              trash: !f.trash,
              favorite: false,
              folder: "",
            }))
          }
        >
          <Trash2 />
          {filter.trash ? "返回资产" : "回收站"}
        </Button>
      </FieldSet>
      <Library
        key={JSON.stringify(filter)}
        filter={filter}
        onLock={setLocked}
        onFolder={(folder) => setFilter((f) => ({ ...f, folder }))}
        refresh={refresh}
        onRefresh={() => setRefresh((n) => n + 1)}
      />
      {upload && (
        <UploadDialog
          onClose={() => setUpload(false)}
          onUploaded={() => setRefresh((n) => n + 1)}
        />
      )}
      <GenerationComposer />
    </BasicLayout>
  );
}
