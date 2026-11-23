"use client";

import type { ComponentProps, ReactNode } from "react";
import { SidebarInset } from "@/components/ui/sidebar";
import { resolveAdminSection, type Screen } from "./routes";
import { ProductHeader } from "./product-header";
import { ProductSider } from "./product-sider";

/** Ordinary pages share one navigation tree; screen owns only the tool policy. */
export function ProductNavigation({
  screen,
  hideSearch,
  children,
  onMobileCloseAutoFocus,
  ...header
}: Omit<
  ComponentProps<typeof ProductHeader>,
  | "adminLayout"
  | "assetLayout"
  | "inlineNavigation"
  | "showSearch"
  | "showUtilities"
> & {
  screen: Screen;
  hideSearch?: boolean;
  children: ReactNode;
  onMobileCloseAutoFocus: ComponentProps<
    typeof ProductSider
  >["onMobileCloseAutoFocus"];
}) {
  const adminSection = resolveAdminSection(
    screen,
    header.actor?.platform_role === "platform_admin",
  );
  return (
    <>
      <ProductSider
        screen={screen}
        onMobileCloseAutoFocus={onMobileCloseAutoFocus}
      />
      <SidebarInset asChild>
        <div className="product-content" data-product-navigation={screen}>
          <ProductHeader
            {...header}
            floating={screen === "home" && !header.isMobile}
            adminLayout={Boolean(adminSection)}
            assetLayout={screen === "assets"}
            inlineNavigation={screen === "billing"}
            showSearch={
              !hideSearch &&
              screen !== "home" &&
              (!adminSection || Boolean(header.search))
            }
            showUtilities={!adminSection}
          />
          {children}
        </div>
      </SidebarInset>
    </>
  );
}
