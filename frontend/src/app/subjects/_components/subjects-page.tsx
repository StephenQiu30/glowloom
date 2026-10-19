"use client";
import { useState } from "react";
import { PageLayout } from "@/components/layout/page-layout";
import { ProjectPicker } from "@/components/projects/project-picker";
import { type Project } from "@/components/projects/project";
import { SubjectList } from "@/components/subjects/subject-list";
import {
  GenerationDialog,
  type GenerationSeed,
} from "@/components/generation/generation-dialog";
import { subjectText } from "@/components/subjects/contracts";
import { kinds } from "@/components/subjects/contracts";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty";
export function SubjectsPage() {
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  const [project, setProject] = useState<Project | null>(null),
    [kind, setKind] = useState<API.Kind>("character"),
    [generation, setGeneration] = useState<GenerationSeed | null>(null);
  return (
    <PageLayout
      screen="subjects"
      search={{
        value: search,
        onValueChange: setSearch,
        onSearch: setQuery,
        label: "搜索主体",
        placeholder: "搜索已加载主体",
        disabled: !project,
      }}
    >
      <div className="flow-page-heading subjects-heading">
        <div>
          <h1>主体</h1>
          <p>保持角色与商品的一致性，在生成和画布中通过 @ 引用。</p>
        </div>
        <ProjectPicker value={project} onChange={setProject} compact />
      </div>
      {project ? (
        <SubjectList
          key={project.id + kind}
          searchQuery={query}
          projectID={project.id}
          kind={kind}
          filters={
            <ToggleGroup
              type="single"
              value={kind}
              onValueChange={(v) => {
                if (v in kinds) setKind(v as API.Kind);
              }}
              className="subject-tabs"
              aria-label="主体分类"
            >
              {(Object.keys(kinds) as API.Kind[]).map((k) => (
                <ToggleGroupItem key={k} value={k}>
                  {kinds[k]}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          }
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
        <Empty className="min-h-80">
          <EmptyHeader>
            <EmptyTitle>选择项目，查看你的主体</EmptyTitle>
            <EmptyDescription>
              主体与参考素材保存在所属项目中。
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      {generation && (
        <GenerationDialog
          seed={generation}
          onClose={() => setGeneration(null)}
        />
      )}
    </PageLayout>
  );
}
