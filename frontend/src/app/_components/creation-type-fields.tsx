"use client";

import { useId } from "react";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import type { PromptDraft } from "@/components/generation/prompt-document";
import { creationTypes, type CreationType } from "./creation-types";

/** Type-owned preparation fields; unavailable services never submit generic tasks. */
export function CreationTypeFields({
  type,
  draft,
  onChange,
}: {
  type: CreationType;
  draft: PromptDraft;
  onChange: (draft: PromptDraft) => void;
  onUnavailable: (feature: string) => void;
}) {
  const id = useId();
  const update = (params: PromptDraft["params"]) =>
    onChange({ ...draft, params: { ...draft.params, ...params } });
  if (type === "image" || type === "video") return null;
  return (
    <div
      className="flex w-full flex-col gap-3 px-4 pb-3"
      aria-label={`${creationTypes.find((item) => item.id === type)?.label}专属选项`}
    >
      {type === "agent" && (
        <p className="text-xs text-muted-foreground">
          选择项目后开始图片创作会话，规划与生成分别确认积分。
        </p>
      )}
      {type === "music" && (
        <>
          <ToggleGroup
            type="single"
            value={String(draft.params.music_mode ?? "song")}
            onValueChange={(value) => value && update({ music_mode: value })}
            aria-label="音乐模式"
          >
            <ToggleGroupItem value="song">歌曲</ToggleGroupItem>
            <ToggleGroupItem value="instrumental">纯音乐</ToggleGroupItem>
          </ToggleGroup>
          <Field>
            <FieldLabel htmlFor={`${id}-duration`}>时长（秒）</FieldLabel>
            <Input
              id={`${id}-duration`}
              type="number"
              min={30}
              max={draft.params.music_mode === "instrumental" ? 120 : 240}
              step={1}
              value={Number(draft.params.duration_ms ?? 60000) / 1000}
              onChange={(event) =>
                update({ duration_ms: Number(event.target.value) * 1000 })
              }
            />
          </Field>
        </>
      )}
      {type === "audio" && (
        <>
          <ToggleGroup
            type="single"
            value={draft.mode === "omni_audio" ? "omni_audio" : "text2audio"}
            aria-label="音频模式"
            onValueChange={(mode) =>
              mode &&
              onChange({ ...draft, mode, modelVersion: null, params: {} })
            }
          >
            <ToggleGroupItem value="text2audio">文本朗读</ToggleGroupItem>
            <ToggleGroupItem value="omni_audio">全能配音</ToggleGroupItem>
          </ToggleGroup>
          {draft.mode === "omni_audio" && (
            <p className="text-xs text-muted-foreground">
              用文字描述角色、台词与时间段，最长 120 秒。可参考最多 3 条音频或 1
              张图片。
            </p>
          )}
        </>
      )}
      {type === "avatar" && (
        <p className="text-sm text-muted-foreground">
          选择项目后添加角色图片与讲话音频，支持 720p 和 1080p。音频需少于 60
          秒。
        </p>
      )}
      {type === "motion" && (
        <p className="text-sm text-muted-foreground">
          选择项目后添加角色图片与动作视频。视频最长 30 秒，支持裁剪首秒。
        </p>
      )}
    </div>
  );
}
