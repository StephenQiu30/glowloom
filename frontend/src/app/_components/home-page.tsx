"use client";
import { PageLayout } from "@/components/layout/page-layout";
import { InspirationGallery } from "@/components/inspiration/gallery";
import { HomeCreation } from "./home-creation";

export function HomePage() {
  return (
    <PageLayout screen="home" fullBleed>
      <div className="home-page">
        <HomeCreation />
        <InspirationGallery searchQuery="" />
      </div>
    </PageLayout>
  );
}
