"use client";
import { useState } from "react";
import { ProductShell } from "@/components/layout/product-shell";
import { ProjectPicker } from "@/components/projects/project-picker";
import { type Project } from "@/components/projects/project";
import { SubjectList } from "@/components/subjects/subject-list";
import {
  GenerationDialog,
  type GenerationSeed,
} from "@/components/generation/generation-dialog";
import { subjectText } from "@/components/subjects/contracts";
import { kinds } from "@/components/subjects/contracts";
import { Button } from "@/components/ui/button";
export function SubjectsPage() {
  const [project, setProject] = useState<Project | null>(null),
    [kind, setKind] = useState<CreationAPI.Kind>("character"),
    [generation, setGeneration] = useState<GenerationSeed | null>(null);
  return (
    <ProductShell screen="subjects">
      <div className="flow-page-heading subjects-heading">
        <h1>主体</h1>
        <p>管理角色、场景与物品，在项目创作中保持一致。</p>
      </div>
      <ProjectPicker value={project} onChange={setProject} />
      <div className="subject-tabs" aria-label="主体分类">
        {(Object.keys(kinds) as CreationAPI.Kind[]).map((k) => (
          <Button
            key={k}
            variant={kind === k ? "secondary" : "ghost"}
            aria-pressed={kind === k}
            onClick={() => setKind(k)}
          >
            {kinds[k]}
          </Button>
        ))}
      </div>
      {project ? (
        <SubjectList
          key={project.id + kind}
          projectID={project.id}
          kind={kind}
          onGenerate={(subject) => {
            const text = subjectText(subject);
            setGeneration({
              project,
              subject,
              prompt: `主体：${text.name}。${text.description ?? ""}`,
              capability: "image",
            });
          }}
        />
      ) : (
        <div className="empty-state">
          <h2>选择项目，查看你的主体</h2>
          <p>主体与参考素材保存在所属项目中。</p>
        </div>
      )}
      {generation && (
        <GenerationDialog
          seed={generation}
          onClose={() => setGeneration(null)}
        />
      )}
    </ProductShell>
  );
}
