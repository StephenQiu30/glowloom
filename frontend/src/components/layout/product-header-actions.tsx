"use client";

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
  actor?: API.EmailSessionView["actor"];
  isMobile: boolean;
  onLogin: () => void;
  onLogout: () => void;
  onFeature: (feature: string) => void;
};

/** Shared controls own their dialogs and shortcuts, independent of page routes. */
export function ProductHeaderActions({
  search,
  actor,
  isMobile,
  onLogin,
  onLogout,
  onFeature,
}: ProductHeaderActionsProps) {
  return (
    <div className="product-header-actions">
      <ProductSearch search={search} isMobile={isMobile} />
      <ThemeToggle />
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon-lg"
            className="max-md:size-11"
            aria-label="消息中心"
            onClick={() => onFeature("消息中心")}
          >
            <Bell />
          </Button>
        </TooltipTrigger>
        <TooltipContent>消息中心</TooltipContent>
      </Tooltip>
      <div className="product-account-slot">
        {actor ? (
          <AccountMenu actor={actor} onLogout={onLogout} />
        ) : (
          <Button className="w-full" onClick={onLogin}>
            登录
          </Button>
        )}
      </div>
    </div>
  );
}
