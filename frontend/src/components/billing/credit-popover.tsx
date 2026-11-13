"use client";

import Link from "next/link";
import { useState } from "react";
import { Coins, Sparkle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useCreditWallet } from "./use-credit-wallet";
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from "@/components/ui/popover";

export function CreditPopover({
  navigation = false,
  showBalance = false,
}: {
  navigation?: boolean;
  showBalance?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const {
    data: wallet,
    isLoading,
    error,
  } = useCreditWallet(undefined, open || showBalance);
  const Icon = navigation ? Sparkle : Coins;
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size={navigation ? "toolbar-adaptive" : "default"}
          aria-label="积分余额"
          className={
            navigation
              ? "product-credit-action"
              : "max-[359px]:hidden max-md:size-11 max-md:p-0"
          }
        >
          <Icon aria-hidden="true" data-icon="inline-start" />
          <span className={showBalance ? undefined : "max-md:hidden"}>
            {showBalance && wallet?.enabled ? wallet.general_credits : "积分"}
          </span>
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
        {wallet?.enabled ? (
          <>
            <p className="font-medium">可用积分 {wallet.general_credits}</p>
            <p className="text-sm text-muted-foreground">
              冻结 {wallet.held_credits} 积分 ·{" "}
              {wallet.payer === "team" ? "团队付款账户" : "个人付款账户"}
            </p>
          </>
        ) : (
          <p className="font-medium" role="status">
            {isLoading
              ? "正在读取积分"
              : error
                ? "积分暂不可读取"
                : "积分服务暂未开放"}
          </p>
        )}
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
