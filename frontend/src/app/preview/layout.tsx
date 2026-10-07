import { Toaster } from "@/components/ui/sonner";

export default function PreviewLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="dark preview-charts min-h-svh bg-background text-foreground">
      <Toaster theme="dark" richColors />
      {children}
    </div>
  );
}
