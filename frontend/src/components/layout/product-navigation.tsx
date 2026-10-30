"use client";

import type { ComponentProps, ReactNode } from "react";
import { SidebarInset } from "@/components/ui/sidebar";
import type { Screen } from "./routes";
import { ProductHeader } from "./product-header";
import { ProductSider } from "./product-sider";

/** Ordinary pages share one navigation tree; screen owns only the tool policy. */
export function ProductNavigation({
  screen,
  children,
  onMobileCloseAutoFocus,
  ...header
}: Omit<
  ComponentProps<typeof ProductHeader>,
  "assetLayout" | "showSearch" | "showUtilities"
> & {
  screen: Screen;
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
            assetLayout={screen === "assets"}
            showSearch={screen !== "home"}
            showUtilities={screen === "home"}
          />
          {children}
        </div>
      </SidebarInset>
    </>
  );
}
