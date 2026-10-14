"use client";
import { useState } from "react";
import dynamic from "next/dynamic";
import { Button } from "@/components/ui/button";
import { ProjectPicker } from "@/components/projects/project-picker";
import type { Project } from "@/components/projects/project";
import { PromptEditor } from "./prompt-editor";
import { emptyPromptDraft } from "./prompt-document";
const RichGeneration = dynamic(
  () => import("./rich-generation").then((m) => m.RichGeneration),
  { ssr: false, loading: () => <p role="status">正在打开创作编辑器…</p> },
);
/** The home workspace expands in place, so its owner and undo history survive view changes. */
export function GenerationComposer() {
  const [draft, setDraft] = useState(emptyPromptDraft),
    [replicaId] = useState(() => crypto.randomUUID()),
    [capability, setCapability] = useState<"image" | "video">("image"),
    [project, setProject] = useState<Project | null>(null),
    [choosing, setChoosing] = useState(false);
  if (project)
    return (
      <div className="asset-composer">
        <p className="text-sm text-muted-foreground">
          {project.name} · 项目草稿
        </p>
        <RichGeneration
          projectID={project.id}
          seed={{ prompt: "", capability, composer: draft }}
        />
      </div>
    );
  return (
    <section className="asset-composer space-y-3" aria-label="资产创作">
      <p className="text-sm text-muted-foreground">
        本地临时输入；选择项目后保存完整草稿。
      </p>
      <div className="min-h-28 rounded-lg border p-3 [&_.tiptap]:min-h-24 [&_.tiptap]:outline-none">
        <PromptEditor
          contextKey="guest/home"
          replicaId={replicaId}
          draft={draft}
          onReady={() => {}}
          onChange={(snapshot) => setDraft(snapshot.draft)}
          onRequestSubmit={() => setChoosing(true)}
        />
      </div>
      <div className="flex flex-wrap gap-2">
        <select
          aria-label="资产生成类型"
          className="rounded border bg-background px-3"
          value={capability}
          onChange={(e) => setCapability(e.target.value as "image" | "video")}
        >
          <option value="image">图片生成</option>
          <option value="video">视频生成</option>
        </select>
        <Button variant="outline" onClick={() => setChoosing(true)}>
          参考素材与主体
        </Button>
        <Button onClick={() => setChoosing(true)}>选择项目与生成参数</Button>
      </div>
      {choosing && <ProjectPicker value={project} onChange={setProject} />}
    </section>
  );
}
