"use client";

import { useEffect, useId, useState } from "react";
import {
  getAdminModel,
  listAdminModels,
  listAdminProviders,
  listModelReleases,
} from "@/api/settings";
import { Button } from "@/components/ui/button";
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
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
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
      <Field>
        <FieldLabel htmlFor={`${fieldID}-source`}>来源模型</FieldLabel>
        <NativeSelect
          id={`${fieldID}-source`}
          value={source}
          onChange={(e) => {
            setSource(e.target.value);
            setDetail(null);
            setReleases([]);
            setReleaseID("");
            setLoading(!!e.target.value);
            setError("");
          }}
        >
          <NativeSelectOption value="">选择同平台已配置模型</NativeSelectOption>
          {models.map((m) => (
            <NativeSelectOption key={m.id} value={m.id}>
              {m.display_name}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </Field>
      {source ? (
        <Field>
          <FieldLabel htmlFor={`${fieldID}-release`}>来源发布组合</FieldLabel>
          <NativeSelect
            id={`${fieldID}-release`}
            value={releaseID}
            disabled={loading}
            onChange={(e) => setReleaseID(e.target.value)}
          >
            <NativeSelectOption value="">
              {loading ? "正在读取发布…" : "选择明确的模型与价格版本"}
            </NativeSelectOption>
            {releases.map((r) => (
              <NativeSelectOption key={r.id} value={r.id}>
                {r.status === "active" ? "已上线" : "待验证"} · {r.id}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </Field>
      ) : null}
      {source && !loading && !releases.length ? (
        <p className="text-sm text-muted-foreground">
          此模型尚无发布组合，请先在模型配置中补齐参数、价格与上游成本。
        </p>
      ) : null}
      {error ? <FeedbackToast message={error} /> : null}
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
      {!templates.length ? (
        <p className="text-xs text-muted-foreground">
          无模板时仅保存连接配置。
        </p>
      ) : null}
    </FieldSet>
  );
}
