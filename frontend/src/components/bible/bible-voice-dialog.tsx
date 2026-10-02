"use client";
import { useId, useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { BibleWriteDialog } from "./bible-entry-dialog";
import { BibleAssetSelect } from "./bible-reference-picker";
import {
  bibleScopeKey,
  getBibleVoiceModel,
  getBibleVoices,
} from "./bible-queries";
import {
  voiceInputSchema,
  voiceParamsSchema,
  type BibleIdentity,
  type BibleVoiceChoice,
  type voiceContentSchema,
} from "./bible-model";
import type { Model } from "@/components/catalog/queries";
import type { BibleWriter } from "./use-bible-writer";
import type { z } from "zod";

const supported = ["speed", "pitch", "volume", "emotion", "language"] as const;
export function declaredVoiceParams(
  model: Model,
  entries: Record<string, string>,
) {
  if (!model.current_version) throw new Error("当前声音模型版本不可读取。");
  const result: Record<string, string | number> = {};
  for (const [field, entry] of Object.entries(entries)) {
    if (entry === "") continue;
    const parameter = model.current_version.param_schema.find(
      (item) => item.field === field,
    );
    if (!parameter || !supported.some((name) => name === field))
      throw new Error("原参数已不在当前模型的安全声明中，请核对后明确删除。");
    const value =
      field === "emotion" || field === "language" ? entry : Number(entry);
    if (
      typeof value === "number"
        ? !Number.isFinite(value) ||
          (parameter.type !== "integer" && parameter.type !== "number") ||
          (parameter.type === "integer" && !Number.isInteger(value)) ||
          (parameter.min !== undefined && value < parameter.min) ||
          (parameter.max !== undefined && value > parameter.max) ||
          (parameter.step !== undefined &&
            Math.abs(
              (value - (parameter.min ?? 0)) / parameter.step -
                Math.round((value - (parameter.min ?? 0)) / parameter.step),
            ) > 1e-8)
        : parameter.type !== "string" || !value.trim()
    )
      throw new Error("声音参数不符合当前模型的类型或范围。");
    if (parameter.enum && !parameter.enum.includes(value))
      throw new Error("声音参数必须来自当前模型的正式枚举。");
    result[field] = value;
  }
  const { modes, param_schema: schema } = model.current_version;
  if (
    !modes.some((mode) =>
      schema.every((parameter) => {
        const active =
          !parameter.for_modes?.length || parameter.for_modes.includes(mode);
        if (parameter.field === "voice_id") return active;
        if (Object.hasOwn(result, parameter.field)) return active;
        return !(
          active &&
          parameter.required &&
          parameter.default === undefined &&
          supported.some((field) => field === parameter.field)
        );
      }),
    )
  )
    throw new Error(
      "当前参数没有兼容的正式模型模式，或缺少没有默认值的必填参数。",
    );
  return voiceParamsSchema.parse(result);
}
function choiceKey(choice: BibleVoiceChoice) {
  return JSON.stringify([
    choice.model_key,
    choice.model_version,
    choice.voice_key,
  ]);
}
export function BibleVoiceDialog({
  identity,
  id: entryId,
  revision: initialRevision,
  current,
  writer,
  onClose,
  onCloseAutoFocus,
}: {
  identity: BibleIdentity;
  id: string;
  revision: number;
  current?: z.infer<typeof voiceContentSchema>;
  writer: BibleWriter;
  onClose: () => void;
  onCloseAutoFocus?: (event: Event) => void;
}) {
  const id = useId(),
    [revision, setRevision] = useState(initialRevision),
    [dirty, setDirty] = useState(false),
    [kind, setKind] = useState<"sample" | "catalog">(current?.kind ?? "sample"),
    [sampleId, setSampleId] = useState(
      current?.kind === "sample" ? current.sample.media.asset_id : "",
    ),
    [sampleName, setSampleName] = useState(
      current?.kind === "sample" ? current.sample.name : "",
    ),
    [instructions, setInstructions] = useState(current?.instructions ?? ""),
    [selected, setSelected] = useState(
      current?.kind === "catalog"
        ? JSON.stringify([
            current.catalog.model_key,
            current.catalog.model_version,
            current.catalog.voice_key,
          ])
        : "",
    ),
    [entries, setEntries] = useState<Record<string, string>>(() =>
      current?.kind === "catalog"
        ? Object.fromEntries(
            Object.entries(current.catalog.params).map(([key, value]) => [
              key,
              String(value),
            ]),
          )
        : {},
    ),
    [error, setError] = useState<string>();
  const voices = useInfiniteQuery({
    queryKey: [...bibleScopeKey(identity), "voices"],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ signal, pageParam }) =>
      getBibleVoices(identity, pageParam, signal),
    getNextPageParam: (page) => page.next_cursor,
    enabled: kind === "catalog",
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
  const choices = voices.data?.pages.flatMap((page) => page.voices) ?? [],
    choice = choices.find((item) => choiceKey(item) === selected);
  const model = useQuery({
    queryKey: [...bibleScopeKey(identity), "voice-model", selected],
    queryFn: ({ signal }) => getBibleVoiceModel(identity, choice!, signal),
    enabled: Boolean(choice) && kind === "catalog",
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
  function changed() {
    setDirty(true);
    setError(undefined);
  }
  async function prepare() {
    const detail = writer.latest?.detail;
    if (
      detail?.head.id === entryId &&
      !detail.head.deleted &&
      !detail.head.redirect_id &&
      (await writer.discardRejected())
    )
      setRevision(detail.head.revision);
  }
  const parameters =
    model.data?.current_version?.param_schema.filter((parameter) =>
      supported.some((name) => name === parameter.field),
    ) ?? [];
  return (
    <BibleWriteDialog
      title="绑定角色声音"
      writer={writer}
      dirty={dirty}
      onClose={onClose}
      onCloseAutoFocus={onCloseAutoFocus}
      onPrepareLatest={prepare}
    >
      <p className="text-xs break-all">
        角色 {entryId} · 冻结版本 {revision}
      </p>
      <fieldset disabled={writer.locked} className="space-y-4">
        <div>
          <Label htmlFor={`${id}-kind`}>声音来源</Label>
          <select
            id={`${id}-kind`}
            className="h-10 w-full rounded-md bg-muted px-2"
            value={kind}
            onChange={(event) => {
              setKind(event.target.value === "catalog" ? "catalog" : "sample");
              changed();
            }}
          >
            <option value="sample">正式项目音频样本</option>
            <option value="catalog">已启用模型的正式声音</option>
          </select>
        </div>
        {kind === "sample" ? (
          <>
            <BibleAssetSelect
              identity={identity}
              kind="audio"
              label="声音样本原件"
              value={sampleId}
              locked={writer.locked}
              onChange={(value) => {
                setSampleId(value);
                changed();
              }}
            />
            <Label htmlFor={`${id}-sample-name`}>样本名称</Label>
            <Textarea
              id={`${id}-sample-name`}
              value={sampleName}
              onChange={(event) => {
                setSampleName(event.target.value);
                changed();
              }}
            />
            <p>样本绑定保留正式音频事实，当前不代表已执行克隆或生成。</p>
          </>
        ) : (
          <div className="space-y-3">
            <Label htmlFor={`${id}-catalog`}>当前正式声音</Label>
            <select
              id={`${id}-catalog`}
              className="h-10 w-full rounded-md bg-muted px-2"
              value={selected}
              onChange={(event) => {
                setSelected(event.target.value);
                setEntries({});
                changed();
              }}
            >
              <option value="">明确选择当前模型声音</option>
              {selected && !choice && (
                <option value={selected}>原声音 · 当前版本尚未核验</option>
              )}
              {choices.map((item) => (
                <option key={choiceKey(item)} value={choiceKey(item)}>
                  {item.display_name} · {item.model_key} · v{item.model_version}
                </option>
              ))}
            </select>
            {voices.isError && (
              <div>
                <p role="alert">正式声音目录无法读取。</p>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => void voices.refetch()}
                >
                  重试声音目录
                </Button>
              </div>
            )}
            {voices.hasNextPage && (
              <Button
                type="button"
                variant="outline"
                disabled={voices.isFetchingNextPage}
                onClick={() => void voices.fetchNextPage()}
              >
                读取更多正式声音
              </Button>
            )}
            {!voices.isPending && !voices.isError && choices.length === 0 && (
              <p>
                项目当前没有可用的正式模型声音，请在模型配置中完成真实接入。
              </p>
            )}
            {model.isError && (
              <div>
                <p role="alert">
                  选中声音的模型版本或安全参数声明已变化，当前不能绑定。
                </p>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => void model.refetch()}
                >
                  重新核验声音模型
                </Button>
              </div>
            )}
            {parameters.map((parameter) => (
              <div key={parameter.field}>
                <Label htmlFor={`${id}-${parameter.field}`}>
                  {parameter.label}
                </Label>
                {parameter.enum ? (
                  <select
                    id={`${id}-${parameter.field}`}
                    className="h-10 w-full rounded-md bg-muted px-2"
                    value={entries[parameter.field] ?? ""}
                    onChange={(event) => {
                      setEntries((values) => ({
                        ...values,
                        [parameter.field]: event.target.value,
                      }));
                      changed();
                    }}
                  >
                    <option value="">不显式提供</option>
                    {parameter.enum.map((value) => (
                      <option key={String(value)} value={String(value)}>
                        {String(value)}
                      </option>
                    ))}
                  </select>
                ) : (
                  <Input
                    id={`${id}-${parameter.field}`}
                    type={
                      parameter.type === "number" ||
                      parameter.type === "integer"
                        ? "number"
                        : "text"
                    }
                    min={parameter.min}
                    max={parameter.max}
                    step={
                      parameter.step ??
                      (parameter.type === "integer" ? 1 : "any")
                    }
                    value={entries[parameter.field] ?? ""}
                    onChange={(event) => {
                      setEntries((values) => ({
                        ...values,
                        [parameter.field]: event.target.value,
                      }));
                      changed();
                    }}
                  />
                )}
                {parameter.description && (
                  <p className="text-xs text-muted-foreground">
                    {parameter.description}
                  </p>
                )}
                {parameter.for_modes?.length ? (
                  <p className="text-xs text-muted-foreground">
                    适用模式：{parameter.for_modes.join("、")}
                  </p>
                ) : null}
                {parameter.required && parameter.default === undefined ? (
                  <p className="text-xs text-muted-foreground">
                    此模式启用时必须显式提供。
                  </p>
                ) : null}
              </div>
            ))}
            {Object.keys(entries)
              .filter(
                (field) =>
                  !parameters.some((parameter) => parameter.field === field) &&
                  entries[field] !== "",
              )
              .map((field) => (
                <div key={field}>
                  <p role="alert">
                    原参数 {field} 不在当前安全声明内，不能静默丢弃。
                  </p>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => {
                      setEntries((values) => {
                        const next = { ...values };
                        delete next[field];
                        return next;
                      });
                      changed();
                    }}
                  >
                    明确删除原声音参数 {field}
                  </Button>
                </div>
              ))}
            <p>仅发送显式输入的正式参数，不自动填入模型默认值。</p>
          </div>
        )}
        <Label htmlFor={`${id}-instructions`}>声音使用说明</Label>
        <Textarea
          id={`${id}-instructions`}
          value={instructions}
          onChange={(event) => {
            setInstructions(event.target.value);
            changed();
          }}
        />
        <Button
          disabled={
            kind === "catalog" &&
            (!choice || !model.isSuccess || !model.isFetchedAfterMount)
          }
          onClick={async () => {
            try {
              const voice = voiceInputSchema.parse(
                kind === "sample"
                  ? {
                      kind,
                      sample: { asset_id: sampleId, name: sampleName },
                      ...(instructions ? { instructions } : {}),
                    }
                  : {
                      kind,
                      catalog: {
                        model_key: choice!.model_key,
                        expected_model_version: choice!.model_version,
                        voice_key: choice!.voice_key,
                        params: declaredVoiceParams(model.data!, entries),
                      },
                      ...(instructions ? { instructions } : {}),
                    },
              );
              await writer.submit({
                action: "voice_bind",
                kind: "character",
                id: entryId,
                body: { expected_revision: revision, voice },
              });
            } catch (cause) {
              setError(
                cause instanceof Error
                  ? cause.message
                  : "声音输入无效，请保留草稿。",
              );
            }
          }}
        >
          明确保存声音绑定
        </Button>
      </fieldset>
      {error && <p role="alert">{error}</p>}
    </BibleWriteDialog>
  );
}
