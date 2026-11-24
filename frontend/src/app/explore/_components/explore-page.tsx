"use client";

import { useRouter } from "next/navigation";
import { Fragment, useRef, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { PageLayout } from "@/components/layout/page-layout";
import { HeaderNavigation } from "@/components/layout/header-navigation";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import { ExploreReferenceExamples } from "@/components/inspiration/gallery";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Separator } from "@/components/ui/separator";
import { exploreCategories, featuredCategory } from "./explore-categories";
import { OverflowNavigation } from "./overflow-navigation";

export function ExplorePage({ initialQuery = "" }: { initialQuery?: string }) {
  const [search, setSearch] = useState(initialQuery);
  const router = useRouter();
  const featuredTab = useRef<HTMLButtonElement>(null);
  const [selection, setSelection] = useState({
    category: featuredCategory.id,
    scene: "",
  });
  const category =
    exploreCategories.find((item) => item.id === selection.category) ??
    featuredCategory;
  const scene = category.scenes.find((item) => item.id === selection.scene);
  const isFeatured = category.id === featuredCategory.id;
  const sceneLabel = category.kind === "skill" ? "技能分类" : "作品场景";
  const content =
    isFeatured || (category.sceneExamples && scene) ? (
      <ExploreReferenceExamples
        scene={isFeatured ? undefined : scene?.label}
        query=""
      />
    ) : (
      <ContentState
        title={`${category.kind === "skill" ? "技能" : category.kind === "activity" ? "活动" : "作品"}尚未开放`}
        actions={
          <Button
            variant="secondary"
            onClick={() => {
              setSelection({ category: featuredCategory.id, scene: "" });
              featuredTab.current?.focus();
            }}
          >
            浏览精选
          </Button>
        }
      />
    );

  return (
    <PageLayout
      screen="explore"
      search={{
        mode: "explore",
        value: search,
        onValueChange: setSearch,
        onSearch: (value) =>
          router.push(
            `/explore/search?result=${encodeURIComponent(value)}&type=${category.searchType ?? "inspiration"}`,
          ),
        label: "搜索",
        placeholder: "搜索",
      }}
    >
      <PageHeader title="探索" hideTitle />
      <Tabs
        value={category.id}
        onValueChange={(id) => {
          const next = exploreCategories.find((item) => item.id === id);
          if (next && next.id !== category.id)
            setSelection({
              category: next.id,
              scene: next.scenes[0]?.id ?? "",
            });
        }}
      >
        <HeaderNavigation label="探索导航" active={category.id}>
          <TabsList variant="workspace" aria-label="探索分类">
            {exploreCategories.map((item) => (
              <Fragment key={item.id}>
                {item.id === "skill" && (
                  <Separator
                    orientation="vertical"
                    className="mx-1 h-3.5 shrink-0"
                  />
                )}
                <TabsTrigger
                  value={item.id}
                  ref={
                    item.id === featuredCategory.id ? featuredTab : undefined
                  }
                >
                  {item.label}
                </TabsTrigger>
              </Fragment>
            ))}
          </TabsList>
        </HeaderNavigation>
        <TabsContent value={category.id}>
          {category.scenes.length ? (
            <Tabs
              key={category.id}
              value={selection.scene}
              onValueChange={(value) =>
                setSelection({ category: category.id, scene: value })
              }
              className="min-w-0 gap-(--layout-gap)"
            >
              <OverflowNavigation label={sceneLabel}>
                <TabsList variant="secondary" aria-label={sceneLabel}>
                  {category.scenes.map((item) => (
                    <TabsTrigger key={item.id} value={item.id}>
                      {item.label}
                    </TabsTrigger>
                  ))}
                </TabsList>
              </OverflowNavigation>
              {category.scenes.map((item) => (
                <TabsContent key={item.id} value={item.id}>
                  {item.id === selection.scene ? content : null}
                </TabsContent>
              ))}
            </Tabs>
          ) : (
            content
          )}
        </TabsContent>
      </Tabs>
    </PageLayout>
  );
}
