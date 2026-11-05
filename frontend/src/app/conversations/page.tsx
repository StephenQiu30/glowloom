import { ConversationPage } from "@/components/conversations/conversation-page";
import { PageLayout } from "@/components/layout/page-layout";
import { isProjectID } from "@/components/projects/project";
export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ projectId?: string }>;
}) {
  const { projectId } = await searchParams;
  return (
    <PageLayout screen="conversations" fullBleed hideSearch>
      <ConversationPage
        projectID={isProjectID(projectId) ? projectId : undefined}
      />
    </PageLayout>
  );
}
