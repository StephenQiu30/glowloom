"use client";

import { useEffect, useId, useRef, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ApiError } from "@/lib/request";
import type { ProjectAction } from "./project-list";
import {
  PROJECTS_KEY,
  getProject,
  listStylePresets,
  transitionProject,
  updateProject,
  type ProjectChange,
  type ProjectDetail,
  type ProjectSummary,
} from "./queries";
import { matchesPreset, type StylePreset } from "./creation";

type Props = {
  project: ProjectSummary;
  action: ProjectAction;
  onClose: () => void;
  onChanged: () => void;
};
const labels = {
  settings: [
    "项目设置",
    "保存项目设置",
    "名称、描述与生成设置。画幅、风格类型和分辨率在创建后固定。",
  ],
  archive: [
    "归档项目",
    "确认归档",
    "归档后画布和素材可以查看。完成正在处理的任务后才能归档。",
  ],
  unarchive: [
    "取消归档",
    "确认取消归档",
    "恢复项目编辑，继续使用已有画布和素材。",
  ],
  delete: [
    "移入回收站",
    "确认移入回收站",
    "项目、画布和素材将保留 30 天。期间可在回收站恢复。",
  ],
  restore: [
    "恢复项目",
    "确认恢复",
    "恢复已有画布和素材，并保留移入回收站之前的归档状态。",
  ],
} as const;
export function ProjectManagementDialog({
  project,
  action,
  onClose,
  onChanged,
}: Props) {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [unknown, setUnknown] = useState(false);
  const inFlight = useRef(false);
  const attempt = useRef<{ key: string; body: ProjectChange } | null>(null);
  const detail = useQuery({
    queryKey: [...PROJECTS_KEY, "detail", project.id],
    queryFn: ({ signal }) => getProject(project.id, signal),
    enabled: action === "settings",
    retry: false,
    refetchOnWindowFocus: false,
  });
  const presets = useInfiniteQuery({
    queryKey: [...PROJECTS_KEY, "style-presets"],
    enabled: action === "settings",
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) => listStylePresets(pageParam, signal),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
    refetchOnWindowFocus: false,
  });
  useEffect(() => {
    if (!pending && !unknown) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [pending, unknown]);
  async function submit(body: ProjectChange) {
    if (inFlight.current) return;
    inFlight.current = true;
    setPending(true);
    setError(null);
    if (!attempt.current)
      attempt.current = {
        key: crypto.randomUUID(),
        body: structuredClone(body),
      };
    const frozen = attempt.current;
    try {
      if (action === "settings")
        await updateProject(project.id, frozen.body, frozen.key);
      else
        await transitionProject(
          project.id,
          action,
          frozen.body.expected_revision!,
          frozen.key,
        );
      attempt.current = null;
      setUnknown(false);
      onChanged();
      onClose();
    } catch (failure) {
      const unresolved =
        !(failure instanceof ApiError) ||
        failure.status === 0 ||
        failure.status >= 500;
      setUnknown(unresolved);
      setError(failure);
      if (!unresolved) attempt.current = null;
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }
  const locked = pending || unknown;
  const [title, submitLabel, description] = labels[action];
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !locked) onClose();
      }}
    >
      <DialogContent
        showCloseButton={false}
        className="max-h-[90dvh] overflow-y-auto"
      >
        <DialogHeader>
          <DialogTitle>
            {title} · {project.name}
          </DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {error !== null && (
          <Alert variant="destructive">
            <AlertTitle>
              {unknown ? "修改结果尚未确认" : "项目操作未完成"}
            </AlertTitle>
            <AlertDescription>
              {unknown
                ? "请保持此窗口，核验原请求。重试会使用同一份修改内容。"
                : error instanceof Error
                  ? error.message
                  : "服务暂时不可用。"}
              {error instanceof ApiError && error.requestId && (
                <p>请求编号：{error.requestId}</p>
              )}
            </AlertDescription>
          </Alert>
        )}
        {!locked &&
          error instanceof ApiError &&
          (error.code === "revision_conflict" ||
            error.code === "state_conflict") && (
            <Button
              variant="outline"
              onClick={() => {
                onChanged();
                onClose();
              }}
            >
              读取最新项目列表
            </Button>
          )}
        {action === "settings" ? (
          detail.isPending ? (
            <p role="status">正在读取项目设置…</p>
          ) : detail.error ? (
            <Alert variant="destructive">
              <AlertTitle>项目设置未能读取</AlertTitle>
              <AlertDescription>{detail.error.message}</AlertDescription>
              <Button variant="outline" onClick={() => void detail.refetch()}>
                重试读取设置
              </Button>
            </Alert>
          ) : (
            detail.data && (
              <ProjectSettingsForm
                key={`${detail.data.id}:${detail.data.revision}`}
                project={detail.data}
                presets={
                  presets.data?.pages.flatMap((page) => page.items) ?? []
                }
                presetsPending={presets.isFetching}
                presetsError={presets.error}
                hasMorePresets={presets.hasNextPage}
                onLoadPresets={() => {
                  if (presets.error) void presets.refetch();
                  else void presets.fetchNextPage();
                }}
                disabled={locked || detail.data.status === "archived"}
                onSubmit={submit}
              />
            )
          )
        ) : (
          <p className="text-sm">
            {project.name}
            {action === "restore" && project.purge_after
              ? ` · 恢复期限：${new Date(project.purge_after).toLocaleString("zh-CN")}`
              : ""}
          </p>
        )}
        <DialogFooter>
          <Button variant="outline" disabled={locked} onClick={onClose}>
            关闭
          </Button>
          {unknown ? (
            <Button
              disabled={pending}
              onClick={() => {
                if (attempt.current) void submit(attempt.current.body);
              }}
            >
              核验原修改请求
            </Button>
          ) : (
            action !== "settings" && (
              <Button
                variant={action === "delete" ? "destructive" : "default"}
                disabled={pending}
                onClick={() =>
                  void submit({ expected_revision: project.revision })
                }
              >
                {pending ? "正在提交…" : submitLabel}
              </Button>
            )
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type Settings = {
  name: string;
  description: string;
  preset: string;
  overseas: boolean;
};
function ProjectSettingsForm({
  project,
  presets,
  presetsPending,
  presetsError,
  hasMorePresets,
  onLoadPresets,
  disabled,
  onSubmit,
}: {
  project: ProjectDetail;
  presets: readonly StylePreset[];
  presetsPending: boolean;
  presetsError: Error | null;
  hasMorePresets: boolean;
  onLoadPresets: () => void;
  disabled: boolean;
  onSubmit: (body: ProjectChange) => Promise<void>;
}) {
  const id = useId();
  const form = useForm<Settings>({
    defaultValues: {
      name: project.name,
      description: project.description,
      preset: project.style_preset_id ?? "none",
      overseas: project.allow_overseas_models,
    },
  });
  const available = presets.filter((preset) =>
    matchesPreset(
      {
        style_type: project.style_type,
        style_subtype: project.style_subtype ?? "",
      },
      preset,
    ),
  );
  const storedPresetMissing =
    project.style_preset_id &&
    !available.some((preset) => preset.id === project.style_preset_id);
  return (
    <form
      aria-label="项目设置表单"
      onSubmit={form.handleSubmit(async (values) => {
        const body: ProjectChange = { expected_revision: project.revision };
        const name = values.name.trim();
        if (name !== project.name) body.name = name;
        if (values.description !== project.description)
          body.description = values.description;
        const preset = values.preset === "none" ? null : values.preset;
        if (preset !== project.style_preset_id)
          body.style_preset_id =
            preset ?? "00000000-0000-0000-0000-000000000000";
        if (values.overseas !== project.allow_overseas_models)
          body.allow_overseas_models = values.overseas;
        if (Object.keys(body).length > 1) await onSubmit(body);
      })}
    >
      <FieldGroup>
        <Field data-invalid={Boolean(form.formState.errors.name)}>
          <FieldLabel htmlFor={`${id}-name`}>项目名称</FieldLabel>
          <Input
            id={`${id}-name`}
            disabled={disabled}
            aria-invalid={Boolean(form.formState.errors.name)}
            {...form.register("name", {
              validate: (value) =>
                (Boolean(value.trim()) &&
                  Array.from(value.trim()).length <= 50 &&
                  !value.includes("\0")) ||
                "项目名称须为 1–50 个字符。",
            })}
          />
          <FieldError>{form.formState.errors.name?.message}</FieldError>
        </Field>
        <Field data-invalid={Boolean(form.formState.errors.description)}>
          <FieldLabel htmlFor={`${id}-description`}>项目描述</FieldLabel>
          <Textarea
            id={`${id}-description`}
            disabled={disabled}
            aria-invalid={Boolean(form.formState.errors.description)}
            {...form.register("description", {
              validate: (value) =>
                !value.includes("\0") || "项目描述含有无效字符。",
            })}
          />
          <FieldError>{form.formState.errors.description?.message}</FieldError>
        </Field>
        <Field>
          <FieldLabel htmlFor={`${id}-preset`}>风格预设</FieldLabel>
          <Controller
            name="preset"
            control={form.control}
            render={({ field }) => (
              <Select
                disabled={disabled}
                value={field.value}
                onValueChange={field.onChange}
              >
                <SelectTrigger id={`${id}-preset`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="none">不使用预设</SelectItem>
                    {storedPresetMissing && (
                      <SelectItem value={project.style_preset_id!}>
                        保留已保存的预设
                      </SelectItem>
                    )}
                    {available.map((preset) => (
                      <SelectItem key={preset.id} value={preset.id}>
                        {preset.name}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            )}
          />
          {(hasMorePresets || presetsError) && (
            <Button
              type="button"
              variant="outline"
              disabled={disabled || presetsPending}
              onClick={onLoadPresets}
            >
              {presetsError ? "重试读取预设" : "加载更多预设"}
            </Button>
          )}
          {presetsError && (
            <FieldDescription>{presetsError.message}</FieldDescription>
          )}
        </Field>
        <Field orientation="horizontal">
          <FieldLabel htmlFor={`${id}-overseas`}>允许海外模型</FieldLabel>
          <Controller
            name="overseas"
            control={form.control}
            render={({ field }) => (
              <Checkbox
                id={`${id}-overseas`}
                disabled={disabled}
                checked={field.value}
                onCheckedChange={(value) => field.onChange(value === true)}
              />
            )}
          />
        </Field>
        <FieldDescription>
          画幅 {project.aspect_ratio} ·{" "}
          {project.style_type === "realistic" ? "写实" : "风格化"} ·{" "}
          {project.resolution}
        </FieldDescription>
        {project.status === "archived" && (
          <p className="text-sm text-muted-foreground">
            项目已归档，取消归档后可修改设置。
          </p>
        )}
        <Button type="submit" disabled={disabled || !form.formState.isDirty}>
          保存项目设置
        </Button>
      </FieldGroup>
    </form>
  );
}
