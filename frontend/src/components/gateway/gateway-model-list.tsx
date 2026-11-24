"use client";

import { useState } from "react";
import { Eye, Route, Search, X } from "lucide-react";
import Link from "next/link";
import { ProviderLogo } from "@/components/providers/provider-logo";
import { Badge } from "@/components/ui/badge";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { ReactNode } from "react";
import { modelPreset, type GatewayConfiguration } from "@/lib/ai-gateway-admin";
import { GatewayAction } from "./gateway-action";
import { priceSummary } from "./gateway-fields";
import { protocolLabels } from "./gateway-forms";
import { GatewayTable } from "./gateway-table";

const categories = [
  ["all", "全部"],
  ["image", "图片"],
  ["video", "视频"],
  ["audio", "音频"],
  ["text", "文本"],
] as const;

const capabilityLabels: Record<string, string> = {
  "image.generate": "图片生成",
  "video.generate": "视频生成",
  "audio.generate": "音频生成",
  "audio.transcribe": "音频转写",
  "text.embed": "文本向量",
  "text.generate": "文本生成",
};

type RouteRow = GatewayConfiguration["routes"][number];

export function GatewayModelList({
  configuration,
  startRoute,
  toolbarActions,
}: {
  configuration: GatewayConfiguration;
  startRoute: (id: string) => void;
  toolbarActions: ReactNode;
}) {
  const [category, setCategory] = useState("all");
  const [draft, setDraft] = useState("");
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("all");

  const routes = new Map<string, RouteRow[]>();
  for (const row of configuration.routes) {
    const entries = routes.get(row.route.model_id);
    if (entries) entries.push(row);
    else routes.set(row.route.model_id, [row]);
  }
  const activeCredentials = new Set(
    configuration.credentials
      .filter(({ state }) => state === "active")
      .map(({ id }) => id),
  );
  const candidates = new Map<
    string,
    GatewayConfiguration["onboarding"][number]
  >();
  for (const candidate of configuration.onboarding) {
    if (
      candidate.provider_key &&
      (candidate.state === "prepared" ||
        candidate.state === "verifying" ||
        candidate.state === "failed") &&
      !candidates.has(candidate.model_id)
    ) {
      candidates.set(candidate.model_id, candidate);
    }
  }
  const matching = configuration.models.flatMap((row) => {
    const { model, enabled } = row;
    const modelRoutes = routes.get(model.id) ?? [];
    const capability = model.capability || "text.generate";
    const usableRoute = modelRoutes.find(
      ({ route, enabled }) =>
        enabled && activeCredentials.has(route.credential),
    );
    const available = enabled && Boolean(usableRoute);
    const primaryRoute =
      usableRoute ??
      modelRoutes.find((route) => route.enabled) ??
      modelRoutes[0];
    const candidate = row.draft ? candidates.get(model.id) : undefined;
    const upstream =
      primaryRoute?.route.upstream_model ?? candidate?.upstream_model;
    const adapter = primaryRoute?.provider_key ?? candidate?.provider_key;
    const preset = modelPreset(
      configuration.model_presets,
      model,
      adapter,
      upstream,
    );
    const providerKey = primaryRoute
      ? primaryRoute.provider_key
      : (candidate?.provider_key ?? preset?.adapter);
    const displayName =
      preset && model.name === preset.upstream ? preset.name : model.name;
    if (category !== "all" && !capability.startsWith(`${category}.`)) return [];
    if (status !== "all" && available !== (status === "available")) return [];
    if (
      query &&
      ![
        model.name,
        model.id,
        displayName,
        upstream ?? preset?.upstream ?? "",
        preset?.provider ?? "",
        ...modelRoutes.flatMap(({ route }) => [
          route.upstream_model,
          route.group,
          route.base_url,
        ]),
      ].some((term) => term.toLowerCase().includes(query))
    )
      return [];
    return [
      {
        row,
        modelRoutes,
        capability,
        available,
        providerKey,
        displayName,
        upstream: upstream ?? preset?.upstream,
      },
    ];
  });
  const channelCount = matching.reduce(
    (count, { modelRoutes }) => count + modelRoutes.length,
    0,
  );
  const filtered = category !== "all" || status !== "all" || Boolean(query);

  return (
    <section aria-label="模型目录" className="flex min-w-0 flex-col gap-2">
      <Tabs
        value={category}
        onValueChange={setCategory}
        className="min-w-0 gap-2"
      >
        <TabsList variant="secondary" aria-label="模型能力分类">
          {categories.map(([key, label]) => (
            <TabsTrigger key={key} value={key}>
              {label}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value={category} className="flex min-w-0 flex-col gap-2">
          <GatewayTable
            toolbarActions={toolbarActions}
            filters={
              <>
                <form
                  role="search"
                  aria-label="搜索模型目录"
                  className="w-full sm:w-72"
                  onSubmit={(event) => {
                    event.preventDefault();
                    setQuery(draft.trim().toLowerCase());
                  }}
                >
                  <InputGroup className="max-md:min-h-11">
                    <InputGroupInput
                      type="text"
                      aria-label="搜索模型、上游型号或渠道"
                      placeholder="搜索模型、上游型号或渠道"
                      value={draft}
                      onChange={(event) => setDraft(event.target.value)}
                    />
                    <InputGroupAddon
                      align="inline-end"
                      className="py-0 has-[>button]:mr-0"
                    >
                      {draft && (
                        <GatewayAction
                          icon={X}
                          label="清空模型搜索"
                          className="size-7 max-md:size-11"
                          onClick={() => {
                            setDraft("");
                            setQuery("");
                          }}
                        />
                      )}
                      <GatewayAction
                        icon={Search}
                        className="size-7 max-md:size-11"
                        label="搜索模型"
                        type="submit"
                      />
                    </InputGroupAddon>
                  </InputGroup>
                </form>
                <Select value={status} onValueChange={setStatus}>
                  <SelectTrigger
                    aria-label="模型可用状态"
                    className="max-md:min-h-11"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="all">全部状态</SelectItem>
                      <SelectItem value="available">可用</SelectItem>
                      <SelectItem value="unavailable">不可用</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </>
            }
            summary={
              <p
                role="status"
                className="text-sm text-muted-foreground tabular-nums"
              >
                {matching.length} 个模型 · {channelCount} 条渠道
              </p>
            }
            label="销售模型"
            columns={["模型", "上游型号", "能力", "销售价", "状态", "操作"]}
            empty={filtered ? "没有符合条件的模型" : "暂无模型"}
            externalFilterKey={JSON.stringify([category, query, status])}
            rows={matching.map(
              ({
                row,
                modelRoutes,
                capability,
                available,
                providerKey,
                displayName,
                upstream,
              }) => {
                const { model, enabled, draft } = row;
                return {
                  id: model.id,
                  label: displayName,
                  cells: [
                    <span
                      key="name"
                      className="flex items-center gap-2.5 font-medium"
                    >
                      <ProviderLogo
                        adapterKey={providerKey}
                        iconOnly
                        className="size-5"
                      />
                      <span>{displayName}</span>
                    </span>,
                    modelRoutes.length ? (
                      <div key="routes" className="flex flex-col gap-1">
                        {modelRoutes.map(({ route }, index) => (
                          <div
                            key={route.id ?? index}
                            className="flex flex-col gap-0.5"
                          >
                            {route.id ? (
                              <Link
                                href={`/providers/${route.id}`}
                                className="wrap-anywhere text-primary underline-offset-4 hover:underline focus-visible:underline"
                              >
                                {route.upstream_model}
                              </Link>
                            ) : (
                              <span>{route.upstream_model}</span>
                            )}
                            {route.group !== route.upstream_model && (
                              <span className="text-xs text-muted-foreground">
                                {route.group}
                              </span>
                            )}
                          </div>
                        ))}
                      </div>
                    ) : (
                      (upstream ?? "—")
                    ),
                    capabilityLabels[capability] ??
                      protocolLabels[model.protocol] ??
                      model.protocol,
                    priceSummary(model.price, model.protocol),
                    <Badge
                      key="status"
                      variant={available ? "secondary" : "outline"}
                    >
                      {available ? "可用" : "不可用"}
                    </Badge>,
                    <>
                      <GatewayAction
                        label="查看版本"
                        icon={Eye}
                        href={`/models/${model.id}`}
                      />
                      {(enabled || draft) && (
                        <GatewayAction
                          onClick={() => startRoute(model.id)}
                          label="准备采购"
                          icon={Route}
                        />
                      )}
                    </>,
                  ],
                };
              },
            )}
          />
        </TabsContent>
      </Tabs>
    </section>
  );
}
