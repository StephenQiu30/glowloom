"use client";
import { readErrorMessage } from "@/lib/read-error";
import { providerConnectionName } from "@/lib/provider-connection-name";
import { ContentState } from "@/components/layout/content-state";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { ColumnMenu } from "@/components/governance/column-menu";
import { Plus, X } from "lucide-react";
import {
  billingStates,
  PricingSources,
  pricingUnits,
} from "./preset-pricing-review";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  AdminHeading,
  SelectionBar,
} from "@/components/governance/admin-layout";
import { useBatchOperation } from "@/components/governance/use-batch-operation";
import { BatchDialog } from "@/components/governance/batch-dialog";
import { modelBatchAction } from "@/components/governance/batch-actions";
import {
  DataTable,
  type DataTableColumn,
} from "@/components/canvas/runtime/components/account/list/data-table";
import { ListPagination } from "@/components/canvas/runtime/components/layout/workspace/list-pagination";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useIsMobile } from "@/hooks/use-mobile";
import { useSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import {
  listAdminCapabilities,
  listAdminModels,
  listAdminProviders,
} from "@/api/settings";
import { zeroID } from "@/components/catalog/mutation";
import { AddModelForm } from "./add-model-form";
import { ModelDetail } from "./model-detail";

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
      key={`${session.actor.id}:${props.initialProvider ?? ""}:${props.openAdd ?? false}:${props.initialReviewModel ?? ""}`}
      {...props}
    />
  );
}
function ModelAdministration({
  initialProvider = "",
  openAdd = false,
  initialReviewModel = "",
}: ModelsPageProps) {
  const mobile = useIsMobile();
  const [visibility, setVisibility] = useState<Record<string, boolean>>({});
  const [createOpen, setCreateOpen] = useState(openAdd && !initialReviewModel);
  const addTrigger = useRef<HTMLButtonElement>(null);
  const [pricingReview, setPricingReview] = useState(false);
  const [billingStatus, setBillingStatus] = useState("");
  const [pricingModel, setPricingModel] = useState("");
  const pricingPanel = useRef<HTMLElement>(null);
  const pricingTrigger = useRef<string | null>(null);
  useEffect(() => {
    if (!pricingReview) return;
    if (pricingModel) pricingPanel.current?.focus();
    else if (pricingTrigger.current) {
      document.getElementById(pricingTrigger.current)?.focus();
      pricingTrigger.current = null;
    }
  }, [pricingReview, pricingModel]);
  const [selectedRows, setSelectedRows] = useState<API.CreatedModel[]>([]);
  const [pageSize, setPageSize] = useState(20);
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
  const [selected, setSelected] = useState(initialReviewModel);
  const [next, setNext] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const saved = () => setRefresh((v) => v + 1);
  useEffect(() => {
    const abort = new AbortController();
    Promise.all([
      listAdminModels(
        {
          ...(pricingReview
            ? {
                pricing_review: true,
                billing_status: billingStatus || undefined,
              }
            : {}),
          limit: pageSize,
          cursor: cursors.at(-1) || undefined,
          provider_id: initialProvider || undefined,
        },
        { signal: abort.signal },
      ),
      listAdminProviders({ limit: 200 }, { signal: abort.signal }),
      listAdminCapabilities({ signal: abort.signal }),
    ])
      .then(([m, p, c]) => {
        if (!abort.signal.aborted) {
          setModels(m.items ?? []);
          setNext(m.next_cursor ?? "");
          setProviders(p.items ?? []);
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
  }, [
    refresh,
    cursors,
    pageSize,
    initialProvider,
    pricingReview,
    billingStatus,
  ]);
  const visible = models;
  const archivedConnection = providers.some(
    (item) =>
      item.provider?.id === initialProvider &&
      item.provider.status === "archived",
  );
  const locked = loading || !!error || batch.busy || batch.unresolved;
  const columns: DataTableColumn<API.CreatedModel>[] = [
    {
      id: "model",
      header: "模型",
      cell: ({ row }) => (
        <div className="flex min-w-0 flex-col gap-1">
          <span className="break-words whitespace-normal">
            {row.original.display_name}
          </span>
        </div>
      ),
    },
    {
      id: "provider",
      header: "服务连接",
      cell: ({ row }) => {
        const provider = providers.find(
          (p) => p.provider?.id === row.original.provider_id,
        )?.provider;
        return providerConnectionName(provider) ?? "—";
      },
    },
    {
      id: "capability",
      header: "能力",
      cell: ({ row }) =>
        ({
          "video.generate": "视频生成",
          "image.generate": "图片生成",
          "text.generate": "文本生成",
        })[row.original.capability ?? ""] ?? row.original.capability,
    },
    {
      id: "status",
      header: "状态",
      cell: ({ row }) => (
        <Badge variant="secondary">
          {row.original.status === "archived"
            ? "已归档"
            : row.original.status === "active"
              ? "启用"
              : row.original.status === "disabled"
                ? row.original.has_publication
                  ? "已停用"
                  : !row.original.current_version_id ||
                      row.original.current_version_id === zeroID
                    ? "待配置"
                    : row.original.has_price
                      ? "待验证"
                      : "价格待审核"
                : "未知"}
        </Badge>
      ),
    },
    {
      id: "actions",
      header: "操作",
      cell: ({ row }) => (
        <Button
          variant="ghost"
          disabled={locked}
          aria-label={`查看 ${row.original.display_name}`}
          onClick={() => setSelected(row.original.id!)}
        >
          查看
        </Button>
      ),
    },
  ];
  const pricingColumns: DataTableColumn<API.CreatedModel>[] = [
    {
      id: "model",
      header: "模型 / 服务",
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
      cell: ({ row }) =>
        pricingUnits[row.original.billing?.unit ?? ""] ?? "待确认",
    },
    {
      id: "procurement",
      header: "采购价",
      cell: ({ row }) =>
        row.original.billing?.procurement_ready ? "已审核" : "待核验",
    },
    {
      id: "sales",
      header: "销售价",
      cell: ({ row }) =>
        row.original.billing?.sale_published
          ? "已发布"
          : row.original.billing?.sale_ready
            ? "已保存"
            : "缺失",
    },
    {
      id: "billing",
      header: "发布状态",
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
          onClick={(event) => {
            pricingTrigger.current = event.currentTarget.id;
            setPricingModel(row.original.id!);
          }}
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
      <AdminHeading title={pricingReview ? "预设计价审核" : "模型管理"} />
      <div className="flex flex-wrap items-center justify-end gap-2">
        {!loading && !error && initialProvider ? (
          <div className="mr-auto flex flex-wrap items-center gap-2 text-sm">
            <span className="text-muted-foreground">
              {archivedConnection ? "恢复连接后可添加模型" : "本页按连接筛选"}
            </span>
            <Button asChild variant="link" size="toolbar">
              <Link href="/models">查看全部模型</Link>
            </Button>
          </div>
        ) : null}
        {!pricingReview && !loading && !error && !mobile ? (
          <ColumnMenu
            columns={[
              { id: "provider", label: "服务连接" },
              { id: "capability", label: "能力" },
            ]}
            visibility={visibility}
            onChange={setVisibility}
          />
        ) : null}
        {pricingReview && (
          <>
            <Select
              value={billingStatus || "all"}
              disabled={locked}
              onValueChange={(value) => {
                setCursors([""]);
                setLoading(true);
                setPricingModel("");
                setBillingStatus(value === "all" ? "" : value);
              }}
            >
              <SelectTrigger aria-label="计价状态" size="toolbar">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部计价状态</SelectItem>
                {Object.entries(billingStates).map(([value, label]) => (
                  <SelectItem key={value} value={value}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <PricingSources
              adapters={providers.flatMap(({ provider }) =>
                provider?.adapter_key ? [provider.adapter_key] : [],
              )}
            />
          </>
        )}
        <Button
          variant="outline"
          size="toolbar"
          disabled={locked}
          onClick={() => {
            setSelectedRows([]);
            setCursors([""]);
            setPricingModel("");
            setBillingStatus("");
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
      {error ? (
        <ContentState
          state="error"
          title="模型暂时无法读取"
          description={error}
          actions={
            <Button
              onClick={() => {
                setError("");
                setLoading(true);
                setRefresh((v) => v + 1);
              }}
            >
              重试
            </Button>
          }
        />
      ) : null}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent
          aria-describedby={undefined}
          showCloseButton={false}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            if (!selected) addTrigger.current?.focus();
          }}
          className="flex flex-col overflow-hidden border border-border p-4 sm:max-w-[640px] sm:p-6"
        >
          <DialogHeader>
            <DialogTitle size="lg" className="text-xl">
              添加模型
            </DialogTitle>
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
                setSelected(id);
              }}
              onCredentialSaved={saved}
            />
          ) : null}
        </DialogContent>
      </Dialog>
      {loading ? <ContentState state="loading" title="正在读取模型" /> : null}
      {!loading && !error ? (
        <>
          {!pricingReview && selectedRows.length ? (
            <SelectionBar
              count={selectedRows.length}
              total={visible.length}
              disabled={locked}
              onClear={() => setSelectedRows([])}
            >
              <Button
                variant="destructive"
                size="toolbar"
                disabled={locked}
                onClick={() =>
                  void batch.prepare(selectedRows, modelBatchAction)
                }
              >
                停用模型
              </Button>
            </SelectionBar>
          ) : null}
          <DataTable
            density="comfortable"
            label={pricingReview ? "预设计价列表" : "模型列表"}
            columns={pricingReview ? pricingColumns : columns}
            data={visible}
            emptyTitle={
              pricingReview
                ? billingStatus
                  ? "没有匹配的预设"
                  : "暂无已接入的预设"
                : initialProvider
                  ? "本页没有此连接的模型"
                  : "还没有模型"
            }
            columnVisibility={
              pricingReview
                ? { unit: !mobile, procurement: !mobile, sales: !mobile }
                : {
                    provider: !mobile && visibility.provider !== false,
                    capability: !mobile && visibility.capability !== false,
                  }
            }
            selection={
              pricingReview
                ? undefined
                : {
                    selected: selectedRows,
                    onChange: setSelectedRows,
                    getLabel: (r) => r.display_name ?? r.id ?? "模型",
                    pageLabel: "选择当前页全部模型",
                    disabled: locked,
                  }
            }
          />
          <ListPagination
            mode="cursor"
            label="模型分页"
            current={cursors.length}
            pageSize={pageSize}
            pageSizeOptions={[20, 50]}
            itemCount={visible.length}
            hasNextPage={!!next}
            disabled={locked}
            onPageChange={(page) => {
              setSelectedRows([]);
              setLoading(true);
              setCursors((c) =>
                page < c.length ? c.slice(0, -1) : [...c, next],
              );
            }}
            onPageSizeChange={(size) => {
              setSelectedRows([]);
              setLoading(true);
              setPageSize(size);
              setCursors([""]);
            }}
          />
        </>
      ) : null}
      {pricingReview && pricingModel && (
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
            onClick={() => setPricingModel("")}
          >
            <X />
          </Button>
          <ModelDetail
            key={pricingModel}
            id={pricingModel}
            initialView="pricing"
            pricingReview
            capabilities={capabilities}
            providers={providers}
            onSaved={saved}
          />
        </section>
      )}
      {batch.action ? (
        <Button
          variant="outline"
          className="self-start"
          ref={registerResultTrigger}
          onClick={() => batch.setOpen(true)}
        >
          查看批量操作结果
        </Button>
      ) : null}
      <BatchDialog key={batch.generation} batch={batch} />
      <Dialog
        open={!!selected}
        onOpenChange={(open) => {
          if (!open) setSelected("");
        }}
      >
        <DialogContent
          aria-describedby={undefined}
          className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl"
        >
          <DialogHeader className="sr-only">
            <DialogTitle>模型详情</DialogTitle>
          </DialogHeader>
          {selected ? (
            <ModelDetail
              key={selected}
              id={selected}
              initialView={
                selected === initialReviewModel ? "release" : undefined
              }
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
