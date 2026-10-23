import type { ReactNode } from "react";

/** Final Figma PageTitle: one title/action row for every workspace page. */
export function PageHeading({
  title,
  actions,
}: {
  title: string;
  actions?: ReactNode;
}) {
  return (
    <div className="page-heading">
      <h1>{title}</h1>
      {actions ? <div className="page-heading-actions">{actions}</div> : null}
    </div>
  );
}
