import { notFound } from "next/navigation";
import { AdminScreen, type AdminPage } from "@/features/workbench/admin-pages";
import { PreviewBoundary } from "@/features/workbench/workbench-components";
export default async function AdminPageRoute({
  params,
}: {
  params: Promise<{ section: string }>;
}) {
  const { section } = await params;
  if (!["users", "providers", "models", "audit", "health"].includes(section))
    notFound();
  return (
    <PreviewBoundary>
      <AdminScreen section={section as AdminPage} />
    </PreviewBoundary>
  );
}
