"use client";

import { useRouter } from "next/navigation";
import { Fragment, useState } from "react";
import { PageLayout } from "@/components/layout/page-layout";
import { PageToolbar } from "@/components/layout/page-toolbar";
import { ContentState } from "@/components/layout/content-state";
import { ExploreReferenceExamples } from "@/components/inspiration/gallery";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Separator } from "@/components/ui/separator";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { exploreCategories } from "./explore-categories";
import { OverflowNavigation } from "./overflow-navigation";
import styles from "./explore.module.css";

export function ExplorePage({ initialQuery = "" }: { initialQuery?: string }) {
  const [search, setSearch] = useState(initialQuery);
  const router = useRouter();
  const [selection, setSelection] = useState({
    category: exploreCategories[0].id,
    scene: exploreCategories[0].scenes[0].id,
  });
  const category = exploreCategories.find(
    (item) => item.id === selection.category,
  )!;
  const scene = category.scenes.find((item) => item.id === selection.scene);

  return (
    <PageLayout
      screen="explore"
      search={{
        mode: "explore",
        value: search,
        onValueChange: setSearch,
        onSearch: (value) =>
          router.push(
            `/explore/search?result=${encodeURIComponent(value)}&type=${category.kind === "skill" ? "skill" : ["work.drama", "work.film", "work.animation"].includes(category.id) ? "shortFilm" : "inspiration"}`,
          ),
        label: "搜索",
        placeholder: "搜索",
      }}
    >
      <h1 className="sr-only">探索</h1>
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
        <PageToolbar inHeader>
          <OverflowNavigation label="探索分类" primary>
            <TabsList
              variant="line"
              className={styles.categories}
              aria-label="探索分类"
            >
              {exploreCategories.map((item) => (
                <Fragment key={item.id}>
                  {item.id === "skill" && (
                    <Separator
                      orientation="vertical"
                      className="mx-1 h-3.5 shrink-0"
                    />
                  )}
                  <TabsTrigger value={item.id}>{item.label}</TabsTrigger>
                </Fragment>
              ))}
            </TabsList>
          </OverflowNavigation>
        </PageToolbar>
        <TabsContent value={category.id}>
          {category.scenes.length > 0 && (
            <div className={styles.scenes}>
              <OverflowNavigation
                key={category.id}
                label={category.kind === "skill" ? "技能分类" : "作品场景"}
              >
                <ToggleGroup
                  className={styles.sceneList}
                  type="single"
                  value={selection.scene}
                  aria-label={
                    category.kind === "skill" ? "技能分类" : "作品场景"
                  }
                  onValueChange={(value) => {
                    if (value)
                      setSelection({ category: category.id, scene: value });
                  }}
                >
                  {category.scenes.map((item) => (
                    <ToggleGroupItem key={item.id} value={item.id}>
                      {item.label}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
              </OverflowNavigation>
            </div>
          )}
          {category.id === "work.advertising" && scene ? (
            <ExploreReferenceExamples scene={scene.label} query="" />
          ) : (
            <ContentState
              title={`${category.kind === "skill" ? "技能" : category.kind === "activity" ? "活动" : "作品"}尚未开放`}
              description={
                category.kind === "work"
                  ? "该场景的公开作品尚未开放。可前往广告营销查看示例作品。"
                  : `公开${category.label}尚未开放。`
              }
            />
          )}
        </TabsContent>
      </Tabs>
    </PageLayout>
  );
}
