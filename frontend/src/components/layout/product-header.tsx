"use client";

import type { Ref } from "react";
import { Bell, Menu } from "lucide-react";
import { ThemeToggle } from "@/components/controls/theme-controls";
import { AccountMenu } from "@/components/account/account-menu";
import { Brand } from "@/components/brand/brand";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { screens, type Screen } from "./routes";

/** Figma ProductHeader 620:24416; shared by ordinary product pages. */
export function ProductHeader({
  screen,
  actor,
  isMobile,
  navigationOpen,
  navigationTriggerRef,
  onOpenNavigation,
  onLogin,
  onLogout,
  onFeature,
}: {
  screen: Screen;
  actor?: API.EmailSessionView["actor"];
  isMobile: boolean;
  navigationOpen: boolean;
  navigationTriggerRef?: Ref<HTMLButtonElement>;
  onOpenNavigation: () => void;
  onLogin: () => void;
  onLogout: () => void;
  onFeature: (feature: string) => void;
}) {
  const title = screens.find((item) => item.id === screen)?.name;

  return (
    <header className="product-header" aria-label="页面顶栏">
      <div className="product-header-location">
        {isMobile ? (
          <>
            <Button
              variant="ghost"
              size="icon-lg"
              ref={navigationTriggerRef}
              className="mobile-navigation-trigger"
              aria-label="打开导航"
              aria-expanded={navigationOpen}
              onClick={onOpenNavigation}
            >
              <Menu />
            </Button>
            <Brand />
          </>
        ) : null}
        <span className="truncate text-sm font-medium" title={title}>
          {title}
        </span>
      </div>
      <div className="product-header-actions">
        {actor ? (
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
        ) : null}
        <ThemeToggle />
        {actor ? (
          <AccountMenu actor={actor} onLogout={onLogout} />
        ) : (
          <Button size="lg" className="max-md:h-11" onClick={onLogin}>
            登录 / 注册
          </Button>
        )}
      </div>
    </header>
  );
}
