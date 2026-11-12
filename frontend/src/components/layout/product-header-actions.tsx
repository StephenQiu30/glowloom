"use client";

import type { ReactNode } from "react";
import { Scissors } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Toolbar, ToolbarGroup } from "@/components/ui/toolbar";
import type { ProductAccountActionsProps } from "./product-account-actions";
import { ProductGlobalActions } from "./product-global-actions";
import { ProductSearch, type PageSearch } from "./product-search";

export type ProductHeaderActionsProps = ProductAccountActionsProps & {
  search?: PageSearch;
  assetLayout?: boolean;
  pageActions?: ReactNode;
  showSearch?: boolean;
  showUtilities?: boolean;
};

/** Page-specific tools never change the shared account area's composition or identity. */
export function ProductHeaderActions({
  assetLayout = false,
  pageActions,
  search,
  showSearch = true,
  showUtilities = false,
  ...accountProps
}: ProductHeaderActionsProps) {
  return (
    <Toolbar
      className="product-header-actions"
      data-asset-header-actions={assetLayout || undefined}
    >
      {assetLayout ? (
        <ToolbarGroup
          key="tools"
          data-asset-tool-group
          aria-label="资产工具"
          className="product-page-tools max-w-full flex-wrap"
        >
          {showSearch && (
            <div data-asset-search className="contents">
              <ProductSearch search={search} />
            </div>
          )}
          {pageActions}
          {showSearch && (
            <Button
              variant="ghost"
              size="toolbar-icon"
              className="hidden md:inline-flex"
              aria-label="去剪映剪辑"
              title="剪辑导出暂未开放"
              onClick={() => accountProps.onFeature("资产剪辑导出")}
            >
              <Scissors />
            </Button>
          )}
        </ToolbarGroup>
      ) : (
        showSearch && <ProductSearch key="search" search={search} />
      )}
      <ProductGlobalActions
        key="account"
        showUtilities={showUtilities}
        {...accountProps}
      />
    </Toolbar>
  );
}
