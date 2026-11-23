"use client";

import { useSelectedLayoutSegment } from "next/navigation";
import type { ReactNode } from "react";
import { billingSections } from "@/components/billing/billing-sections";
import { HeaderLinks } from "@/components/layout/header-navigation";

const links = Object.entries(billingSections).map(([id, section]) => ({
  id,
  ...section,
}));

export default function BillingLayout({ children }: { children: ReactNode }) {
  const active = useSelectedLayoutSegment() ?? "overview";
  return (
    <>
      <HeaderLinks label="账务导航" active={active} links={links} />
      {children}
    </>
  );
}
