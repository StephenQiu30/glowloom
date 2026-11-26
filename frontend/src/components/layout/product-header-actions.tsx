"use client";

import { Toolbar } from "@/components/ui/toolbar";
import type { ProductAccountActionsProps } from "./product-account-actions";
import { ProductAccountActions } from "./product-account-actions";
import { ProductUtilityNavigation } from "./product-utility-navigation";
import { ProductSearch, type PageSearch } from "./product-search";

export type ProductHeaderActionsProps = ProductAccountActionsProps & {
  search?: PageSearch;
  showSearch?: boolean;
  showUtilities?: boolean;
};

/** Page-specific tools never change the shared account area's composition or identity. */
export function ProductHeaderActions({
  search,
  showSearch = true,
  showUtilities = false,
  ...accountProps
}: ProductHeaderActionsProps) {
  return (
    <Toolbar density="navigation" className="product-header-actions">
      {showSearch && <ProductSearch search={search} />}
      {showUtilities && <ProductUtilityNavigation />}
      <ProductAccountActions key="account" {...accountProps} />
    </Toolbar>
  );
}
