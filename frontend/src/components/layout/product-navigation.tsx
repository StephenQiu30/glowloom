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
  "assetLayout" | "showSearch" | "showUtilities"
> & {
  screen: Screen;
  hideSearch?: boolean;
  children: ReactNode;
  onMobileCloseAutoFocus: ComponentProps<
    typeof ProductSider
  >["onMobileCloseAutoFocus"];
}) {
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
            assetLayout={screen === "assets"}
            showSearch={
              !hideSearch &&
              screen !== "home" &&
              (!resolveAdminSection(screen) || Boolean(header.search))
            }
            showUtilities={screen === "home"}
          />
          {children}
        </div>
      </SidebarInset>
    </>
  );
}
