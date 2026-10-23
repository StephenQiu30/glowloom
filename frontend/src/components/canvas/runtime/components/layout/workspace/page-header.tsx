import type { ReactNode } from "react";

/** Shared page geometry; actions retain their business permissions and state. */
export function PageHeader({
  title,
  description,
  children,
}: {
  title: ReactNode;
  description?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <header className="workspace-page-header">
      <div className="workspace-page-heading">
        <h1>{title}</h1>
        {description ? (
          <div className="workspace-page-description">{description}</div>
        ) : null}
      </div>
      {children ? (
        <div className="workspace-page-actions">{children}</div>
      ) : null}
    </header>
  );
}
