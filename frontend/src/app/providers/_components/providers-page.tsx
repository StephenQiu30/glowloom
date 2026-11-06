"use client";
import { readErrorMessage } from "@/lib/read-error";
import { ContentState } from "@/components/layout/content-state";

import { useEffect, useId, useRef, useState } from "react";
import { AdminHeading } from "@/components/governance/admin-layout";
import { useBatchOperation } from "@/components/governance/use-batch-operation";
import { BatchDialog } from "@/components/governance/batch-dialog";
import {
  connectionDeleteAction,
  type ConnectionRow,
} from "@/components/governance/batch-actions";
import { ListPagination } from "@/components/canvas/runtime/components/layout/workspace/list-pagination";
import { Plus } from "lucide-react";
import { toast } from "sonner";
import { useSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@/components/ui/alert-dialog";
import { Skeleton } from "@/components/ui/skeleton";
import {
  listAdminProviders,
  listProviderPresets,
  listAdminProviderAdapters,
  updateProviderPresetState,
} from "@/api/settings";
import {
  MutationStatus,
  useCatalogMutation,
} from "@/components/catalog/mutation";
import { PresetEditor } from "./preset-editor";
import { ConnectionDetail } from "./connection-detail";
import { ConnectionReleaseReview } from "./connection-release-review";
import { PresetConnect } from "./preset-connect";
import { ConnectionList, regionLabel } from "./connection-list";
import { ColumnMenu } from "@/components/governance/column-menu";
import { useIsMobile } from "@/hooks/use-mobile";
import {
  ProviderPresetList,
  type PresetStateAction,
} from "./provider-preset-list";
import { connectionPresets, providerLabel } from "./provider-label";

type Confirmation = { preset: API.Preset; action: PresetStateAction };

type ProvidersPageProps = {
  initialConnection?: string;
  search?: string;
  initialModel?: string;
  initialRelease?: string;
  initialMode?: string;
  onPresetViewChange?: (showingPresets: boolean) => void;
};

export function ProvidersPage(props: ProvidersPageProps) {
  const session = useSession();
  if (session.actor.platform_role !== "platform_admin") {
    return (
      <ContentState
        state="error"
        title="暂无访问权限"
        description="此页面仅向平台管理员开放。"
      />
    );
  }
  return <ProviderAdministration key={session.actor.id} {...props} />;
}

function ProviderAdministration({
  initialConnection = "",
  search = "",
  initialModel,
  initialRelease,
  initialMode,
  onPresetViewChange,
}: ProvidersPageProps) {
  const mobile = useIsMobile();
  const [visibility, setVisibility] = useState<Record<string, boolean>>({
    check: false,
    credential: false,
  });
  const overlayTrigger = useRef<HTMLElement | null>(null);
  function rememberTrigger() {
    const active = document.activeElement;
    overlayTrigger.current =
      active instanceof HTMLElement && active.closest('[role="menu"]')
        ? document.querySelector<HTMLElement>(
            '[data-slot="dropdown-menu-trigger"][data-state="open"]',
          )
        : active instanceof HTMLElement
          ? active
          : null;
  }
  function restoreFocus() {
    const original = overlayTrigger.current;
    const key = original?.dataset.connectionTrigger;
    const target = original?.isConnected
      ? original
      : key
        ? Array.from(
            document.querySelectorAll<HTMLElement>("[data-connection-trigger]"),
          ).find((node) => node.dataset.connectionTrigger === key)
        : null;
    target?.focus();
  }
  const [adapters, setAdapters] = useState<API.AdapterMetadata[]>([]);
  const [presetsOpen, setPresetsOpen] = useState(false);
  const [providers, setProviders] = useState<API.ProviderListItem[]>([]);
  const [selection, setSelection] = useState<{
    scope: string;
    rows: ConnectionRow[];
  }>({ scope: search, rows: [] });
  // Invalidate the owned snapshot on every query transition, including a return
  // to a previous query. Keep any running batch's frozen entries independent.
  if (selection.scope !== search) setSelection({ scope: search, rows: [] });
  const [cursors, setCursors] = useState<string[]>([""]);
  const [pageSize, setPageSize] = useState(20);
  const [nextCursor, setNextCursor] = useState("");
  const [refresh, setRefresh] = useState(0);
  const { registerResultTrigger, registerFallbackTrigger, ...batch } =
    useBatchOperation<ConnectionRow>((ids) => {
      setSelection((s) => ({
        ...s,
        rows: s.rows.filter((r) => !ids.includes(r.id)),
      }));
      setRefresh((v) => v + 1);
    });
  const [presets, setPresets] = useState<API.Preset[]>([]);
  const [selected, setSelected] = useState(initialConnection);
  const [context, setContext] = useState({
    model: initialModel,
    release: initialRelease,
    mode: initialMode,
  });
  const [detailOpen, setDetailOpen] = useState(Boolean(initialConnection));
  const detailContent = useRef<HTMLDivElement>(null);
  const [reviewOpen, setReviewOpen] = useState(false);
  const reviewTitleId = useId();
  const reviewTrigger = useRef<HTMLButtonElement>(null);
  const [connect, setConnect] = useState(false);
  const connectVisible = useRef(false);
  const [connectionError, setConnectionError] = useState("");
  const [registryError, setRegistryError] = useState("");
  useEffect(
    () => () => {
      connectVisible.current = false;
    },
    [],
  );
  const [connectionsLoading, setConnectionsLoading] = useState(true);
  const [registryLoading, setRegistryLoading] = useState(true);
  const loading = registryLoading || (!presetsOpen && connectionsLoading);
  const error = registryError || (!presetsOpen ? connectionError : "");
  const [editor, setEditor] = useState<API.Preset | "new" | null>(null);
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  const mutation = useCatalogMutation(() => {
    setConfirmation(null);
    setRefresh((v) => v + 1);
  }, "registry");
  // Observe unresolved onboarding identities; only the original form may recover
  // creation and proceed to the credential phase.
  const pendingCreate = useCatalogMutation(
    () => {},
    "preset.onboarding.create",
  );
  const pendingKey = useCatalogMutation(
    () => {},
    "preset.onboarding.credential",
  );
  useEffect(() => {
    if (presetsOpen) return;
    const controller = new AbortController();
    listAdminProviders(
      { limit: pageSize, cursor: cursors.at(-1) || undefined },
      { signal: controller.signal },
    )
      .then((connections) => {
        if (controller.signal.aborted) return;
        const items = connections.items ?? [];
        if (!items.length && cursors.length > 1) {
          setCursors(cursors.slice(0, -1));
          return;
        }
        setProviders(items);
        setNextCursor(connections.next_cursor ?? "");
        setConnectionError("");
        setConnectionsLoading(false);
      })
      .catch((error) => {
        if (controller.signal.aborted) return;
        setConnectionError(readErrorMessage(error));
        setConnectionsLoading(false);
      });
    return () => controller.abort();
  }, [refresh, cursors, pageSize, presetsOpen]);
  useEffect(() => {
    const controller = new AbortController();
    Promise.all([
      listProviderPresets({ signal: controller.signal }),
      listAdminProviderAdapters({ signal: controller.signal }),
    ])
      .then(([registry, metadata]) => {
        if (controller.signal.aborted) return;
        setPresets(registry.items ?? []);
        setAdapters(metadata);
        setRegistryError("");
        setRegistryLoading(false);
      })
      .catch((error) => {
        if (controller.signal.aborted) return;
        setRegistryError(readErrorMessage(error));
        setRegistryLoading(false);
      });
    return () => controller.abort();
  }, [refresh]);
  function showPresetView(open: boolean) {
    setPresetsOpen(open);
    setSelection({ scope: search, rows: [] });
    onPresetViewChange?.(open);
  }
  function openConnect() {
    rememberTrigger();
    connectVisible.current = true;
    setConnect(true);
  }
  function openEditor(preset: API.Preset | "new") {
    rememberTrigger();
    setEditor(preset);
  }
  function openConfirmation(value: Confirmation) {
    rememberTrigger();
    setConfirmation(value);
  }
  function openConnection(id: string, trigger: HTMLElement) {
    overlayTrigger.current = trigger;
    setContext({ model: undefined, release: undefined, mode: undefined });
    const url = new URL(window.location.href);
    for (const name of ["view", "model", "release", "mode"])
      url.searchParams.delete(name);
    url.searchParams.set("connection", id);
    window.history.replaceState(null, "", url);
    setSelected(id);
    setDetailOpen(true);
  }
  function openReview(trigger: HTMLButtonElement) {
    reviewTrigger.current = trigger;
    setReviewOpen(true);
  }
  useEffect(() => {
    if (reviewOpen) {
      document.getElementById(reviewTitleId)?.focus();
    } else if (reviewTrigger.current) {
      reviewTrigger.current.focus();
      reviewTrigger.current = null;
    }
  }, [reviewOpen, reviewTitleId]);
  function closeDetail(open: boolean) {
    if (!open && reviewOpen) {
      setReviewOpen(false);
      return;
    }
    setDetailOpen(open);
    if (!open) {
      const url = new URL(window.location.href);
      for (const name of ["connection", "view", "model", "release", "mode"])
        url.searchParams.delete(name);
      window.history.replaceState(null, "", url);
    }
  }
  function closeConnect() {
    connectVisible.current = false;
    setConnect(false);
    setRefresh((value) => value + 1);
  }
  function connected() {
    setRefresh((value) => value + 1);
    if (!connectVisible.current) {
      return;
    }
    connectVisible.current = false;
    setConnect(false);
    closeDetail(false);
    toast.success("服务已接入");
  }
  const query = search.trim().toLocaleLowerCase();
  const matches = (text: string) => text.toLocaleLowerCase().includes(query);
  const visibleProviders = providers
    .filter(
      (p) =>
        p.provider?.id &&
        matches(
          `${p.provider.name} ${p.provider.adapter_key} ${providerLabel(p.provider.adapter_key, adapters)} ${regionLabel(p.provider.region)}`,
        ),
    )
    .map((item) => ({ ...item, id: item.provider!.id! }));
  const visiblePresets = presets.filter((p) =>
    matches(
      `${p.document?.name} ${p.slug} ${p.document?.config?.adapter_key} ${regionLabel(p.document?.config?.region)}`,
    ),
  );
  function retry() {
    setConnectionError("");
    setRegistryError("");
    setRegistryLoading(true);
    if (!presetsOpen) setConnectionsLoading(true);
    setRefresh((v) => v + 1);
  }
  const selectedRows =
    selection.scope === search
      ? selection.rows.filter((r) =>
          visibleProviders.some((p) => p.id === r.id),
        )
      : [];
  const batchLocked = batch.busy || batch.unresolved;
  const connectionLocked = batchLocked || Boolean(pendingKey.pending);
  if (batch.denied)
    return (
      <ContentState
        state="error"
        title="暂无访问权限"
        description="管理员权限或会话已失效，请重新登录后继续。"
      />
    );
  return (
    <div className="flex w-full flex-col gap-5">
      <AdminHeading title={presetsOpen ? "供应商预设" : "AI 服务接入"}>
        {presetsOpen ? (
          <>
            <Button
              variant="ghost"
              size="toolbar"
              onClick={() => showPresetView(false)}
            >
              返回 AI 服务接入
            </Button>
            <Button
              size="toolbar"
              disabled={loading || Boolean(error)}
              onClick={() => openEditor("new")}
            >
              <Plus data-icon="inline-start" />
              新建预设
            </Button>
          </>
        ) : (
          <>
            {batch.submitted ? (
              <Button
                variant="ghost"
                size="toolbar"
                ref={registerResultTrigger}
                onClick={() => batch.setOpen(true)}
              >
                查看批量操作结果
              </Button>
            ) : null}
            {selectedRows.length && !loading && !error ? (
              <div
                className="flex flex-wrap items-center gap-2"
                role="group"
                aria-label="批量操作"
              >
                <span aria-live="polite" className="text-sm">
                  已选 {selectedRows.length} 项
                </span>
                <Button
                  variant="ghost"
                  size="toolbar"
                  disabled={batchLocked}
                  onClick={() => setSelection({ scope: search, rows: [] })}
                >
                  清除选择
                </Button>
                <Button
                  variant="outline"
                  size="toolbar"
                  disabled={connectionLocked}
                  onClick={() =>
                    void batch.prepare(selectedRows, connectionDeleteAction)
                  }
                >
                  删除连接
                </Button>
              </div>
            ) : (
              <>
                <Button
                  variant="ghost"
                  size="toolbar"
                  disabled={batchLocked}
                  onClick={() => showPresetView(true)}
                >
                  供应商预设
                </Button>
                {!loading &&
                !error &&
                !mobile &&
                visibleProviders.length > 0 ? (
                  <ColumnMenu
                    columns={[
                      { id: "adapter", label: "供应商" },
                      { id: "models", label: "接入模型" },
                      { id: "credential", label: "配置状态" },
                      { id: "check", label: "最近检查" },
                    ]}
                    visibility={visibility}
                    onChange={setVisibility}
                  />
                ) : null}
                <Button
                  ref={registerFallbackTrigger}
                  size="toolbar"
                  onClick={openConnect}
                  disabled={loading || Boolean(error) || batchLocked}
                >
                  <Plus data-icon="inline-start" />
                  接入服务
                </Button>
              </>
            )}
          </>
        )}
      </AdminHeading>
      {mutation.message || mutation.pending ? (
        <MutationStatus mutation={mutation} />
      ) : null}
      {!presetsOpen && (pendingCreate.pending || pendingKey.pending) ? (
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant="secondary">保存待确认</Badge>
          <Button variant="outline" onClick={openConnect}>
            继续接入
          </Button>
        </div>
      ) : null}
      {loading ? (
        <div
          role="status"
          aria-label={presetsOpen ? "正在加载供应商预设" : "正在加载 AI 服务"}
          className="flex flex-col gap-4"
        >
          {[0, 1, 2].map((n) => (
            <Skeleton key={n} className="h-20 w-full" />
          ))}
        </div>
      ) : error ? (
        <ContentState
          state="error"
          title={
            presetsOpen ? "供应商预设暂时无法读取" : "服务连接暂时无法读取"
          }
          description={error}
          actions={<Button onClick={retry}>重试</Button>}
        />
      ) : presetsOpen ? (
        <ProviderPresetList
          items={visiblePresets}
          adapters={adapters}
          busy={mutation.busy}
          filtered={Boolean(query)}
          onEdit={openEditor}
          onState={(preset, action) => openConfirmation({ preset, action })}
        />
      ) : (
        <section aria-label="已添加的服务" className="flex flex-col gap-3">
          {visibleProviders.length ? (
            <ConnectionList
              adapters={adapters}
              items={visibleProviders}
              visibility={visibility}
              disabled={connectionLocked}
              selection={{
                selected: selectedRows,
                onChange: (updater) =>
                  setSelection({
                    scope: search,
                    rows:
                      typeof updater === "function"
                        ? updater(selectedRows)
                        : updater,
                  }),
              }}
              pendingCredential={
                pendingKey.pending?.action === "credential.set"
                  ? pendingKey.pending.target
                  : undefined
              }
              onSelect={openConnection}
              onDelete={(row, trigger) =>
                void batch.prepare(
                  [row],
                  connectionDeleteAction,
                  false,
                  trigger,
                )
              }
            />
          ) : (
            <ContentState title={query ? "没有匹配的连接" : "还没有连接"} />
          )}
          <ListPagination
            mode="cursor"
            label="服务连接分页"
            current={cursors.length}
            pageSize={pageSize}
            pageSizeOptions={[20, 50]}
            itemCount={visibleProviders.length}
            hasNextPage={!!nextCursor}
            disabled={batchLocked}
            onPageChange={(next) => {
              setSelection({ scope: search, rows: [] });
              setConnectionsLoading(true);
              setCursors((c) =>
                next < c.length ? c.slice(0, -1) : [...c, nextCursor],
              );
            }}
            onPageSizeChange={(size) => {
              setPageSize(size);
              setCursors([""]);
              setSelection({ scope: search, rows: [] });
              setConnectionsLoading(true);
            }}
          />
          {query ? (
            <p className="text-xs text-muted-foreground">
              筛选当前页的连接；翻页后重新选择。
            </p>
          ) : null}
        </section>
      )}
      <BatchDialog key={batch.generation} batch={batch} />
      <Dialog
        open={Boolean(connect)}
        onOpenChange={(open) => {
          if (!open) closeConnect();
        }}
      >
        <DialogContent
          aria-describedby={undefined}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            if (!detailOpen) restoreFocus();
          }}
          className="flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden border border-border p-4 sm:max-w-[640px] sm:p-6 max-md:[&_[data-slot=dialog-close]]:size-11"
        >
          <DialogHeader>
            <DialogTitle size="lg" className="text-xl">
              接入服务
            </DialogTitle>
          </DialogHeader>
          {connect ? (
            <PresetConnect
              adapters={adapters}
              presets={connectionPresets(
                presets,
                pendingCreate.pending?.target,
              )}
              providers={providers}
              onCreated={(id) => {
                setSelected(id);
                setRefresh((value) => value + 1);
              }}
              onSaved={connected}
              onCancel={closeConnect}
            />
          ) : null}
        </DialogContent>
      </Dialog>
      <Dialog open={detailOpen} onOpenChange={closeDetail}>
        <DialogContent
          ref={detailContent}
          {...(reviewOpen ? { "aria-labelledby": reviewTitleId } : {})}
          aria-describedby={
            reviewOpen ? `${reviewTitleId}-description` : undefined
          }
          showCloseButton={!reviewOpen}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            restoreFocus();
          }}
          onOpenAutoFocus={(event) => {
            const title = detailContent.current?.querySelector<HTMLElement>(
              "[data-connection-title]",
            );
            if (title) {
              event.preventDefault();
              title.focus();
            }
          }}
          className={
            reviewOpen
              ? "flex flex-col overflow-hidden border border-border p-4 sm:max-w-[640px] sm:p-6"
              : "flex flex-col overflow-hidden sm:max-w-lg max-md:[&_[data-slot=dialog-close]]:size-11"
          }
        >
          {selected ? (
            <div className={reviewOpen ? "hidden" : "contents"}>
              <ConnectionDetail
                key={selected}
                id={selected}
                Title={DialogTitle}
                onReview={openReview}
                initialModel={context.model}
                initialRelease={context.release}
                initialMode={context.mode}
                revision={refresh}
                presets={presets}
                onSaved={() => setRefresh((v) => v + 1)}
              />
            </div>
          ) : null}
          {selected && reviewOpen ? (
            <ConnectionReleaseReview
              id={selected}
              titleId={reviewTitleId}
              onClose={() => setReviewOpen(false)}
              onSaved={() => setRefresh((value) => value + 1)}
            />
          ) : null}
        </DialogContent>
      </Dialog>
      {editor ? (
        <PresetEditor
          adapters={adapters}
          returnFocusRef={overlayTrigger}
          key={editor === "new" ? "new" : editor.id}
          preset={editor === "new" ? undefined : editor}
          presets={presets}
          onClose={() => setEditor(null)}
          onSaved={() => {
            setEditor(null);
            setRefresh((v) => v + 1);
          }}
        />
      ) : null}
      <AlertDialog
        open={Boolean(confirmation)}
        onOpenChange={(open) => {
          if (!open && !mutation.busy) setConfirmation(null);
        }}
      >
        <AlertDialogContent
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            restoreFocus();
          }}
        >
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmation?.action === "archive"
                ? confirmation.preset.archived
                  ? "恢复预设？"
                  : "归档预设？"
                : confirmation?.preset.emergency_disabled
                  ? "解除紧急停用？"
                  : "紧急停用预设？"}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmation?.action === "archive"
                ? confirmation.preset.archived
                  ? "恢复将解除新建连接与升级采用该预设的归档限制。"
                  : "归档后，新建连接与升级无法采用该预设。已接入的连接继续使用原版本。"
                : confirmation?.preset.emergency_disabled
                  ? "解除后，已接入的连接可按当前配置接收新任务。"
                  : "将阻止采用该预设的已有连接接收新任务。已派发任务可继续查询和恢复。"}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={mutation.busy}>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={mutation.busy}
              onClick={(event) => {
                event.preventDefault();
                if (!confirmation) return;
                const p = confirmation.preset;
                void mutation.run("preset.state", p.id!, (request) =>
                  updateProviderPresetState(
                    { id: p.id! },
                    {
                      request_id: request,
                      expected_revision: p.revision,
                      archived:
                        confirmation.action === "archive"
                          ? !p.archived
                          : p.archived,
                      emergency_disabled:
                        confirmation.action === "emergency"
                          ? !p.emergency_disabled
                          : p.emergency_disabled,
                    },
                  ),
                );
              }}
            >
              {mutation.busy ? "正在保存…" : "确认"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
