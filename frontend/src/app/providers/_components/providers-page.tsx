"use client";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty";

import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { LoadingState } from "@/components/layout/loading-state";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";

import { useEffect, useState, useId } from "react";
import Link from "next/link";
import { useSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import { DownloadButton } from "@/components/media/download-button";
import { Input } from "@/components/ui/input";
import {
  connectProviderPreset,
  exportProviderPreset,
  listAdminProviders,
  listProviderPresets,
  updateProviderPresetState,
} from "@/api/settings";
import {
  catalogMessage,
  MutationStatus,
  useCatalogMutation,
} from "@/components/catalog/mutation";
import { PresetEditor } from "./preset-editor";
import { ConnectionDetail } from "./connection-detail";

export function ProvidersPage() {
  const formFieldId = useId();

  const session = useSession();
  const [tab, setTab] = useState<"connections" | "presets">("connections");
  const [providers, setProviders] = useState<API.ProviderListItem[]>([]);
  const [presets, setPresets] = useState<API.Preset[]>([]);
  const [selected, setSelected] = useState("");
  const [refresh, setRefresh] = useState(0);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [editor, setEditor] = useState<API.Preset | "new" | null>(null);
  const [presetID, setPresetID] = useState("");
  const mutation = useCatalogMutation(() => setRefresh((value) => value + 1));
  useEffect(() => {
    const controller = new AbortController();
    Promise.all([
      listAdminProviders({ limit: 200 }, { signal: controller.signal }),
      listProviderPresets({ signal: controller.signal }),
    ])
      .then(([connections, registry]) => {
        if (!controller.signal.aborted) {
          setProviders(connections.items ?? []);
          setPresets(registry.items ?? []);
          setError("");
          setLoading(false);
        }
      })
      .catch((error) => {
        if (!controller.signal.aborted) {
          setError(catalogMessage(error));
          setLoading(false);
        }
      });
    return () => controller.abort();
  }, [refresh]);
  if (session.actor.platform_role !== "platform_admin")
    return (
      <Empty>
        <EmptyHeader>
          <EmptyTitle>暂无访问权限</EmptyTitle>
          <EmptyDescription>此页面仅向平台管理员开放。</EmptyDescription>
        </EmptyHeader>
      </Empty>
    );
  const preset = presets.find((item) => item.id === presetID);
  return (
    <div className="mx-auto w-full max-w-6xl space-y-6 p-4 sm:p-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">AI 平台接入</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            从预设创建连接，填写平台 Key，再验证并发布模型。
          </p>
        </div>
        <Button variant="outline" asChild>
          <Link href="/models">模型配置</Link>
        </Button>
      </header>
      <div className="flex gap-2" role="group" aria-label="平台管理视图">
        {(
          [
            ["connections", "连接"],
            ["presets", "预设管理"],
          ] as const
        ).map(([id, label]) => (
          <Button
            key={id}
            aria-pressed={tab === id}
            variant={tab === id ? "default" : "outline"}
            onClick={() => setTab(id)}
          >
            {label}
          </Button>
        ))}
      </div>
      <MutationStatus mutation={mutation} />
      {error ? (
        <FeedbackToast
          message={error}
          action={{
            label: "重新加载",
            onClick: () => {
              setError("");
              setRefresh((v) => v + 1);
            },
          }}
        />
      ) : null}
      {loading ? <LoadingState>正在加载平台配置…</LoadingState> : null}
      {tab === "connections" ? (
        <>
          <form
            onSubmit={async (event) => {
              event.preventDefault();
              const form = new FormData(event.currentTarget);
              if (!preset?.id || !preset.version_id) return;
              const result = await mutation.run(
                "preset.connect",
                preset.id,
                (id) =>
                  connectProviderPreset(
                    { id: preset.id! },
                    {
                      request_id: id,
                      version_id: preset.version_id,
                      key: String(form.get("key")),
                      name: String(form.get("name")),
                    },
                  ),
              );
              if (result?.provider_id) setSelected(result.provider_id);
            }}
          >
            <FieldGroup className="grid gap-3 rounded-xl border p-4 sm:grid-cols-2">
              <h2 className="font-medium sm:col-span-2">添加连接</h2>
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-1`}>
                  <span>平台预设</span>
                </FieldLabel>
                <NativeSelect
                  id={`${formFieldId}-field-1`}
                  required
                  value={presetID}
                  onChange={(e) => setPresetID(e.target.value)}
                  className="h-10 w-full"
                >
                  <NativeSelectOption value="">选择平台</NativeSelectOption>
                  {presets
                    .filter((p) => !p.archived && !p.emergency_disabled)
                    .map((p) => (
                      <NativeSelectOption key={p.id} value={p.id}>
                        {p.document?.name} · v{p.version}
                      </NativeSelectOption>
                    ))}
                </NativeSelect>
              </Field>
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-2`}>
                  <span>连接名称</span>
                </FieldLabel>
                <Input
                  id={`${formFieldId}-field-2`}
                  name="name"
                  required
                  maxLength={100}
                  placeholder="例如：主生产连接"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-3`}>
                  <span>连接标识</span>
                </FieldLabel>
                <Input
                  id={`${formFieldId}-field-3`}
                  name="key"
                  required
                  pattern="[a-z][a-z0-9_-]{0,63}"
                  placeholder="例如：ark-primary"
                />
              </Field>
              <div className="flex items-end">
                <Button disabled={mutation.busy || !preset}>创建连接</Button>
              </div>
              {preset ? (
                <p className="text-xs break-all text-muted-foreground sm:col-span-2">
                  {preset.document?.config?.base_url} ·
                  创建后处于禁用状态，验证发布后才接受新任务。
                </p>
              ) : null}
            </FieldGroup>
          </form>
          <div className="grid gap-4 lg:grid-cols-[minmax(220px,1fr)_minmax(0,2fr)]">
            <div className="space-y-2" role="region" aria-label="连接列表">
              {providers.length === 0 && !loading ? (
                <p className="rounded-xl border border-dashed p-6 text-sm text-muted-foreground">
                  还没有连接。选择内置预设开始配置。
                </p>
              ) : null}
              {providers.map(({ provider, credential, model_count }) => (
                <Button
                  variant={selected === provider?.id ? "secondary" : "outline"}
                  type="button"
                  key={provider?.id}
                  onClick={() => setSelected(provider?.id ?? "")}
                  aria-pressed={selected === provider?.id}
                  className="h-auto w-full flex-col items-start gap-1 p-4 text-left whitespace-normal"
                >
                  <span className="block font-medium">{provider?.name}</span>
                  <span className="mt-1 block text-xs text-muted-foreground">
                    {provider?.adapter_key} ·{" "}
                    {provider?.status === "active" ? "已启用" : "已禁用"} ·{" "}
                    {model_count ?? 0} 个模型
                  </span>
                  <span className="mt-2 block text-xs">
                    {credential?.last4
                      ? `Key ••••${credential.last4}`
                      : "未配置 Key"}
                  </span>
                </Button>
              ))}
            </div>
            {selected ? (
              <ConnectionDetail
                key={`${selected}:${refresh}`}
                id={selected}
                presets={presets}
                onSaved={() => setRefresh((v) => v + 1)}
              />
            ) : (
              <p className="rounded-xl border p-6 text-sm text-muted-foreground">
                选择连接查看 Key、检查结果及配置版本。
              </p>
            )}
          </div>
        </>
      ) : (
        <>
          <div className="flex justify-end">
            <Button onClick={() => setEditor("new")}>导入自定义预设</Button>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            {presets.map((p) => (
              <article key={p.id} className="space-y-3 rounded-xl border p-4">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <h2 className="font-medium">{p.document?.name}</h2>
                  <span className="text-xs text-muted-foreground">
                    {p.origin === "builtin" ? "内置" : "自定义"} · v{p.version}
                  </span>
                </div>
                <p className="text-xs break-all text-muted-foreground">
                  {p.document?.config?.base_url}
                </p>
                <p className="text-sm">
                  {p.emergency_disabled
                    ? "紧急停用"
                    : p.archived
                      ? "已归档"
                      : "可用于新连接"}
                </p>
                <div className="flex flex-wrap gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => setEditor(p)}
                  >
                    {p.origin === "builtin" ? "复制为自定义" : "编辑新版本"}
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
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={mutation.busy}
                    onClick={() =>
                      mutation.run("preset.state", p.id!, (id) =>
                        updateProviderPresetState(
                          { id: p.id! },
                          {
                            request_id: id,
                            expected_revision: p.revision,
                            archived: !p.archived,
                            emergency_disabled: p.emergency_disabled,
                          },
                        ),
                      )
                    }
                  >
                    {p.archived ? "恢复预设" : "归档"}
                  </Button>
                  <Button
                    size="sm"
                    variant={p.emergency_disabled ? "outline" : "destructive"}
                    disabled={mutation.busy}
                    onClick={() =>
                      mutation.run("preset.state", p.id!, (id) =>
                        updateProviderPresetState(
                          { id: p.id! },
                          {
                            request_id: id,
                            expected_revision: p.revision,
                            archived: p.archived,
                            emergency_disabled: !p.emergency_disabled,
                          },
                        ),
                      )
                    }
                  >
                    {p.emergency_disabled ? "解除紧急停用" : "停止新任务"}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  归档只停止新连接使用此预设；紧急停用停止相关新任务。
                </p>
              </article>
            ))}
          </div>
          {editor ? (
            <PresetEditor
              key={editor === "new" ? "new" : editor.id}
              preset={editor === "new" ? undefined : editor}
              onClose={() => setEditor(null)}
              onSaved={() => {
                setEditor(null);
                setRefresh((v) => v + 1);
              }}
            />
          ) : null}
        </>
      )}
    </div>
  );
}
