"use client";

import Link from "next/link";
import { useState } from "react";
import { Coins, Sparkle } from "lucide-react";
import { RequestError } from "@/lib/request";
import { Button } from "@/components/ui/button";
import { payerLabel } from "./wallet-presentation";
import { useCreditWallet } from "./use-credit-wallet";
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from "@/components/ui/popover";

export function CreditWalletSummary({
  wallet,
  isLoading,
  error,
}: Pick<ReturnType<typeof useCreditWallet>, "isLoading" | "error"> & {
  wallet: API.WalletView | undefined;
}) {
  return wallet?.enabled && wallet.general_credits != null ? (
    <div className="grid gap-1">
      <p className="text-sm font-medium">可用积分 {wallet.general_credits}</p>
      <p className="text-xs text-muted-foreground">
        冻结 {wallet.held_credits} 积分 · {payerLabel(wallet.payer)}
      </p>
    </div>
  ) : (
    <p className="text-sm font-medium" role="status">
      {isLoading
        ? "正在读取积分"
        : error instanceof RequestError && error.status === 403
          ? "当前账号无权查看积分"
          : error || !wallet
            ? "积分暂不可读取"
            : "积分服务暂未开放"}
    </p>
  );
}

export function CreditPopover({
  navigation = false,
  showBalance = false,
  canvasScopeID,
  canNavigate = true,
}: {
  navigation?: boolean;
  showBalance?: boolean;
  canvasScopeID?: string;
  canNavigate?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const {
    data: wallet,
    isLoading,
    error,
  } = useCreditWallet(undefined, open || showBalance, undefined, canvasScopeID);
  const Icon = navigation || canvasScopeID ? Sparkle : Coins;
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size={
            canvasScopeID
              ? "toolbar"
              : navigation
                ? "toolbar-adaptive"
                : "default"
          }
          aria-label="积分余额"
          title="积分余额"
          className={
            canvasScopeID
              ? "canvas-toolbar-button tabular-nums"
              : navigation
                ? "product-credit-action"
                : "max-[359px]:hidden max-md:size-11 max-md:p-0"
          }
        >
          <Icon aria-hidden="true" data-icon="inline-start" />
          <span
            className={!showBalance || navigation ? "max-md:hidden" : undefined}
          >
            {showBalance && wallet?.enabled && wallet.general_credits != null
              ? wallet.general_credits
              : "积分"}
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
        <CreditWalletSummary
          wallet={wallet}
          isLoading={isLoading}
          error={error}
        />
        <div className="flex flex-wrap gap-2">
          {canNavigate ? (
            <Button asChild variant="outline">
              <Link href="/billing" onClick={() => setOpen(false)}>
                积分总览
              </Link>
            </Button>
          ) : (
            <Button variant="outline" disabled>
              积分总览
            </Button>
          )}
          {!canvasScopeID && (
            <Button asChild variant="outline">
              <Link href="/billing/membership" onClick={() => setOpen(false)}>
                会员中心
              </Link>
            </Button>
          )}
        </div>
        {!canNavigate && (
          <p className="text-xs text-muted-foreground">
            画布同步完成后可前往账单。
          </p>
        )}
      </PopoverContent>
    </Popover>
  );
}
