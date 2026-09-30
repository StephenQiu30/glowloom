import { notFound } from "next/navigation";
import {
  AdminScreen,
  type AdminPage,
} from "@/components/workbench/admin-pages";
import { PreviewBoundary } from "@/components/workbench/workbench-components";
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
