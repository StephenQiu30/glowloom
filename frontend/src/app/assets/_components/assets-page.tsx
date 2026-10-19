"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import {
  Field,
  FieldLabel,
  FieldDescription,
  FieldGroup,
} from "@/components/ui/field";
import Link from "next/link";
import { cn } from "cn";
import { useEffect, useRef, useState } from "react";
import {
  Trash2,
  Heart,
  Folder,
  RotateCw,
  Download,
  ArrowLeft,
  History,
} from "lucide-react";
import { PageLayout } from "@/components/layout/page-layout";
import { useLoginDialog } from "@/components/layout/basic-layout";
import { useOptionalSession } from "@/components/auth/session";
import { RequestError } from "@/lib/request";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from "@/components/ui/item";
import {
  DataPagination,
  type PageSize,
} from "@/components/media/data-pagination";
import { Input } from "@/components/ui/input";
import {
  Card,
  CardContent,
  CardHeader,
  CardAction,
  CardTitle,
  CardDescription,
} from "@/components/ui/card";
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  EmptyDescription,
  EmptyContent,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { Checkbox } from "@/components/ui/checkbox";
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
import {
  LibraryToolbar,
  mediaKinds as kinds,
  type LibraryFilter as Filter,
} from "./library-toolbar";
function Library({
  filter,
  onFilter,
  onLock,
  pageSize,
  onPageSizeChange,
}: {
  filter: Filter;
  onFilter: (filter: Filter) => void;
  onLock: (locked: boolean) => void;
  pageSize: PageSize;
  onPageSizeChange: (size: PageSize) => void;
}) {
  const session = useOptionalSession();
  const login = useLoginDialog();
  const [section, setSection] = useState("library");
  const [upload, setUpload] = useState(false);
  const [authRequired, setAuthRequired] = useState(false);
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
  const actorID = session?.actor.id;
  useEffect(() => () => onLock(false), [onLock]);
  useEffect(() => {
    if (!actorID || section !== "library") return;
    const controller = new AbortController();
    let adjustingPage = false;
    listMediaLibrary(
      {
        scope: "personal",
        kind: filter.kind,
        search: filter.search,
        favorite_only: filter.favorite,
        recent_only: filter.recent,
        catalog_state: filter.trash ? "trashed" : "active",
        folder_id: filter.folder || undefined,
        order: filter.order,
        page: pageIndex,
        page_size: pageSize,
      },
      { signal: controller.signal },
    )
      .then((v) => requireLibraryPage(v, actorID))
      .then((result) => {
        if (!controller.signal.aborted) {
          const lastPage = Math.max(1, Math.ceil(result.total / pageSize));
          if (pageIndex > lastPage) {
            adjustingPage = true;
            setPageIndex(lastPage);
            setSelected([]);
            return;
          }
          setPage(result);
          setError("");
        }
      })
      .catch((e) => {
        if (!controller.signal.aborted) {
          setPage(null);
          if (e instanceof RequestError && e.status === 401)
            setAuthRequired(true);
          else setError(mediaError(e));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted && !adjustingPage) setLoading(false);
      });
    return () => controller.abort();
  }, [filter, actorID, section, pageIndex, pageSize, retry]);
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
  const authenticated = !!session && !authRequired;
  const locked = saving || !!pendingOperation;
  const activeFolder = page?.folders.find((f) => f.id === filter.folder);
  return (
    <Tabs
      value={section}
      onValueChange={(value) => {
        setSection(value);
        setBatch(false);
        setSelected([]);
        if (value === "library" && authenticated) setLoading(true);
      }}
      asChild
    >
      <section className="media-library" aria-label="素材库">
        <h1 className="sr-only">素材库</h1>
        <LibraryToolbar
          filter={filter}
          onFilter={onFilter}
          section={section}
          disabled={!authenticated}
          locked={locked}
          batch={batch}
          canCreateFolder={!!page && !loading}
          onBatch={() => {
            setBatch(!batch);
            setSelected([]);
          }}
          onFolder={() => setFolderOpen(true)}
          onUpload={() => setUpload(true)}
        />
        <TabsContent
          value={section}
          className="flex min-h-0 flex-1 flex-col gap-6"
        >
          {!authenticated ? (
            <Empty className="library-login-state">
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <Folder aria-hidden="true" />
                </EmptyMedia>
                <EmptyTitle>登录查看你的素材</EmptyTitle>
              </EmptyHeader>
              <EmptyDescription>
                登录后，上传、收藏和整理你的创作素材。
              </EmptyDescription>
              <EmptyContent>
                <Button size="lg" className="min-h-11 px-6" onClick={login}>
                  登录 / 注册
                </Button>
              </EmptyContent>
            </Empty>
          ) : section === "history" ? (
            <Empty className="library-login-state">
              <EmptyHeader>
                <EmptyMedia>
                  <History aria-hidden="true" />
                </EmptyMedia>
                <EmptyTitle>在画布中查看生成记录</EmptyTitle>
                <EmptyDescription>
                  生成记录随所属画布保存，打开画布即可继续查看和创作。
                </EmptyDescription>
              </EmptyHeader>
              <EmptyContent>
                <Button asChild variant="outline" size="lg">
                  <Link href="/projects">查看我的画布</Link>
                </Button>
              </EmptyContent>
            </Empty>
          ) : (
            <>
              {writeError && (
                <FeedbackToast
                  message={writeError}
                  action={
                    saving
                      ? undefined
                      : {
                          label: pendingOperation
                            ? "重试原操作"
                            : "刷新素材版本",
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
                <Item variant="outline" size="sm">
                  <ItemContent>
                    <FieldGroup>
                      <Field orientation="horizontal">
                        <Checkbox
                          id="library-select-page"
                          aria-label="选择本页全部素材"
                          checked={
                            selection.length > 0 &&
                            selection.length < (page?.items.length ?? 0)
                              ? "indeterminate"
                              : !!page?.items.length &&
                                selection.length === page.items.length
                          }
                          disabled={locked || loading}
                          onCheckedChange={(v) =>
                            setSelected(
                              v === true
                                ? (page?.items.map((i) => i.id) ?? [])
                                : [],
                            )
                          }
                        />
                        <FieldLabel htmlFor="library-select-page">
                          已选择 {selection.length} 项
                        </FieldLabel>
                      </Field>
                    </FieldGroup>
                  </ItemContent>
                  <ItemActions>
                    <Button
                      className="max-md:min-h-11"
                      disabled={
                        !selection.length || saving || !!pendingOperation
                      }
                      variant={filter.trash ? "outline" : "destructive"}
                      onClick={() =>
                        setConfirm(
                          filter.trash ? "restore_items" : "recycle_items",
                        )
                      }
                    >
                      {filter.trash ? <RotateCw /> : <Trash2 />}
                      {filter.trash ? "恢复" : "移入回收站"}
                    </Button>
                  </ItemActions>
                </Item>
              )}
              {page &&
                !loading &&
                !error &&
                !filter.trash &&
                !filter.folder &&
                page.folders.length > 0 && (
                  <section className="library-folders" aria-label="我的文件夹">
                    <Item className="library-section-heading p-0">
                      <ItemContent>
                        <ItemTitle role="heading" aria-level={2}>
                          我的文件夹
                        </ItemTitle>
                      </ItemContent>
                      <ItemActions>
                        <ItemDescription>
                          {page.folders.length} 个文件夹
                        </ItemDescription>
                      </ItemActions>
                    </Item>
                    <ItemGroup
                      className="library-folder-grid"
                      role="group"
                      aria-label="文件夹"
                    >
                      {page.folders.map((f) => (
                        <Item asChild variant="muted" key={f.id}>
                          <Button
                            variant="ghost"
                            className="library-folder-card"
                            disabled={saving || !!pendingOperation}
                            onClick={() =>
                              onFilter({ ...filter, folder: f.id! })
                            }
                          >
                            <ItemMedia variant="icon">
                              <Folder data-icon="inline-start" />
                            </ItemMedia>
                            <ItemContent className="min-w-0 items-start">
                              <ItemTitle className="max-w-full truncate">
                                {f.name}
                              </ItemTitle>
                              {typeof page.folder_counts?.[f.id!] ===
                                "number" && (
                                <ItemDescription>
                                  {page.folder_counts[f.id!]} 个素材
                                </ItemDescription>
                              )}
                            </ItemContent>
                          </Button>
                        </Item>
                      ))}
                    </ItemGroup>
                  </section>
                )}
              {page && !loading && !error && (
                <Item className="library-section-heading flex-nowrap p-0">
                  {filter.folder && (
                    <Button
                      variant="ghost"
                      size="icon-lg"
                      aria-label="返回全部素材"
                      disabled={locked}
                      onClick={() => onFilter({ ...filter, folder: "" })}
                    >
                      <ArrowLeft />
                    </Button>
                  )}
                  <ItemContent className="min-w-0">
                    <ItemTitle
                      role="heading"
                      aria-level={2}
                      className="max-w-full truncate"
                    >
                      {filter.trash
                        ? "回收站"
                        : activeFolder?.name ||
                          (filter.favorite
                            ? "收藏"
                            : `全部${kinds.find((k) => k.id === filter.kind)?.label ?? "素材"}`)}
                    </ItemTitle>
                  </ItemContent>
                  <ItemActions>
                    <ItemDescription>共 {page.total} 个素材</ItemDescription>
                    <Button
                      variant="ghost"
                      size="icon-lg"
                      aria-label="刷新素材"
                      disabled={locked}
                      onClick={reload}
                    >
                      <RotateCw />
                    </Button>
                  </ItemActions>
                </Item>
              )}
              {loading ? (
                <ItemGroup
                  className="asset-grid"
                  role="status"
                  aria-label="正在读取素材"
                >
                  {[0, 1, 2, 3].map((n) => (
                    <Skeleton key={n} className="aspect-video rounded-xl" />
                  ))}
                </ItemGroup>
              ) : error ? (
                <Empty className="library-login-state">
                  <FeedbackToast message={error} />
                  <EmptyHeader>
                    <EmptyTitle>无法读取素材</EmptyTitle>
                    <EmptyDescription>请重试读取。</EmptyDescription>
                  </EmptyHeader>
                  <Button variant="outline" onClick={reload}>
                    重试读取素材
                  </Button>
                </Empty>
              ) : page?.items.length === 0 ? (
                <Empty className="library-login-state">
                  <EmptyHeader>
                    <EmptyMedia variant="icon">
                      <Folder />
                    </EmptyMedia>
                    <EmptyTitle>
                      {filter.search
                        ? "没有匹配的素材"
                        : filter.trash
                          ? "回收站为空"
                          : "这里还没有素材"}
                    </EmptyTitle>
                    <EmptyDescription>
                      {filter.search
                        ? "试试其他关键词，或调整筛选条件。"
                        : filter.trash
                          ? "移入回收站的素材会显示在这里。"
                          : "上传素材，开始积累你的创作灵感。"}
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              ) : (
                <ItemGroup className="asset-grid">
                  {page?.items.map((item) => (
                    <Card
                      key={item.id}
                      variant="media"
                      className="asset-card"
                      role="listitem"
                    >
                      <CardContent className="p-0">
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
                          disabled={locked || loading}
                          onCheckedChange={(v) =>
                            setSelected((ids) =>
                              v === true
                                ? [...ids, item.id]
                                : ids.filter((id) => id !== item.id),
                            )
                          }
                        />
                      )}
                      <CardHeader className="px-0">
                        <CardTitle
                          className="min-w-0 self-center truncate"
                          title={item.title}
                        >
                          {item.title}
                        </CardTitle>
                        <CardAction className="row-span-1">
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            className="max-md:size-11"
                            aria-label={`${item.favorite ? "取消收藏" : "收藏"} ${item.title}`}
                            aria-pressed={item.favorite}
                            disabled={
                              saving || !!pendingOperation || filter.trash
                            }
                            onClick={() => favorite(item)}
                          >
                            <Heart
                              className={cn(item.favorite && "fill-current")}
                            />
                          </Button>
                        </CardAction>
                        <CardDescription className="col-span-2">
                          {kinds.find((k) => k.id === item.kind)?.label ??
                            "素材"}
                          {item.media?.width
                            ? ` · ${item.media.width} × ${item.media.height}`
                            : ""}{" "}
                          · {item.source_label || "个人素材"}
                        </CardDescription>
                      </CardHeader>
                    </Card>
                  ))}
                </ItemGroup>
              )}
              {page && !error && (
                <DataPagination
                  label="素材分页"
                  page={pageIndex}
                  pageSize={pageSize}
                  total={page.total}
                  disabled={loading || locked}
                  onPageChange={(next) => {
                    setPageIndex(next);
                    setSelected([]);
                    setDetail(null);
                    setLoading(true);
                  }}
                  onPageSizeChange={onPageSizeChange}
                />
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
                    个人素材 · {detail?.media?.file_name ?? detail?.kind}
                  </DialogDescription>
                  {detail && (
                    <>
                      <div className="relative flex aspect-video items-center justify-center overflow-hidden rounded-xl bg-background">
                        <AssetPreview key={detail.id} item={detail} large />
                      </div>
                      <Item className="p-0">
                        <ItemContent>
                          <ItemDescription>
                            {detail.media
                              ? `${(detail.media.byte_size ?? 0) / 1024 / 1024 < 0.01 ? "< 0.01" : ((detail.media.byte_size ?? 0) / 1024 / 1024).toFixed(2)} MB`
                              : ""}
                          </ItemDescription>
                        </ItemContent>
                        <ItemActions>
                          <Button
                            variant="outline"
                            disabled={
                              saving || !!pendingOperation || filter.trash
                            }
                            onClick={() => favorite(detail)}
                          >
                            <Heart data-icon="inline-start" />
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
                              <Download data-icon="inline-start" />
                              下载原件
                            </DownloadButton>
                          )}
                        </ItemActions>
                      </Item>
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
                    <FieldGroup>
                      <Field>
                        <FieldLabel htmlFor="library-folder-name">
                          文件夹名称
                        </FieldLabel>
                        <Input
                          id="library-folder-name"
                          value={folderName}
                          maxLength={40}
                          disabled={saving || !!pendingOperation}
                          onChange={(e) => setFolderName(e.target.value)}
                        />
                        <FieldDescription>最多 40 个字符</FieldDescription>
                      </Field>
                    </FieldGroup>
                    <DialogFooter className="mt-5">
                      <Button
                        type="submit"
                        disabled={
                          saving || !!pendingOperation || !folderName.trim()
                        }
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
                    {confirm === "restore_items"
                      ? "恢复所选素材？"
                      : "移入回收站？"}
                  </DialogTitle>
                  <DialogDescription>
                    将处理已选择的 {selection.length} 项素材。
                    {confirm === "restore_items"
                      ? "恢复后可在素材列表中继续使用。"
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
                      {saving
                        ? "正在处理…"
                        : pendingOperation
                          ? "重试原操作"
                          : "确认"}
                    </Button>
                  </DialogFooter>
                  {writeError && <></>}
                </DialogContent>
              </Dialog>
              {upload && (
                <UploadDialog
                  onClose={() => setUpload(false)}
                  onUploaded={reload}
                />
              )}
            </>
          )}
        </TabsContent>
      </section>
    </Tabs>
  );
}
export function AssetsPage() {
  const [pageSize, setPageSize] = useState<PageSize>(12);
  const [filter, setFilter] = useState<Filter>({
    kind: "image",
    search: "",
    favorite: false,
    trash: false,
    folder: "",
    order: "updated_desc",
    recent: false,
  });
  const [search, setSearch] = useState("");
  const [locked, setLocked] = useState(false);
  return (
    <PageLayout
      screen="assets"

      fullBleed
      search={{
        value: search,
        onValueChange: setSearch,
        onSearch: (value) => setFilter((f) => ({ ...f, search: value })),
        label: "搜索素材",
        disabled: locked,
      }}
    >
      <Library
        key={JSON.stringify([filter, pageSize])}
        filter={filter}
        onFilter={setFilter}
        onLock={setLocked}
        pageSize={pageSize}
        onPageSizeChange={setPageSize}
      />
    </PageLayout>
  );
}
