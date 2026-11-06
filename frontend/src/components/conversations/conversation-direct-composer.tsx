"use client";

import { useState, type ReactNode } from "react";
import dynamic from "next/dynamic";
import { ArrowUp, Plus, SlidersHorizontal, Sparkles } from "lucide-react";
import {
  creationCapability,
  creationComposer,
  creationTypes,
  type CreationType,
} from "@/components/generation/creation-types";
import { CreationTypeFields } from "@/components/generation/creation-type-fields";
import { CreationEntryPrompt } from "@/components/generation/creation-entry-prompt";
import { ReferenceStack } from "@/components/generation/reference-stack";
import { ModelPicker } from "@/components/generation/model-picker";
import {
  emptyPromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import type { Project } from "@/components/projects/project";
import { Button } from "@/components/ui/button";
import { InputGroup, InputGroupAddon } from "@/components/ui/input-group";

const RichGeneration = dynamic(() =>
  import("@/components/generation/rich-generation").then(
    (m) => m.RichGeneration,
  ),
);

export function ConversationDirectComposer({
  type,
  contextKey,
  project,
  draft,
  draftID,
  onChange,
  onDraftReady,
  onLock,
  onCloseGuard,
  onChooseProject,
  typeControl,
  projectSetting,
}: {
  type: Exclude<CreationType, "agent">;
  contextKey: string;
  project: Project | null;
  draft?: PromptDraft;
  draftID?: string;
  onChange: (draft: PromptDraft) => void;
  onDraftReady: (id: string) => void;
  onLock: (locked: boolean) => void;
  onCloseGuard: (guard: (() => Promise<boolean>) | null) => void;
  onChooseProject: () => void;
  typeControl: ReactNode;
  projectSetting: ReactNode;
}) {
  const [empty] = useState(() => {
    const value = emptyPromptDraft();
    if (type === "audio") value.mode = "omni_audio";
    return value;
  });
  const [initialDraftID] = useState(draftID);
  const [initialComposer] = useState(() =>
    creationComposer(type, draft ?? empty),
  );
  const value = draft ?? empty;
  const mode = creationTypes.find((item) => item.id === type)!;
  if (project) {
    return (
      <RichGeneration
        projectID={project.id}
        seed={{
          project,
          capability: creationCapability(type),
          creationType: type,
          prompt: "",
          composer: initialComposer,
        }}
        draftID={initialDraftID}
        presentation="home"
        headerActions={typeControl}
        projectControl={projectSetting}
        onLock={onLock}
        onDraftReady={onDraftReady}
        onCloseGuard={onCloseGuard}
      />
    );
  }
  const acceptsReferences =
    type === "image" ||
    type === "video" ||
    (type === "audio" && value.mode === "omni_audio");
  const hasText = value.document.content.some((p) =>
    p.content?.some((node) => node.type === "text" && node.text.trim()),
  );
  return (
    <InputGroup variant="composer" className="creation-composer">
      <div className="creation-editor-body">
        {acceptsReferences && (
          <InputGroupAddon
            align="inline-start"
            className="creation-reference-tools"
          >
            <ReferenceStack
              entries={[]}
              addLabel={type === "video" ? "参考内容" : "上传参考"}
              onAdd={onChooseProject}
              onSelect={() => {}}
              onRemove={() => {}}
            />
          </InputGroupAddon>
        )}
        {(type === "avatar" || type === "motion") && (
          <InputGroupAddon
            align="inline-start"
            className="creation-reference-tools"
          >
            <Button type="button" variant="outline" onClick={onChooseProject}>
              <Plus data-icon="inline-start" />
              角色
            </Button>
            {type === "motion" && (
              <Button type="button" variant="outline" onClick={onChooseProject}>
                <Sparkles data-icon="inline-start" />
                动作
              </Button>
            )}
          </InputGroupAddon>
        )}
        {type === "motion" ? (
          <p className="text-sm text-muted-foreground">
            添加角色和动作后即可生成。
          </p>
        ) : (
          <CreationEntryPrompt
            label="提示词"
            draft={value}
            contextKey={contextKey}
            placeholder={
              type === "avatar"
                ? "说话内容"
                : type === "audio" && value.mode === "omni_audio"
                  ? "输入台词并描述声音，或上传参考音频。"
                  : mode.placeholder
            }
            onChange={onChange}
            onSubmit={() => {
              if (hasText) onChooseProject();
            }}
          />
        )}
      </div>
      {(type === "music" || type === "audio") && (
        <CreationTypeFields
          type={type}
          draft={value}
          onChange={onChange}
          onUnavailable={onChooseProject}
        />
      )}
      <InputGroupAddon align="block-end" className="creation-tools">
        <div className="creation-primary-tools">
          {typeControl}
          <ModelPicker
            models={[]}
            value={null}
            disabled={false}
            onSelect={() => {}}
            onChooseProject={onChooseProject}
          />
        </div>
        {(type === "image" || type === "video") && (
          <Button
            type="button"
            variant="ghost"
            size="toolbar"
            onClick={onChooseProject}
          >
            <SlidersHorizontal data-icon="inline-start" />
            生成参数
          </Button>
        )}
        {type === "avatar" && (
          <Button
            type="button"
            variant="ghost"
            size="toolbar"
            onClick={onChooseProject}
          >
            上传音频
          </Button>
        )}
        {projectSetting}
        <Button
          type="button"
          size="toolbar-icon"
          className="creation-submit ml-auto"
          disabled={!hasText}
          aria-label="生成"
          onClick={onChooseProject}
        >
          <ArrowUp />
        </Button>
      </InputGroupAddon>
    </InputGroup>
  );
}
