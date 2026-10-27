"use client";
import { readErrorMessage } from "@/lib/read-error";
import { ContentState } from "@/components/layout/content-state";

import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

import { useEffect, useState, useId } from "react";
import Link from "next/link";
import { ColumnMenu } from "@/components/governance/column-menu";
import { Plus } from "lucide-react";
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
  DialogDescription,
} from "@/components/ui/dialog";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import { useIsMobile } from "@/hooks/use-mobile";
import { useSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  listAdminModels,
  listAdminProviders,
  listAdminCapabilities,
  createAdminModel,
} from "@/api/settings";
import {
  MutationStatus,
  useCatalogMutation,
  zeroID,
} from "@/components/catalog/mutation";
import { ModelDetail } from "./model-detail";

type ModelsPageProps = { initialProvider?: string };
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
function ModelAdministration({ initialProvider = "" }: ModelsPageProps) {
  const formFieldId = useId();
  const mobile = useIsMobile();
  const [visibility, setVisibility] = useState<Record<string, boolean>>({});
  const [createOpen, setCreateOpen] = useState(false);
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
  const [providerID, setProviderID] = useState(initialProvider);
  const [selected, setSelected] = useState("");
  const [next, setNext] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const saved = () => setRefresh((v) => v + 1);
  const mutation = useCatalogMutation(saved, "models");
  useEffect(() => {
    const abort = new AbortController();
    Promise.all([
      listAdminModels(
        { limit: pageSize, cursor: cursors.at(-1) || undefined },
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
  }, [refresh, cursors, pageSize]);
  const provider = providers.find(
    (p) => p.provider?.id === providerID,
  )?.provider;
  const supported = capabilities.filter(
    (c) =>
      c.key === "video.generate" ||
      (provider?.adapter_key === "openrouter" &&
        ["image.generate", "text.generate"].includes(c.key ?? "")),
  );
  const visible = models.filter(
    (m) => !initialProvider || m.provider_id === initialProvider,
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
          <span className="text-xs break-all whitespace-normal text-muted-foreground">
            {row.original.model_key}
          </span>
        </div>
      ),
    },
    {
      id: "provider",
      header: "服务连接",
      cell: ({ row }) =>
        providers.find((p) => p.provider?.id === row.original.provider_id)
          ?.provider?.name ?? "—",
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
          {row.original.status === "active"
            ? "启用"
            : row.original.status === "disabled"
              ? !row.original.current_version_id ||
                row.original.current_version_id === zeroID
                ? "草稿"
                : "已停用"
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
      <AdminHeading
        title="模型管理"
        description="配置模型与生成能力，验证后逐项发布。"
      >
        <Button disabled={locked} onClick={() => setCreateOpen(true)}>
          <Plus data-icon="inline-start" />
          添加模型
        </Button>
      </AdminHeading>
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
        <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>添加模型</DialogTitle>
            <DialogDescription>
              选择服务连接并创建草稿，保存不会改变线上版本。
            </DialogDescription>
          </DialogHeader>
          <form
            onSubmit={async (event) => {
              event.preventDefault();
              const form = new FormData(event.currentTarget);
              const result = await mutation.run("model.create", zeroID, (id) =>
                createAdminModel(
                  {
                    provider_id: providerID,
                    model_key: String(form.get("key")),
                    display_name: String(form.get("name")),
                    capability: String(form.get("capability")),
                  },
                  { headers: { "Idempotency-Key": id } },
                ),
              );
              if (result?.id) {
                setCreateOpen(false);
                setSelected(result.id);
              }
            }}
          >
            <FieldGroup className="grid gap-4 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-1`}>
                  <span>平台连接</span>
                </FieldLabel>
                <Select
                  required
                  value={providerID}
                  onValueChange={(value) => setProviderID(value)}
                >
                  <SelectTrigger
                    id={`${formFieldId}-field-1`}
                    className="h-10 w-full"
                  >
                    <SelectValue placeholder={"选择连接"} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {providers
                        .filter((p) =>
                          ["volcengine_ark", "openrouter"].includes(
                            p.provider?.adapter_key ?? "",
                          ),
                        )
                        .map((p) =>
                          p.provider?.id ? (
                            <SelectItem
                              key={p.provider?.id}
                              value={p.provider?.id}
                            >
                              {p.provider?.name}
                            </SelectItem>
                          ) : null,
                        )}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-2`}>
                  <span>生成能力</span>
                </FieldLabel>
                <Select
                  key={providerID}
                  name="capability"
                  required
                  defaultValue=""
                >
                  <SelectTrigger
                    id={`${formFieldId}-field-2`}
                    className="h-10 w-full"
                  >
                    <SelectValue placeholder={"选择能力"} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {supported.map((c) =>
                        c.key ? (
                          <SelectItem key={c.key} value={c.key}>
                            {c.key === "video.generate"
                              ? "视频生成"
                              : c.key === "image.generate"
                                ? "图片生成"
                                : "文本生成"}
                          </SelectItem>
                        ) : null,
                      )}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-3`}>
                  <span>显示名称</span>
                </FieldLabel>
                <Input
                  id={`${formFieldId}-field-3`}
                  name="name"
                  required
                  maxLength={100}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-4`}>
                  <span>稳定标识</span>
                </FieldLabel>
                <Input
                  id={`${formFieldId}-field-4`}
                  name="key"
                  required
                  pattern="[a-z][a-z0-9._-]*"
                  maxLength={128}
                  placeholder="例如 creative-video"
                />
              </Field>
              <Button
                className="w-fit"
                disabled={mutation.busy || loading || !!error || !providerID}
              >
                创建模型草稿
              </Button>
            </FieldGroup>
          </form>
          <MutationStatus mutation={mutation} />
        </DialogContent>
      </Dialog>
      <MutationStatus mutation={mutation} />
      {loading ? <ContentState state="loading" title="正在读取模型" /> : null}
      {!loading && !error ? (
        <>
          {initialProvider ? (
            <p className="text-sm text-muted-foreground">
              筛选当前页此连接的模型。
              <Link href="/models" className="underline">
                查看全部模型
              </Link>
            </p>
          ) : null}
          <SelectionBar
            count={selectedRows.length}
            total={visible.length}
            disabled={locked}
            onClear={() => setSelectedRows([])}
          >
            <Button
              variant="destructive"
              disabled={locked}
              onClick={() => void batch.prepare(selectedRows, modelBatchAction)}
            >
              停用模型
            </Button>
          </SelectionBar>
          {!mobile ? (
            <div className="self-end">
              <ColumnMenu
                columns={[
                  { id: "provider", label: "服务连接" },
                  { id: "capability", label: "能力" },
                ]}
                visibility={visibility}
                onChange={setVisibility}
              />
            </div>
          ) : null}
          <DataTable
            density="comfortable"
            label="模型列表"
            columns={columns}
            data={visible}
            emptyTitle={initialProvider ? "本页没有此连接的模型" : "还没有模型"}
            columnVisibility={{
              provider: !mobile && visibility.provider !== false,
              capability: !mobile && visibility.capability !== false,
            }}
            selection={{
              selected: selectedRows,
              onChange: setSelectedRows,
              getLabel: (r) => r.display_name ?? r.id ?? "模型",
              pageLabel: "选择当前页全部模型",
              disabled: locked,
            }}
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
      <Sheet
        open={!!selected}
        onOpenChange={(open) => {
          if (!open) setSelected("");
        }}
      >
        <SheetContent className="overflow-y-auto p-6 data-[side=right]:w-full data-[side=right]:sm:max-w-3xl">
          <SheetHeader>
            <SheetTitle>模型详情</SheetTitle>
            <SheetDescription>管理模型配置、验证与发布。</SheetDescription>
          </SheetHeader>
          {selected ? (
            <ModelDetail
              key={`${selected}:${refresh}`}
              id={selected}
              capabilities={capabilities}
              onSaved={saved}
            />
          ) : null}
        </SheetContent>
      </Sheet>
    </div>
  );
}
