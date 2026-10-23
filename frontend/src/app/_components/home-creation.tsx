"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { useRef, useState, type ReactNode } from "react";
import { ArrowUp, AtSign, Plus } from "lucide-react";
import { cn } from "cn";
import { useOptionalSession } from "@/components/auth/session";
import { useLoginDialog } from "@/components/layout/basic-layout";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { UnavailableDialog } from "@/components/availability/unavailable";
import { NewCanvasButton } from "@/components/projects/new-canvas-button";
import { ProjectPicker } from "@/components/projects/project-picker";
import type { Project } from "@/components/projects/project";
import type { GenerationSeed } from "@/components/generation/generation-dialog";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Field, FieldGroup } from "@/components/ui/field";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { HomePrompt } from "./home-prompt";

const RichGeneration = dynamic(
  () =>
    import("@/components/generation/rich-generation").then(
      (module) => module.RichGeneration,
    ),
  {
    loading: () => (
      <Skeleton
        className="h-64 w-full rounded-xl"
        aria-label="正在加载创作编辑器"
      />
    ),
  },
);
const creationTypes = [
  {
    id: "agent",
    label: "Agent 模式",
    placeholder: "描述你的创作目标，或添加参考素材。",
  },
  {
    id: "image",
    label: "图片生成",
    placeholder: "描述你的想法，或添加参考素材。",
  },
  {
    id: "video",
    label: "视频生成",
    placeholder: "描述画面、动作与镜头，或添加参考素材。",
  },
  { id: "music", label: "音乐生成", placeholder: "描述曲风、情绪与节奏。" },
  {
    id: "audio",
    label: "音频生成",
    placeholder: "输入台词，描述音色与表达方式。",
  },
  {
    id: "avatar",
    label: "数字人",
    placeholder: "描述数字人的形象、台词与表演。",
  },
  {
    id: "motion",
    label: "动作模仿",
    placeholder: "描述你希望主体模仿的动作。",
  },
] as const;
type CreationType = (typeof creationTypes)[number]["id"];
type Workspace = {
  project: Project;
  type: "image" | "video";
  seed: GenerationSeed;
  draftID?: string;
};

/** Page-owned entry; generation, references and pricing remain owned by RichGeneration. */
export function HomeCreation({
  prompt,
  onPromptChange,
  recovery,
}: {
  prompt: string;
  onPromptChange: (value: string) => void;
  recovery?: ReactNode;
}) {
  const session = useOptionalSession();
  const login = useLoginDialog();
  const [mode, setMode] = useState("generate");
  const [type, setType] = useState<CreationType>("image");
  const [project, setProject] = useState<Project | null>(null);
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [projectOpen, setProjectOpen] = useState(false);
  const [locked, setLocked] = useState(false);
  const [switching, setSwitching] = useState(false);
  const [error, setError] = useState("");
  const [feature, setFeature] = useState<string | null>(null);
  const guard = useRef<(() => Promise<boolean>) | null>(null);
  const switchFlight = useRef(false);
  const drafts = useRef(new Map<string, string>());
  const activeType = creationTypes.find((item) => item.id === type)!;
  const supported = type === "image" || type === "video";

  async function select(nextType: CreationType, nextProject = project) {
    if (locked || switchFlight.current) return;
    switchFlight.current = true;
    setSwitching(true);
    try {
      if (guard.current && !(await guard.current())) {
        setError("当前草稿尚未确认保存，请处理保存或未决生成后再切换。");
        return;
      }
      setError("");
      setType(nextType);
      setProject(nextProject);
      setWorkspace(
        nextProject && (nextType === "image" || nextType === "video")
          ? {
              project: nextProject,
              type: nextType,
              seed: { project: nextProject, capability: nextType, prompt },
              draftID: drafts.current.get(`${nextProject.id}/${nextType}`),
            }
          : null,
      );
    } catch {
      setError("草稿保存尚未确认，当前输入已保留，请重试。");
    } finally {
      switchFlight.current = false;
      setSwitching(false);
    }
  }
  function prepare() {
    if (!supported) {
      setFeature(activeType.label);
      return;
    }
    if (!session) {
      login();
      return;
    }
    setProjectOpen(true);
  }
  const projectControl = session ? (
    <ProjectPicker
      compact
      value={project}
      disabled={locked || switching}
      open={projectOpen}
      onOpenChange={setProjectOpen}
      onChange={(next) => void select(type, next)}
    />
  ) : null;
  const typeControl = (
    <Select
      value={type}
      disabled={locked || switching}
      onValueChange={(value) => void select(value as CreationType)}
    >
      <SelectTrigger
        aria-label="创作类型"
        className="w-auto border-0 bg-transparent shadow-none"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {creationTypes.map((item) => (
          <SelectItem key={item.id} value={item.id}>
            {item.label}
            {!["image", "video"].includes(item.id) ? " · 暂未开放" : ""}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );

  return (
    <section className="creation-hero" aria-label="开始创作">
      <h1>你好，今天想要创作什么？</h1>
      {recovery}
      {error ? <FeedbackToast message={error} /> : null}
      <ToggleGroup
        type="single"
        variant="pill"
        size="hero"
        value={mode}
        disabled={locked || switching}
        onValueChange={(value) => {
          if (value) setMode(value);
        }}
        className="creation-toggle"
        aria-label="创作模式"
      >
        <ToggleGroupItem value="generate">生成</ToggleGroupItem>
        <ToggleGroupItem value="canvas">画布</ToggleGroupItem>
      </ToggleGroup>
      <div hidden={mode !== "generate"}>
        <div className="creation-tabs mt-4">
          {/* One active panel, owned by the selected type; switching first flushes its saved draft. */}
          <div>
            {workspace ? (
              <RichGeneration
                key={`${workspace.project.id}/${workspace.type}`}
                projectID={workspace.project.id}
                seed={workspace.seed}
                draftID={workspace.draftID}
                presentation="home"
                headerActions={
                  <>
                    {typeControl}
                    {projectControl}
                  </>
                }
                onLock={setLocked}
                onDraftReady={(id) =>
                  drafts.current.set(
                    `${workspace.project.id}/${workspace.type}`,
                    id,
                  )
                }
                onCloseGuard={(next) => {
                  guard.current = next;
                }}
              />
            ) : (
              <form
                onSubmit={(event) => {
                  event.preventDefault();
                  if (prompt.trim() || !supported) prepare();
                }}
              >
                <FieldGroup>
                  <Field>
                    <InputGroup
                      variant="composer"
                      className="creation-composer"
                    >
                      <InputGroupAddon
                        align="block-start"
                        className="creation-reference-tools"
                      >
                        <Button
                          type="button"
                          variant="secondary"
                          size="icon"
                          aria-label="添加参考素材"
                          onClick={prepare}
                        >
                          <Plus data-icon="inline-start" />
                        </Button>
                        <div className="ml-auto flex min-w-0 items-center gap-1">
                          {projectControl}
                          <Button
                            type="button"
                            variant="ghost"
                            onClick={() => setFeature("更多创作工具")}
                          >
                            更多工具
                          </Button>
                        </div>
                      </InputGroupAddon>
                      <HomePrompt
                        value={prompt}
                        onChange={onPromptChange}
                        placeholder={activeType.placeholder}
                        onSubmit={() => {
                          if (prompt.trim() || !supported) prepare();
                        }}
                      />
                      <InputGroupAddon
                        align="block-end"
                        className="creation-tools"
                      >
                        {typeControl}
                        <Button type="button" variant="ghost" onClick={prepare}>
                          选择模型
                        </Button>
                        <Button type="button" variant="ghost" onClick={prepare}>
                          参数
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          aria-label="添加主体或素材引用"
                          onClick={prepare}
                        >
                          <AtSign />
                        </Button>
                        <span className="creation-credit-hint">
                          {supported ? "提交前确认积分" : "暂未开放"}
                        </span>
                        <Button
                          type="submit"
                          size="icon"
                          disabled={supported && !prompt.trim()}
                          aria-label={
                            supported
                              ? "开始生成"
                              : `${activeType.label}暂未开放`
                          }
                        >
                          <ArrowUp />
                        </Button>
                      </InputGroupAddon>
                    </InputGroup>
                  </Field>
                </FieldGroup>
              </form>
            )}
          </div>
        </div>
      </div>
      <div
        hidden={mode !== "canvas"}
        className={cn(mode === "canvas" && "home-canvas-launch")}
      >
        <div className="home-canvas-list">
          <NewCanvasButton variant="tile">
            <Plus />
            <span>新建画布</span>
          </NewCanvasButton>
          <Button variant="tile" asChild>
            <Link href="/projects">
              <span>我的画布</span>
              <small>查看已保存的创作</small>
            </Link>
          </Button>
        </div>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (!session) login();
            else setFeature("从提示词创建画布");
          }}
        >
          <InputGroup variant="composer" className="canvas-idea">
            <InputGroupInput
              aria-label="画布创作想法"
              placeholder="输入创作想法，开始一张新画布"
              value={prompt}
              onChange={(event) => onPromptChange(event.target.value)}
            />
            <InputGroupAddon align="inline-end">
              <Button
                type="submit"
                size="icon"
                disabled={!prompt.trim()}
                aria-label="从想法创建画布"
              >
                <ArrowUp />
              </Button>
            </InputGroupAddon>
          </InputGroup>
        </form>
      </div>
      <UnavailableDialog feature={feature} onClose={() => setFeature(null)} />
    </section>
  );
}
