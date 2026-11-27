import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { ToolbarGroup } from "@/components/ui/toolbar";
import { ArrowUpRight, Store } from "lucide-react";

export function ProductUtilityNavigation({
  showCanvasHistory,
}: {
  showCanvasHistory: boolean;
}) {
  return (
    <nav aria-label="快捷导航" className="product-utility-navigation">
      <ToolbarGroup>
        {showCanvasHistory && (
          <>
            <Button asChild size="toolbar" variant="ghost">
              <Link href="/assets?view=canvas&collection=legacy">
                历史画布{" "}
                <ArrowUpRight aria-hidden="true" data-icon="inline-end" />
              </Link>
            </Button>
            <Separator
              orientation="vertical"
              className="account-toolbar-divider"
            />
          </>
        )}
        <Button asChild size="toolbar" variant="ghost">
          <Link href="/billing/recharge">
            <Store aria-hidden="true" data-icon="inline-start" />
            积分超市
          </Link>
        </Button>
      </ToolbarGroup>
    </nav>
  );
}
