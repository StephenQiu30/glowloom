"use client";

import {
  ProductAccountActions,
  type ProductAccountActionsProps,
} from "./product-account-actions";
import { ProductUtilityNavigation } from "./product-utility-navigation";

/** Figma ProductGlobalActions 3658:120858; floats above the desktop home. */
export function ProductGlobalActions({
  showUtilities = false,
  ...props
}: ProductAccountActionsProps & { showUtilities?: boolean }) {
  return (
    <>
      {showUtilities ? <ProductUtilityNavigation /> : null}
      <ProductAccountActions key="account" {...props} />
    </>
  );
}
