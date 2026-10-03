"use client";

import { useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { z } from "zod";
import { setAdminModelStatus } from "@/api/settings";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  ADMIN_KEY,
  adminError,
  adminWriteOptions,
  parseAdmin,
  queryAdminModel,
  queryAdminModels,
  queryCapabilities,
  queryProviders,
  type Capability,
  type ModelDetail,
} from "./admin-queries";
import { MODELS_KEY } from "./queries";
import {
  AdminDialog,
  AdminFailure,
  AdminMessage,
  formatAdminTime,
} from "./admin-ui";
import {
  ModelCreateForm,
  ModelPriceForm,
  ModelVersionForm,
} from "./admin-model-forms";

function ModelHistory({ detail }: { detail: ModelDetail }) {
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>配置版本</CardTitle>
          <CardDescription>
            发布后不可修改。调整配置需要发布下一版本。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {detail.versions.length ? (
            detail.versions.map((version) => (
              <details key={version.id} className="rounded-lg border p-4">
                <summary className="cursor-pointer text-sm font-medium">
                  v{version.version_no} · {version.provider_model_id}
                  {version.id === detail.model.current_version_id
                    ? " · 当前配置"
                    : ""}
                </summary>
                <div className="mt-4 space-y-3 text-sm">
                  <p>
                    {version.modes.join("、")} · 队列 {version.queue}
                  </p>
                  <p>
                    最长耗时 {version.expected_max_ms}ms · 审核{" "}
                    {version.moderation} · 查询{" "}
                    {version.supports_query ? "支持" : "不支持"} · 取消{" "}
                    {version.supports_cancel ? "支持" : "不支持"} · 回调{" "}
                    {version.supports_callback ? "支持" : "不支持"}
                  </p>
                  <p className="text-muted-foreground">
                    {formatAdminTime(version.create_time)}
                  </p>
                  <p>输入与输出限制</p>
                  <pre className="max-h-72 overflow-auto rounded-md bg-muted p-3 text-xs">
                    {JSON.stringify(version.limits, null, 2)}
                  </pre>
                  <p>生成参数</p>
                  <pre className="max-h-72 overflow-auto rounded-md bg-muted p-3 text-xs">
                    {JSON.stringify(version.param_schema, null, 2)}
                  </pre>
                </div>
              </details>
            ))
          ) : (
            <p className="text-sm text-muted-foreground">尚未发布配置。</p>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>价格版本</CardTitle>
          <CardDescription>
            展示全部已发布价格及各自的生效时间。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {detail.prices.length ? (
            detail.prices.map((price) => (
              <details key={price.id} className="rounded-lg border p-4">
                <summary className="cursor-pointer text-sm font-medium">
                  v{price.version_no} · {price.currency} / {price.unit}
                </summary>
                <div className="mt-4 space-y-3 text-sm">
                  <p>生效于 {formatAdminTime(price.effective_from)}</p>
                  <p>人民币汇率：{price.fx_rate_to_cny ?? "按人民币计价"}</p>
                  <pre className="max-h-72 overflow-auto rounded-md bg-muted p-3 text-xs">
                    {JSON.stringify(price.rule, null, 2)}
                  </pre>
                  <p className="text-muted-foreground">
                    发布于 {formatAdminTime(price.create_time)}
                  </p>
                </div>
              </details>
            ))
          ) : (
            <p className="text-sm text-muted-foreground">尚未发布价格。</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
function ModelDetails({
  id,
  capabilities,
}: {
  id: string;
  capabilities: Capability[];
}) {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: [...ADMIN_KEY, "model", id],
    queryFn: ({ signal }) => queryAdminModel(id, signal),
    retry: false,
    refetchOnWindowFocus: false,
  });
  function refresh() {
    void client.invalidateQueries({ queryKey: ADMIN_KEY });
    void client.invalidateQueries({ queryKey: MODELS_KEY });
  }
  const mutation = useMutation({
    mutationFn: async () => {
      if (!query.data) throw new Error("missing model");
      const { model } = query.data;
      return parseAdmin(
        z.object({
          id: z.string().uuid(),
          revision: z.number().int().positive(),
          status: z.enum(["active", "disabled"]),
        }),
        await setAdminModelStatus(
          { id },
          {
            expected_revision: model.revision,
            status: model.status === "active" ? "disabled" : "active",
          },
          adminWriteOptions(),
        ),
      );
    },
    onSuccess: refresh,
    retry: false,
  });
  if (query.isPending) return <p role="status">载入模型详情…</p>;
  if (query.isError)
    return (
      <AdminFailure error={query.error} retry={() => void query.refetch()} />
    );
  const detail = query.data;
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>{detail.model.display_name}</CardTitle>
          <CardDescription>
            {detail.model.model_key} · {detail.model.capability} · 修订{" "}
            {detail.model.revision}
            <br />
            模型状态：{detail.model.status === "active" ? "启用" : "禁用"}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              disabled={
                mutation.isPending ||
                (detail.model.status === "disabled" &&
                  (!detail.versions.length || !detail.prices.length))
              }
              onClick={() => mutation.mutate()}
            >
              {detail.model.status === "active" ? "禁用模型" : "启用模型"}
            </Button>
            <Button
              variant="ghost"
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              读取最新模型配置
            </Button>
          </div>
          {detail.model.status === "disabled" &&
            (!detail.versions.length || !detail.prices.length) && (
              <p className="text-sm text-muted-foreground">
                发布配置与价格后，才能启用模型。
              </p>
            )}
          <AdminMessage
            message={mutation.isError ? adminError(mutation.error) : ""}
            error
          />
          <div className="flex flex-wrap gap-2">
            <AdminDialog
              title="发布配置版本"
              description="新版本会成为当前配置。已创建任务继续使用原来的冻结配置。"
            >
              {(close) => (
                <ModelVersionForm
                  detail={detail}
                  capability={capabilities.find(
                    (c) => c.key === detail.model.capability,
                  )}
                  done={() => {
                    refresh();
                    close();
                  }}
                />
              )}
            </AdminDialog>
            <AdminDialog
              title="发布价格版本"
              description="价格按照生效时间应用，已冻结的报价保持原价格。"
            >
              {(close) => (
                <ModelPriceForm
                  detail={detail}
                  done={() => {
                    refresh();
                    close();
                  }}
                />
              )}
            </AdminDialog>
          </div>
        </CardContent>
      </Card>
      <ModelHistory detail={detail} />
    </div>
  );
}
export function SettingsModels() {
  const client = useQueryClient();
  const [selected, setSelected] = useState("");
  const models = useInfiniteQuery({
    queryKey: [...ADMIN_KEY, "models"],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ signal, pageParam }) => queryAdminModels(signal, pageParam),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
  });
  const providers = useInfiniteQuery({
    queryKey: [...ADMIN_KEY, "providers"],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ signal, pageParam }) => queryProviders(signal, pageParam),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
  });
  const capabilities = useQuery({
    queryKey: [...ADMIN_KEY, "capabilities"],
    queryFn: ({ signal }) => queryCapabilities(signal),
    retry: false,
  });
  if (models.isPending) return <p role="status">载入模型目录…</p>;
  if (models.isError)
    return (
      <AdminFailure error={models.error} retry={() => void models.refetch()} />
    );
  const items = models.data.pages.flatMap((page) => page.items);
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          模型关联渠道与能力，配置和价格分别保留版本。
        </p>
        <AdminDialog
          title="新建模型"
          description="新模型初始为禁用状态，发布配置与价格后可以启用。"
          disabled={!providers.data || !capabilities.data}
        >
          {(close) => (
            <ModelCreateForm
              providers={
                providers.data?.pages.flatMap((p) =>
                  p.items.map((item) => item.provider),
                ) ?? []
              }
              capabilities={capabilities.data ?? []}
              done={(model) => {
                setSelected(model.id);
                void client.invalidateQueries({ queryKey: ADMIN_KEY });
                close();
              }}
            />
          )}
        </AdminDialog>
      </div>
      {(providers.isError || capabilities.isError) && (
        <AdminFailure
          error={providers.error ?? capabilities.error}
          retry={() => {
            void providers.refetch();
            void capabilities.refetch();
          }}
        />
      )}
      {providers.hasNextPage && (
        <Button
          variant="ghost"
          disabled={providers.isFetchingNextPage}
          onClick={() => void providers.fetchNextPage()}
        >
          载入更多可选渠道
        </Button>
      )}
      <div className="grid min-w-0 items-start gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
        <Card>
          <CardHeader>
            <CardTitle>模型目录</CardTitle>
            <CardDescription>已载入 {items.length} 个模型</CardDescription>
          </CardHeader>
          <CardContent>
            {items.length ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>模型</TableHead>
                    <TableHead>能力</TableHead>
                    <TableHead>状态</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((model) => (
                    <TableRow
                      key={model.id}
                      data-state={
                        selected === model.id ? "selected" : undefined
                      }
                    >
                      <TableCell>
                        <Button
                          variant="link"
                          className="h-auto max-w-56 truncate p-0"
                          onClick={() => setSelected(model.id)}
                        >
                          {model.display_name}
                        </Button>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {model.model_key}
                        </p>
                      </TableCell>
                      <TableCell className="text-xs">
                        {model.capability}
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline">
                          {model.status === "active" ? "启用" : "禁用"}
                        </Badge>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <Empty>
                <EmptyHeader>
                  <EmptyTitle>尚无模型</EmptyTitle>
                  <EmptyDescription>
                    为渠道创建模型，发布生成参数与价格。
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            )}
            {models.hasNextPage && (
              <Button
                variant="outline"
                disabled={models.isFetchingNextPage}
                onClick={() => void models.fetchNextPage()}
              >
                加载更多模型
              </Button>
            )}
          </CardContent>
        </Card>
        {selected ? (
          <ModelDetails
            key={selected}
            id={selected}
            capabilities={capabilities.data ?? []}
          />
        ) : (
          <Empty>
            <EmptyHeader>
              <EmptyTitle>选择一个模型</EmptyTitle>
              <EmptyDescription>
                查看历史、发布下一版本或调整模型状态。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
      </div>
    </div>
  );
}
