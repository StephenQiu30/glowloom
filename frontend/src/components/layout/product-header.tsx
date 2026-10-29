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
  assetLayout = false,
  toolbarRef,
  pageActionsRef,
  replaceBase = false,
  search,
  actor,
  isMobile,
  navigationOpen,
  navigationTriggerRef,
  onOpenNavigation,
  onLogin,
  onLogout,
  onFeature,
}: {
  assetLayout?: boolean;
  toolbarRef?: Ref<HTMLDivElement>;
  pageActionsRef?: Ref<HTMLDivElement>;
  replaceBase?: boolean;
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
  const common = replaceBase ? null : (
    <ProductHeaderActions
      key="common"
      assetLayout={assetLayout}
      pageActions={assetLayout ? pageActions : undefined}
      search={search}
      actor={actor}
      isMobile={isMobile}
      onLogin={onLogin}
      onLogout={onLogout}
      onFeature={onFeature}
    />
  );
  return (
    <header
      data-asset-layout={assetLayout || undefined}
      data-replace-base={replaceBase || undefined}
      className="product-header"
      aria-label="页面顶栏"
    >
      <div className="product-header-location">
        {isMobile ? (
          <>
            <Button
              variant="ghost"
              size="icon-lg"
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
      {isMobile
        ? [common, navigation, assetLayout && !replaceBase ? null : pageActions]
        : [
            navigation,
            assetLayout && !replaceBase ? null : pageActions,
            common,
          ]}
    </header>
  );
}
