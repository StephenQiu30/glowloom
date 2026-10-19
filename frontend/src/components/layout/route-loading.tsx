"use client";

import { usePathname } from "next/navigation";
import { ApplicationLoading } from "./application-state";
import { BasicLayout } from "./basic-layout";
import { PageLoading } from "./loading-state";
import { screens, screenHref } from "./routes";

const pendingSearch = {
  value: "",
  onValueChange: () => {},
  onSearch: () => {},
  label: "搜索",
  disabled: true,
};

/** Keep the ordinary page shell in place while the route payload is pending. */
export function RouteLoading() {
  const pathname = usePathname();
  const screen =
    screens.find((screen) => screenHref(screen.id) === pathname) ??
    screens.find((screen) => pathname.startsWith(`${screenHref(screen.id)}/`));
  if (
    !screen ||
    ["canvas", "reset-password"].includes(screen.id)
  )
    return <ApplicationLoading />;
  return (
    <BasicLayout screen={screen.id} allowGuest search={pendingSearch}>
      <PageLoading />
    </BasicLayout>
  );
}
