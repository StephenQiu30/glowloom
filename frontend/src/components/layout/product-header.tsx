"use client";

import type { Ref } from "react";
import { Bell, Crown, Ellipsis, Gift, Menu } from "lucide-react";
import { AccountMenu } from "@/components/account/account-menu";
import { Brand } from "@/components/brand/brand";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
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
          <>
            {!isMobile ? (
              <Button
                variant="outline"
                size="lg"
                onClick={() => onFeature("积分商城")}
              >
                <Gift data-icon="inline-start" />
                积分商城
              </Button>
            ) : null}
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
            {isMobile ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-lg"
                    className="size-11"
                    aria-label="更多操作"
                  >
                    <Ellipsis />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent
                  align="end"
                  sideOffset={8}
                  collisionPadding={16}
                  className="w-48 max-w-[calc(100vw-2rem)]"
                >
                  <DropdownMenuGroup>
                    <DropdownMenuItem
                      className="min-h-11"
                      onSelect={() => onFeature("积分商城")}
                    >
                      <Gift />
                      积分商城
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      className="min-h-11"
                      onSelect={() => onFeature("会员")}
                    >
                      <Crown />
                      开通会员
                    </DropdownMenuItem>
                  </DropdownMenuGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : (
              <Button
                variant="ghost"
                size="lg"
                onClick={() => onFeature("会员")}
              >
                开通会员
              </Button>
            )}
            <AccountMenu actor={actor} onLogout={onLogout} />
          </>
        ) : (
          <Button size="lg" className="max-md:h-11" onClick={onLogin}>
            登录 / 注册
          </Button>
        )}
      </div>
    </header>
  );
}
