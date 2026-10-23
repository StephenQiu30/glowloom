"use client";
import { useState } from "react";
import { PageLayout } from "@/components/layout/page-layout";
import { PageHeading } from "@/components/layout/page-heading";
import { Button } from "@/components/ui/button";
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
        placeholder: "搜索本页主体",
        disabled: !project,
      }}
    >
      {!project ? (
        <PageHeading
          title="主体库"
          actions={<Button disabled>新建主体</Button>}
        />
      ) : null}
      {project ? (
        <SubjectList
          pageTitle="主体库"
          key={project.id + kind}
          searchQuery={query}
          projectID={project.id}
          kind={kind}
          filters={
            <>
              <ProjectPicker value={project} onChange={setProject} compact />
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
            </>
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
        <>
          <div className="page-toolbar">
            <ProjectPicker value={project} onChange={setProject} compact />
          </div>
          <Empty className="min-h-80">
            <EmptyHeader>
              <EmptyTitle>选择项目，查看你的主体</EmptyTitle>
              <EmptyDescription>
                主体与参考素材保存在所属项目中。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        </>
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
