"use client";

import { usePathname } from "next/navigation";
import { ApplicationLoading } from "./application-state";
import { PageLoading } from "./loading-state";
import { PageLayout } from "./page-layout";
import { resolveScreen } from "./routes";

/** Keep the ordinary page shell in place while the route payload is pending. */
export function RouteLoading() {
  const pathname = usePathname();
  const screen = resolveScreen(pathname);
  if (!screen || ["canvas", "reset-password"].includes(screen.id))
    return <ApplicationLoading />;
  return (
    <PageLayout screen={screen.id}>
      <PageLoading />
    </PageLayout>
  );
}
