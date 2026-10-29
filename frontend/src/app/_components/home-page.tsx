"use client";
import { useState } from "react";
import { PageLayout } from "@/components/layout/page-layout";
import { InspirationGallery } from "@/components/inspiration/gallery";
import { HomeCreation } from "./home-creation";

export function HomePage() {
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  return (
    <PageLayout
      screen="home"
      fullBleed
      search={{
        value: search,
        onValueChange: setSearch,
        onSearch: setQuery,
        label: "搜索灵感",
      }}
    >
      <div className="home-page">
        <HomeCreation />
        <InspirationGallery searchQuery={query} />
      </div>
    </PageLayout>
  );
}
