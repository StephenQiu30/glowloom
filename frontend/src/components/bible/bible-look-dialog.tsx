"use client";
import { useId, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useFieldArray, useForm, useWatch } from "react-hook-form";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { BibleWriteDialog } from "./bible-entry-dialog";
import { BibleReferencePicker } from "./bible-reference-picker";
import {
  bibleScopeKey,
  getBibleFormalEpisodes,
  getBibleFormalScenes,
} from "./bible-queries";
import {
  lookInputSchema,
  type BibleIdentity,
  type BibleLook,
} from "./bible-model";
import type { BibleWriter } from "./use-bible-writer";
import type { z } from "zod";

export type BibleLookFrame = {
  action: "look_create" | "look_update" | "references";
  id: string;
  revision: number;
  look?: BibleLook;
};
function ScopeRow({
  identity,
  index,
  episodeId,
  sceneKey,
  locked,
  onChange,
  onRemove,
}: {
  identity: BibleIdentity;
  index: number;
  episodeId: string;
  sceneKey?: string;
  locked: boolean;
  onChange: (episode: string, scene?: string) => void;
  onRemove: () => void;
}) {
  const id = useId();
  const episodes = useQuery({
    queryKey: [...bibleScopeKey(identity), "formal-episodes"],
    queryFn: ({ signal }) => getBibleFormalEpisodes(identity, signal),
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
  const scenes = useQuery({
    queryKey: [...bibleScopeKey(identity), "formal-scenes", episodeId],
    queryFn: ({ signal }) => getBibleFormalScenes(identity, episodeId, signal),
    enabled: Boolean(episodeId),
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
  return (
    <div className="space-y-2">
      <Label htmlFor={`${id}-episode`}>适用范围{index + 1}正式分集</Label>
      <select
        id={`${id}-episode`}
        value={episodeId}
        disabled={locked || episodes.isPending}
        onChange={(event) => onChange(event.target.value)}
        className="h-10 w-full rounded-md bg-muted px-2"
      >
        <option value="">选择已确认分集</option>
        {episodeId && !episodes.data?.some((item) => item.id === episodeId) && (
          <option value={episodeId}>原分集 · {episodeId}</option>
        )}
        {episodes.data?.map((item) => (
          <option key={item.id} value={item.id}>
            {item.seq_no} · {item.title} · {item.id}
          </option>
        ))}
      </select>
      <Label htmlFor={`${id}-scene`}>适用范围{index + 1}正式场景</Label>
      <select
        id={`${id}-scene`}
        value={sceneKey ?? ""}
        disabled={locked || !episodeId || scenes.isPending}
        onChange={(event) =>
          onChange(episodeId, event.target.value || undefined)
        }
        className="h-10 w-full rounded-md bg-muted px-2"
      >
        <option value="">整集</option>
        {sceneKey &&
          !scenes.data?.some((item) => item.scene_key === sceneKey) && (
            <option value={sceneKey}>原场景 · {sceneKey}</option>
          )}
        {scenes.data?.map((item) => (
          <option key={item.scene_key} value={item.scene_key}>
            {item.seq_no} · {item.heading} · {item.scene_key}
          </option>
        ))}
      </select>
      {(episodes.isError || scenes.isError) && (
        <div>
          <p role="alert">
            当前正式分集或已确认结构无法读取，原身份保留；当前草稿不代表身份仍可采纳。
          </p>
          <Button
            type="button"
            variant="outline"
            disabled={locked}
            onClick={() => {
              void episodes.refetch();
              if (episodeId) void scenes.refetch();
            }}
          >
            重新读取正式范围
          </Button>
        </div>
      )}
      <Button
        type="button"
        variant="ghost"
        disabled={locked}
        onClick={onRemove}
      >
        移除适用范围{index + 1}
      </Button>
    </div>
  );
}
export function BibleLookDialog({
  identity,
  frame,
  writer,
  onClose,
  onCloseAutoFocus,
}: {
  identity: BibleIdentity;
  frame: BibleLookFrame;
  writer: BibleWriter;
  onClose: () => void;
  onCloseAutoFocus?: (event: Event) => void;
}) {
  const id = useId(),
    [dirty, setDirty] = useState(false),
    [revision, setRevision] = useState(frame.revision),
    [scopePage, setScopePage] = useState(0),
    [error, setError] = useState<string>();
  const form = useForm<z.infer<typeof lookInputSchema>>({
    defaultValues: frame.look
      ? {
          name: frame.look.name,
          description: frame.look.description,
          default: frame.look.default,
          applies_to: frame.look.applies_to ?? [],
        }
      : { name: "", default: false, applies_to: [] },
  });
  const rows = useFieldArray({ control: form.control, name: "applies_to" });
  const scopes = useWatch({ control: form.control, name: "applies_to" });
  const isDefault = useWatch({ control: form.control, name: "default" });
  const pageCount = Math.max(1, Math.ceil(rows.fields.length / 25));
  const visiblePage = Math.min(scopePage, pageCount - 1);
  async function prepare() {
    const latest = writer.latest?.detail;
    if (
      !latest ||
      latest.head.deleted ||
      latest.head.redirect_id ||
      latest.current.kind !== "character" ||
      latest.head.id !== frame.id
    )
      return;
    if (
      frame.look &&
      !latest.current.character.looks.some((look) => look.id === frame.look?.id)
    ) {
      setError("原造型已不存在，当前不能替换原意图。");
      return;
    }
    if (await writer.discardRejected()) setRevision(latest.head.revision);
  }
  return (
    <BibleWriteDialog
      title={
        frame.action === "references"
          ? "编辑造型六类参考图"
          : frame.action === "look_create"
            ? "新建角色造型"
            : "编辑角色造型"
      }
      writer={writer}
      dirty={dirty}
      onClose={onClose}
      onCloseAutoFocus={onCloseAutoFocus}
      onPrepareLatest={prepare}
    >
      <p className="text-xs break-all">
        角色 {frame.id} · 造型 {frame.look?.id ?? "尚未创建"} · 冻结版本{" "}
        {revision}
      </p>
      {frame.action === "references" && frame.look ? (
        <BibleReferencePicker
          identity={identity}
          initial={frame.look.references ?? []}
          locked={writer.locked}
          onDirty={() => setDirty(true)}
          onSave={async (references) =>
            writer.submit({
              action: "references",
              kind: "character",
              id: frame.id,
              lookId: frame.look!.id,
              body: { expected_revision: revision, references },
            })
          }
        />
      ) : (
        <form
          className="space-y-4"
          onChange={() => {
            setDirty(true);
            setError(undefined);
          }}
          onSubmit={form.handleSubmit(async (value) => {
            try {
              const look = lookInputSchema.parse(value);
              await writer.submit({
                action:
                  frame.action === "look_create"
                    ? "look_create"
                    : "look_update",
                kind: "character",
                id: frame.id,
                ...(frame.look ? { lookId: frame.look.id } : {}),
                body: { expected_revision: revision, look },
              });
            } catch {
              setError("造型名称、完整范围或默认标记无效，请保留原草稿。");
            }
          })}
        >
          <fieldset disabled={writer.locked} className="space-y-4">
            <div>
              <Label htmlFor={`${id}-name`}>造型名称</Label>
              <Textarea id={`${id}-name`} {...form.register("name")} />
            </div>
            <div>
              <Label htmlFor={`${id}-description`}>造型说明</Label>
              <Textarea
                id={`${id}-description`}
                {...form.register("description", {
                  setValueAs: (value: string) => value || undefined,
                })}
              />
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id={`${id}-default`}
                checked={isDefault}
                onCheckedChange={(checked) => {
                  form.setValue("default", checked === true);
                  setDirty(true);
                }}
              />
              <Label htmlFor={`${id}-default`}>设为默认造型</Label>
            </div>
            <p>
              全角色始终恰好一个默认造型。分集和场景只能选择当前正式身份；空范围表示角色通用造型。
            </p>
            {rows.fields
              .slice(visiblePage * 25, (visiblePage + 1) * 25)
              .map((scope, offset) => {
                const index = visiblePage * 25 + offset;
                return (
                  <ScopeRow
                    key={scope.id}
                    identity={identity}
                    index={index}
                    episodeId={scopes?.[index]?.episode_id ?? ""}
                    sceneKey={scopes?.[index]?.scene_key}
                    locked={writer.locked}
                    onChange={(episode_id, scene_key) => {
                      form.setValue(`applies_to.${index}`, {
                        episode_id,
                        ...(scene_key ? { scene_key } : {}),
                      });
                      setDirty(true);
                    }}
                    onRemove={() => {
                      rows.remove(index);
                      setDirty(true);
                    }}
                  />
                );
              })}
            {pageCount > 1 ? (
              <nav
                aria-label="正式适用范围分页"
                className="flex flex-wrap items-center gap-2"
              >
                <Button
                  type="button"
                  variant="outline"
                  disabled={visiblePage === 0}
                  onClick={() => setScopePage(visiblePage - 1)}
                >
                  上一页适用范围
                </Button>
                <span role="status">
                  第{visiblePage + 1}/{pageCount}页 · 共{rows.fields.length}
                  个完整范围
                </span>
                <Button
                  type="button"
                  variant="outline"
                  disabled={visiblePage + 1 >= pageCount}
                  onClick={() => setScopePage(visiblePage + 1)}
                >
                  下一页适用范围
                </Button>
              </nav>
            ) : null}
            <Button
              type="button"
              variant="outline"
              disabled={rows.fields.length >= 2500}
              onClick={() => {
                setScopePage(Math.floor(rows.fields.length / 25));
                rows.append({ episode_id: "" });
                setDirty(true);
              }}
            >
              添加正式适用范围
            </Button>
            <Button type="submit">保存完整造型</Button>
          </fieldset>
        </form>
      )}
      {error && <p role="alert">{error}</p>}
    </BibleWriteDialog>
  );
}
