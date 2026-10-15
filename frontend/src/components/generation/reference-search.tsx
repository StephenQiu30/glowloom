"use client";
import { LoadingState } from "@/components/layout/loading-state";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Checkbox } from "@/components/ui/checkbox";
import {
  FieldSet,
  FieldLegend,
  Field,
  FieldLabel,
  FieldGroup,
} from "@/components/ui/field";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { useEffect, useState, useId } from "react";
import { listMediaAssets } from "@/api/media";
import { getBibleEntry, listBibleEntries } from "@/api/bible";
import {
  requireSubject,
  subjectText,
  type Subject,
} from "@/components/subjects/contracts";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandInput,
  CommandList,
  CommandGroup,
  CommandItem,
  CommandEmpty,
} from "@/components/ui/command";
import { flowError } from "./contracts";

type Candidate = {
  id: string;
  name: string;
  media?: API.AssetSummary;
  subject?: Subject;
};
type Kind = "image" | "video" | "audio" | "character" | "location" | "prop";
const kinds: Record<Kind, string> = {
  image: "图片",
  video: "视频",
  audio: "音频",
  character: "角色主体",
  location: "场景主体",
  prop: "物品主体",
};

/** Searches authorized pages only; query/context changes invalidate every outstanding page. */
export function ReferenceSearch({
  projectID,
  initialQuery,
  onSelect,
  onClose,
}: {
  projectID: string;
  initialQuery: string;
  onSelect: (value: { media?: API.AssetSummary; subject?: Subject }[]) => void;
  onClose: () => void;
}) {
  const formFieldId = useId();

  const [query, setQuery] = useState(initialQuery),
    [kind, setKind] = useState<Kind>("image");
  const [budget, setBudget] = useState(1),
    [retry, setRetry] = useState(0);
  const [result, setResult] = useState<{
    items: Candidate[];
    loading: boolean;
    more: boolean;
    error: string;
  }>({ items: [], loading: true, more: false, error: "" });
  const [selected, setSelected] = useState<Candidate[]>([]);
  useEffect(() => {
    const controller = new AbortController();
    const signal = controller.signal;
    const timer = setTimeout(() => {
      setResult({ items: [], loading: true, more: false, error: "" });
      setSelected([]);
      void (async () => {
        const items: Candidate[] = [];
        let cursor: string | undefined;
        const isMedia =
          kind === "image" || kind === "video" || kind === "audio";
        for (let page = 0; page < budget * 5; page++) {
          if (signal.aborted) return;
          if (isMedia) {
            const response = await listMediaAssets(
              { pid: projectID, kind, limit: 40, cursor },
              { signal },
            );
            if (
              !Array.isArray(response.items) ||
              response.items.some(
                (a) => !a.id || a.project_id !== projectID || a.kind !== kind,
              )
            )
              throw new Error("素材检索返回了不匹配的来源");
            for (const media of response.items)
              if (
                (media.file_name ?? "")
                  .toLocaleLowerCase()
                  .includes(query.toLocaleLowerCase())
              )
                items.push({
                  id: media.id!,
                  name: media.file_name ?? "未命名素材",
                  media,
                });
            cursor = response.next_cursor;
          } else {
            const response = await listBibleEntries(
              { pid: projectID, kind, limit: 40, cursor },
              { signal },
            );
            if (!Array.isArray(response.entries))
              throw new Error("主体检索回执不完整");
            const heads = response.entries.filter((entry) => {
              if (
                !entry.head?.id ||
                entry.head.project_id !== projectID ||
                entry.head.kind !== kind
              )
                throw new Error("主体检索范围不匹配");
              return true;
            });
            // Existing list summaries may omit the pinned version. Resolve in bounded batches.
            for (let i = 0; i < heads.length; i += 8) {
              const subjects = await Promise.all(
                heads
                  .slice(i, i + 8)
                  .map(async (entry) =>
                    requireSubject(
                      await getBibleEntry(
                        { pid: projectID, kind, id: entry.head!.id! },
                        { signal },
                      ),
                      projectID,
                      kind,
                      entry.head!.id,
                    ),
                  ),
              );
              for (const subject of subjects)
                if (
                  subjectText(subject)
                    .name?.toLocaleLowerCase()
                    .includes(query.toLocaleLowerCase())
                )
                  items.push({
                    id: subject.head.id,
                    name: subjectText(subject).name!,
                    subject,
                  });
              if (signal.aborted) return;
            }
            cursor = response.next_cursor;
          }
          if (!cursor) break;
        }
        if (!signal.aborted)
          setResult({ items, loading: false, more: !!cursor, error: "" });
      })().catch((error) => {
        if (!signal.aborted)
          setResult({
            items: [],
            loading: false,
            more: false,
            error: flowError(error),
          });
      });
    }, 180);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [projectID, query, kind, budget, retry]);
  return (
    <div
      className="flex flex-col gap-2 rounded-lg border p-3"
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          onClose();
        }
      }}
    >
      <NativeSelect
        aria-label="引用检索类型"
        className="w-full"
        value={kind}
        onChange={(event) => {
          setKind(event.target.value as Kind);
          setBudget(1);
          setResult({ items: [], loading: true, more: false, error: "" });
        }}
      >
        {Object.entries(kinds).map(([value, label]) => (
          <NativeSelectOption key={value} value={value}>
            {label}
          </NativeSelectOption>
        ))}
      </NativeSelect>
      {result.loading && <LoadingState>正在检索当前项目…</LoadingState>}
      {result.error && (
        <Alert variant="destructive">
          <AlertDescription>
            {result.error}
            <Button variant="ghost" onClick={() => setRetry((n) => n + 1)}>
              重试检索
            </Button>
          </AlertDescription>
        </Alert>
      )}
      <Command shouldFilter={false} loop>
        <CommandInput
          autoFocus
          aria-label="搜索项目引用"
          placeholder="输入完整文件名或主体名称"
          value={query}
          onValueChange={(value) => {
            setQuery(value);
            setBudget(1);
            setResult({ items: [], loading: true, more: false, error: "" });
          }}
        />
        <CommandList aria-label="项目引用候选">
          {!result.loading && !result.error && (
            <CommandEmpty>没有匹配的授权来源。</CommandEmpty>
          )}
          <CommandGroup>
            {result.items.map((item) => (
              <CommandItem
                value={item.id}
                key={item.id}
                onSelect={() => onSelect([item])}
              >
                {item.media && (
                  <span className="relative block size-12 shrink-0 overflow-hidden">
                    <ProjectMediaPreview
                      projectID={projectID}
                      id={item.id}
                      kind={item.media.kind}
                    />
                  </span>
                )}
                <span className="min-w-0 break-all">
                  {item.name}
                  <span className="block text-xs text-muted-foreground">
                    {kinds[kind]} · {item.id}
                  </span>
                </span>
              </CommandItem>
            ))}
          </CommandGroup>
        </CommandList>
      </Command>
      {!!result.items.length && (
        <FieldSet>
          <FieldLegend>按勾选顺序批量插入</FieldLegend>
          <FieldGroup>
            {result.items.map((item, fieldIndex0) => (
              <Field key={item.id} orientation="horizontal">
                <Checkbox
                  id={`${formFieldId}-field-1-${fieldIndex0}`}

                  checked={selected.some((s) => s.id === item.id)}
                  onCheckedChange={(event) =>
                    setSelected((previous) =>
                      event === true
                        ? [...previous, item]
                        : previous.filter((s) => s.id !== item.id),
                    )
                  }
                />
                <FieldLabel htmlFor={`${formFieldId}-field-1-${fieldIndex0}`}>
                  <span>{item.name}</span>
                </FieldLabel>
              </Field>
            ))}
            <Button
              disabled={!selected.length}
              onClick={() => onSelect(selected)}
            >
              插入所选 {selected.length} 项
            </Button>
          </FieldGroup>
        </FieldSet>
      )}
      {result.more && (
        <Button variant="ghost" onClick={() => setBudget((n) => n + 1)}>
          继续检索后续素材
        </Button>
      )}
      <Button variant="ghost" onClick={onClose}>
        返回正文
      </Button>
    </div>
  );
}
