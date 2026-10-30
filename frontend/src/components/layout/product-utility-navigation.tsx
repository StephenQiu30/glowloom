import Link from "next/link";
import { Button } from "@/components/ui/button";
import { ProductNavigationIcon } from "./product-navigation-icon";

/** Desktop utilities use the existing project and billing destinations. */
export function ProductUtilityNavigation() {
  return (
    <nav aria-label="快捷导航" className="product-utility-navigation">
      <Button asChild variant="ghost" className="product-history-action">
        <Link href="/projects">
          历史画布 <span aria-hidden="true">↗</span>
        </Link>
      </Button>
      <Button asChild variant="outline" className="product-utility-action">
        <Link href="/billing/recharge">
          <ProductNavigationIcon name="store" />
          积分超市
        </Link>
      </Button>
    </nav>
  );
}
