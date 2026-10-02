"use client";
import { useId, useState } from "react";
import {
  useFieldArray,
  useForm,
  useWatch,
  type UseFormReturn,
} from "react-hook-form";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  parseStructureDocument,
  type StructureDocument,
  type StructureItem,
} from "./review-model";
export type StructureBase = {
  expected_revision: number;
  expected_episode_revision: number;
  base_structure_version_no: number;
};
export function StructureEditForm({
  base,
  initialDocument,
  episodeStart,
  episodeEnd,
  locked,
  onDirty,
  onSubmit,
}: {
  base: StructureBase;
  initialDocument: StructureDocument;
  episodeStart: number;
  episodeEnd: number;
  locked: boolean;
  onDirty: (dirty: boolean) => void;
  onSubmit: (
    body: StructureBase & { document: StructureDocument },
  ) => void | Promise<void>;
}) {
  const id = useId();
  const [error, setError] = useState<string>();
  const form = useForm<StructureDocument>({ defaultValues: initialDocument });
  const scenes = useFieldArray({ control: form.control, name: "scenes" });
  const unassigned = useFieldArray({
    control: form.control,
    name: "unassigned_lines",
  });
  const busy = locked || form.formState.isSubmitting;
  function changed() {
    onDirty(true);
    setError(undefined);
  }
  function moveScene(index: number, direction: -1 | 1) {
    const next = form.getValues("scenes").slice();
    const target = index + direction;
    if (target < 0 || target >= next.length) return;
    [next[index], next[target]] = [next[target], next[index]];
    scenes.replace(next.map((scene, n) => ({ ...scene, seq_no: n + 1 })));
    changed();
  }
  function removeScene(index: number) {
    const next = form.getValues("scenes").filter((_, n) => n !== index);
    scenes.replace(next.map((scene, n) => ({ ...scene, seq_no: n + 1 })));
    changed();
  }
  return (
    <form
      className="space-y-4"
      onChange={changed}
      onSubmit={form.handleSubmit(async (document) => {
        try {
          const checked = parseStructureDocument(
            document,
            episodeStart,
            episodeEnd,
          );
          setError(undefined);
          await onSubmit({ ...base, document: checked });
        } catch (cause) {
          setError(cause instanceof Error ? cause.message : "手工结构无效。");
        }
      })}
    >
      <p className="text-sm">
        正文坐标 [{episodeStart}, {episodeEnd}) 按 Unicode
        字符计数。行动、台词和未归属行保留各自稳定身份；候选保存后另明确确认。空白和未提取的正文不被标成已解析。
      </p>
      <div className="space-y-4">
        {scenes.fields.map((scene, index) => (
          <fieldset
            key={scene.id}
            disabled={busy}
            className="min-w-0 space-y-3 rounded-lg border p-3"
          >
            <legend className="px-1">场景 {index + 1}</legend>
            <p className="text-xs break-all">稳定身份：{scene.scene_key}</p>
            <div className="grid gap-3 sm:grid-cols-3">
              <div>
                <Label htmlFor={`${id}-heading-${index}`}>
                  场景{index + 1}标题
                </Label>
                <Input
                  id={`${id}-heading-${index}`}
                  {...form.register(`scenes.${index}.heading`)}
                />
              </div>
              <div>
                <Label htmlFor={`${id}-location-${index}`}>
                  场景{index + 1}地点
                </Label>
                <Input
                  id={`${id}-location-${index}`}
                  {...form.register(`scenes.${index}.location_text`)}
                />
              </div>
              <div>
                <Label htmlFor={`${id}-time-${index}`}>
                  场景{index + 1}时段
                </Label>
                <Input
                  id={`${id}-time-${index}`}
                  {...form.register(`scenes.${index}.time_of_day`)}
                />
              </div>
              <div>
                <Label htmlFor={`${id}-start-${index}`}>
                  场景{index + 1}正文起点
                </Label>
                <Input
                  id={`${id}-start-${index}`}
                  type="number"
                  {...form.register(`scenes.${index}.span_start`, {
                    valueAsNumber: true,
                  })}
                />
              </div>
              <div>
                <Label htmlFor={`${id}-end-${index}`}>
                  场景{index + 1}正文终点
                </Label>
                <Input
                  id={`${id}-end-${index}`}
                  type="number"
                  {...form.register(`scenes.${index}.span_end`, {
                    valueAsNumber: true,
                  })}
                />
              </div>
            </div>
            <SceneItems form={form} index={index} changed={changed} />
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="outline"
                disabled={busy || index === 0}
                onClick={() => moveScene(index, -1)}
              >
                上移场景{index + 1}
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={busy || index === scenes.fields.length - 1}
                onClick={() => moveScene(index, 1)}
              >
                下移场景{index + 1}
              </Button>
              <Button
                type="button"
                variant="destructive"
                disabled={busy}
                onClick={() => removeScene(index)}
              >
                移除场景{index + 1}及其行
              </Button>
            </div>
          </fieldset>
        ))}
      </div>
      <Button
        type="button"
        variant="outline"
        disabled={busy}
        onClick={() => {
          const current = form.getValues("scenes");
          const start = current.at(-1)?.span_end ?? episodeStart;
          scenes.append({
            scene_key: crypto.randomUUID(),
            seq_no: current.length + 1,
            heading: "",
            location_text: "",
            time_of_day: "",
            span_start: start,
            span_end: episodeEnd,
            items: [],
          });
          changed();
        }}
      >
        添加手工场景
      </Button>
      <section className="space-y-3" aria-label="未归属行">
        <h3 className="font-medium">未归属行</h3>
        {unassigned.fields.map((line, index) => (
          <UnassignedRow
            key={line.id}
            form={form}
            index={index}
            busy={busy}
            sceneCount={scenes.fields.length}
            onRemove={() => {
              unassigned.remove(index);
              changed();
            }}
            onAssign={(target) => {
              const current = form.getValues(`unassigned_lines.${index}`);
              const scene = form.getValues(`scenes.${target}`);
              if (!scene) return;
              form.setValue(
                `scenes.${target}.items`,
                [
                  ...scene.items,
                  {
                    ...current,
                    type: "line",
                    kind: "dialogue",
                  } satisfies StructureItem,
                ].sort((a, b) => a.span_start - b.span_start),
                { shouldDirty: true },
              );
              unassigned.remove(index);
              changed();
            }}
          />
        ))}
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          onClick={() => {
            unassigned.append({
              line_key: crypto.randomUUID(),
              content: "",
              span_start: episodeStart,
              span_end: episodeEnd,
            });
            changed();
          }}
        >
          添加未归属行
        </Button>
      </section>
      {error && (
        <p role="alert" className="text-destructive">
          {error}
        </p>
      )}
      <Button type="submit" disabled={busy}>
        {form.formState.isSubmitting ? "正在保存结构…" : "保存手工结构候选"}
      </Button>
    </form>
  );
}
function SceneItems({
  form,
  index,
  changed,
}: {
  form: UseFormReturn<StructureDocument>;
  index: number;
  changed: () => void;
}) {
  const id = useId();
  const rows = useFieldArray({
    control: form.control,
    name: `scenes.${index}.items`,
  });
  const values =
    useWatch({ control: form.control, name: `scenes.${index}.items` }) ?? [];
  return (
    <section className="space-y-3" aria-label={`场景${index + 1}行动和台词`}>
      {rows.fields.map((row, n) => {
        const item = values[n] ?? row;
        const label = `场景${index + 1}行${n + 1}`;
        return (
          <div key={row.id} className="space-y-2 rounded-lg border p-3">
            <p className="text-xs break-all">行身份：{item.line_key}</p>
            <Label htmlFor={`${id}-type-${n}`}>{label}类型</Label>
            <select
              id={`${id}-type-${n}`}
              value={item.type}
              className="h-9 w-full rounded-md border bg-background px-2"
              onChange={(event) => {
                const common = {
                  line_key: item.line_key,
                  content: item.content,
                  span_start: item.span_start,
                  span_end: item.span_end,
                };
                rows.update(
                  n,
                  event.target.value === "action"
                    ? { ...common, type: "action" }
                    : { ...common, type: "line", kind: "dialogue" },
                );
                changed();
              }}
            >
              <option value="action">行动</option>
              <option value="line">台词</option>
            </select>
            <Label htmlFor={`${id}-content-${n}`}>{label}正文</Label>
            <Textarea
              id={`${id}-content-${n}`}
              {...form.register(`scenes.${index}.items.${n}.content`)}
            />
            <div className="grid gap-3 sm:grid-cols-2">
              <div>
                <Label htmlFor={`${id}-start-${n}`}>{label}正文起点</Label>
                <Input
                  id={`${id}-start-${n}`}
                  type="number"
                  {...form.register(`scenes.${index}.items.${n}.span_start`, {
                    valueAsNumber: true,
                  })}
                />
              </div>
              <div>
                <Label htmlFor={`${id}-end-${n}`}>{label}正文终点</Label>
                <Input
                  id={`${id}-end-${n}`}
                  type="number"
                  {...form.register(`scenes.${index}.items.${n}.span_end`, {
                    valueAsNumber: true,
                  })}
                />
              </div>
            </div>
            {item.type === "line" && (
              <>
                <Label htmlFor={`${id}-kind-${n}`}>{label}台词分类</Label>
                <select
                  id={`${id}-kind-${n}`}
                  className="h-9 w-full rounded-md border bg-background px-2"
                  {...form.register(`scenes.${index}.items.${n}.kind`)}
                >
                  <option value="dialogue">对话</option>
                  <option value="voiceover">旁白</option>
                  <option value="inner">内心</option>
                </select>
                <Label htmlFor={`${id}-speaker-${n}`}>{label}说话者文本</Label>
                <Input
                  id={`${id}-speaker-${n}`}
                  {...form.register(`scenes.${index}.items.${n}.speaker_text`, {
                    setValueAs: (value: string) => value || undefined,
                  })}
                />
                <Label htmlFor={`${id}-character-${n}`}>
                  {label}正式角色UUID（可空）
                </Label>
                <Input
                  id={`${id}-character-${n}`}
                  {...form.register(`scenes.${index}.items.${n}.character_id`, {
                    setValueAs: (value: string) => value || undefined,
                  })}
                />
                <Label htmlFor={`${id}-emotion-${n}`}>{label}情绪</Label>
                <Input
                  id={`${id}-emotion-${n}`}
                  {...form.register(`scenes.${index}.items.${n}.emotion`, {
                    setValueAs: (value: string) => value || undefined,
                  })}
                />
              </>
            )}
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                rows.remove(n);
                changed();
              }}
            >
              移除{label}
            </Button>
          </div>
        );
      })}
      <div className="flex flex-wrap gap-2">
        {(["action", "line"] as const).map((type) => (
          <Button
            key={type}
            type="button"
            variant="outline"
            onClick={() => {
              const scene = form.getValues(`scenes.${index}`);
              const common = {
                line_key: crypto.randomUUID(),
                content: "",
                span_start: scene.items.at(-1)?.span_end ?? scene.span_start,
                span_end: scene.span_end,
              };
              rows.append(
                type === "action"
                  ? { ...common, type }
                  : { ...common, type, kind: "dialogue" },
              );
              changed();
            }}
          >
            {type === "action" ? "添加行动" : "添加台词"}到场景{index + 1}
          </Button>
        ))}
      </div>
    </section>
  );
}
function UnassignedRow({
  form,
  index,
  busy,
  sceneCount,
  onRemove,
  onAssign,
}: {
  form: UseFormReturn<StructureDocument>;
  index: number;
  busy: boolean;
  sceneCount: number;
  onRemove: () => void;
  onAssign: (index: number) => void;
}) {
  const id = useId();
  const [target, setTarget] = useState(0);
  return (
    <fieldset disabled={busy} className="space-y-2 rounded-lg border p-3">
      <legend>未归属行 {index + 1}</legend>
      <Label htmlFor={`${id}-content`}>未归属行{index + 1}正文</Label>
      <Textarea
        id={`${id}-content`}
        {...form.register(`unassigned_lines.${index}.content`)}
      />
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <Label htmlFor={`${id}-start`}>未归属行{index + 1}起点</Label>
          <Input
            id={`${id}-start`}
            type="number"
            {...form.register(`unassigned_lines.${index}.span_start`, {
              valueAsNumber: true,
            })}
          />
        </div>
        <div>
          <Label htmlFor={`${id}-end`}>未归属行{index + 1}终点</Label>
          <Input
            id={`${id}-end`}
            type="number"
            {...form.register(`unassigned_lines.${index}.span_end`, {
              valueAsNumber: true,
            })}
          />
        </div>
      </div>
      <Label htmlFor={`${id}-target`}>未归属行{index + 1}目标场景</Label>
      <select
        id={`${id}-target`}
        value={target}
        className="h-9 w-full rounded-md border bg-background px-2"
        onChange={(event) => setTarget(Number(event.target.value))}
      >
        {Array.from({ length: sceneCount }, (_, n) => (
          <option key={n} value={n}>
            场景 {n + 1}
          </option>
        ))}
      </select>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={busy || !sceneCount || target >= sceneCount}
          onClick={() => onAssign(target)}
        >
          将未归属行{index + 1}分配到所选场景
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          onClick={onRemove}
        >
          移除未归属行{index + 1}
        </Button>
      </div>
    </fieldset>
  );
}
