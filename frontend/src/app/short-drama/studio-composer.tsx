import { useId, useRef, useState } from "react";
import {
  AtSign,
  FileText,
  Film,
  Frame,
  Paintbrush,
  PenLine,
  Plus,
  SlidersHorizontal,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Textarea } from "@/components/ui/textarea";
import { redrawMarkets, type StudioInput } from "./studio-input";
import styles from "./studio.module.css";

const settings = [
  { key: "reference", label: "参考素材", field: "参考链接", icon: AtSign },
  { key: "style", label: "风格", field: "剧本风格", icon: Paintbrush },
  { key: "topic", label: "选题", field: "题材或主题", icon: PenLine },
  { key: "ratio", label: "比例", field: "画面比例", icon: Frame },
  {
    key: "episodes",
    label: "生成设置",
    field: "计划集数",
    icon: SlidersHorizontal,
  },
] as const;

const starters = [
  [
    "剧本医生",
    "请根据我提供的剧本，梳理情节、人物动机和每集钩子，并提出修改建议。",
  ],
  [
    "玄幻仙侠短剧创意",
    "请构思一部玄幻仙侠短剧，主角意外获得一件灵器，却因此被昔日同门追杀。",
  ],
  [
    "都市恋爱短剧创意",
    "请构思一部都市恋爱短剧，主角因一次身份误会与旧日恋人重逢。",
  ],
  [
    "小说拆解",
    "请将我提供的小说拆解为短剧大纲，列出关键人物、主线冲突和各集反转。",
  ],
  [
    "创意生成剧本",
    "请将我提供的创意扩写为分集剧本，包含场景、人物动作与对白。",
  ],
] as const;

export function StudioComposer({
  mode,
  disabled,
  signedIn,
  onStart,
}: {
  mode: StudioInput["kind"];
  disabled: boolean;
  signedIn: boolean;
  onStart: (input: StudioInput) => void;
}) {
  const id = useId();
  const fileInput = useRef<HTMLInputElement>(null);
  const editor = useRef<HTMLTextAreaElement>(null);
  const [text, setText] = useState("");
  const [file, setFile] = useState<File>();
  const [fileError, setFileError] = useState("");
  const [values, setValues] = useState<
    Partial<Record<(typeof settings)[number]["key"] | "market", string>>
  >({});
  const creative = mode === "create";
  const uploading = mode === "upload";
  const fileType = uploading ? "TXT 或 DOCX" : "MP4 或 MOV";
  const fileLimit = uploading ? 20 : 500;

  function selectFile(selected?: File) {
    if (!selected) return;
    const format = uploading ? /\.(txt|docx)$/i : /\.(mp4|mov)$/i;
    if (
      !format.test(selected.name) ||
      selected.size < 1 ||
      selected.size > fileLimit * 1024 * 1024
    ) {
      setFileError(`请选择 ${fileLimit} MB 以内的 ${fileType} 文件。`);
      return;
    }
    setFileError("");
    setFile(selected);
  }

  function update(key: keyof typeof values, value: string) {
    setValues((previous) => ({ ...previous, [key]: value }));
  }

  function start() {
    if (creative) {
      const notes = settings.flatMap(({ key, field }) => {
        const value = values[key]?.trim();
        return value ? [`${field}：${value}`] : [];
      });
      onStart({
        kind: "create",
        prompt: [text.trim(), ...notes].filter(Boolean).join("\n"),
      });
    } else if (uploading) {
      onStart({ kind: "upload", text, file });
    } else {
      onStart({
        kind: "redraw",
        file,
        style: values.style ?? "",
        market: values.market ?? "",
      });
    }
  }

  const reference = (
    <Field>
      <FieldLabel htmlFor={`${id}-reference`}>参考链接</FieldLabel>
      <Input
        id={`${id}-reference`}
        type="url"
        value={values.reference ?? ""}
        maxLength={500}
        placeholder="https://"
        onChange={(event) => update("reference", event.target.value)}
      />
      <p className="text-xs text-muted-foreground">
        链接会随创作要求带入项目。
      </p>
    </Field>
  );

  const tile = (
    <>
      {file ? <FileText aria-hidden="true" /> : <Plus aria-hidden="true" />}
      <span className="max-w-full truncate">
        {file?.name ??
          (creative ? "添加参考" : uploading ? "上传剧本" : "上传源视频")}
      </span>
      <span className="text-xs font-normal text-muted-foreground">
        {creative ? "添加参考链接" : `${fileType} · ${fileLimit} MB 以内`}
      </span>
    </>
  );
  const tileClass =
    "h-auto min-h-24 w-full min-w-0 flex-col gap-2 rounded-xl border border-dashed border-border bg-transparent px-3 py-4 md:min-h-40";

  return (
    <div className="space-y-3">
      <div
        className={`${styles.panel} overflow-hidden rounded-b-2xl border border-t-0 bg-card`}
      >
        <div className="grid gap-3 p-3 md:grid-cols-[minmax(0,1fr)_minmax(0,3fr)] md:p-4">
          <div className="min-w-0">
            {creative ? (
              <Popover>
                <PopoverTrigger asChild>
                  <Button variant="ghost" className={tileClass}>
                    {tile}
                  </Button>
                </PopoverTrigger>
                <PopoverContent align="start">{reference}</PopoverContent>
              </Popover>
            ) : (
              <>
                <Input
                  ref={fileInput}
                  type="file"
                  hidden
                  accept={uploading ? ".txt,.docx" : ".mp4,.mov"}
                  onChange={(event) => {
                    selectFile(event.target.files?.[0]);
                    event.target.value = "";
                  }}
                />
                <Button
                  variant="ghost"
                  className={tileClass}
                  onClick={() => fileInput.current?.click()}
                >
                  {tile}
                </Button>
                {fileError && (
                  <p role="alert" className="mt-1 text-xs text-destructive">
                    {fileError}
                  </p>
                )}
                {file && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setFile(undefined);
                      setFileError("");
                    }}
                  >
                    移除文件
                  </Button>
                )}
              </>
            )}
          </div>
          {mode === "redraw" ? (
            <div className="flex min-w-0 flex-col gap-3 rounded-xl bg-muted/40 p-3">
              <div className="flex items-center gap-2 text-sm font-medium">
                <Film aria-hidden="true" className="size-4" /> 转绘设置
              </div>
              <div className="grid gap-3 sm:grid-cols-2">
                <Field>
                  <FieldLabel htmlFor={`${id}-style`}>转绘风格</FieldLabel>
                  <Input
                    id={`${id}-style`}
                    value={values.style ?? ""}
                    maxLength={200}
                    onChange={(event) => update("style", event.target.value)}
                    placeholder="如水墨、写实"
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${id}-market`}>目标市场</FieldLabel>
                  <NativeSelect
                    id={`${id}-market`}
                    value={values.market ?? redrawMarkets[0][0]}
                    onChange={(event) => update("market", event.target.value)}
                    className="w-full"
                  >
                    {redrawMarkets.map(([value, label]) => (
                      <NativeSelectOption key={value} value={value}>
                        {label}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                </Field>
              </div>
            </div>
          ) : (
            <Textarea
              ref={editor}
              aria-label={creative ? "剧本创作要求" : "剧本文本"}
              className="h-full min-h-28 resize-y rounded-xl bg-muted/40 p-3 md:min-h-40"
              maxLength={creative ? 500 : undefined}
              value={text}
              placeholder={
                creative
                  ? "描述你的创作想法，例如角色、情节和故事风格…"
                  : "粘贴剧本文本，可按「第1集」分集…"
              }
              onChange={(event) => setText(event.target.value)}
            />
          )}
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2 border-t px-3 py-2.5 md:px-4">
          {creative && (
            <div className="flex flex-wrap items-center gap-0.5">
              {settings.map(({ key, label, field, icon: Icon }) => (
                <Popover key={key}>
                  <PopoverTrigger asChild>
                    <Button size="sm" variant="ghost" className="px-2">
                      <Icon data-icon="inline-start" aria-hidden="true" />
                      {label}
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent align="start">
                    {key === "reference" ? (
                      reference
                    ) : (
                      <Field>
                        <FieldLabel htmlFor={`${id}-${key}`}>
                          {field}
                        </FieldLabel>
                        {key === "ratio" ? (
                          <NativeSelect
                            id={`${id}-${key}`}
                            value={values[key] ?? ""}
                            onChange={(event) =>
                              update(key, event.target.value)
                            }
                          >
                            <NativeSelectOption value="">
                              由 Agent 建议
                            </NativeSelectOption>
                            {["9:16", "16:9", "1:1", "4:3", "3:4"].map(
                              (ratio) => (
                                <NativeSelectOption key={ratio} value={ratio}>
                                  {ratio}
                                </NativeSelectOption>
                              ),
                            )}
                          </NativeSelect>
                        ) : (
                          <Input
                            id={`${id}-${key}`}
                            type={key === "episodes" ? "number" : "text"}
                            min={key === "episodes" ? 1 : undefined}
                            maxLength={200}
                            value={values[key] ?? ""}
                            onChange={(event) =>
                              update(key, event.target.value)
                            }
                          />
                        )}
                      </Field>
                    )}
                  </PopoverContent>
                </Popover>
              ))}
            </div>
          )}
          <div className="ml-auto flex items-center gap-3">
            {creative && (
              <span className="text-xs text-muted-foreground tabular-nums">
                {text.length} / 500
              </span>
            )}
            <Button
              disabled={
                disabled ||
                (signedIn &&
                  ((creative && !text.trim()) ||
                    (uploading && !file && !text.trim())))
              }
              onClick={start}
            >
              {creative ? "立即生成" : uploading ? "开始导入" : "开始转绘"}
            </Button>
          </div>
        </div>
      </div>
      {creative && (
        <div
          role="group"
          aria-label="推荐创作提示词"
          className="flex flex-wrap gap-2 px-1"
        >
          {starters.map(([label, starter]) => (
            <Button
              key={label}
              variant="secondary"
              size="sm"
              className="rounded-full"
              onClick={() => {
                setText((previous) =>
                  [previous.trim(), starter]
                    .filter(Boolean)
                    .join("\n")
                    .slice(0, 500),
                );
                editor.current?.focus();
              }}
            >
              {label}
            </Button>
          ))}
        </div>
      )}
    </div>
  );
}
