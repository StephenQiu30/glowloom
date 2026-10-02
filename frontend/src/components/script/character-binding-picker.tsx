"use client";
import { useEffect, useId, useRef, useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import type { ScriptScope } from "./source-intent";
import {
  characterBindingKey,
  listCharacterBindings,
  resolveCharacterBinding,
  type CharacterBinding,
} from "./character-binding-queries";

type Props = {
  scope: ScriptScope;
  label: string;
  binding?: CharacterBinding;
  locked: boolean;
  onChange: (binding: CharacterBinding | undefined) => void;
  onPendingChange: (pending: boolean) => void;
};
export function CharacterBindingPicker(props: Props) {
  const { origin, actorId, orgId, projectId } = props.scope;
  return (
    <ScopedCharacterPicker
      key={JSON.stringify([origin, actorId, orgId, projectId])}
      {...props}
    />
  );
}
function ScopedCharacterPicker({
  scope,
  label,
  binding,
  locked,
  onChange,
  onPendingChange,
}: Props) {
  const [open, setOpen] = useState(false),
    [pending, setPending] = useState(false),
    [error, setError] = useState<string>();
  const [selected, setSelected] =
    useState<Awaited<ReturnType<typeof resolveCharacterBinding>>>();
  const active = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      if (active.current) {
        active.current.abort();
        active.current = null;
        onPendingChange(false);
      }
    },
    [onPendingChange],
  );
  async function select(id: string) {
    if (!id || locked || active.current) return;
    const controller = new AbortController();
    active.current = controller;
    setPending(true);
    onPendingChange(true);
    setError(undefined);
    try {
      const resolved = await resolveCharacterBinding(
        scope,
        id,
        controller.signal,
      );
      if (controller.signal.aborted) return;
      onChange({
        character_id: resolved.character_id,
        character_version_id: resolved.character_version_id,
      });
      setSelected(resolved);
    } catch (cause) {
      if (!controller.signal.aborted)
        setError(
          cause instanceof Error
            ? cause.message
            : "确认角色无法读取，原绑定保持不变。",
        );
    } finally {
      if (active.current !== controller) return;
      active.current = null;
      if (!controller.signal.aborted) {
        setPending(false);
        onPendingChange(false);
      }
    }
  }
  return (
    <section
      aria-label={`${label}正式角色绑定`}
      className="flex min-w-0 flex-col gap-2"
    >
      <p className="text-xs wrap-anywhere">
        {binding ? (
          <>
            原角色 {binding.character_id} · 原不变版本{" "}
            {binding.character_version_id ?? "尚未冻结"}
          </>
        ) : (
          "未绑定正式角色"
        )}
      </p>
      <p className="text-xs text-muted-foreground">
        保留原绑定不更新历史版本。明确选择时重新核验当前确认版本；清空同时移除角色与版本。
      </p>
      {selected &&
        binding?.character_id === selected.character_id &&
        binding.character_version_id === selected.character_version_id && (
          <p role="status" className="text-sm wrap-anywhere">
            已明确选择确认角色：{selected.name}
            {selected.aliases.length > 0 &&
              ` · 别名：${selected.aliases.join("、")}`}
          </p>
        )}
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={locked || pending}
          aria-expanded={open}
          onClick={() => setOpen(!open)}
        >
          {label}
          {open ? "收起正式角色" : "选择正式角色"}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={locked || pending || !binding}
          onClick={() => {
            onChange(undefined);
            setError(undefined);
            setSelected(undefined);
          }}
        >
          {label}清除角色绑定
        </Button>
      </div>
      {open && !locked && (
        <CharacterOptions
          scope={scope}
          label={label}
          disabled={pending}
          onSelect={select}
        />
      )}
      {pending && <p role="status">正在核验确切角色与确认版本…</p>}
      {error && (
        <p role="alert" className="text-sm wrap-anywhere text-destructive">
          {error} 原角色与历史版本保持不变。
        </p>
      )}
    </section>
  );
}
function CharacterOptions({
  scope,
  label,
  disabled,
  onSelect,
}: {
  scope: ScriptScope;
  label: string;
  disabled: boolean;
  onSelect: (id: string) => Promise<void>;
}) {
  const id = useId();
  const page = useInfiniteQuery({
    queryKey: characterBindingKey(scope),
    queryFn: ({ pageParam, signal }) =>
      listCharacterBindings(scope, pageParam, signal),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (value) => value.next_cursor,
    staleTime: 0,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  });
  if (!page.isSuccess || !page.isFetchedAfterMount)
    return (
      <div className="flex flex-col gap-2">
        <p role={page.isError ? "alert" : "status"}>
          {page.isError
            ? "当前项目角色授权无法核验，原绑定保持不变。"
            : "正在核验当前身份与正式角色…"}
        </p>
        {page.isError && (
          <Button
            type="button"
            variant="outline"
            onClick={() => void page.refetch()}
          >
            重新读取确认角色
          </Button>
        )}
      </div>
    );
  const choices = Array.from(
    new Map(
      page.data.pages.flatMap((p) => p.items).map((item) => [item.id, item]),
    ).values(),
  );
  return (
    <Field data-disabled={disabled}>
      <FieldLabel htmlFor={id}>{label}正式确认角色</FieldLabel>
      <select
        id={id}
        className="h-9 w-full min-w-0 rounded-md border bg-background px-2"
        value=""
        disabled={disabled}
        onChange={(event) => void onSelect(event.target.value)}
      >
        <option value="">明确选择角色与确认版本</option>
        {choices.map((item) => (
          <option key={item.id} value={item.id}>
            {item.name}
            {item.aliases.length > 0 &&
              ` · 别名：${item.aliases.join("、")}`} · {item.id} · 确认{" "}
            {item.confirmedVersionId}
          </option>
        ))}
      </select>
      <FieldDescription>
        当前已读取 {choices.length}{" "}
        个确认角色，未确认或已合并身份不可选；允许同名角色按身份区分。
      </FieldDescription>
      {page.isFetchNextPageError && (
        <p role="alert">后续页读取失败，已读取的原绑定保持不变。</p>
      )}
      {page.hasNextPage && (
        <Button
          type="button"
          variant="outline"
          disabled={disabled || page.isFetching}
          onClick={() => void page.fetchNextPage()}
        >
          读取更多确认角色
        </Button>
      )}
    </Field>
  );
}
