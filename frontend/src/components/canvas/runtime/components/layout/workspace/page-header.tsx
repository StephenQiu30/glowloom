import type { ReactNode } from "react";
import { cn } from "cn";

/** Shared page geometry; actions retain their business permissions and state. */
export function PageHeader({
  title,
  hideTitle = false,
  description,
  children,
}: {
  title: ReactNode;
  hideTitle?: boolean;
  description?: ReactNode;
  children?: ReactNode;
}) {
  if (hideTitle && !description && !children)
    return <h1 className="sr-only">{title}</h1>;
  return (
    <header className="workspace-page-header">
      {hideTitle ? <h1 className="sr-only">{title}</h1> : null}
      {!hideTitle || description ? (
        <div className="workspace-page-heading">
          {!hideTitle ? <h1>{title}</h1> : null}
          {description ? (
            <div className="workspace-page-description">{description}</div>
          ) : null}
        </div>
      ) : null}
      {children ? (
        <div className={cn("workspace-page-actions", hideTitle && "ml-auto")}>
          {children}
        </div>
      ) : null}
    </header>
  );
}
