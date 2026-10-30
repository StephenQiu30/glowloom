"use client";

import type { ReactNode } from "react";
import { ButtonGroup } from "@/components/ui/button-group";
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
  isMobile: boolean;
  showSearch?: boolean;
};

/** Page-specific tools never change the shared account area's composition or identity. */
export function ProductHeaderActions({
  assetLayout = false,
  pageActions,
  search,
  isMobile,
  showSearch = true,
  ...accountProps
}: ProductHeaderActionsProps) {
  return (
    <div
      className="product-header-actions"
      data-asset-header-actions={assetLayout || undefined}
    >
      {assetLayout ? (
        <ButtonGroup
          key="tools"
          variant="asset-tools"
          data-asset-tool-group
          aria-label="资产工具"
          className="max-w-full flex-wrap"
        >
          {showSearch && (
            <div data-asset-search className="contents">
              <ProductSearch search={search} isMobile={isMobile} />
            </div>
          )}
          {pageActions}
        </ButtonGroup>
      ) : (
        showSearch && (
          <ProductSearch key="search" search={search} isMobile={isMobile} />
        )
      )}
      <ProductUtilityNavigation />
      <ProductAccountActions key="account" {...accountProps} />
    </div>
  );
}
