"use client";

import Link from "next/link";
import type { ReactNode } from "react";
import { CreditPopover } from "@/components/billing/credit-popover";
import { ButtonGroup } from "@/components/ui/button-group";
import { Bell } from "lucide-react";
import { ThemeToggle } from "@/components/controls/theme-controls";
import { AccountMenu } from "@/components/account/account-menu";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ProductSearch, type PageSearch } from "./product-search";

export type ProductHeaderActionsProps = {
  search?: PageSearch;
  assetLayout?: boolean;
  pageActions?: ReactNode;
  actor?: API.EmailSessionView["actor"];
  isMobile: boolean;
  onLogin: () => void;
  onLogout: () => void;
  onFeature: (feature: string) => void;
};

/** Shared controls own their dialogs and shortcuts, independent of page routes. */
export function ProductHeaderActions({
  assetLayout = false,
  pageActions,
  search,
  actor,
  isMobile,
  onLogin,
  onLogout,
  onFeature,
}: ProductHeaderActionsProps) {
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
  if (assetLayout) {
    return (
      <div className="product-header-actions" data-asset-header-actions>
        <ButtonGroup
          variant="asset-tools"
          data-asset-tool-group
          aria-label="资产工具"
          className="max-w-full flex-wrap"
        >
          <div data-asset-search className="contents">
            <ProductSearch search={search} isMobile={isMobile} />
          </div>
          {pageActions}
        </ButtonGroup>
        <ButtonGroup
          variant="outline"
          data-asset-account-group
          aria-label="通知与账号"
          className="shrink-0"
        >
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
      </div>
    );
  }
  return (
    <div className="product-header-actions">
      <ProductSearch search={search} isMobile={isMobile} />
      <ThemeToggle />
      {notification}
      <div className="product-account-slot">{account}</div>
    </div>
  );
}
