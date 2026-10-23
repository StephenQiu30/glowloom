import Link from "next/link";
import { Button } from "@/components/ui/button";

/** Quiet legal footer; never competes with page actions. */
export function ProductFooter() {
  return (
    <footer className="product-footer" aria-label="法律信息">
      <span className="mr-auto text-xs text-muted-foreground">
        © {new Date().getFullYear()} 织光 Glowloom
      </span>
      <Button asChild variant="link" size="sm">
        <Link href="/terms" target="_blank" rel="noopener noreferrer">
          服务条款
        </Link>
      </Button>
      <Button asChild variant="link" size="sm">
        <Link href="/privacy" target="_blank" rel="noopener noreferrer">
          隐私政策
        </Link>
      </Button>
    </footer>
  );
}
