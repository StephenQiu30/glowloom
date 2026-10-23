"use client";

import { useState } from "react";
import { PageLayout } from "@/components/layout/page-layout";
import { PageHeading } from "@/components/layout/page-heading";
import { InspirationGallery } from "@/components/inspiration/gallery";

export function ExplorePage({ initialQuery = "" }: { initialQuery?: string }) {
  const [search, setSearch] = useState(initialQuery);
  const [query, setQuery] = useState(initialQuery);
  return (
    <PageLayout
      screen="explore"

      search={{
        value: search,
        onValueChange: setSearch,
        onSearch: setQuery,
        label: "搜索作品",
      }}
    >
      <PageHeading title="探索" />
      <InspirationGallery showUpdates={false} searchQuery={query} />
    </PageLayout>
  );
}
