"use client";
import { readErrorMessage } from "@/lib/read-error";
import { ContentState } from "@/components/layout/content-state";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import {
  ChevronDown,
  Globe,
  MoreHorizontal,
  Plus,
  Server,
  ShieldCheck,
} from "lucide-react";
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
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
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
  CardFooter,
} from "@/components/ui/card";

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
import { ServiceTrial } from "./service-trial";
import { PresetEditor } from "./preset-editor";
import { ConnectionDetail } from "./connection-detail";
import { PresetConnect } from "./preset-connect";
import { ConnectionList, regionLabel } from "./connection-list";

type Confirmation =
  | { kind: "provider"; provider: API.CreatedProvider }
  | { kind: "preset"; preset: API.Preset; action: "archive" | "emergency" };

type ProvidersPageProps = { initialConnection?: string; search?: string };

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
}: ProvidersPageProps) {
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
  const [presetsOpen, setPresetsOpen] = useState(false);
  const [trialOpen, setTrialOpen] = useState(false);
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
  useEffect(() => {
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
          setError(readErrorMessage(error));
          setLoading(false);
        }
      });
    return () => controller.abort();
  }, [refresh]);
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
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-8 px-4 py-6 md:px-8 md:py-10">
      <header className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex flex-col gap-2">
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <ShieldCheck className="size-3.5" />
            仅管理员
          </span>
          <h1 className="text-2xl font-medium tracking-tight">AI 服务接入</h1>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            className="max-md:h-11"
            onClick={() => setTrialOpen(true)}
          >
            开发试验
          </Button>
          <Button variant="ghost" className="max-md:h-11" asChild>
            <Link href="/models">模型管理</Link>
          </Button>
          <Button
            className="max-md:h-11"
            onClick={() => openConnect(true)}
            disabled={loading || !!error}
          >
            <Plus data-icon="inline-start" />
            接入服务
          </Button>
        </div>
      </header>
      {mutation.message || mutation.pending ? (
        <MutationStatus mutation={mutation} />
      ) : null}
      {feedback ? <FeedbackToast message={feedback} /> : null}
      {pendingCreate.pending || pendingKey.pending ? (
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant="secondary">保存待确认</Badge>
          <Button variant="outline" onClick={() => openConnect(true)}>
            继续接入
          </Button>
        </div>
      ) : null}
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
        <ContentState
          state="error"
          title="服务连接暂时无法读取"
          description={error}
          actions={<Button onClick={retry}>重试</Button>}
        />
      ) : (
        <>
          <section aria-label="已添加的服务" className="flex flex-col gap-4">
            <h2 className="text-sm font-medium text-muted-foreground">
              服务{" "}
              <span className="ml-1 tabular-nums">
                {visibleProviders.length}
              </span>
            </h2>
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
              <ContentState
                title={query ? "没有匹配的连接" : "还没有连接"}
                description={
                  query ? "试试其他关键词。" : "选择平台预设，添加 API Key。"
                }
                actions={
                  !query ? (
                    <Button onClick={() => openConnect(true)}>接入服务</Button>
                  ) : undefined
                }
              />
            )}
          </section>
          <Collapsible
            open={presetsOpen || Boolean(query)}
            onOpenChange={setPresetsOpen}
            className="border-t pt-5"
          >
            <div className="flex items-center justify-between gap-3">
              <CollapsibleTrigger asChild>
                <Button
                  variant="ghost"
                  className="group -ml-3 text-muted-foreground max-md:h-11"
                >
                  预设管理
                  <ChevronDown className="size-4 transition-transform group-data-[state=open]:rotate-180" />
                </Button>
              </CollapsibleTrigger>
              {presetsOpen || query ? (
                <Button variant="outline" onClick={() => openEditor("new")}>
                  新建预设
                </Button>
              ) : null}
            </div>
            <CollapsibleContent className="pt-4">
              <div className="grid gap-3 sm:grid-cols-2">
                {visiblePresets.map((p) => {
                  const Icon =
                    p.document?.config?.adapter_key === "volcengine_ark"
                      ? Server
                      : Globe;
                  return (
                    <Card
                      key={p.id}
                      className="min-w-0 gap-4 [--card-spacing:--spacing(4)]"
                    >
                      <CardHeader>
                        <CardTitle>
                          <h2 className="flex items-center gap-3">
                            <Icon className="size-5" />
                            {p.document?.name}
                          </h2>
                        </CardTitle>
                        <CardDescription>
                          {regionLabel(p.document?.config?.region)}
                          {p.emergency_disabled
                            ? " · 紧急停用"
                            : p.archived
                              ? " · 已归档"
                              : p.origin === "builtin"
                                ? " · 内置"
                                : " · 自定义"}
                        </CardDescription>
                      </CardHeader>
                      <CardFooter className="flex-wrap gap-2">
                        <Button
                          disabled={p.archived || p.emergency_disabled}
                          className="max-md:h-11"
                          onClick={() => openConnect(p)}
                        >
                          使用预设
                        </Button>
                        <Collapsible className="w-full">
                          <CollapsibleTrigger asChild>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="text-muted-foreground"
                            >
                              维护预设
                              <ChevronDown className="size-3.5" />
                            </Button>
                          </CollapsibleTrigger>
                          <CollapsibleContent className="flex flex-wrap gap-2 pt-3">
                            <Button
                              variant="outline"
                              className="max-md:h-11"
                              onClick={() => openEditor(p)}
                            >
                              {p.origin === "builtin"
                                ? "复制并编辑"
                                : "编辑新版本"}
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
                          </CollapsibleContent>
                        </Collapsible>
                      </CardFooter>
                    </Card>
                  );
                })}
              </div>
              {!visiblePresets.length ? (
                <ContentState title="没有匹配的预设" />
              ) : null}
            </CollapsibleContent>
          </Collapsible>
        </>
      )}
      {trialOpen ? (
        <ServiceTrial
          open={trialOpen}
          onOpenChange={setTrialOpen}
          providers={providers}
        />
      ) : null}
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
            <DialogDescription className="sr-only">
              选择平台并添加 API Key。
            </DialogDescription>
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
