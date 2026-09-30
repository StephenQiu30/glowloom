"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { LoaderCircle, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ApiError } from "@/lib/request";
import {
  creationBody,
  initialProjectDraft,
  matchesPreset,
  styleSubtypes,
  updateProjectDraft,
  type CreationBody,
  type ProjectDraft,
  type StylePreset,
} from "./creation";

export function projectError(error: unknown) {
  if (!(error instanceof ApiError)) return "服务暂时不可用，请稍后重试。";
  if (error.status === 0 || error.status >= 500)
    return "创建结果尚未确认。请保持配置不变并重试，以查询同一次创建结果。";
  if (error.status === 400 || error.status === 422)
    return "项目配置未通过服务端校验，请检查名称、画幅、风格及预设。";
  if (error.status === 409)
    return "创建请求与服务端记录冲突，请检查配置后重试。";
  return error.message;
}

type Props = {
  authenticationRequired?: boolean;
  onSubmit: (body: CreationBody, key: string) => Promise<{ id: string }>;
  onCreated: (id: string) => void;
  onOpenChange?: (open: boolean) => void;
  presets: readonly StylePreset[];
  presetsPending: boolean;
  presetsError?: Error | null;
  hasMorePresets?: boolean;
  loadingMorePresets?: boolean;
  onLoadMorePresets?: () => void;
  onRetryPresets?: () => void;
};
export function CreateProjectDialog({
  authenticationRequired = false,
  onSubmit,
  onCreated,
  onOpenChange,
  presets,
  presetsPending,
  presetsError,
  hasMorePresets,
  loadingMorePresets,
  onLoadMorePresets,
  onRetryPresets,
}: Props) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<ProjectDraft>({ ...initialProjectDraft });
  const [errors, setErrors] = useState<
    Partial<Record<keyof ProjectDraft, string>>
  >({});
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [leave, setLeave] = useState<{ run: () => void } | null>(null);
  const attempt = useRef<string | null>(null);
  const inFlight = useRef(false);
  const leaving = useRef(false);
  const dirty =
    open &&
    (JSON.stringify(draft) !== JSON.stringify(initialProjectDraft) ||
      Boolean(error) ||
      pending);
  const editingDisabled = pending || authenticationRequired;
  const availablePresets = presets.filter((preset) =>
    matchesPreset(draft, preset),
  );
  const changeOpen = useCallback(
    (value: boolean) => {
      setOpen(value);
      onOpenChange?.(value);
    },
    [onOpenChange],
  );
  const requestLeave = useCallback(
    (run: () => void) => {
      if (dirty && !leaving.current) setLeave({ run });
      else run();
    },
    [dirty],
  );
  const clear = () => {
    changeOpen(false);
    setDraft({ ...initialProjectDraft });
    setErrors({});
    setError(null);
    attempt.current = null;
  };
  const update = (patch: Partial<ProjectDraft>) => {
    if (inFlight.current || authenticationRequired) return;
    if (
      Object.entries(patch).every(
        ([key, value]) => draft[key as keyof ProjectDraft] === value,
      )
    )
      return;
    setDraft((current) => updateProjectDraft(current, patch));
    setError(null);
    setErrors({});
    attempt.current = null;
  };
  useEffect(() => {
    if (!open) return;
    const savedHref = window.location.href;
    const warn = (event: BeforeUnloadEvent) => {
      if (dirty && !leaving.current) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    const click = (event: MouseEvent) => {
      if (
        !dirty ||
        leaving.current ||
        event.button ||
        event.ctrlKey ||
        event.metaKey ||
        event.shiftKey ||
        event.altKey
      )
        return;
      const anchor =
        event.target instanceof Element
          ? event.target.closest("a[href]")
          : null;
      if (
        !(anchor instanceof HTMLAnchorElement) ||
        anchor.target === "_blank" ||
        anchor.hasAttribute("download") ||
        anchor.getAttribute("href")?.startsWith("#") ||
        anchor.href === window.location.href
      )
        return;
      event.preventDefault();
      event.stopPropagation();
      const target = anchor.href;
      requestLeave(() => {
        leaving.current = true;
        window.location.assign(target);
      });
    };
    const back = (event: PopStateEvent) => {
      if (!dirty || leaving.current) return;
      const target = window.location.href;
      event.stopImmediatePropagation();
      window.history.pushState(window.history.state, "", savedHref);
      requestLeave(() => {
        leaving.current = true;
        window.location.assign(target);
      });
    };
    window.addEventListener("beforeunload", warn);
    document.addEventListener("click", click, true);
    window.addEventListener("popstate", back, true);
    return () => {
      window.removeEventListener("beforeunload", warn);
      document.removeEventListener("click", click, true);
      window.removeEventListener("popstate", back, true);
    };
  }, [dirty, open, requestLeave]);
  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (inFlight.current || authenticationRequired) return;
    const result = creationBody(draft, presets);
    setErrors(result.errors);
    if (!result.body) return;
    const key = attempt.current ?? crypto.randomUUID();
    attempt.current = key;
    inFlight.current = true;
    setPending(true);
    setError(null);
    let created: { id: string };
    try {
      created = await onSubmit(result.body, key);
    } catch (failure) {
      setError(failure);
      setPending(false);
      inFlight.current = false;
      return;
    }
    inFlight.current = false;
    setPending(false);
    leaving.current = true;
    setLeave(null);
    clear();
    onCreated(created.id);
  }
  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (value) {
            leaving.current = false;
            changeOpen(true);
          } else requestLeave(clear);
        }}
      >
        <DialogTrigger asChild>
          <Button disabled={authenticationRequired}>
            <Plus data-icon="inline-start" />
            新建项目
          </Button>
        </DialogTrigger>
        <DialogContent
          className="max-h-[90dvh] overflow-y-auto ring-0 sm:max-w-xl"
          showCloseButton={false}
        >
          <DialogHeader>
            <DialogTitle>新建项目</DialogTitle>
            <DialogDescription>
              设定作品规格。画幅与风格类型创建后不可修改。
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={submit} noValidate className="flex flex-col gap-6">
            <FieldGroup>
              <Field
                data-invalid={Boolean(errors.name)}
                data-disabled={editingDisabled}
              >
                <FieldLabel htmlFor="new-project-name">项目名称</FieldLabel>
                <Input
                  id="new-project-name"
                  value={draft.name}
                  onChange={(event) => update({ name: event.target.value })}
                  disabled={editingDisabled}
                  aria-invalid={Boolean(errors.name)}
                  aria-describedby="new-project-name-help"
                  autoComplete="off"
                  autoFocus
                />
                <FieldDescription id="new-project-name-help">
                  1–50 个字符，可与已有项目重名。
                </FieldDescription>
                <FieldError>{errors.name}</FieldError>
              </Field>
              <Field data-disabled={editingDisabled}>
                <FieldLabel htmlFor="new-project-description">
                  描述（可选）
                </FieldLabel>
                <Textarea
                  id="new-project-description"
                  value={draft.description}
                  onChange={(event) =>
                    update({ description: event.target.value })
                  }
                  disabled={editingDisabled}
                  placeholder="这个故事想讲什么？"
                />
              </Field>
              <Field data-disabled={editingDisabled}>
                <FieldTitle id="new-project-aspect-label">画幅</FieldTitle>
                <ToggleGroup
                  type="single"
                  value={draft.aspect_ratio}
                  onValueChange={(value) => {
                    if (value === "9:16" || value === "16:9")
                      update({ aspect_ratio: value });
                  }}
                  disabled={editingDisabled}
                  aria-labelledby="new-project-aspect-label"
                >
                  <ToggleGroupItem value="16:9">16:9 横屏</ToggleGroupItem>
                  <ToggleGroupItem value="9:16">9:16 竖屏</ToggleGroupItem>
                </ToggleGroup>
              </Field>
              <Field data-disabled={editingDisabled}>
                <FieldTitle id="new-project-style-label">风格类型</FieldTitle>
                <ToggleGroup
                  type="single"
                  value={draft.style_type}
                  onValueChange={(value) => {
                    if (value === "realistic" || value === "stylized")
                      update({ style_type: value });
                  }}
                  disabled={editingDisabled}
                  aria-labelledby="new-project-style-label"
                >
                  <ToggleGroupItem value="realistic">写实</ToggleGroupItem>
                  <ToggleGroupItem value="stylized">风格化</ToggleGroupItem>
                </ToggleGroup>
              </Field>
              {draft.style_type === "stylized" && (
                <Field
                  data-invalid={Boolean(errors.style_subtype)}
                  data-disabled={editingDisabled}
                >
                  <FieldTitle id="new-project-subtype-label">子风格</FieldTitle>
                  <ToggleGroup
                    type="single"
                    value={draft.style_subtype}
                    onValueChange={(value) => {
                      const subtype = styleSubtypes.find(
                        ([item]) => item === value,
                      )?.[0];
                      if (subtype) update({ style_subtype: subtype });
                    }}
                    disabled={editingDisabled}
                    aria-labelledby="new-project-subtype-label"
                    aria-invalid={Boolean(errors.style_subtype)}
                    className="flex-wrap"
                  >
                    {styleSubtypes.map(([value, label]) => (
                      <ToggleGroupItem key={value} value={value}>
                        {label}
                      </ToggleGroupItem>
                    ))}
                  </ToggleGroup>
                  <FieldError>{errors.style_subtype}</FieldError>
                </Field>
              )}
              <Field
                data-invalid={Boolean(errors.style_preset_id)}
                data-disabled={editingDisabled}
              >
                <FieldLabel htmlFor="new-project-preset">
                  风格预设（可选）
                </FieldLabel>
                <Select
                  value={draft.style_preset_id || "none"}
                  onValueChange={(value) =>
                    update({ style_preset_id: value === "none" ? "" : value })
                  }
                  disabled={editingDisabled || presetsPending}
                >
                  <SelectTrigger
                    id="new-project-preset"
                    aria-invalid={Boolean(errors.style_preset_id)}
                  >
                    <SelectValue placeholder="不使用预设" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="none">不使用预设</SelectItem>
                      {availablePresets.map((preset) => (
                        <SelectItem key={preset.id} value={preset.id}>
                          {preset.name}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <FieldDescription>
                  {authenticationRequired
                    ? "请重新登录以读取组织预设。"
                    : presetsPending
                      ? "正在读取组织预设…"
                      : presetsError
                        ? "预设暂时不可用，仍可不使用预设创建。"
                        : availablePresets.length
                          ? "来自当前组织的可用预设。"
                          : "当前已读取的预设中没有匹配项，可不使用预设。"}
                </FieldDescription>
                {presetsError && (
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={onRetryPresets}
                    disabled={editingDisabled}
                  >
                    重试读取预设
                  </Button>
                )}
                {hasMorePresets && (
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={onLoadMorePresets}
                    disabled={editingDisabled || loadingMorePresets}
                  >
                    {loadingMorePresets ? "正在读取…" : "读取更多预设"}
                  </Button>
                )}
                <FieldError>{errors.style_preset_id}</FieldError>
              </Field>
            </FieldGroup>
            <p className="text-sm text-muted-foreground">
              输出为 1080p，境外模型默认关闭。新项目预算为
              ¥0，设置预算后才能开始生成。
            </p>
            {authenticationRequired && (
              <Alert variant="destructive" className="border-0 bg-muted/40">
                <AlertTitle>会话需要重新确认</AlertTitle>
                <AlertDescription>
                  配置已保留，创建已暂停。
                  <Link href="/login?returnTo=%2Fprojects">重新登录</Link>
                </AlertDescription>
              </Alert>
            )}
            {Boolean(error) && (
              <Alert variant="destructive" className="border-0 bg-muted/40">
                <AlertTitle>项目尚未确认创建</AlertTitle>
                <AlertDescription>
                  {projectError(error)}
                  {error instanceof ApiError && error.requestId && (
                    <p>请求编号：{error.requestId}</p>
                  )}
                  {!authenticationRequired &&
                    error instanceof ApiError &&
                    (error.status === 401 ||
                      error.code === "must_change_password") && (
                      <Link href="/login?returnTo=%2Fprojects">重新登录</Link>
                    )}
                </AlertDescription>
              </Alert>
            )}
            <DialogFooter className="border-0 bg-transparent">
              <Button
                type="button"
                variant="ghost"
                onClick={() => requestLeave(clear)}
              >
                取消
              </Button>
              <Button type="submit" disabled={editingDisabled}>
                {pending && (
                  <LoaderCircle
                    data-icon="inline-start"
                    className="animate-spin"
                  />
                )}
                {pending ? "正在创建…" : error ? "重试创建" : "创建并进入画布"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog
        open={Boolean(leave)}
        onOpenChange={(value) => {
          if (!value) setLeave(null);
        }}
      >
        <DialogContent className="ring-0">
          <DialogHeader>
            <DialogTitle>离开项目创建？</DialogTitle>
            <DialogDescription>
              {pending
                ? "创建请求正在处理，请等待结果后再离开。"
                : "离开将丢失当前填写的配置。若创建结果尚未确认，项目可能已经创建；重试当前配置可确认同一次请求的结果。"}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="border-0">
            <Button variant="ghost" onClick={() => setLeave(null)}>
              继续填写
            </Button>
            <Button
              disabled={pending}
              onClick={() => {
                const action = leave?.run;
                setLeave(null);
                leaving.current = true;
                action?.();
              }}
            >
              放弃并离开
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
