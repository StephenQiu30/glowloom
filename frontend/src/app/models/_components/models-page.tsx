"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
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
  catalogMessage,
  MutationStatus,
  useCatalogMutation,
  zeroID,
} from "@/components/catalog/mutation";
import { ModelDetail } from "./model-detail";

export function ModelsPage() {
  const session = useSession();
  const [models, setModels] = useState<API.CreatedModel[]>([]);
  const [providers, setProviders] = useState<API.ProviderListItem[]>([]);
  const [capabilities, setCapabilities] = useState<API.CapabilitySummary[]>([]);
  const [providerID, setProviderID] = useState("");
  const [selected, setSelected] = useState("");
  const [cursor, setCursor] = useState("");
  const [next, setNext] = useState("");
  const [refresh, setRefresh] = useState(0);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const saved = () => setRefresh((v) => v + 1);
  const mutation = useCatalogMutation(saved, "models");
  useEffect(() => {
    const abort = new AbortController();
    Promise.all([
      listAdminModels(
        { limit: 50, cursor: cursor || undefined },
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
          setError(catalogMessage(e));
          setLoading(false);
        }
      });
    return () => abort.abort();
  }, [refresh, cursor]);
  const provider = providers.find(
    (p) => p.provider?.id === providerID,
  )?.provider;
  const supported = capabilities.filter(
    (c) =>
      c.key === "video.generate" ||
      (provider?.adapter_key === "openrouter" &&
        ["image.generate", "text.generate"].includes(c.key ?? "")),
  );
  if (session.actor.platform_role !== "platform_admin")
    return <p role="alert">此页面仅向平台管理员开放。</p>;
  return (
    <div className="mx-auto w-full max-w-6xl space-y-6 p-4 sm:p-6">
      <header className="flex flex-wrap justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">模型管理</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            配置模型和计费规则，完成验证后发布。保存草稿不会改变线上版本。
          </p>
        </div>
        <Button asChild variant="outline">
          <Link href="/providers">管理平台连接</Link>
        </Button>
      </header>
      {error ? (
        <p role="alert" className="text-destructive">
          {error}
        </p>
      ) : null}
      <form
        className="grid gap-3 rounded-xl border p-4 sm:grid-cols-2"
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
          if (result?.id) setSelected(result.id);
        }}
      >
        <h2 className="font-medium sm:col-span-2">添加模型</h2>
        <label className="space-y-1 text-sm">
          平台连接
          <select
            required
            className="h-10 w-full rounded-md border bg-background px-3"
            value={providerID}
            onChange={(e) => setProviderID(e.target.value)}
          >
            <option value="">选择连接</option>
            {providers
              .filter((p) =>
                ["volcengine_ark", "openrouter"].includes(
                  p.provider?.adapter_key ?? "",
                ),
              )
              .map((p) => (
                <option key={p.provider?.id} value={p.provider?.id}>
                  {p.provider?.name}
                </option>
              ))}
          </select>
        </label>
        <label className="space-y-1 text-sm">
          生成能力
          <select
            key={providerID}
            name="capability"
            required
            className="h-10 w-full rounded-md border bg-background px-3"
            defaultValue=""
          >
            <option value="">选择能力</option>
            {supported.map((c) => (
              <option key={c.key} value={c.key}>
                {c.key === "video.generate"
                  ? "视频生成"
                  : c.key === "image.generate"
                    ? "图片生成"
                    : "文本生成"}
              </option>
            ))}
          </select>
        </label>
        <label className="space-y-1 text-sm">
          显示名称
          <Input name="name" required maxLength={100} />
        </label>
        <label className="space-y-1 text-sm">
          稳定标识
          <Input
            name="key"
            required
            pattern="[a-z][a-z0-9._-]*"
            maxLength={128}
            placeholder="例如 creative-video"
          />
        </label>
        <Button className="w-fit" disabled={mutation.busy || !providerID}>
          创建模型草稿
        </Button>
      </form>
      <MutationStatus mutation={mutation} />
      {loading ? <p role="status">正在加载模型…</p> : null}
      <div className="grid items-start gap-4 lg:grid-cols-[16rem_1fr]">
        <section className="min-w-0 space-y-2" aria-label="模型列表">
          {!loading && !models.length ? (
            <p className="text-sm text-muted-foreground">
              还没有模型，请先添加一个。
            </p>
          ) : null}
          {models.map((m) => (
            <Button
              key={m.id}
              variant={selected === m.id ? "secondary" : "outline"}
              className="h-auto w-full flex-col items-start gap-1 p-3 text-left whitespace-normal"
              onClick={() => setSelected(m.id ?? "")}
            >
              <span className="break-all">{m.display_name}</span>
              <span className="text-xs text-muted-foreground">
                {m.capability} ·{" "}
                {m.status === "active" ? "已启用" : "草稿 / 已停用"}
              </span>
            </Button>
          ))}
          <div className="flex flex-wrap gap-2">
            {cursor ? (
              <Button variant="outline" onClick={() => setCursor("")}>
                回到首页
              </Button>
            ) : null}
            {next ? (
              <Button variant="outline" onClick={() => setCursor(next)}>
                下一页
              </Button>
            ) : null}
          </div>
        </section>
        {selected ? (
          <ModelDetail
            key={`${selected}:${refresh}`}
            id={selected}
            capabilities={capabilities}
            onSaved={saved}
          />
        ) : (
          <p className="rounded-xl border p-6 text-sm text-muted-foreground">
            选择一个模型，配置上游模型 ID、参数、价格和发布版本。
          </p>
        )}
      </div>
    </div>
  );
}
