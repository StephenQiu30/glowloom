import { Suspense } from "react";
import { LoginScreen } from "@/features/auth/login-screen";

export const metadata = { title: "登录 | Lanverse" };
export default function LoginPage() {
  return (
    <Suspense
      fallback={
        <main className="p-10">
          <p role="status">正在加载登录页…</p>
        </main>
      }
    >
      <LoginScreen />
    </Suspense>
  );
}
