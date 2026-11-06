import { redirect } from "next/navigation";
import { isProjectID } from "@/components/projects/project";

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ project_id?: string | string[] }>;
}) {
  const { project_id: projectID } = await searchParams;
  redirect(
    isProjectID(projectID)
      ? `/short-drama/${projectID}?tab=episodes`
      : "/short-drama",
  );
}
