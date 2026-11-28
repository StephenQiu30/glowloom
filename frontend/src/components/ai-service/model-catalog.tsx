"use client";

import { useState, type ReactNode } from "react";
import { Eye } from "lucide-react";
import { ProviderLogo } from "@/components/providers/provider-logo";
import type { AiServiceConfiguration } from "@/lib/ai-service-admin";
import { modelPreset } from "@/lib/ai-service-admin";
import { AiServiceAction } from "./admin-action";
import { SelectField, priceSummary } from "./admin-fields";
import { AiServiceTable } from "./admin-table";

const categories = [
  ["all", "全部能力"],
  ["image", "图片"],
  ["video", "视频"],
  ["audio", "音频"],
  ["text", "文本"],
] as const;

const capabilityLabels: Record<string, string> = {
  "image.generate": "图片生成",
  "video.generate": "视频生成",
  "audio.generate": "音频生成",
  "text.embed": "文本向量",
  "text.generate": "文本生成",
};

export function ModelCatalog({
  configuration,
  toolbarActions,
}: {
  configuration: AiServiceConfiguration;
  toolbarActions: ReactNode;
}) {
  const [category, setCategory] = useState("all");
  const credentials = new Set(
    configuration.credentials
      .filter(({ state }) => state === "active")
      .map(({ id }) => id),
  );
  const channels = new Map<string, AiServiceConfiguration["routes"]>();
  for (const row of configuration.routes) {
    if (!row.enabled || !credentials.has(row.route.credential)) continue;
    const existing = channels.get(row.route.model_id);
    if (existing) existing.push(row);
    else channels.set(row.route.model_id, [row]);
  }

  return (
    <AiServiceTable
      toolbarActions={toolbarActions}
      label="可用模型"
      columns={["模型", "能力", "销售价", "可用渠道", "操作"]}
      empty="暂无可用模型"
      externalFilterKey={category}
      filters={
        <SelectField
          label="模型能力"
          options={categories}
          value={category}
          onChange={(event) => setCategory(event.target.value)}
        />
      }
      rows={configuration.models.flatMap(({ model, enabled }) => {
        const usable = channels.get(model.id);
        const capability = model.capability ?? "text.generate";
        if (
          !enabled ||
          !usable?.length ||
          (category !== "all" && !capability.startsWith(`${category}.`))
        )
          return [];
        const { provider_key, route } = usable[0];
        const preset = modelPreset(
          configuration.model_presets,
          model,
          provider_key,
          route.upstream_model,
        );
        const name =
          preset && model.name === preset.upstream ? preset.name : model.name;
        return [
          {
            id: model.id,
            label: name,
            terms: [
              model.name,
              ...usable.map(({ route }) => route.upstream_model),
            ],
            cells: [
              <span key="name" className="flex items-center gap-2 font-medium">
                <ProviderLogo adapterKey={provider_key} iconOnly />
                {name}
              </span>,
              capabilityLabels[capability] ?? capability,
              priceSummary(model.price, model.protocol),
              usable.length,
              <AiServiceAction
                key="detail"
                label="配置售价"
                icon={Eye}
                href={`/models/${model.id}`}
              />,
            ],
          },
        ];
      })}
    />
  );
}
