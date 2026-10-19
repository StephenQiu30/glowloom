import { PageLayout } from "@/components/layout/page-layout";
import { InspirationGallery } from "@/components/inspiration/gallery";

export function AuthEntry() {
  return (
    <PageLayout screen="home">
      <InspirationGallery />
    </PageLayout>
  );
}
