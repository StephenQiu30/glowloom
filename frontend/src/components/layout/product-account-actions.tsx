"use client";

import Link from "next/link";
import { Bell } from "lucide-react";
import { CreditPopover } from "@/components/billing/credit-popover";
import { ThemeToggle } from "@/components/controls/theme-controls";
import { AccountMenu } from "@/components/account/account-menu";
import { Button } from "@/components/ui/button";
import { ButtonGroup } from "@/components/ui/button-group";
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
          size="icon-lg"
          className="max-md:size-11"
          aria-label="消息中心"
          onClick={() => onFeature("消息中心")}
        >
          <Bell data-icon="inline-start" />
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
    <ButtonGroup
      variant="outline"
      data-account-actions
      aria-label="通知与账号"
      className="shrink-0"
    >
      {!actor ? <ThemeToggle /> : null}
      {notification}
      {actor ? (
        <>
          <CreditPopover />
          <Button asChild variant="ghost" className="max-lg:hidden">
            <Link href="/billing/membership">会员中心</Link>
          </Button>
        </>
      ) : null}
      {account}
    </ButtonGroup>
  );
}
