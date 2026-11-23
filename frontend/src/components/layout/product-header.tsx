"use client";

import type { Ref } from "react";
import { Menu } from "lucide-react";
import { Brand } from "@/components/brand/brand";
import { Button } from "@/components/ui/button";
import {
  ProductHeaderActions,
  type ProductHeaderActionsProps,
} from "./product-header-actions";

/** Figma ProductHeader 620:24416; shared by ordinary product pages. */
export function ProductHeader({
  floating = false,
  inlineNavigation = false,
  toolbarRef,
  pageActionsRef,
  replaceBase = false,
  search,
  showSearch,
  showUtilities,
  actor,
  isMobile,
  navigationOpen,
  navigationTriggerRef,
  onOpenNavigation,
  onLogin,
  onLogout,
}: {
  floating?: boolean;
  inlineNavigation?: boolean;
  toolbarRef?: Ref<HTMLDivElement>;
  pageActionsRef?: Ref<HTMLDivElement>;
  replaceBase?: boolean;
  isMobile: boolean;
  navigationOpen: boolean;
  navigationTriggerRef?: Ref<HTMLButtonElement>;
  onOpenNavigation: () => void;
} & ProductHeaderActionsProps) {
  const navigation = (
    <div key="navigation" className="product-header-toolbar" ref={toolbarRef} />
  );
  const pageActions = (
    <div
      key="page-actions"
      className="product-header-page-actions"
      ref={pageActionsRef}
    />
  );
  const common = (
    <ProductHeaderActions
      key="common"
      search={search}
      showSearch={showSearch && !replaceBase}
      showUtilities={showUtilities}
      actor={actor}
      onLogin={onLogin}
      onLogout={onLogout}
    />
  );
  return (
    <div
      data-inline-navigation={inlineNavigation || undefined}
      data-replace-base={replaceBase || undefined}
      className={floating ? "product-global-actions" : "product-header"}
      role={floating ? "group" : "banner"}
      aria-label={floating ? "全局操作" : "页面顶栏"}
    >
      <div className="product-header-location">
        {isMobile ? (
          <>
            <Button
              variant="ghost"
              size="toolbar-icon"
              ref={navigationTriggerRef}
              className="mobile-navigation-trigger"
              aria-label="打开导航"
              aria-expanded={navigationOpen}
              onClick={onOpenNavigation}
            >
              <Menu />
            </Button>
            <Brand />
          </>
        ) : null}
      </div>
      {isMobile && !inlineNavigation
        ? [common, navigation, pageActions]
        : [navigation, pageActions, common]}
    </div>
  );
}
