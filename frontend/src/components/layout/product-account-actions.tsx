"use client";

import { AccountToolbarItems } from "@/components/account/account-toolbar-items";
import { ToolbarGroup } from "@/components/ui/toolbar";

export type ProductAccountActionsProps = {
  actor?: API.EmailSessionView["actor"];
  onLogin: () => void;
  onLogout: () => void;
};

export function ProductAccountActions(props: ProductAccountActionsProps) {
  return (
    <ToolbarGroup
      data-account-actions
      data-guest-account={!props.actor || undefined}
      aria-label="通知与账号"
    >
      <AccountToolbarItems {...props} />
    </ToolbarGroup>
  );
}
