"use client";

import { useRef, useState } from "react";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { savePromptPreference } from "@/api/settings";
import { ApiError } from "@/lib/request";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { AdminField, AdminMessage } from "./admin-ui";
import {
  PROMPT_PREFERENCES_KEY,
  preferenceError,
  promptCustomizationSchema,
  promptMode,
  queryPromptPreferences,
  type PromptPreference,
} from "./settings-preferences-queries";

function editorSchema(preference: PromptPreference) {
  return z
    .object({ mode: promptMode, content: z.string() })
    .superRefine((value, ctx) => {
      if (value.mode === "inherit") return;
      if (!value.content.trim())
        ctx.addIssue({
          code: "custom",
          path: ["content"],
          message: "请填写创作要求",
        });
      if ([...value.content].length > 12000)
        ctx.addIssue({
          code: "custom",
          path: ["content"],
          message: "创作要求最多 12000 字",
        });
      const placeholders = value.content.match(/\{\{[^{}]+\}\}/g) ?? [];
      const allowed = new Set(
        preference.definition.variables.map((variable) => variable.placeholder),
      );
      if (
        placeholders.some((placeholder) => !allowed.has(placeholder)) ||
        /\{\{|\}\}/.test(value.content.replace(/\{\{[^{}]+\}\}/g, ""))
      )
        ctx.addIssue({
          code: "custom",
          path: ["content"],
          message: "仅支持下方声明的变量，请检查双花括号",
        });
    });
}
type Values = { mode: z.infer<typeof promptMode>; content: string };
function valuesFor(preference: PromptPreference): Values {
  return {
    mode: preference.customization?.mode ?? "inherit",
    content: preference.customization?.content ?? "",
  };
}

export function PromptPreferenceEditor({
  initial,
  saved,
  dirty,
  reload,
}: {
  initial: PromptPreference;
  saved: (value: PromptPreference) => void;
  dirty: (value: boolean) => void;
  reload: () => Promise<PromptPreference>;
}) {
  const [snapshot, setSnapshot] = useState(() => initial);
  const [message, setMessage] = useState("");
  const [error, setError] = useState(false);
  const [confirm, setConfirm] = useState<"restore" | "reload" | null>(null);
  const [restoring, setRestoring] = useState(false);
  const [reading, setReading] = useState(false);
  const request = useRef<{ fingerprint: string; key: string } | null>(null);
  const textarea = useRef<HTMLTextAreaElement | null>(null);
  const definition = snapshot.definition;
  const form = useForm<Values>({
    resolver: zodResolver(editorSchema(snapshot)),
    defaultValues: valuesFor(snapshot),
  });
  const [mode, content] = useWatch({
    control: form.control,
    name: ["mode", "content"],
  });
  const pending = form.formState.isSubmitting || restoring || reading;
  const contentField = form.register("content");
  const preview =
    mode === "inherit"
      ? definition.content
      : mode === "append"
        ? `${definition.content}\n\n【个人创作要求】\n${content}`
        : content;

  async function persist(values: Values) {
    const body = {
      expected_revision: snapshot.customization?.revision ?? 0,
      base_template_id: definition.template_id,
      mode: values.mode,
      content: values.mode === "inherit" ? "" : values.content.trim(),
    };
    const fingerprint = JSON.stringify(body);
    if (!request.current || request.current.fingerprint !== fingerprint)
      request.current = { fingerprint, key: crypto.randomUUID() };
    try {
      const parsed = promptCustomizationSchema.safeParse(
        await savePromptPreference({ operation: definition.operation }, body, {
          headers: { "Idempotency-Key": request.current.key },
        }),
      );
      if (
        !parsed.success ||
        parsed.data.operation !== definition.operation ||
        parsed.data.revision !== body.expected_revision + 1 ||
        parsed.data.base_template_id !== body.base_template_id ||
        parsed.data.mode !== body.mode ||
        parsed.data.content !== body.content
      )
        throw new ApiError(502, "invalid_response");
      const next = { ...snapshot, customization: parsed.data, outdated: false };
      setSnapshot(next);
      form.reset(valuesFor(next));
      request.current = null;
      setMessage(
        values.mode === "inherit" ? "已恢复为默认模板。" : "提示词偏好已保存。",
      );
      setError(false);
      setConfirm(null);
      dirty(false);
      saved(next);
    } catch (failure) {
      setMessage(preferenceError(failure));
      setError(true);
    }
  }
  async function refresh() {
    setReading(true);
    try {
      const latest = await reload();
      setSnapshot(latest);
      form.reset(valuesFor(latest));
      request.current = null;
      dirty(false);
      setMessage("已载入服务端最新偏好。");
      setError(false);
      setConfirm(null);
    } catch (failure) {
      setMessage(preferenceError(failure));
      setError(true);
    } finally {
      setReading(false);
    }
  }
  return (
    <Card className="min-w-0">
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <CardTitle>{definition.label}</CardTitle>
          <Badge variant="outline">
            {snapshot.customization
              ? `个人版本 ${snapshot.customization.revision}`
              : "使用默认模板"}
          </Badge>
        </div>
        <CardDescription>{definition.description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <p className="text-sm text-muted-foreground">
          偏好归属于当前工作区的个人身份。当前可保存创作要求；剧本和分镜自动生成尚未开放。
        </p>
        {snapshot.outdated && (
          <p role="alert" className="text-sm text-destructive">
            默认模板已有更新。当前替换内容仍保留旧基线，请比较下方模板后再保存或恢复默认。
          </p>
        )}
        {(initial.definition.template_id !== definition.template_id ||
          initial.customization?.revision !==
            snapshot.customization?.revision) && (
          <p role="status" className="text-sm text-muted-foreground">
            服务端偏好已有变化，重新读取前保留当前输入与编辑修订。
          </p>
        )}
        <form
          className="space-y-5"
          onChange={() => dirty(true)}
          onSubmit={(event) => void form.handleSubmit(persist)(event)}
        >
          <fieldset disabled={pending} className="space-y-3">
            <legend className="text-sm font-medium">创作要求模式</legend>
            <div className="flex flex-wrap gap-4">
              {(
                [
                  { value: "inherit", label: "使用默认" },
                  { value: "append", label: "追加要求" },
                  { value: "rewrite", label: "替换创作模板" },
                ] as const
              ).map((item) => (
                <label
                  key={item.value}
                  className="flex cursor-pointer items-center gap-2 text-sm"
                >
                  <input
                    type="radio"
                    value={item.value}
                    {...form.register("mode")}
                  />
                  {item.label}
                </label>
              ))}
            </div>
          </fieldset>
          {mode !== "inherit" && (
            <>
              <AdminField
                id="personal-prompt-content"
                label={mode === "append" ? "追加的创作要求" : "个人创作模板"}
                error={form.formState.errors.content?.message}
              >
                <Textarea
                  id="personal-prompt-content"
                  rows={12}
                  className="min-h-64 font-mono text-sm"
                  disabled={pending}
                  {...contentField}
                  ref={(node) => {
                    contentField.ref(node);
                    textarea.current = node;
                  }}
                />
              </AdminField>
              <p className="text-xs text-muted-foreground">
                {[...content].length} / 12000
                字。服务端的项目上下文和输出格式始终保留。
              </p>
              {definition.variables.length > 0 && (
                <div className="space-y-2">
                  <p className="text-sm font-medium">可用变量</p>
                  <div className="flex flex-wrap gap-2">
                    {definition.variables.map((variable) => (
                      <Button
                        key={variable.placeholder}
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={pending}
                        title={variable.placeholder}
                        onClick={() => {
                          const input = textarea.current,
                            current = form.getValues("content");
                          const start = input?.selectionStart ?? current.length,
                            end = input?.selectionEnd ?? current.length;
                          form.setValue(
                            "content",
                            current.slice(0, start) +
                              variable.placeholder +
                              current.slice(end),
                            { shouldDirty: true, shouldValidate: true },
                          );
                          dirty(true);
                          input?.focus();
                          input?.setSelectionRange(
                            start + variable.placeholder.length,
                            start + variable.placeholder.length,
                          );
                        }}
                      >
                        {variable.label}
                      </Button>
                    ))}
                  </div>
                </div>
              )}
            </>
          )}
          <AdminMessage message={message} error={error} />
          <div className="flex flex-wrap gap-3">
            <Button type="submit" disabled={!form.formState.isDirty || pending}>
              {form.formState.isSubmitting ? "保存中…" : "保存提示词偏好"}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={
                pending || (!snapshot.customization && !form.formState.isDirty)
              }
              onClick={() => setConfirm("restore")}
            >
              恢复默认
            </Button>
            <Button
              type="button"
              variant="ghost"
              disabled={pending}
              onClick={() =>
                form.formState.isDirty ? setConfirm("reload") : void refresh()
              }
            >
              重新读取
            </Button>
          </div>
        </form>
        <details>
          <summary className="cursor-pointer text-sm font-medium">
            默认模板 · v{definition.template_version}
          </summary>
          <pre className="mt-3 max-h-96 overflow-auto rounded-lg bg-muted p-4 text-xs leading-6 break-words whitespace-pre-wrap">
            {definition.content}
          </pre>
        </details>
        <details>
          <summary className="cursor-pointer text-sm font-medium">
            预览创作模板
          </summary>
          <pre className="mt-3 max-h-96 overflow-auto rounded-lg bg-muted p-4 text-xs leading-6 break-words whitespace-pre-wrap">
            {preview}
          </pre>
          <p className="mt-2 text-xs text-muted-foreground">
            预览保留变量占位符，实际任务还会加入受保护的上下文与输出约束。
          </p>
        </details>
        <details>
          <summary className="cursor-pointer text-sm font-medium">
            受保护的输出格式
          </summary>
          <pre className="mt-3 max-h-80 overflow-auto rounded-lg bg-muted p-4 text-xs leading-6 break-words whitespace-pre-wrap">
            {definition.output_contract}
          </pre>
        </details>
        <Dialog
          open={confirm !== null}
          onOpenChange={(open) => {
            if (!open && !restoring && !reading) setConfirm(null);
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>
                {confirm === "restore"
                  ? "恢复默认提示词？"
                  : "放弃当前提示词修改？"}
              </DialogTitle>
              <DialogDescription>
                {confirm === "restore"
                  ? "当前操作将使用默认创作模板，已保存和未保存的个人创作要求都会被替换。其他操作的偏好保留。"
                  : "重新读取会用服务端最新偏好替换当前未保存输入。"}
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button
                variant="outline"
                disabled={restoring || reading}
                onClick={() => setConfirm(null)}
              >
                继续编辑
              </Button>
              <Button
                disabled={restoring || reading}
                onClick={() => {
                  if (confirm === "reload") void refresh();
                  else {
                    setRestoring(true);
                    void persist({ mode: "inherit", content: "" }).finally(() =>
                      setRestoring(false),
                    );
                  }
                }}
              >
                {restoring || reading
                  ? "处理中…"
                  : confirm === "restore"
                    ? "确认恢复默认"
                    : "放弃修改并读取"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardContent>
    </Card>
  );
}

export function SettingsPrompts() {
  const cache = useQueryClient();
  const query = useQuery({
    queryKey: PROMPT_PREFERENCES_KEY,
    queryFn: ({ signal }) => queryPromptPreferences(signal),
    retry: false,
  });
  const [selected, setSelected] = useState("");
  const [dirty, setDirty] = useState(false);
  const [pending, setPending] = useState<string | null>(null);
  if (query.isPending) return <p role="status">载入提示词模板与个人偏好…</p>;
  if (query.isError)
    return (
      <div role="alert" className="space-y-3">
        <p>{preferenceError(query.error)}</p>
        <Button variant="outline" onClick={() => void query.refetch()}>
          重新读取
        </Button>
      </div>
    );
  const preference =
    query.data.find((item) => item.definition.operation === selected) ??
    query.data[0];
  return (
    <div className="grid min-w-0 gap-6 xl:grid-cols-[240px_minmax(0,1fr)]">
      <nav
        aria-label="提示词操作"
        className="flex flex-wrap content-start gap-2 xl:flex-col"
      >
        {query.data.map((item) => (
          <Button
            key={item.definition.operation}
            variant={item === preference ? "secondary" : "ghost"}
            className="justify-start"
            onClick={() => {
              if (item === preference) return;
              if (dirty) setPending(item.definition.operation);
              else setSelected(item.definition.operation);
            }}
          >
            {item.definition.label}
          </Button>
        ))}
      </nav>
      <PromptPreferenceEditor
        key={preference.definition.operation}
        initial={preference}
        dirty={setDirty}
        saved={(value) =>
          cache.setQueryData<PromptPreference[]>(
            PROMPT_PREFERENCES_KEY,
            (existing) =>
              existing?.map((item) =>
                item.definition.operation === value.definition.operation
                  ? value
                  : item,
              ),
          )
        }
        reload={async () => {
          const response = await query.refetch();
          if (response.error) throw response.error;
          const found = response.data?.find(
            (item) =>
              item.definition.operation === preference.definition.operation,
          );
          if (!found) throw new ApiError(502, "invalid_response");
          return found;
        }}
      />
      <Dialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) setPending(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>放弃当前未保存的修改？</DialogTitle>
            <DialogDescription>
              切换操作会关闭当前编辑内容。已保存的偏好保留在服务端。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPending(null)}>
              继续编辑
            </Button>
            <Button
              onClick={() => {
                if (pending) setSelected(pending);
                setPending(null);
                setDirty(false);
              }}
            >
              放弃修改并切换
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
