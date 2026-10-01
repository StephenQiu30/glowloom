"use client";

import { useState } from "react";
import {
  Controller,
  useForm,
  type UseFormRegisterReturn,
} from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { z } from "zod";
import { createAdminProvider, updateAdminProvider } from "@/gen/api/settings";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { FieldGroup } from "@/components/ui/field";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
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
  providerSchema,
  queryProvider,
  queryProviders,
  type Provider,
} from "./admin-queries";
import { MODELS_KEY } from "./queries";
import {
  AdminDialog,
  AdminFailure,
  AdminField,
  AdminMessage,
  AdminSelect,
  formatAdminTime,
} from "./admin-ui";
import { SettingsCredentials } from "./settings-credentials";

const positive = z.number().int().min(1, "请输入大于零的整数").max(2147483647);
const limitsSchema = z.object({
  concurrency_limit: positive,
  rate_limit_per_min: positive,
  status: z.enum(["active", "disabled"]),
});
const createSchema = z.object({
  key: z
    .string()
    .regex(/^[a-z][a-z0-9_-]{0,63}$/, "使用小写字母开头的英文标识，最多 64 位"),
  name: z.string().trim().min(1, "请输入渠道名称").max(100),
  adapter_key: z.enum(["volcengine_ark", "minimax", "openrouter"]),
  region: z.enum(["domestic", "overseas"]),
  concurrency_limit: positive,
  rate_limit_per_min: positive,
});
function ProviderCreate({ done }: { done: (provider: Provider) => void }) {
  const form = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: {
      key: "",
      name: "",
      adapter_key: "volcengine_ark",
      region: "domestic",
      concurrency_limit: 1,
      rate_limit_per_min: 60,
    },
  });
  const mutation = useMutation({
    mutationFn: async (values: z.infer<typeof createSchema>) =>
      parseAdmin(
        providerSchema,
        await createAdminProvider(values, adminWriteOptions()),
      ),
    onSuccess: done,
    retry: false,
  });
  return (
    <form
      onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
      className="space-y-5"
    >
      <FieldGroup>
        <AdminField
          id="provider-key"
          label="渠道标识"
          error={form.formState.errors.key?.message}
        >
          <Input id="provider-key" {...form.register("key")} />
        </AdminField>
        <AdminField
          id="provider-name"
          label="渠道名称"
          error={form.formState.errors.name?.message}
        >
          <Input id="provider-name" {...form.register("name")} />
        </AdminField>
        <AdminField id="provider-adapter" label="适配器">
          <Controller
            name="adapter_key"
            control={form.control}
            render={({ field }) => (
              <AdminSelect
                id="provider-adapter"
                value={field.value}
                onChange={field.onChange}
                options={[
                  { value: "volcengine_ark", label: "火山引擎 Ark" },
                  { value: "minimax", label: "MiniMax" },
                  { value: "openrouter", label: "OpenRouter" },
                ]}
              />
            )}
          />
        </AdminField>
        <AdminField id="provider-region" label="处理地区">
          <Controller
            name="region"
            control={form.control}
            render={({ field }) => (
              <AdminSelect
                id="provider-region"
                value={field.value}
                onChange={field.onChange}
                options={[
                  { value: "domestic", label: "境内" },
                  { value: "overseas", label: "境外" },
                ]}
              />
            )}
          />
        </AdminField>
        <ProviderLimits form={form} />
      </FieldGroup>
      <AdminMessage
        message={mutation.isError ? adminError(mutation.error) : ""}
        error
      />
      <Button type="submit" disabled={mutation.isPending}>
        创建渠道
      </Button>
    </form>
  );
}
function ProviderLimits({
  form,
}: {
  form: {
    register: (
      name: "concurrency_limit" | "rate_limit_per_min",
      options: { valueAsNumber: true },
    ) => UseFormRegisterReturn;
    formState: {
      errors: {
        concurrency_limit?: { message?: string };
        rate_limit_per_min?: { message?: string };
      };
    };
  };
}) {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <AdminField
        id="provider-concurrency"
        label="最大并发数"
        error={form.formState.errors.concurrency_limit?.message}
      >
        <Input
          id="provider-concurrency"
          type="number"
          min={1}
          {...form.register("concurrency_limit", { valueAsNumber: true })}
        />
      </AdminField>
      <AdminField
        id="provider-rate"
        label="每分钟请求上限"
        error={form.formState.errors.rate_limit_per_min?.message}
      >
        <Input
          id="provider-rate"
          type="number"
          min={1}
          {...form.register("rate_limit_per_min", { valueAsNumber: true })}
        />
      </AdminField>
    </div>
  );
}
export function ProviderRevisionForm({
  provider,
  saved,
}: {
  provider: Provider;
  saved: () => void;
}) {
  const form = useForm<z.infer<typeof limitsSchema>>({
    resolver: zodResolver(limitsSchema),
    defaultValues: provider,
  });
  const mutation = useMutation({
    mutationFn: async (values: z.infer<typeof limitsSchema>) =>
      parseAdmin(
        providerSchema,
        await updateAdminProvider(
          { id: provider.id },
          { ...values, expected_revision: provider.revision },
          adminWriteOptions(),
        ),
      ),
    onSuccess: saved,
    retry: false,
  });
  return (
    <Card>
      <CardHeader>
        <CardTitle>{provider.name}</CardTitle>
        <CardDescription>
          {provider.key} · {provider.adapter_key} ·{" "}
          {provider.region === "domestic" ? "境内" : "境外"} · 修订{" "}
          {provider.revision}
          <br />
          更新于 {formatAdminTime(provider.update_time)}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
          className="space-y-5"
        >
          <FieldGroup>
            <ProviderLimits form={form} />
            <AdminField id="provider-status" label="渠道状态">
              <Controller
                name="status"
                control={form.control}
                render={({ field }) => (
                  <AdminSelect
                    id="provider-status"
                    value={field.value}
                    onChange={field.onChange}
                    options={[
                      { value: "active", label: "启用" },
                      { value: "disabled", label: "禁用（停止新提交）" },
                    ]}
                  />
                )}
              />
            </AdminField>
          </FieldGroup>
          <AdminMessage
            message={
              mutation.isError
                ? adminError(mutation.error)
                : mutation.isSuccess
                  ? "渠道配置已保存。"
                  : ""
            }
            error={mutation.isError}
          />
          <Button
            type="submit"
            disabled={mutation.isPending || mutation.isSuccess}
          >
            保存渠道配置
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
function ProviderDetails({ id }: { id: string }) {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: [...ADMIN_KEY, "provider", id],
    queryFn: ({ signal }) => queryProvider(id, signal),
    retry: false,
    refetchOnWindowFocus: false,
  });
  function refresh() {
    void client.invalidateQueries({ queryKey: ADMIN_KEY });
    void client.invalidateQueries({ queryKey: MODELS_KEY });
  }
  if (query.isPending) return <p role="status">载入渠道详情…</p>;
  if (query.isError)
    return (
      <AdminFailure error={query.error} retry={() => void query.refetch()} />
    );
  return (
    <div className="space-y-5">
      <Button
        variant="ghost"
        disabled={query.isFetching}
        onClick={() => void query.refetch()}
      >
        读取最新渠道配置
      </Button>
      <ProviderRevisionForm
        key={query.data.provider.revision}
        provider={query.data.provider}
        saved={refresh}
      />
      <SettingsCredentials
        key={`${id}-${query.data.credential?.id ?? "none"}`}
        detail={query.data}
        refresh={refresh}
      />
    </div>
  );
}
export function SettingsProviders() {
  const client = useQueryClient();
  const [selected, setSelected] = useState("");
  const query = useInfiniteQuery({
    queryKey: [...ADMIN_KEY, "providers"],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ signal, pageParam }) => queryProviders(signal, pageParam),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
  });
  if (query.isPending) return <p role="status">载入渠道目录…</p>;
  if (query.isError)
    return (
      <AdminFailure error={query.error} retry={() => void query.refetch()} />
    );
  const items = query.data.pages.flatMap((page) => page.items);
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          渠道决定模型的适配器、处理地区和提交额度。
        </p>
        <AdminDialog
          title="新建渠道"
          description="创建渠道后，使用适配器声明的字段配置凭据。渠道标识、适配器和地区在创建后固定。"
        >
          {(close) => (
            <ProviderCreate
              done={(provider) => {
                setSelected(provider.id);
                void client.invalidateQueries({ queryKey: ADMIN_KEY });
                close();
              }}
            />
          )}
        </AdminDialog>
      </div>
      <div className="grid min-w-0 items-start gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
        <Card>
          <CardHeader>
            <CardTitle>渠道目录</CardTitle>
            <CardDescription>已载入 {items.length} 个渠道</CardDescription>
          </CardHeader>
          <CardContent>
            {items.length ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>渠道</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>模型</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map(({ provider, model_count }) => (
                    <TableRow
                      key={provider.id}
                      data-state={
                        selected === provider.id ? "selected" : undefined
                      }
                    >
                      <TableCell>
                        <Button
                          variant="link"
                          className="h-auto max-w-56 truncate p-0"
                          onClick={() => setSelected(provider.id)}
                        >
                          {provider.name}
                        </Button>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {provider.adapter_key}
                        </p>
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline">
                          {provider.status === "active" ? "启用" : "禁用"}
                        </Badge>
                      </TableCell>
                      <TableCell>{model_count}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <Empty>
                <EmptyHeader>
                  <EmptyTitle>尚无渠道</EmptyTitle>
                  <EmptyDescription>
                    创建渠道后即可配置模型与凭据。
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            )}
            {query.hasNextPage && (
              <Button
                variant="outline"
                disabled={query.isFetchingNextPage}
                onClick={() => void query.fetchNextPage()}
              >
                加载更多渠道
              </Button>
            )}
          </CardContent>
        </Card>
        {selected ? (
          <ProviderDetails key={selected} id={selected} />
        ) : (
          <Empty>
            <EmptyHeader>
              <EmptyTitle>选择一个渠道</EmptyTitle>
              <EmptyDescription>
                查看详情、调整额度并管理当前凭据。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
      </div>
    </div>
  );
}
