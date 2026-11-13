"use client";
import { readErrorMessage } from "@/lib/read-error";
import { providerConnectionName } from "@/lib/provider-connection-name";
import { ContentState } from "@/components/layout/content-state";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import {
  AdminTable,
  type AdminTableColumn,
  type AdminTableFilterValues,
} from "@/components/governance/admin-table";
import { ChevronDown, Plus, X } from "lucide-react";
import {
  billingStates,
  PricingSources,
  pricingUnits,
} from "./preset-pricing-review";
import { useBatchOperation } from "@/components/governance/use-batch-operation";
import { BatchDialog } from "@/components/governance/batch-dialog";
import { modelBatchAction } from "@/components/governance/batch-actions";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useIsMobile } from "@/hooks/use-mobile";
import { useSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import {
  listAdminCapabilities,
  listAdminModels,
  listAdminProviders,
} from "@/api/settings";
import { AddModelForm } from "./add-model-form";
import { ModelDetail, type ModelDetailView } from "./model-detail";
import { ModelRowEnabledSwitch } from "./model-enabled-switch";

const modelSections = [
  ["configuration", "模型设置"],
  ["pricing", "价格"],
] as const;

function ModelRowActions({
  model,
  disabled,
  onOpen,
}: {
  model: API.CreatedModel;
  disabled: boolean;
  onOpen: (view: ModelDetailView, trigger: HTMLButtonElement | null) => void;
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  const openingDetail = useRef(false);
  if (model.status === "archived")
    return (
      <Button
        variant="outline"
        size="sm"
        className="max-md:h-11"
        disabled={disabled}
        id={`model-manage-${model.id}`}
        aria-label={`管理已归档模型 ${model.display_name}`}
        onClick={(event) => onOpen("configuration", event.currentTarget)}
      >
        管理
      </Button>
    );
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          ref={trigger}
          variant="outline"
          size="sm"
          className="max-md:h-11"
          disabled={disabled}
          id={`model-manage-${model.id}`}
          aria-label={`管理 ${model.display_name}`}
        >
          管理
          <ChevronDown data-icon="inline-end" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        onCloseAutoFocus={(event) => {
          if (openingDetail.current) event.preventDefault();
          openingDetail.current = false;
        }}
      >
        <DropdownMenuGroup>
          {modelSections.map(([view, label]) => (
            <DropdownMenuItem
              key={view}
              onSelect={() => {
                openingDetail.current = true;
                onOpen(view, trigger.current);
              }}
            >
              {label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

type ModelsPageProps = {
  initialProvider?: string;
  openAdd?: boolean;
  initialReviewModel?: string;
};
export function ModelsPage(props: ModelsPageProps) {
  const session = useSession();
  if (session.actor.platform_role !== "platform_admin")
    return (
      <ContentState
        state="error"
        title="暂无访问权限"
        description="此页面仅向平台管理员开放。"
      />
    );
  return (
    <ModelAdministration
      key={`${session.actor.id}:${props.initialProvider ?? ""}`}
      {...props}
    />
  );
}
type ModelFilter =
  "name" | "provider_id" | "capability" | "status" | "billing_status";

const capabilityLabels: Record<string, string> = {
  "video.generate": "视频生成",
  "image.generate": "图片生成",
  "text.generate": "文本生成",
  "audio.generate": "音频生成",
};

const modelStatusLabels = { available: "可用", unavailable: "不可用" };

function modelStatus(model: API.CreatedModel): keyof typeof modelStatusLabels {
  return model.available === true ? "available" : "unavailable";
}

async function listModelConnections(signal: AbortSignal) {
  const items: API.ProviderListItem[] = [];
  let cursor: string | undefined;
  do {
    const page = await listAdminProviders({ limit: 200, cursor }, { signal });
    items.push(...(page.items ?? []));
    cursor = page.next_cursor ?? undefined;
  } while (cursor && !signal.aborted);
  return items;
}

function ModelAdministration({
  initialProvider = "",
  openAdd = false,
  initialReviewModel,
}: ModelsPageProps) {
  const mobile = useIsMobile();
  const [createOpen, setCreateOpen] = useState(openAdd && !initialReviewModel);
  const [pricingReview, setPricingReview] = useState(false);
  const [selectedRows, setSelectedRows] = useState<API.CreatedModel[]>([]);
  const [pageSize, setPageSize] = useState(20);
  const [filters, setFilters] = useState<AdminTableFilterValues<ModelFilter>>(
    {},
  );
  const [cursors, setCursors] = useState<string[]>([""]);
  const [refresh, setRefresh] = useState(0);
  const { registerResultTrigger, ...batch } =
    useBatchOperation<API.CreatedModel>((ids) => {
      setSelectedRows((rows) => rows.filter((r) => !ids.includes(r.id!)));
      setRefresh((v) => v + 1);
    });
  const [models, setModels] = useState<API.CreatedModel[]>([]);
  const [providers, setProviders] = useState<API.ProviderListItem[]>([]);
  const [capabilities, setCapabilities] = useState<API.CapabilitySummary[]>([]);
  const [selected, setSelected] = useState<{
    id: string;
    view: ModelDetailView;
  } | null>(
    initialReviewModel
      ? { id: initialReviewModel, view: "configuration" }
      : null,
  );
  const detailTrigger = useRef<HTMLElement | null>(null);
  const addTrigger = useRef<HTMLButtonElement>(null);
  const pricingPanel = useRef<HTMLElement>(null);
  const selectedID = selected?.id;
  useEffect(() => {
    if (!pricingReview) return;
    if (selectedID) pricingPanel.current?.focus();
    else if (detailTrigger.current) {
      document.getElementById(detailTrigger.current.id)?.focus();
      detailTrigger.current = null;
    }
  }, [pricingReview, selectedID]);
  function openDetail(
    id: string,
    view: ModelDetailView,
    trigger: HTMLElement | null,
  ) {
    detailTrigger.current = trigger;
    setSelected({ id, view });
  }
  const [next, setNext] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const saved = useCallback(() => setRefresh((v) => v + 1), []);
  useEffect(() => {
    const abort = new AbortController();
    Promise.all([
      listAdminModels(
        {
          ...filters,
          ...(pricingReview ? { pricing_review: true } : {}),
          limit: pageSize,
          cursor: cursors.at(-1) || undefined,
          provider_id: initialProvider || filters.provider_id,
        },
        { signal: abort.signal },
      ),
      listModelConnections(abort.signal),
      listAdminCapabilities({ signal: abort.signal }),
    ])
      .then(([m, p, c]) => {
        if (!abort.signal.aborted) {
          setModels(m.items ?? []);
          setNext(m.next_cursor ?? "");
          setProviders(p);
          setCapabilities(c ?? []);
          setError("");
          setLoading(false);
        }
      })
      .catch((e) => {
        if (!abort.signal.aborted) {
          setError(readErrorMessage(e));
          setLoading(false);
        }
      });
    return () => abort.abort();
  }, [refresh, cursors, pageSize, initialProvider, filters, pricingReview]);
  const connectionOptions = providers.flatMap(({ provider }) => {
    const name = providerConnectionName(provider);
    return provider?.id && name ? [{ value: provider.id, label: name }] : [];
  });
  const connectionCounts = new Map<string, number>();
  for (const { label } of connectionOptions) {
    connectionCounts.set(label, (connectionCounts.get(label) ?? 0) + 1);
  }
  const connectionRanks = new Map<string, number>();
  const filteredConnection = providers.find(
    (item) => item.provider?.id === initialProvider,
  )?.provider;
  const archivedConnection = filteredConnection?.status === "archived";
  const locked = loading || !!error || batch.busy || batch.unresolved;
  // Keep the switch mounted while list refreshes, preserving progress and feedback.
  const enabledCell = useCallback(
    ({ row }: { row: { original: API.CreatedModel } }) => (
      <ModelRowEnabledSwitch
        model={row.original}
        onSaved={saved}
        disabled={locked}
      />
    ),
    [locked, saved],
  );
  const columns: AdminTableColumn<API.CreatedModel, ModelFilter>[] = [
    {
      id: "model",
      header: "模型",
      filter: {
        name: "name",
        label: "模型名称",
        type: "text",
        placeholder: "名称或标识",
      },
      cell: ({ row }) => {
        const provider = mobile
          ? providers.find(
              (item) => item.provider?.id === row.original.provider_id,
            )?.provider
          : undefined;
        return (
          <div className="flex min-w-0 flex-col gap-1">
            <Button
              size="toolbar"
              variant="link"
              className="h-auto justify-start p-0 text-left break-words whitespace-normal text-foreground"
              id={`model-overview-${row.original.id}`}
              aria-label={`查看 ${row.original.display_name}`}
              disabled={locked}
              onClick={(event) =>
                openDetail(
                  row.original.id!,
                  "configuration",
                  event.currentTarget,
                )
              }
            >
              {row.original.display_name}
            </Button>
            {mobile ? (
              <span className="text-xs text-muted-foreground">
                {capabilityLabels[row.original.capability ?? ""] ??
                  row.original.capability}{" "}
                · 按用量计费
              </span>
            ) : null}
            {provider?.id ? (
              locked ? (
                <span className="text-xs text-muted-foreground">
                  {providerConnectionName(provider)}
                </span>
              ) : (
                <Link
                  className="text-xs text-muted-foreground underline underline-offset-2"
                  href={`/providers?connection=${encodeURIComponent(provider.id)}`}
                >
                  {providerConnectionName(provider)}
                </Link>
              )
            ) : null}
          </div>
        );
      },
    },
    {
      id: "provider",
      header: "服务连接",
      settings: { hideOnMobile: true },
      filter: initialProvider
        ? undefined
        : {
            name: "provider_id",
            type: "select",
            options: connectionOptions.map(({ value, label }) => {
              if ((connectionCounts.get(label) ?? 0) < 2)
                return { value, label };
              const rank = (connectionRanks.get(label) ?? 0) + 1;
              connectionRanks.set(label, rank);
              return { value, label: `${label} · 连接 ${rank}` };
            }),
          },
      cell: ({ row }) => {
        const provider = providers.find(
          (p) => p.provider?.id === row.original.provider_id,
        )?.provider;
        if (!provider?.id) return "—";
        if (locked) return providerConnectionName(provider);
        return (
          <Button
            size="toolbar"
            asChild
            variant="link"
            className="h-auto justify-start p-0 text-left whitespace-normal"
          >
            <Link
              href={`/providers?connection=${encodeURIComponent(provider.id)}`}
            >
              {providerConnectionName(provider)}
            </Link>
          </Button>
        );
      },
    },
    {
      id: "capability",
      header: "能力",
      settings: { hideOnMobile: true },
      filter: {
        name: "capability",
        type: "select",
        options: capabilities.flatMap(({ key }) =>
          key ? [{ value: key, label: capabilityLabels[key] ?? key }] : [],
        ),
      },
      cell: ({ row }) =>
        capabilityLabels[row.original.capability ?? ""] ??
        row.original.capability,
    },
    {
      id: "price",
      header: "积分",
      settings: { hideOnMobile: true },
      cell: () => <span className="text-muted-foreground">按用量计费</span>,
    },
    {
      id: "status",
      header: "可用性",
      filter: {
        name: "status",
        type: "select",
        options: Object.entries(modelStatusLabels).map(([value, label]) => ({
          value,
          label,
        })),
      },
      cell: ({ row }) => {
        const status = modelStatus(row.original);
        return (
          <Badge variant="secondary">
            {status ? modelStatusLabels[status] : "未知"}
          </Badge>
        );
      },
    },
    {
      id: "enabled",
      header: "启用",
      cell: enabledCell,
    },
    {
      id: "actions",
      header: "操作",
      cell: ({ row }) => (
        <ModelRowActions
          model={row.original}
          disabled={locked}
          onOpen={(view, trigger) =>
            openDetail(row.original.id!, view, trigger)
          }
        />
      ),
    },
  ];
  const pricingColumns: AdminTableColumn<API.CreatedModel, ModelFilter>[] = [
    {
      id: "model",
      header: "模型 / 服务",
      filter: columns[0].filter,
      cell: ({ row }) => {
        const provider = providers.find(
          (item) => item.provider?.id === row.original.provider_id,
        )?.provider;
        return (
          <div className="flex min-w-0 flex-col gap-1">
            <span className="break-words">{row.original.display_name}</span>
            <span className="text-xs text-muted-foreground">
              {providerConnectionName(provider)}
            </span>
          </div>
        );
      },
    },
    {
      id: "unit",
      header: "计量方式",
      settings: { defaultVisible: false, hideOnMobile: true },
      cell: ({ row }) =>
        pricingUnits[row.original.billing?.unit ?? ""] ?? "待确认",
    },
    {
      id: "billing",
      header: "价格状态",
      filter: {
        name: "billing_status",
        label: "计价状态",
        type: "select",
        options: Object.entries(billingStates).map(([value, label]) => ({
          value,
          label,
        })),
      },
      cell: ({ row }) => (
        <Badge
          variant={
            row.original.billing?.status === "billable"
              ? "secondary"
              : "outline"
          }
        >
          {billingStates[row.original.billing?.status ?? ""] ?? "待核验"}
        </Badge>
      ),
    },
    {
      id: "actions",
      header: "操作",
      cell: ({ row }) => (
        <Button
          variant="outline"
          size="sm"
          className="max-md:h-11"
          disabled={locked || !row.original.id}
          id={`model-price-review-${row.original.id}`}
          aria-label={`审核计价 ${row.original.display_name}`}
          onClick={(event) =>
            openDetail(row.original.id!, "pricing", event.currentTarget)
          }
        >
          {row.original.billing?.status === "billable" ? "查看" : "审核"}
        </Button>
      ),
    },
  ];
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
      <AdminTable
        title={pricingReview ? "预设计价审核" : "模型管理"}
        label={pricingReview ? "预设计价列表" : "模型列表"}
        columns={pricingReview ? pricingColumns : columns}
        rows={models}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            {pricingReview && (
              <PricingSources
                adapters={providers.flatMap(({ provider }) =>
                  provider?.adapter_key ? [provider.adapter_key] : [],
                )}
              />
            )}
            <Button
              variant="outline"
              size="toolbar"
              disabled={locked}
              onClick={() => {
                setSelectedRows([]);
                setCursors([""]);
                setFilters({});
                setSelected(null);
                setLoading(true);
                setPricingReview((value) => !value);
              }}
            >
              {pricingReview ? "返回模型管理" : "计价审核"}
            </Button>
            {!pricingReview && (
              <Button
                size="toolbar"

                disabled={locked || archivedConnection}
                ref={addTrigger}
                onClick={() => setCreateOpen(true)}
              >
                <Plus data-icon="inline-start" />
                添加模型
              </Button>
            )}
          </div>
        }
        toolbarExtra={
          batch.action ? (
            <Button
              size="toolbar"
              variant="outline"
              ref={registerResultTrigger}
              onClick={() => batch.setOpen(true)}
            >
              查看批量操作结果
            </Button>
          ) : null
        }
        context={
          !loading && !error && initialProvider ? (
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <Badge variant="secondary">
                {providerConnectionName(filteredConnection) ?? "已选连接"}
              </Badge>
              {archivedConnection ? (
                <span className="text-muted-foreground">
                  恢复连接后可添加模型
                </span>
              ) : null}
              <Button asChild variant="link">
                <Link href="/models">查看全部模型</Link>
              </Button>
            </div>
          ) : null
        }
        filter={{
          label: pricingReview ? "计价筛选" : "模型筛选",
          disabled: batch.busy || batch.unresolved,
          onApply: (values) => {
            setSelectedRows([]);
            setCursors([""]);
            setLoading(true);
            setFilters(values);
          },
        }}
        loading={loading ? "正在读取模型" : undefined}
        error={
          error
            ? {
                title: "模型暂时无法读取",
                description: error,
                onRetry: () => {
                  setError("");
                  setLoading(true);
                  setRefresh((v) => v + 1);
                },
              }
            : undefined
        }
        emptyTitle={
          Object.keys(filters).length
            ? "没有匹配的模型"
            : pricingReview
              ? "暂无已接入的预设"
              : initialProvider
                ? "本页没有此连接的模型"
                : "还没有模型"
        }
        selection={
          pricingReview
            ? undefined
            : {
                selected: selectedRows,
                onChange: setSelectedRows,
                getLabel: (row) => row.display_name ?? row.id ?? "模型",
                pageLabel: "选择当前页全部模型",
                disabled: locked,
                actions: (
                  <Button
                    variant="destructive"
                    disabled={locked}
                    onClick={() =>
                      void batch.prepare(selectedRows, modelBatchAction)
                    }
                  >
                    停用模型
                  </Button>
                ),
              }
        }
        pagination={{
          label: "模型分页",
          current: cursors.length,
          pageSize,
          pageSizeOptions: [20, 50],
          hasNextPage: !!next,
          disabled: locked,
          onPageChange: (page) => {
            setSelectedRows([]);
            setLoading(true);
            setCursors((c) =>
              page < c.length ? c.slice(0, -1) : [...c, next],
            );
          },
          onPageSizeChange: (size) => {
            setSelectedRows([]);
            setLoading(true);
            setPageSize(size);
            setCursors([""]);
          },
        }}
      />
      {pricingReview && selected && (
        <section
          ref={pricingPanel}
          tabIndex={-1}
          aria-label="审核价目"
          className="relative scroll-m-4 rounded-lg border border-border bg-popover p-4 outline-none sm:p-5"
        >
          <Button
            type="button"
            aria-label="关闭计价审核"
            variant="ghost"
            size="icon"
            className="absolute top-3 right-3 max-md:size-11"
            onClick={() => setSelected(null)}
          >
            <X />
          </Button>
          <ModelDetail
            key={selected.id}
            id={selected.id}
            initialView="pricing"
            pricingReview
            capabilities={capabilities}
            providers={providers}
            onSaved={saved}
          />
        </section>
      )}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent
          aria-describedby={undefined}
          showCloseButton={false}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            if (!selected) addTrigger.current?.focus();
          }}
          className="flex flex-col overflow-hidden border border-border p-4 sm:max-w-[560px] sm:p-6"
        >
          <DialogHeader>
            <DialogTitle size="lg">添加模型</DialogTitle>
          </DialogHeader>
          {createOpen ? (
            <AddModelForm
              providers={providers.filter(
                (item) => item.provider?.status !== "archived",
              )}
              initialProvider={initialProvider}
              onAdded={(id) => {
                setCursors([""]);
                saved();
                setCreateOpen(false);
                openDetail(id, "configuration", addTrigger.current);
              }}
              onCredentialSaved={saved}
            />
          ) : null}
        </DialogContent>
      </Dialog>
      <BatchDialog key={batch.generation} batch={batch} />
      <Dialog
        open={!!selected && !pricingReview}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
      >
        <DialogContent
          aria-describedby={undefined}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            const trigger = detailTrigger.current;
            const target = trigger?.isConnected
              ? trigger
              : (document.getElementById(trigger?.id ?? "") ??
                addTrigger.current);
            target?.focus();
          }}
          className="max-h-[calc(100dvh-2rem)] overflow-y-auto p-4 sm:max-w-[560px] sm:p-6"
        >
          <DialogHeader className="sr-only">
            <DialogTitle>模型详情</DialogTitle>
          </DialogHeader>
          {selected ? (
            <ModelDetail
              key={selected.id}
              id={selected.id}
              initialView={selected.view}
              capabilities={capabilities}
              providers={providers}
              onSaved={saved}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}
