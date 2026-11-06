import { PageLayout } from "@/components/layout/page-layout";
import { ShortDramaStudio } from "./studio";

export const metadata = { title: "短剧工作室 · 织光" };

export default function Page() {
  return (
    <PageLayout screen="short-drama" hideSearch>
      <ShortDramaStudio />
    </PageLayout>
  );
}
