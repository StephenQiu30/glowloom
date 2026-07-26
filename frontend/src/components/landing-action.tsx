"use client";

import Link from "next/link";

import { Button } from "@/components/ui/button";

export function LandingAction() {
  return (
    <Button asChild className="mt-8">
      <Link href="/projects">进入制作工作区</Link>
    </Button>
  );
}
