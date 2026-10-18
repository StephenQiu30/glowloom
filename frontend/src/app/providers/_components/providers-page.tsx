"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Globe, MoreHorizontal, Plus, Server } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { toast } from "sonner";
import { useSession } from "@/components/auth/session";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { DownloadButton } from "@/components/media/download-button";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
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
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from "@/components/ui/card";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
  EmptyContent,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import {
  exportProviderPreset,
  listAdminProviders,
  listProviderPresets,
  updateProviderPresetState,
  updateAdminProvider,
} from "@/api/settings";
import { RequestError } from "@/lib/request";
import {
  catalogMessage,
  MutationStatus,
  useCatalogMutation,
} from "@/components/catalog/mutation";
import { PresetEditor } from "./preset-editor";
import { ConnectionDetail } from "./connection-detail";
import { PresetConnect } from "./preset-connect";
import { ConnectionList, regionLabel } from "./connection-list";

type Confirmation =
  | { kind: "provider"; provider: API.CreatedProvider }
  | { kind: "preset"; preset: API.Preset; action: "archive" | "emergency" };

export function ProvidersPage({
  initialConnection = "",
  search = "",
}: {
  initialConnection?: string;
  search?: string;
}) {
  const session = useSession();
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
    if (overlayTrigger.current?.isConnected) overlayTrigger.current.focus();
  }
  const [tab, setTab] = useState("connections");
  const [providers, setProviders] = useState<API.ProviderListItem[]>([]);
  const [presets, setPresets] = useState<API.Preset[]>([]);
  const [selected, setSelected] = useState(initialConnection);
  const [detailOpen, setDetailOpen] = useState(Boolean(initialConnection));
  const [connect, setConnect] = useState<API.Preset | true | null>(null);
  const connectVisible = useRef(false);
  const [refresh, setRefresh] = useState(0);
  const [error, setError] = useState("");
  const [feedback, setFeedback] = useState("");
  const readback = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      connectVisible.current = false;
      readback.current?.abort();
    },
    [],
  );
  const [loading, setLoading] = useState(true);
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
  const admin = session.actor.platform_role === "platform_admin";
  useEffect(() => {
    if (!admin) return;
    const controller = new AbortController();
    Promise.all([
      listAdminProviders({ limit: 200 }, { signal: controller.signal }),
      listProviderPresets({ signal: controller.signal }),
    ])
      .then(([connections, registry]) => {
        if (controller.signal.aborted) return;
        setProviders(connections.items ?? []);
        setPresets(registry.items ?? []);
        setError("");
        setLoading(false);
      })
      .catch((error) => {
        if (!controller.signal.aborted) {
          setError(catalogMessage(error));
          setLoading(false);
        }
      });
    return () => controller.abort();
  }, [refresh, admin]);
  function openConnect(preset: API.Preset | true) {
    rememberTrigger();
    connectVisible.current = true;
    setConnect(preset);
  }
  function openEditor(preset: API.Preset | "new") {
    rememberTrigger();
    setEditor(preset);
  }
  function openConfirmation(value: Confirmation) {
    rememberTrigger();
    setConfirmation(value);
  }
  function selectConnection(id: string) {
    setSelected(id);
    const url = new URL(window.location.href);
    url.searchParams.set("connection", id);
    window.history.replaceState(null, "", url);
  }
  function openConnection(id: string) {
    rememberTrigger();
    selectConnection(id);
    setDetailOpen(true);
  }
  function closeDetail(open: boolean) {
    setDetailOpen(open);
    if (!open) {
      const url = new URL(window.location.href);
      url.searchParams.delete("connection");
      window.history.replaceState(null, "", url);
    }
  }
  function closeConnect() {
    connectVisible.current = false;
    readback.current?.abort();
    setConnect(null);
  }
  async function connected(id: string) {
    if (!connectVisible.current) {
      setRefresh((value) => value + 1);
      return;
    }
    readback.current?.abort();
    const controller = new AbortController();
    readback.current = controller;
    try {
      const result = await listAdminProviders(
        { limit: 200 },
        { signal: controller.signal },
      );
      if (controller.signal.aborted) return;
      const saved = result.items?.find((item) => item.provider?.id === id);
      if (!saved || saved.credential?.status !== "active")
        throw new RequestError("request_unconfirmed");
      setProviders(result.items ?? []);
      selectConnection(id);
      connectVisible.current = false;
      setConnect(null);
      setDetailOpen(true);
      toast.success("连接已保存");
    } catch (error) {
      if (!controller.signal.aborted) setFeedback(catalogMessage(error));
    }
  }
  if (!admin)
    return (
      <Empty>
        <EmptyHeader>
          <EmptyTitle>暂无访问权限</EmptyTitle>
          <EmptyDescription>此页面仅向平台管理员开放。</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button asChild variant="outline">
            <Link href="/">返回创作</Link>
          </Button>
        </EmptyContent>
      </Empty>
    );
  const query = search.trim().toLocaleLowerCase();
  const matches = (text: string) => text.toLocaleLowerCase().includes(query);
  const visibleProviders = providers.filter(
    (p) =>
      p.provider?.id &&
      matches(
        `${p.provider.name} ${p.provider.adapter_key} ${regionLabel(p.provider.region)}`,
      ),
  );
  const visiblePresets = presets.filter((p) =>
    matches(
      `${p.document?.name} ${p.slug} ${p.document?.config?.adapter_key} ${regionLabel(p.document?.config?.region)}`,
    ),
  );
  function retry() {
    setError("");
    setLoading(true);
    setRefresh((v) => v + 1);
  }
  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 p-4 md:p-8">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="text-2xl font-medium">AI 服务接入</h1>
          <p className="text-sm text-muted-foreground">管理平台连接与预设。</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="lg" className="max-md:h-11" asChild>
            <Link href="/models">模型管理</Link>
          </Button>
          <Button
            size="lg"
            className="max-md:h-11"
            onClick={() =>
              tab === "connections" ? openConnect(true) : openEditor("new")
            }
            disabled={loading || !!error}
          >
            <Plus data-icon="inline-start" />
            {tab === "connections" ? "接入服务" : "新建预设"}
          </Button>
        </div>
      </header>
      {mutation.message || mutation.pending ? (
        <MutationStatus mutation={mutation} />
      ) : null}
      {error ? <FeedbackToast message={error} /> : null}
      {feedback ? <FeedbackToast message={feedback} /> : null}
      {pendingCreate.pending || pendingKey.pending ? (
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant="secondary">保存待确认</Badge>
          <Button variant="outline" onClick={() => openConnect(true)}>
            继续接入
          </Button>
        </div>
      ) : null}
      <Tabs value={tab} onValueChange={setTab} className="gap-6">
        <TabsList aria-label="平台管理视图" className="max-md:h-11">
          <TabsTrigger value="connections">连接</TabsTrigger>
          <TabsTrigger value="presets">预设管理</TabsTrigger>
        </TabsList>
        {loading ? (
          <div
            role="status"
            aria-label="正在加载 AI 服务"
            className="flex flex-col gap-4"
          >
            {[0, 1, 2].map((n) => (
              <Skeleton key={n} className="h-20 w-full" />
            ))}
          </div>
        ) : error ? (
          <Empty>
            <EmptyHeader>
              <EmptyTitle>暂时无法加载</EmptyTitle>
            </EmptyHeader>
            <EmptyContent>
              <Button variant="outline" onClick={retry}>
                重新加载
              </Button>
            </EmptyContent>
          </Empty>
        ) : (
          <>
            <TabsContent value="connections" className="flex flex-col gap-6">
              {visibleProviders.length ? (
                <ConnectionList
                  items={visibleProviders}
                  pendingCredential={
                    pendingKey.pending?.action === "credential.set"
                      ? pendingKey.pending.target
                      : undefined
                  }
                  onSelect={openConnection}
                  onDisable={(provider) =>
                    openConfirmation({ kind: "provider", provider })
                  }
                />
              ) : (
                <Empty>
                  <EmptyHeader>
                    <EmptyTitle>
                      {query ? "没有匹配的连接" : "还没有连接"}
                    </EmptyTitle>
                    <EmptyDescription>
                      {query
                        ? "试试其他关键词。"
                        : "选择平台预设，添加 API Key。"}
                    </EmptyDescription>
                  </EmptyHeader>
                  {!query ? (
                    <EmptyContent>
                      <Button
                        onClick={() => openConnect(true)}
                        className="max-md:h-11"
                      >
                        接入服务
                      </Button>
                    </EmptyContent>
                  ) : null}
                </Empty>
              )}
              <p className="text-xs text-muted-foreground">
                保存连接后，检查与模型发布分别进行。
              </p>
            </TabsContent>
            <TabsContent value="presets" className="flex flex-col gap-6">
              <div className="grid gap-6 md:grid-cols-2">
                {visiblePresets.map((p) => {
                  const Icon =
                    p.document?.config?.adapter_key === "volcengine_ark"
                      ? Server
                      : Globe;
                  return (
                    <Card
                      key={p.id}
                      className="min-w-0 [--card-spacing:--spacing(6)]"
                    >
                      <CardHeader>
                        <CardTitle>
                          <h2 className="flex items-center gap-3">
                            <Icon className="size-5" />
                            {p.document?.name}
                          </h2>
                        </CardTitle>
                        <CardDescription>
                          {p.origin === "builtin" ? "内置" : "自定义"} ·{" "}
                          {regionLabel(p.document?.config?.region)} ·{" "}
                          {p.document?.config?.adapter_key} · v
                          {p.version ?? "未知"}
                        </CardDescription>
                      </CardHeader>
                      <CardContent className="flex flex-col gap-2">
                        <p className="text-sm text-muted-foreground">
                          {p.document?.model_templates?.length
                            ? `${p.document.model_templates.length} 个模型模板`
                            : "不包含模型模板"}
                        </p>
                        {p.archived || p.emergency_disabled ? (
                          <p className="text-sm">
                            {p.emergency_disabled ? "紧急停用" : "已归档"}
                          </p>
                        ) : null}
                      </CardContent>
                      <CardFooter className="flex-wrap gap-2">
                        <Button
                          disabled={p.archived || p.emergency_disabled}
                          className="max-md:h-11"
                          onClick={() => openConnect(p)}
                        >
                          使用预设
                        </Button>
                        <Button
                          variant="outline"
                          className="max-md:h-11"
                          onClick={() => openEditor(p)}
                        >
                          {p.origin === "builtin" ? "复制并编辑" : "编辑新版本"}
                        </Button>
                        <DownloadButton
                          filename={`provider-preset-${p.id}.json`}
                          load={async () =>
                            new Blob(
                              [
                                JSON.stringify(
                                  await exportProviderPreset({ id: p.id! }),
                                  null,
                                  2,
                                ),
                              ],
                              { type: "application/json" },
                            )
                          }
                        >
                          导出
                        </DownloadButton>
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="max-md:size-11"
                              aria-label={`${p.document?.name}预设的操作`}
                            >
                              <MoreHorizontal data-icon="inline-start" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuGroup>
                              <DropdownMenuItem
                                disabled={mutation.busy}
                                onSelect={() =>
                                  openConfirmation({
                                    kind: "preset",
                                    preset: p,
                                    action: "archive",
                                  })
                                }
                              >
                                {p.archived ? "恢复预设" : "归档"}
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                disabled={mutation.busy}
                                onSelect={() =>
                                  openConfirmation({
                                    kind: "preset",
                                    preset: p,
                                    action: "emergency",
                                  })
                                }
                              >
                                {p.emergency_disabled
                                  ? "解除紧急停用"
                                  : "停止新任务"}
                              </DropdownMenuItem>
                            </DropdownMenuGroup>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </CardFooter>
                    </Card>
                  );
                })}
              </div>
              {!visiblePresets.length ? (
                <Empty>
                  <EmptyHeader>
                    <EmptyTitle>没有匹配的预设</EmptyTitle>
                  </EmptyHeader>
                </Empty>
              ) : null}
              <p className="text-xs text-muted-foreground">
                预设变更生成新版本，不自动改写已有连接。
              </p>
            </TabsContent>
          </>
        )}
      </Tabs>
      <Dialog
        open={Boolean(connect)}
        onOpenChange={(open) => {
          if (!open) closeConnect();
        }}
      >
        <DialogContent
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            if (!detailOpen) restoreFocus();
          }}
          className="flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden p-6 sm:max-w-lg max-md:[&_[data-slot=dialog-close]]:size-11"
        >
          <DialogHeader>
            <DialogTitle>接入 AI 服务</DialogTitle>
            <DialogDescription>选择平台预设，添加 API Key。</DialogDescription>
          </DialogHeader>
          {connect ? (
            <PresetConnect
              presets={presets}
              providers={providers}
              initialPreset={connect === true ? undefined : connect.id}
              onCreated={selectConnection}
              onSaved={(id) => void connected(id)}
              onCancel={closeConnect}
            />
          ) : null}
        </DialogContent>
      </Dialog>
      <Sheet open={detailOpen} onOpenChange={closeDetail}>
        <SheetContent
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            restoreFocus();
          }}
          className="gap-0 data-[side=right]:w-full data-[side=right]:sm:max-w-[520px] max-md:[&_[data-slot=sheet-close]]:size-11"
        >
          <SheetHeader className="sr-only">
            <SheetTitle>连接详情</SheetTitle>
            <SheetDescription>查看连接检查、Key 和配置。</SheetDescription>
          </SheetHeader>
          <div className="min-h-0 flex-1 overflow-y-auto p-6">
            {selected ? (
              <ConnectionDetail
                id={selected}
                revision={refresh}
                presets={presets}
                onSaved={() => setRefresh((v) => v + 1)}
              />
            ) : null}
          </div>
        </SheetContent>
      </Sheet>
      {editor ? (
        <PresetEditor
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
              {confirmation?.kind === "provider"
                ? "停用连接？"
                : confirmation?.action === "archive"
                  ? confirmation.preset.archived
                    ? "恢复预设？"
                    : "归档预设？"
                  : confirmation?.preset.emergency_disabled
                    ? "解除紧急停用？"
                    : "停止新任务？"}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmation?.kind === "provider"
                ? "停止创建新任务，已有任务继续使用绑定的配置和 Key 恢复。"
                : confirmation?.action === "archive"
                  ? "归档仅限制新连接使用此预设，不改写已有连接。"
                  : "此操作控制相关新任务，不会自动撤销历史 Key。"}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={mutation.busy}>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={mutation.busy}
              onClick={(event) => {
                event.preventDefault();
                if (!confirmation) return;
                if (confirmation.kind === "provider") {
                  const p = confirmation.provider;
                  void mutation.run("provider.disable", p.id!, (request) =>
                    updateAdminProvider(
                      { id: p.id! },
                      { expected_revision: p.revision, status: "disabled" },
                      { headers: { "Idempotency-Key": request } },
                    ),
                  );
                } else {
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
                }
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
