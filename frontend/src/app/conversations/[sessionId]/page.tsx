import { notFound } from "next/navigation";
import { ConversationPage } from "@/components/conversations/conversation-page";
import { PageLayout } from "@/components/layout/page-layout";
import { isProjectID } from "@/components/projects/project";
export default async function Page({
  params,
  searchParams,
}: {
  params: Promise<{ sessionId: string }>;
  searchParams: Promise<{ projectId?: string }>;
}) {
  const [{ sessionId }, { projectId }] = await Promise.all([
    params,
    searchParams,
  ]);
  if (!isProjectID(sessionId) || !isProjectID(projectId)) notFound();
  return (
    <PageLayout screen="conversations" fullBleed>
      <ConversationPage sessionID={sessionId} projectID={projectId} />
    </PageLayout>
  );
}
