"use client";

import Link from "next/link";
import { CreditPopover } from "@/components/billing/credit-popover";
import { ThemeToggle } from "@/components/controls/theme-controls";
import { AccountMenu } from "@/components/account/account-menu";
import { Button } from "@/components/ui/button";
import { ProductNavigationIcon } from "./product-navigation-icon";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

export type ProductAccountActionsProps = {
  actor?: API.EmailSessionView["actor"];
  onLogin: () => void;
  onLogout: () => void;
  onFeature: (feature: string) => void;
};

/** Figma ProductAccountActions 3165:58374; one account area across product routes. */
export function ProductAccountActions({
  actor,
  onLogin,
  onLogout,
  onFeature,
}: ProductAccountActionsProps) {
  const notification = (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          className="product-notification-action"
          aria-label="消息中心"
          onClick={() => onFeature("消息中心")}
        >
          <ProductNavigationIcon name="notification" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>消息中心</TooltipContent>
    </Tooltip>
  );
  const account = actor ? (
    <AccountMenu actor={actor} onLogout={onLogout} />
  ) : (
    <Button onClick={onLogin}>登录</Button>
  );
  return (
    <div
      role="group"
      data-account-actions
      aria-label="通知与账号"
      className="product-account-actions"
    >
      {!actor ? <ThemeToggle /> : null}
      {notification}
      {actor ? (
        <>
          <span className="product-account-divider" aria-hidden="true" />
          <CreditPopover navigation />
          <Button asChild variant="ghost" className="product-membership-action">
            <Link href="/billing/membership">开会员</Link>
          </Button>
        </>
      ) : null}
      {account}
    </div>
  );
}
