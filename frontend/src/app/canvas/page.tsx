import { SessionGate } from "@/components/auth/session";
import { SpaceProvider } from "@/components/spaces/space-context";
import { CanvasPage } from "./_components/canvas-page";

export const metadata = { title: "无限画布" };
export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{
    project_id?: string | string[];
    canvas_id?: string | string[];
  }>;
}) {
  const params = await searchParams;
  return (
    <SessionGate>
      <SpaceProvider>
        <CanvasPage
          projectID={
            typeof params.project_id === "string"
              ? params.project_id
              : undefined
          }
          canvasID={
            typeof params.canvas_id === "string" ? params.canvas_id : undefined
          }
        />
      </SpaceProvider>
    </SessionGate>
  );
}
