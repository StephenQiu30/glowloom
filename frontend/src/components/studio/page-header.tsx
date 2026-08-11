import { Fragment, type ReactNode } from "react";
import Link from "next/link";
import type { VariantProps } from "class-variance-authority";

import { Badge, type badgeVariants } from "@/components/ui/badge";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemHeader,
} from "@/components/ui/item";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/class-names";

type BadgeVariant = VariantProps<typeof badgeVariants>["variant"];

export type PageHeaderCrumb = {
  label: string;
  href?: string;
};

export type PageHeaderBadge = {
  label: ReactNode;
  variant?: BadgeVariant;
};

export function PageHeader({
  actions,
  accessibleTitle,
  badges = [],
  breadcrumbs = [],
  className,
  description,
  eyebrow,
  note,
  title,
}: {
  accessibleTitle?: ReactNode;
  actions?: ReactNode;
  badges?: PageHeaderBadge[];
  breadcrumbs?: PageHeaderCrumb[];
  className?: string;
  description?: ReactNode;
  eyebrow?: ReactNode;
  note?: ReactNode;
  title: ReactNode;
}) {
  return (
    <header className={cn("grid gap-6", className)}>
      {breadcrumbs.length ? (
        <Breadcrumb>
          <BreadcrumbList>
            {breadcrumbs.map((crumb, index) => {
              const current = index === breadcrumbs.length - 1;
              return (
                <Fragment key={`${crumb.href ?? "current"}:${crumb.label}`}>
                  <BreadcrumbItem>
                    {crumb.href && !current ? (
                      <BreadcrumbLink asChild>
                        <Link href={crumb.href}>{crumb.label}</Link>
                      </BreadcrumbLink>
                    ) : (
                      <BreadcrumbPage>{crumb.label}</BreadcrumbPage>
                    )}
                  </BreadcrumbItem>
                  {!current ? <BreadcrumbSeparator /> : null}
                </Fragment>
              );
            })}
          </BreadcrumbList>
        </Breadcrumb>
      ) : null}

      <Item className="items-end gap-6 rounded-none p-0">
        <ItemContent className="min-w-0 gap-0">
          {badges.length ? (
            <ItemHeader className="mb-4 flex-wrap justify-start">
              {badges.map((badge, index) => (
                <Badge key={index} variant={badge.variant ?? "outline"}>
                  {badge.label}
                </Badge>
              ))}
            </ItemHeader>
          ) : eyebrow ? (
            <ItemHeader className="mb-3 justify-start text-sm font-medium text-foreground">
              {eyebrow}
            </ItemHeader>
          ) : null}

          <h1 className="text-4xl font-semibold tracking-tight text-balance sm:text-5xl">
            {title}
          </h1>
          {accessibleTitle ? <h2 className="sr-only">{accessibleTitle}</h2> : null}
          {description ? (
            <ItemDescription className="mt-3 max-w-2xl line-clamp-none text-base leading-7">
              {description}
            </ItemDescription>
          ) : null}
          {note ? (
            <ItemDescription className="mt-4 line-clamp-none font-medium text-foreground">
              {note}
            </ItemDescription>
          ) : null}
        </ItemContent>
        {actions ? <ItemActions className="w-full justify-start sm:w-auto sm:justify-end">{actions}</ItemActions> : null}
      </Item>
      <Separator />
    </header>
  );
}
