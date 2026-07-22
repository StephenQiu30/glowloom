import Link from "next/link";

import { LogoutButton } from "@/components/logout-button";
import { Button } from "@/components/ui/button";
import { hasActiveSession } from "@/lib/session";

export async function SessionActions() {
  if (await hasActiveSession()) {
    return <LogoutButton />;
  }
  return (
    <Button asChild size="sm" variant="ghost">
      <Link href="/login">登录</Link>
    </Button>
  );
}
