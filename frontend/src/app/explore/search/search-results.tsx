"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { PageLayout } from "@/components/layout/page-layout";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { ExploreSearch } from "@/components/layout/explore-search";
import styles from "./search-results.module.css";
const types = [
  { id: "inspiration", label: "灵感" },
  { id: "skill", label: "技能" },
  { id: "shortFilm", label: "短片" },
];
export function ExploreSearchResults({
  query,
  type,
}: {
  query: string;
  type: string;
}) {
  const [value, setValue] = useState(query);
  const router = useRouter();
  function navigate(keyword: string, nextType = type) {
    router.push(
      `/explore/search?result=${encodeURIComponent(keyword)}&type=${nextType}`,
    );
  }
  const search = {
    value,
    onValueChange: setValue,
    onSearch: navigate,
    label: "搜索",
    placeholder: "搜索",
    alwaysExpanded: true,
  };
  return (
    <PageLayout screen="explore" search={{ ...search, mode: "hidden" }}>
      <h1 className="sr-only">搜索结果</h1>
      <div className={styles.searchRow}>
        <Button asChild variant="ghost" size="icon-lg">
          <Link href="/explore" aria-label="返回探索">
            <ArrowLeft />
          </Link>
        </Button>
        <ExploreSearch search={search} />
      </div>
      <Tabs value={type} onValueChange={(next) => navigate(query, next)}>
        <TabsList variant="line" aria-label="搜索类型">
          {types.map((item) => (
            <TabsTrigger key={item.id} value={item.id}>
              {item.label}
            </TabsTrigger>
          ))}
        </TabsList>
        {types.map((item) => (
          <TabsContent key={item.id} value={item.id}>
            <ContentState
              title={
                query.trim()
                  ? `${item.label}搜索尚未开放`
                  : "输入关键词开始搜索"
              }
              description={
                query.trim()
                  ? `“${query}”的公开${item.label}搜索服务尚未接入。`
                  : "搜索灵感、技能或短片。"
              }
            />
          </TabsContent>
        ))}
      </Tabs>
    </PageLayout>
  );
}
