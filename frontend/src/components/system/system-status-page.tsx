import { ArrowRight, FileQuestion, LogIn, ServerCrash, ShieldX } from "lucide-react";
import Link from "next/link";

import { StudioBrand } from "@/components/studio/studio-brand";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";

type SystemStatus = "401" | "403" | "404" | "503";

const statusIcons = {
  "401": LogIn,
  "403": ShieldX,
  "404": FileQuestion,
  "503": ServerCrash,
} as const;

export function SystemStatusPage({
  status,
  title,
  description,
  primaryAction,
  secondaryAction,
}: {
  status: SystemStatus;
  title: string;
  description: string;
  primaryAction: { href: string; label: string };
  secondaryAction?: { href: string; label: string };
}) {
  const StatusIcon = statusIcons[status];

  return (
    <main className="min-h-screen bg-background text-foreground">
      <header className="border-b">
        <div className="mx-auto flex h-[72px] max-w-[1440px] items-center px-5 md:px-8">
          <StudioBrand size="l" />
        </div>
      </header>
      <section className="mx-auto flex min-h-[calc(100vh-73px)] max-w-[1120px] items-center px-5 py-16 md:px-8">
        <div className="w-full max-w-2xl">
          <div className="flex items-center gap-3 text-sm text-muted-foreground">
            <StatusIcon className="size-4" aria-hidden="true" />
            <span className="font-mono">HTTP {status}</span>
          </div>
          <Separator className="my-6" />
          <h1 className="text-4xl font-semibold tracking-tight sm:text-5xl">
            {title}
          </h1>
          <p className="mt-4 max-w-xl text-base leading-7 text-muted-foreground">
            {description}
          </p>
          <div className="mt-8 flex flex-wrap gap-3">
            <Button asChild>
              <Link href={primaryAction.href}>
                {primaryAction.label}
                <ArrowRight aria-hidden="true" />
              </Link>
            </Button>
            {secondaryAction ? (
              <Button asChild variant="outline">
                <Link href={secondaryAction.href}>{secondaryAction.label}</Link>
              </Button>
            ) : null}
          </div>
        </div>
      </section>
    </main>
  );
}
