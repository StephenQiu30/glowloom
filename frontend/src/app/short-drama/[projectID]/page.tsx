import { notFound } from "next/navigation";
import { PageLayout } from "@/components/layout/page-layout";
import { isProjectID } from "@/components/projects/project";
import { DramaProject } from "../drama-project";

export default async function Page({
  params,
  searchParams,
}: {
  params: Promise<{ projectID: string }>;
  searchParams: Promise<{
    tab?: string | string[];
    session?: string | string[];
  }>;
}) {
  const [{ projectID }, { tab, session }] = await Promise.all([
    params,
    searchParams,
  ]);
  if (!isProjectID(projectID)) notFound();
  return (
    <PageLayout screen="short-drama" hideSearch>
      <DramaProject
        projectID={projectID}
        initialTab={
          tab === "create"
            ? "create"
            : tab === "script"
              ? "script"
              : tab === "video"
                ? "video"
                : tab === "episodes"
                  ? "episodes"
                  : "assets"
        }
        initialSession={isProjectID(session) ? session : undefined}
      />
    </PageLayout>
  );
}
