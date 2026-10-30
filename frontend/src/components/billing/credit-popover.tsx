"use client";

import Link from "next/link";
import { useState } from "react";
import { Coins } from "lucide-react";
import { ProductNavigationIcon } from "@/components/layout/product-navigation-icon";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from "@/components/ui/popover";

export function CreditPopover({
  navigation = false,
}: {
  navigation?: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          aria-label="积分余额"
          className={
            navigation
              ? "product-credit-action"
              : "max-[359px]:hidden max-md:size-11 max-md:p-0"
          }
        >
          {navigation ? (
            <ProductNavigationIcon name="credit" />
          ) : (
            <Coins data-icon="inline-start" />
          )}
          <span className="max-md:hidden">积分</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        side="bottom"
        sideOffset={8}
        collisionPadding={16}
        className="flex w-72 max-w-[calc(100vw-32px)] flex-col gap-3"
        aria-label="积分余额"
      >
        <p className="font-medium">积分服务暂未开放</p>
        <p className="text-sm text-muted-foreground">
          开放后可查看可用积分、预占积分和消耗记录。
        </p>
        <div className="flex flex-wrap gap-2">
          <Button asChild variant="outline">
            <Link href="/billing" onClick={() => setOpen(false)}>
              积分总览
            </Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/billing/membership" onClick={() => setOpen(false)}>
              会员中心
            </Link>
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
