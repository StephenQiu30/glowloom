"use client";

import { usePathname } from "next/navigation";
import { ApplicationLoading } from "./application-state";
import { PageLoading, loadingLayoutForScreen } from "./loading-state";
import { PageLayout } from "./page-layout";
import { resolveScreen, type Screen } from "./routes";

export function ScreenLoading({
  screen,
  message,
}: {
  screen: Screen;
  message?: string;
}) {
  const layout = loadingLayoutForScreen(screen);
  const fullBleed =
    layout === "home" || screen === "ai-tools" || screen === "billing";
  return (
    <PageLayout
      screen={layout === "home" ? "home" : screen}
      fullBleed={fullBleed}
    >
      <PageLoading
        layout={layout}
        heading={layout !== "home" && layout !== "canvas"}
        className={
          fullBleed && layout !== "home" ? "workspace-page" : undefined
        }
        message={message}
      />
    </PageLayout>
  );
}

/** Keep the ordinary page shell in place while the route payload is pending. */
export function RouteLoading() {
  const pathname = usePathname();
  const screen = resolveScreen(pathname)?.id;
  if (screen === "canvas")
    return <ApplicationLoading layout="canvas" message="正在打开画布…" />;
  if (!screen || screen === "reset-password") return <ApplicationLoading />;
  return <ScreenLoading screen={screen} />;
}
