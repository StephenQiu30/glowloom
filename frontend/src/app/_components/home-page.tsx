"use client";
import { PageLayout } from "@/components/layout/page-layout";
import { InspirationGallery } from "@/components/inspiration/gallery";
import type { CreationType } from "@/components/generation/creation-types";
import { HomeCreation } from "./home-creation";

export function HomePage({
  initialType = "image",
}: {
  initialType?: CreationType;
}) {
  return (
    <PageLayout screen="home" fullBleed>
      <div className="home-page">
        <HomeCreation initialType={initialType} />
        <InspirationGallery searchQuery="" />
      </div>
    </PageLayout>
  );
}
