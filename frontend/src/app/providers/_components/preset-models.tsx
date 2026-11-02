"use client";

import { useEffect, useId, useState } from "react";
import { cn } from "cn";
import {
  getAdminModel,
  listAdminModels,
  listAdminProviders,
  listModelReleases,
} from "@/api/settings";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Field,
  FieldLabel,
  FieldSet,
  FieldLegend,
  FieldDescription,
} from "@/components/ui/field";
import {
  Item,
  ItemGroup,
  ItemContent,
  ItemDescription,
  ItemActions,
} from "@/components/ui/item";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { catalogMessage } from "@/components/catalog/mutation";
import { templateFromRelease } from "./preset-template";

export function PresetModels({
  adapter,
  templates,
  onChange,
}: {
  adapter: string;
  templates: API.PresetModelTemplate[];
  onChange: (templates: API.PresetModelTemplate[]) => void;
}) {
  const fieldID = useId();
  const [models, setModels] = useState<API.CreatedModel[]>([]);
  const [source, setSource] = useState("");
  const [detail, setDetail] = useState<API.AdminModelDetail | null>(null);
  const [releases, setReleases] = useState<API.ModelRelease[]>([]);
  const [releaseID, setReleaseID] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [creationMode, setCreationMode] = useState<"copy" | "manual">("copy");
  const [manualName, setManualName] = useState("");
  const [manualID, setManualID] = useState("");
  const [manualCap, setManualCap] = useState("text.generate");
  useEffect(() => {
    const controller = new AbortController();
    Promise.all([
      listAdminModels({ limit: 200 }, { signal: controller.signal }),
      listAdminProviders({ limit: 200 }, { signal: controller.signal }),
    ])
      .then(([m, p]) => {
        if (controller.signal.aborted) return;
        const providers = new Set(
          p.items
            ?.filter((p) => p.provider?.adapter_key === adapter)
            .map((p) => p.provider?.id),
        );
        setModels(m.items?.filter((m) => providers.has(m.provider_id)) ?? []);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(catalogMessage(e));
      });
    return () => controller.abort();
  }, [adapter]);
  useEffect(() => {
    if (!source) return;
    const controller = new AbortController();
    Promise.all([
      getAdminModel({ id: source }, { signal: controller.signal }),
      listModelReleases({ id: source }, { signal: controller.signal }),
    ])
      .then(([d, r]) => {
        if (!controller.signal.aborted) {
          setDetail(d);
          setReleases(
            r.items?.filter(
              (r) => r.status === "active" || r.status === "draft",
            ) ?? [],
          );
          setLoading(false);
        }
      })
      .catch((e) => {
        if (!controller.signal.aborted) {
          setError(catalogMessage(e));
          setLoading(false);
        }
      });
    return () => controller.abort();
  }, [source]);
  return (
    <FieldSet aria-label="预设模型模板">
      <FieldLegend variant="label">模型模板（可选）</FieldLegend>
      <FieldDescription>
        复制已有发布的参数、价格和成本，不复制 Key 或验证结果。
      </FieldDescription>
      <ItemGroup>
        {templates.map((t, i) => (
          <Item key={t.key} variant="outline">
            <ItemContent>
              <ItemDescription className="break-all">
                {t.name} · {t.capability} · {t.provider_model_id}
              </ItemDescription>
            </ItemContent>
            <ItemActions>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() =>
                  onChange(templates.filter((_, index) => index !== i))
                }
              >
                移除 {t.name}
              </Button>
            </ItemActions>
          </Item>
        ))}
      </ItemGroup>
      <div className="flex items-center gap-1.5 border-t pt-3">
        <button
          type="button"
          onClick={() => {
            setCreationMode("copy");
            setError("");
          }}
          className={cn(
            "rounded-md px-2.5 py-1 text-xs font-medium transition-colors",
            creationMode === "copy"
              ? "bg-primary text-primary-foreground"
              : "bg-muted text-muted-foreground hover:bg-muted/80",
          )}
        >
          从已有发布复制
        </button>
        <button
          type="button"
          onClick={() => {
            setCreationMode("manual");
            setError("");
          }}
          className={cn(
            "rounded-md px-2.5 py-1 text-xs font-medium transition-colors",
            creationMode === "manual"
              ? "bg-primary text-primary-foreground"
              : "bg-muted text-muted-foreground hover:bg-muted/80",
          )}
        >
          直接添加新模型
        </button>
      </div>

      {creationMode === "copy" ? (
        <>
          <Field>
            <FieldLabel htmlFor={`${fieldID}-source`}>来源模型</FieldLabel>
            <Select
              value={source}
              onValueChange={(value) => {
                setSource(value);
                setDetail(null);
                setReleases([]);
                setReleaseID("");
                setLoading(!!value);
                setError("");
              }}
            >
              <SelectTrigger id={`${fieldID}-source`} className="w-full">
                <SelectValue placeholder={"选择同平台已配置模型"} />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {models.map((m) =>
                    m.id ? (
                      <SelectItem key={m.id} value={m.id}>
                        {m.display_name}
                      </SelectItem>
                    ) : null,
                  )}
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          {source ? (
            <Field>
              <FieldLabel htmlFor={`${fieldID}-release`}>来源发布组合</FieldLabel>
              <Select
                value={releaseID}
                disabled={loading}
                onValueChange={(value) => setReleaseID(value)}
              >
                <SelectTrigger id={`${fieldID}-release`} className="w-full">
                  <SelectValue
                    placeholder={
                      loading ? "正在读取发布…" : "选择明确的模型与价格版本"
                    }
                  />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {releases.map((r) =>
                      r.id ? (
                        <SelectItem key={r.id} value={r.id}>
                          {r.status === "active" ? "已上线" : "待验证"} · {r.id}
                        </SelectItem>
                      ) : null,
                    )}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
          ) : null}
          {source && !loading && !releases.length ? (
            <p className="text-sm text-muted-foreground">
              此模型尚无发布组合，请先在模型配置中补齐参数、价格与上游成本。
            </p>
          ) : null}
          <Button
            type="button"
            variant="outline"
            disabled={!detail || !releaseID || templates.length >= 16}
            onClick={() => {
              try {
                const release = releases.find((r) => r.id === releaseID);
                if (!detail || !release) return;
                const template = templateFromRelease(detail, release);
                if (templates.some((t) => t.key === template.key))
                  throw new Error("该发布已加入模板。");
                onChange([...templates, template]);
                setError("");
              } catch (e) {
                setError(catalogMessage(e));
              }
            }}
          >
            添加模型模板
          </Button>
        </>
      ) : (
        <div className="flex flex-col gap-3 rounded-md border p-3 bg-muted/20">
          <Field>
            <FieldLabel htmlFor={`${fieldID}-manual-name`}>模型显示名称</FieldLabel>
            <Input
              id={`${fieldID}-manual-name`}
              placeholder="例如：Claude Sonnet 5.5"
              value={manualName}
              onChange={(e) => setManualName(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${fieldID}-manual-id`}>上游模型标识</FieldLabel>
            <Input
              id={`${fieldID}-manual-id`}
              placeholder="例如：anthropic/claude-sonnet-5.5"
              value={manualID}
              onChange={(e) => setManualID(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${fieldID}-manual-cap`}>能力类型</FieldLabel>
            <Select value={manualCap} onValueChange={(val) => setManualCap(val)}>
              <SelectTrigger id={`${fieldID}-manual-cap`} className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="text.generate">文本 (text.generate)</SelectItem>
                <SelectItem value="image.generate">图片 (image.generate)</SelectItem>
                <SelectItem value="video.generate">视频 (video.generate)</SelectItem>
                <SelectItem value="audio.generate">音频 (audio.generate)</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <Button
            type="button"
            variant="outline"
            disabled={!manualName.trim() || !manualID.trim() || templates.length >= 16}
            onClick={() => {
              try {
                const cleanKey = manualName
                  .toLowerCase()
                  .replace(/[^a-z0-9]/g, "-")
                  .replace(/^-+|-+$/g, "")
                  .slice(0, 16);
                const key = `m-${cleanKey || "tpl"}-${Date.now().toString(36).slice(-4)}`;
                if (templates.some((t) => t.provider_model_id === manualID.trim())) {
                  throw new Error("该上游模型标识已存在于模板中。");
                }
                const newTemplate: API.PresetModelTemplate = {
                  key,
                  name: manualName.trim(),
                  provider_model_id: manualID.trim(),
                  capability: manualCap,
                  modes:
                    manualCap === "image.generate"
                      ? ["text2image"]
                      : manualCap === "video.generate"
                        ? ["text2video"]
                        : ["text_to_text"],
                  limits: { max_outputs: 1 },
                  param_schema: [],
                  expected_max_ms: 120000,
                  price: {
                    unit: manualCap === "text.generate" ? "per_1k_tokens" : "per_request",
                    rule: { amount: 10 },
                    currency: "CNY",
                    fx_rate_to_cny: "1",
                  },
                  upstream_cost: {
                    currency: "USD",
                    max_micros: 20000,
                    source: "admin-managed",
                    reviewed_at: new Date().toISOString(),
                  },
                };
                onChange([...templates, newTemplate]);
                setManualName("");
                setManualID("");
                setError("");
              } catch (e) {
                setError(catalogMessage(e));
              }
            }}
          >
            确认添加模型模板
          </Button>
        </div>
      )}

      {error ? <FeedbackToast message={error} /> : null}
      {!templates.length ? (
        <p className="text-xs text-muted-foreground">
          无模板时仅保存连接配置。
        </p>
      ) : null}
    </FieldSet>
  );
}
