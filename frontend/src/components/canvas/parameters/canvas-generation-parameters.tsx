"use client";
import {
  videoFieldValue,
  videoParameterFields,
} from "@/components/canvas/runtime/components/generation/lib/video-parameters";
import { useEffect, useId, useMemo } from "react";
import {
  imageRatios,
  imageSizeForRatio,
  readImageSize,
} from "../lib/canvas-image-size";
import useSWR from "swr";
import * as api from "@/components/canvas/runtime/api/generation-bff";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { accountScope } from "@/components/canvas/runtime/store/account-slice";
import { useErrorNotification } from "@/components/canvas/runtime/components/feedback/error-notification";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { ChevronDown } from "lucide-react";
import { seedanceReferenceHint } from "@/components/canvas/runtime/components/generation/lib/generation-failure";

/** Parameter choices reuse the existing generation service, with native typed values. */
export function CanvasGenerationParameters({
  type,
  parameters,
  onChange,
  hasReferences = false,
}: {
  type: string;
  parameters?: CanvasContract.CanvasNodeParametersDTO;
  onChange: (parameters: CanvasContract.CanvasNodeParametersDTO) => void;
  hasReferences?: boolean;
}) {
  const prefix = useId();
  const { account, ready, signingOut } = useAccount();
  const options = useSWR(
    ready && account && !signingOut && ["image", "video"].includes(type)
      ? [
          "canvas-generation-options",
          accountScope(account),
          type,
          parameters?.modelSelection?.connectionId,
          parameters?.modelSelection?.modelId,
        ]
      : null,
    async () => unwrap(await api.getGenerationOptions({ modality: type })),
    { shouldRetryOnError: false, revalidateOnFocus: false },
  );
  useErrorNotification(options.error, "生成参数读取失败");
  const catalog = useSWR(
    ready && account && !signingOut && ["image", "video"].includes(type)
      ? ["canvas-node-models", accountScope(account), type]
      : null,
    async () =>
      unwrap(
        type === "image"
          ? await api.listImageGenerationModels()
          : await api.listVideoGenerationModels(),
      ),
    { shouldRetryOnError: false, revalidateOnFocus: false },
  );
  useErrorNotification(catalog.error, "模型读取失败");
  const selectedModel = catalog.data?.find(
    (model) =>
      String(model.connectionId) ===
        String(parameters?.modelSelection?.connectionId) &&
      model.modelId === parameters?.modelSelection?.modelId,
  );
  const protocols = selectedModel?.nativeCapabilities?.protocols ?? [];
  const referenceHint = seedanceReferenceHint(
    selectedModel?.modelName ?? selectedModel?.providerModelId,
    type === "video" && hasReferences,
  );
  const openRouter = protocols.some((value) => value.startsWith("openrouter_"));
  const nativeFields = useMemo(
    () =>
      type === "video" && selectedModel
        ? videoParameterFields(
            options.data?.parameters ?? [],
            selectedModel.nativeCapabilities,
          )
        : type === "video"
          ? []
          : (options.data?.parameters ?? []),
    [type, selectedModel, options.data?.parameters],
  );
  const sizeField = nativeFields.find((field) => field.key === "size");
  const imageSize = readImageSize(
    String(
      parameters?.modelParameters?.size ?? sizeField?.defaultValue ?? "2K",
    ),
  );
  const sizes = (selectedModel?.imageSizes ?? sizeField?.values ?? []).filter(
    (value) => !["custom", "auto"].includes(value),
  );
  const layered = parameters?.modelParameters?.layer_decomposition === true;
  const fields: CanvasContract.Parameter[] =
    openRouter && type === "image"
      ? [
          ...(sizes.length
            ? [
                {
                  key: "size",
                  label: "图片尺寸",
                  values: ["default", ...sizes],
                  defaultValue: "default",
                },
              ]
            : []),
          {
            key: "aspect_ratio",
            label: "画面比例",
            values: [
              "default",
              ...(selectedModel?.nativeCapabilities?.ratios ?? []),
            ],
            defaultValue: "default",
          },
        ]
      : type === "image"
        ? [
            ...nativeFields.map((field) =>
              field.key === "size" ? { ...field, values: sizes } : field,
            ),
            ...(sizeField && !layered
              ? [
                  {
                    key: "imageRatio",
                    label: "画面比例",
                    values: ["auto", ...imageRatios],
                  },
                ]
              : []),
          ]
        : nativeFields;
  useEffect(() => {
    if (type !== "video" || !selectedModel || !options.data) return;
    const current = parameters?.modelParameters ?? {};
    const next = { ...current };
    let changed = false;
    for (const field of nativeFields) {
      if (!field.key) continue;
      const saved = current[field.key];
      if (
        saved != null &&
        saved !== "default" &&
        saved !== "-1" &&
        saved !== -1
      )
        continue;
      const value = videoFieldValue(field, saved ?? undefined);
      if (!value) continue;
      next[field.key] = field.numeric ? Number(value) : value;
      changed = true;
    }
    if (changed)
      onChange({
        ...parameters,
        modelParameterSchemaId: openRouter
          ? "openrouter-video@1"
          : "ark-video@1",
        modelParameters: next,
      });
  }, [
    type,
    selectedModel,
    options.data,
    nativeFields,
    parameters,
    openRouter,
    onChange,
  ]);
  const fieldValue = (field: CanvasContract.Parameter) =>
    type === "image" && field.key === "size" && !openRouter
      ? imageSize.size
      : field.key === "imageRatio"
        ? imageSize.ratio
        : type === "video"
          ? videoFieldValue(
              field,
              typeof parameters?.modelParameters?.[field.key!] === "number" ||
                typeof parameters?.modelParameters?.[field.key!] === "string"
                ? (parameters?.modelParameters?.[field.key!] as string | number)
                : undefined,
            )
          : String(
              parameters?.modelParameters?.[field.key!] ??
                field.defaultValue ??
                "",
            );
  const displayValue = (value: string) =>
    value === "default" ? "模型默认" : value === "auto" ? "自动" : value;
  function changeField(field: CanvasContract.Parameter, value: string) {
    let key = field.key!;
    if (value === "default") {
      const next = { ...parameters?.modelParameters };
      delete next[key];
      onChange({ ...parameters, modelParameters: next });
      return;
    }
    let next: string | number = field.numeric ? Number(value) : value;
    if (
      type === "image" &&
      !openRouter &&
      (key === "size" || key === "imageRatio") &&
      !layered
    ) {
      const size =
        key === "size"
          ? value
          : sizes.includes(imageSize.size)
            ? imageSize.size
            : String(sizeField?.defaultValue ?? sizes[0]);
      const ratio = key === "imageRatio" ? value : imageSize.ratio;
      const nativeSize = imageSizeForRatio(size, ratio);
      if (!nativeSize || !sizes.includes(size)) return;
      key = "size";
      next = nativeSize;
    }
    onChange({
      ...parameters,
      modelParameterSchemaId: openRouter
        ? `openrouter-${type}@1`
        : type === "image"
          ? "ark-image@1"
          : "ark-video@1",
      modelParameters: { ...parameters?.modelParameters, [key]: next },
    });
  }
  const summary = fields
    .map((field) => {
      const value = displayValue(fieldValue(field));
      return field.key === "duration"
        ? `${value} 秒`
        : field.key === "count"
          ? `${value} 张`
          : String(value);
    })
    .filter(Boolean)
    .join(" · ");
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          aria-label="生成参数"
          className="max-w-full min-w-0"
          title={summary || "生成参数"}
        >
          <span className="truncate">{summary || "生成参数"}</span>
          <ChevronDown data-icon="inline-end" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        side="top"
        align="start"
        aria-label="生成参数设置"
        className="canvas-generation-popover"
        data-canvas-no-zoom
        data-canvas-shortcuts-ignore
        onPointerDown={(event) => event.stopPropagation()}
        onMouseDown={(event) => event.stopPropagation()}
        onEscapeKeyDown={(event) => event.stopPropagation()}
      >
        {(!options.data || catalog.error) && (
          <Button
            variant="ghost"
            size="sm"
            disabled={options.isLoading || catalog.isLoading}
            onClick={() => {
              void options.mutate();
              void catalog.mutate();
            }}
          >
            {options.isLoading ? "正在加载参数…" : "重试"}
          </Button>
        )}
        {options.data && !fields.length && (
          <p className="text-sm text-muted-foreground">
            {type === "video" && !selectedModel
              ? catalog.error
                ? "模型能力读取失败，请重试"
                : catalog.isLoading
                  ? "正在读取模型能力…"
                  : "请先选择可用模型"
              : "暂无可调整参数"}
          </p>
        )}
        <FieldGroup className="gap-5">
          {fields.map((field) => (
            <Field key={field.key}>
              <FieldLabel id={`${prefix}-${field.key}`}>
                {field.label}
              </FieldLabel>
              <ToggleGroup
                type="single"
                value={fieldValue(field)}
                disabled={!selectedModel || !!catalog.error}
                aria-labelledby={`${prefix}-${field.key}`}
                className="canvas-parameter-options"
                data-ratio={/ratio/i.test(field.key ?? "")}
                onValueChange={(value) => {
                  if (value) changeField(field, value);
                }}
              >
                {fieldValue(field) &&
                  !field.values?.includes(fieldValue(field)) && (
                    <ToggleGroupItem value={fieldValue(field)} disabled>
                      {displayValue(fieldValue(field))}
                    </ToggleGroupItem>
                  )}
                {(field.values ?? []).map((value) => (
                  <ToggleGroupItem
                    key={value}
                    value={value}
                    disabled={
                      field.key === "imageRatio" &&
                      !imageSizeForRatio(imageSize.size, value)
                    }
                    aria-label={`${field.label} ${displayValue(value)}`}
                    className={
                      /ratio/i.test(field.key ?? "")
                        ? "canvas-ratio-option"
                        : undefined
                    }
                  >
                    {/ratio/i.test(field.key ?? "") && (
                      <RatioPreview value={value} />
                    )}
                    {displayValue(value)}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            </Field>
          ))}
        </FieldGroup>
        {referenceHint && (
          <p className="mt-3 text-xs leading-5 text-muted-foreground">
            {referenceHint}
          </p>
        )}
      </PopoverContent>
    </Popover>
  );
}

function RatioPreview({ value }: { value: string }) {
  const [width, height] = value.split(":").map(Number);
  const valid = width > 0 && height > 0;
  return (
    <span className="canvas-ratio-preview" aria-hidden="true">
      <span
        style={{
          width: valid ? (18 * width) / Math.max(width, height) : 18,
          height: valid ? (18 * height) / Math.max(width, height) : 18,
        }}
      />
    </span>
  );
}
