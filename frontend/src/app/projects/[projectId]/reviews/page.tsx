import { ReviewWorkbench } from "./review-workbench";

export default async function ProjectReviewsPage({
  params,
  searchParams,
}: {
  params: Promise<{ projectId: string }>;
  searchParams: Promise<{ task?: string | string[] }>;
}) {
  const [{ projectId }, query] = await Promise.all([params, searchParams]);
  return (
    <ReviewWorkbench
      initialTaskId={typeof query.task === "string" ? query.task : undefined}
      projectId={projectId}
    />
  );
}
