import Link from "next/link";
import { Button } from "@/components/ui/button";
import { ArrowUpRight, Store } from "lucide-react";

/** Desktop utilities use the existing project and billing destinations. */
export function ProductUtilityNavigation() {
  return (
    <nav aria-label="快捷导航" className="product-utility-navigation">
      <Button asChild variant="ghost" className="product-history-action">
        <Link href="/assets?view=canvas&collection=legacy">
          历史画布 <ArrowUpRight aria-hidden="true" data-icon="inline-end" />
        </Link>
      </Button>
      <Button asChild variant="outline" className="product-utility-action">
        <Link href="/billing/recharge">
          <Store aria-hidden="true" data-icon="inline-start" />
          积分超市
        </Link>
      </Button>
    </nav>
  );
}
