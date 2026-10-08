import { type ReactNode } from "react";

export function PageHeading({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children?: ReactNode;
}) {
  return (
    <header className="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 className="text-2xl leading-7 font-semibold tracking-tight">
          {title}
        </h1>
        {description ? (
          <p className="mt-1 text-[13px] leading-5 text-muted-foreground">
            {description}
          </p>
        ) : null}
      </div>
      {children ? (
        <div className="flex flex-wrap items-center gap-2 [&>a]:h-9 [&>button]:h-9">
          {children}
        </div>
      ) : null}
    </header>
  );
}
