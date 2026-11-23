"use client";

import { CreditPopover } from "@/components/billing/credit-popover";
import { ThemeToggle } from "@/components/controls/theme-controls";
import { AccountMenu } from "@/components/account/account-menu";
import { Button } from "@/components/ui/button";
import { ToolbarGroup } from "@/components/ui/toolbar";
import { NotificationBell } from "@/components/notifications/notification-bell";
import { Separator } from "@/components/ui/separator";

export type ProductAccountActionsProps = {
  actor?: API.EmailSessionView["actor"];
  onLogin: () => void;
  onLogout: () => void;
};

/** Figma ProductAccountActions 3165:58374; one account area across product routes. */
export function ProductAccountActions({
  actor,
  onLogin,
  onLogout,
}: ProductAccountActionsProps) {
  const account = actor ? (
    <AccountMenu actor={actor} onLogout={onLogout} />
  ) : (
    <Button size="toolbar" onClick={onLogin}>
      登录
    </Button>
  );
  return (
    <ToolbarGroup
      data-account-actions
      data-guest-account={!actor || undefined}
      aria-label="通知与账号"
    >
      {!actor ? <ThemeToggle toolbar /> : null}
      <NotificationBell onLogin={onLogin} />
      {actor ? (
        <>
          <Separator
            orientation="vertical"
            className="product-account-divider"
          />
          <CreditPopover navigation />
        </>
      ) : null}
      {account}
    </ToolbarGroup>
  );
}
