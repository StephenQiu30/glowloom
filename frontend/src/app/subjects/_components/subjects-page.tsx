"use client";
import { useState } from "react";
import { PageLayout } from "@/components/layout/page-layout";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import { ContentState } from "@/components/layout/content-state";
import { UnifiedSubjectLibrary } from "@/components/subjects/unified-library";

export function SubjectsPage() {
  const session = useOptionalSession();
  const space = useSpace();
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  return (
    <PageLayout
      screen="subjects"
      search={{
        mode: "inline",
        value: search,
        onValueChange: setSearch,
        onSearch: setQuery,
        label: "搜索主体",
        placeholder: "搜索",
        disabled: !session || !space.selected,
      }}
    >
      <h1 className="sr-only">主体</h1>
      {session && space.selected ? (
        <UnifiedSubjectLibrary
          toolbarInHeader
          key={`${session.actor.id}/${space.selected.id}`}
          actorID={session.actor.id}
          scopeID={space.selected.id!}
          query={query}
        />
      ) : (
        <ContentState
          state={space.pending ? "loading" : "empty"}
          title={space.pending ? "正在读取创作空间" : "登录后管理主体"}
          description={
            space.error || "在当前空间建立主体，并在不同项目的创作中复用。"
          }
        />
      )}
    </PageLayout>
  );
}
