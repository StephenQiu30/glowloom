"use client";
import Link from "next/link";
import { useId } from "react";
import dayjs from "dayjs";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  CardAction,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldLabel } from "@/components/ui/field";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { WorkspaceResource } from "../lib/workspace-resources";

import { ResourceCover } from "./resource-cover";
export { ResourceCover } from "./resource-cover";

export function WorkspaceResourceCard({
  resource,
  selected = false,
  disabled = false,
  onSelectionChange,
  onAction,
}: {
  resource: WorkspaceResource;
  selected?: boolean;
  disabled?: boolean;
  onSelectionChange?: (checked: boolean) => void;
  onAction?: (
    operation: "rename" | "delete",
    trigger: HTMLElement | null,
  ) => void;
}) {
  const id = useId();
  return (
    <Card
      size="sm"
      className="min-w-0 gap-3 overflow-visible rounded-none bg-transparent py-0 ring-0"
    >
      <CardContent className="relative px-0">
        <Link
          href={resource.href}
          aria-label={`打开${resource.kind === "project" ? "项目" : "画布"} ${resource.title}`}
          className={cn(
            "resource-cover-link",
            selected && "ring-1 ring-foreground",
          )}
        >
          <ResourceCover {...resource} />
        </Link>
        {onSelectionChange && (
          <Field
            orientation="horizontal"
            className="absolute top-2 right-2 size-10"
          >
            <FieldLabel htmlFor={id} className="size-10">
              <span className="sr-only">选择{resource.title}</span>
            </FieldLabel>
            <Checkbox
              className="absolute top-3 left-3"
              id={id}
              aria-label={`选择${resource.kind === "project" ? "项目" : "画布"} ${resource.title}`}
              checked={selected}
              disabled={disabled}
              onCheckedChange={(checked) => onSelectionChange(checked === true)}
            />
          </Field>
        )}
      </CardContent>
      <CardHeader className="min-h-14 items-center gap-y-2 px-0">
        <CardTitle
          title={resource.title}
          className="min-w-0 self-center truncate"
        >
          <Link href={resource.href}>{resource.title}</Link>
        </CardTitle>
        {onAction && (
          <CardAction className="flex gap-2">
            <Button
              variant="ghost"
              size="sm"
              className="h-8 px-2 max-md:min-h-11"
              disabled={disabled}
              aria-label={`编辑 ${resource.title}`}
              onClick={(event) => onAction("rename", event.currentTarget)}
            >
              编辑
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="h-8 px-2 max-md:min-h-11"
              disabled={disabled}
              aria-label={`删除 ${resource.title}`}
              onClick={(event) => onAction("delete", event.currentTarget)}
            >
              删除
            </Button>
          </CardAction>
        )}
        <CardDescription className="text-xs">
          <time dateTime={resource.updatedAt}>
            {dayjs(resource.updatedAt).format("YYYY-MM-DD")}
          </time>
        </CardDescription>
      </CardHeader>
    </Card>
  );
}
