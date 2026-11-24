"use client";

import { useState } from "react";
import dynamic from "next/dynamic";
import Link from "next/link";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import { ContentState } from "@/components/layout/content-state";
import { HeaderNavigation } from "@/components/layout/header-navigation";
import { PageLayout } from "@/components/layout/page-layout";
import { PageHeader } from "@/components/layout/page-header";
import { ProjectPicker } from "@/components/projects/project-picker";
import type { Project } from "@/components/projects/project";
import { kinds } from "@/components/subjects/contracts";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

const SubjectList = dynamic(() =>
  import("@/components/subjects/subject-list").then(
    (module) => module.SubjectList,
  ),
);

function ProjectHistory() {
  const [project, setProject] = useState<Project | null>(null);
  const [kind, setKind] = useState<API.Kind>("character");
  return (
    <>
      <ProjectPicker value={project} onChange={setProject} />
      <Tabs
        value={kind}
        onValueChange={(value) => {
          if (value === "character" || value === "location" || value === "prop")
            setKind(value);
        }}
      >
        <HeaderNavigation label="历史主体分类" active={kind}>
          <TabsList variant="workspace" aria-label="历史主体类型">
            {(Object.keys(kinds) as API.Kind[]).map((value) => (
              <TabsTrigger key={value} value={value}>
                {kinds[value]}
              </TabsTrigger>
            ))}
          </TabsList>
        </HeaderNavigation>
      </Tabs>
      {project ? (
        <section aria-label={`${project.name} · ${kinds[kind]}历史主体`}>
          <SubjectList
            key={`${project.id}/${kind}`}
            projectID={project.id}
            kind={kind}
          />
        </section>
      ) : (
        <ContentState title="选择项目，查看历史主体" />
      )}
    </>
  );
}

export function SubjectHistoryPage() {
  const session = useOptionalSession();
  const space = useSpace();
  return (
    <PageLayout screen="subjects">
      <PageHeader title="项目历史主体" className="mb-(--layout-gap)">
        <Button asChild variant="outline">
          <Link href="/subjects">返回主体</Link>
        </Button>
      </PageHeader>
      {session && space.selected ? (
        <ProjectHistory key={`${session.actor.id}/${space.selected.id}`} />
      ) : (
        <ContentState
          state={space.pending ? "loading" : "empty"}
          title={space.pending ? "正在读取创作空间" : "登录后查看历史主体"}
          description={space.error || undefined}
        />
      )}
    </PageLayout>
  );
}
