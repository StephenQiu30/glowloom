"use client";

import type { ReactNode } from "react";
import { Toolbar, ToolbarGroup } from "@/components/ui/toolbar";
import {
  ProductAccountActions,
  type ProductAccountActionsProps,
} from "./product-account-actions";
import { ProductSearch, type PageSearch } from "./product-search";
import { ProductUtilityNavigation } from "./product-utility-navigation";

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
          className="max-w-full flex-wrap"
        >
          {showSearch && (
            <div data-asset-search className="contents">
              <ProductSearch search={search} />
            </div>
          )}
          {pageActions}
        </ToolbarGroup>
      ) : (
        showSearch && <ProductSearch key="search" search={search} />
      )}
      {showUtilities && <ProductUtilityNavigation />}
      <ProductAccountActions key="account" {...accountProps} />
    </Toolbar>
  );
}
