"use client";

import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";
import { Tabs as TabsPrimitive } from "radix-ui";

function Tabs({
  className,
  orientation = "horizontal",
  ...props
}: React.ComponentProps<typeof TabsPrimitive.Root>) {
  return (
    <TabsPrimitive.Root
      data-slot="tabs"
      data-orientation={orientation}
      className={cn(
        "group/tabs flex gap-2 data-horizontal:flex-col",
        className,
      )}
      {...props}
    />
  );
}

const tabsListVariants = cva(
  "group/tabs-list inline-flex w-fit items-center justify-center rounded-lg p-[3px] text-muted-foreground group-data-horizontal/tabs:h-9 group-data-vertical/tabs:h-fit group-data-vertical/tabs:flex-col data-[variant=line]:rounded-none",
  {
    variants: {
      variant: {
        default:
          "min-w-0 max-w-full justify-start overflow-x-auto bg-muted overscroll-x-contain [scrollbar-width:none] max-md:group-data-horizontal/tabs:min-h-[50px] pointer-coarse:group-data-horizontal/tabs:min-h-[50px]",
        line: "gap-1 bg-transparent",
        secondary:
          "min-w-0 max-w-full justify-start gap-(--secondary-tab-gap) overflow-x-auto rounded-none bg-transparent p-0 overscroll-x-contain [scrollbar-width:none] group-data-horizontal/tabs:h-auto",
        pill: "gap-2 bg-transparent p-0",
        workspace:
          "min-w-0 max-w-full justify-start gap-1 overflow-x-auto rounded-none bg-transparent p-0 [scrollbar-width:none] group-data-horizontal/tabs:h-auto",
        notification: "w-full bg-muted group-data-horizontal/tabs:h-11",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  },
);

const TabsListVariantContext =
  React.createContext<VariantProps<typeof tabsListVariants>["variant"]>(
    "default",
  );

function TabsList({
  className,
  variant = "default",
  children,
  ...props
}: React.ComponentProps<typeof TabsPrimitive.List> &
  VariantProps<typeof tabsListVariants>) {
  return (
    <TabsPrimitive.List
      data-slot="tabs-list"
      data-variant={variant}
      className={cn(tabsListVariants({ variant }), className)}
      {...props}
    >
      <TabsListVariantContext.Provider value={variant}>
        {children}
      </TabsListVariantContext.Provider>
    </TabsPrimitive.List>
  );
}

function TabsTrigger({
  className,
  ...props
}: React.ComponentProps<typeof TabsPrimitive.Trigger>) {
  const variant = React.useContext(TabsListVariantContext);
  return (
    <TabsPrimitive.Trigger
      data-slot="tabs-trigger"
      className={cn(
        "relative inline-flex h-[calc(100%-1px)] flex-1 items-center justify-center gap-1.5 rounded-md border border-transparent px-1.5 py-0.5 text-sm font-medium whitespace-nowrap text-foreground/60 transition-all outline-none group-data-vertical/tabs:w-full group-data-vertical/tabs:justify-start hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 has-data-[icon=inline-end]:pr-1 has-data-[icon=inline-start]:pl-1 group-data-[variant=default]/tabs-list:data-active:shadow-sm group-data-[variant=line]/tabs-list:data-active:shadow-none [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        "group-data-[variant=line]/tabs-list:bg-transparent group-data-[variant=line]/tabs-list:data-active:bg-transparent dark:group-data-[variant=line]/tabs-list:data-active:border-transparent dark:group-data-[variant=line]/tabs-list:data-active:bg-transparent",
        variant !== "workspace" &&
          variant !== "secondary" &&
          variant !== "notification" &&
          "dark:text-muted-foreground dark:hover:text-foreground data-active:bg-background data-active:text-foreground dark:data-active:border-transparent dark:data-active:bg-input/30 dark:data-active:text-foreground",
        variant === "secondary" &&
          "min-h-(--control-height) flex-none rounded-none border-0 px-(--secondary-tab-padding-inline) text-sm text-muted-foreground hover:text-foreground focus-visible:ring-inset pointer-coarse:min-h-11 data-active:text-foreground",
        "group-data-[variant=pill]/tabs-list:rounded-full group-data-[variant=pill]/tabs-list:px-4 group-data-[variant=pill]/tabs-list:font-normal group-data-[variant=pill]/tabs-list:data-active:border-transparent group-data-[variant=pill]/tabs-list:data-active:bg-secondary",
        variant === "default" &&
          "focus-visible:ring-inset max-md:min-h-11 pointer-coarse:min-h-11 dark:data-active:bg-background",
        variant === "workspace" &&
          "h-9 flex-none rounded-none border-0 px-3 text-[length:var(--text-workspace)] leading-[var(--text-workspace--line-height)] font-medium text-workspace-muted-foreground hover:text-workspace-foreground focus-visible:ring-inset max-md:min-h-11 pointer-coarse:min-h-11 data-active:bg-secondary data-active:text-workspace-foreground",
        variant === "notification" &&
          "min-h-9 border-0 text-muted-foreground data-active:bg-accent data-active:text-accent-foreground",
        "after:absolute after:bg-foreground after:opacity-0 after:transition-opacity group-data-horizontal/tabs:after:inset-x-0 group-data-horizontal/tabs:after:bottom-[-5px] group-data-horizontal/tabs:after:h-0.5 group-data-vertical/tabs:after:inset-y-0 group-data-vertical/tabs:after:-right-1 group-data-vertical/tabs:after:w-0.5 group-data-[variant=line]/tabs-list:data-active:after:opacity-100",
        className,
      )}
      {...props}
    />
  );
}

function TabsContent({
  className,
  ...props
}: React.ComponentProps<typeof TabsPrimitive.Content>) {
  return (
    <TabsPrimitive.Content
      data-slot="tabs-content"
      className={cn("flex-1 text-sm outline-none", className)}
      {...props}
    />
  );
}

export { Tabs, TabsList, TabsTrigger, TabsContent, tabsListVariants };
