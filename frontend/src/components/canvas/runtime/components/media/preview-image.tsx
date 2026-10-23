"use client";

import type { ComponentProps } from "react";

/** Local blobs and editable canvas images retain their natural size and DOM refs. */
export function PreviewImage({ alt = "", ...props }: ComponentProps<"img">) {
  // Uploaded blob URLs and dynamic editor geometry bypass the Next image optimizer.
  // eslint-disable-next-line @next/next/no-img-element
  return <img alt={alt} {...props} />;
}
