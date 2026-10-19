import Link from "next/link";
import { Button } from "@/components/ui/button";

/** Legal navigation belongs on account recovery pages, not a second workspace toolbar. */
export function ProductFooter() {
  return (
    <footer className="product-footer" aria-label="法律信息">
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
