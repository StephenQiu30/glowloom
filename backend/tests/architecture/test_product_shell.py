from __future__ import annotations

import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
FRONTEND = ROOT / "frontend/src"


class ProductShellContractTests(unittest.TestCase):
    def test_product_routes_and_shared_shell_exist(self) -> None:
        required = (
            "app/explore/page.tsx",
            "app/login/page.tsx",
            "app/create/page.tsx",
            "app/works/page.tsx",
            "app/admin/page.tsx",
            "components/page-placeholder.tsx",
            "components/site-header.tsx",
        )

        self.assertEqual(
            [path for path in required if not (FRONTEND / path).is_file()],
            [],
        )

    def test_navigation_exposes_the_accepted_information_architecture(self) -> None:
        header = (FRONTEND / "components/site-header.tsx").read_text()
        session_actions = (FRONTEND / "components/session-actions.tsx").read_text()
        layout = (FRONTEND / "app/layout.tsx").read_text()

        self.assertIn('href="/"', header)
        for href in ("/explore", "/create", "/works", "/admin"):
            self.assertIn(f'href: "{href}"', header)
        self.assertIn('href="/login"', session_actions)
        self.assertIn("<SiteHeader />", layout)

    def test_each_route_has_a_distinct_server_rendered_purpose(self) -> None:
        expected = {
            "app/page.tsx": "发现灵感，开始创作",
            "app/explore/page.tsx": "探索灵感",
            "app/login/page.tsx": "邀请制登录",
            "app/create/page.tsx": "开始创作",
            "app/works/page.tsx": "我的作品",
            "app/admin/page.tsx": "管理中心",
        }

        for path, heading in expected.items():
            source = (FRONTEND / path).read_text()
            self.assertIn(heading, source)
            self.assertNotIn('"use client"', source)

    def test_route_states_have_accessible_status_semantics(self) -> None:
        for path in ("app/error.tsx", "app/not-found.tsx"):
            self.assertIn("<h1>", (FRONTEND / path).read_text())

        loading = (FRONTEND / "app/loading.tsx").read_text()
        self.assertIn('aria-busy="true"', loading)
        self.assertIn("页面加载中", loading)

    def test_login_uses_a_small_client_form_and_shadcn_primitives(self) -> None:
        login_page = (FRONTEND / "app/login/page.tsx").read_text()
        login_form = (FRONTEND / "components/login-form.tsx").read_text()

        self.assertIn("<LoginForm", login_page)
        self.assertNotIn('"use client"', login_page)
        self.assertIn('"use client"', login_form)
        for primitive in ("alert", "input", "label"):
            self.assertTrue((FRONTEND / f"components/ui/{primitive}.tsx").is_file())
        for expected in (
            'type="email"',
            'type="password"',
            'fetch("/api/session"',
            "window.location.assign(returnTo)",
            'role="alert"',
        ):
            self.assertIn(expected, login_form)

    def test_web_proxies_sessions_and_protects_private_routes(self) -> None:
        route = (FRONTEND / "app/api/session/route.ts").read_text()
        proxy = (FRONTEND / "proxy.ts").read_text()
        header = (FRONTEND / "components/site-header.tsx").read_text()

        for method in ("GET", "POST", "DELETE"):
            self.assertIn(f"export async function {method}", route)
        self.assertIn('headers.getSetCookie()', route)
        self.assertIn('request.headers.get("x-csrf-token")', route)
        self.assertIn('request.cookies.get("thief_session")', proxy)
        for private_path in ("/create", "/works", "/admin"):
            self.assertIn(private_path, proxy)
        self.assertIn("returnTo", proxy)
        self.assertIn("<SessionActions />", header)

    def test_public_catalog_ui_uses_generated_contracts_and_real_routes(self) -> None:
        required = (
            "app/templates/[id]/page.tsx",
            "components/catalog-filter-form.tsx",
            "components/template-card.tsx",
            "lib/api-schema.d.ts",
            "lib/catalog.ts",
        )
        self.assertEqual(
            [path for path in required if not (FRONTEND / path).is_file()],
            [],
        )

        home = (FRONTEND / "app/page.tsx").read_text()
        explore = (FRONTEND / "app/explore/page.tsx").read_text()
        detail = (FRONTEND / "app/templates/[id]/page.tsx").read_text()
        catalog = (FRONTEND / "lib/catalog.ts").read_text()

        for source in (home, explore, detail):
            self.assertNotIn("PagePlaceholder", source)
            self.assertNotIn('"use client"', source)
        self.assertIn("最新灵感", home)
        for query in ("q", "category", "model", "aspect_ratio", "source"):
            self.assertIn(query, explore)
        self.assertIn("searchParams: Promise", explore)
        self.assertIn("做同款", detail)
        self.assertIn("notFound()", detail)
        self.assertIn('from "@/lib/api-schema"', catalog)
        self.assertIn('cache: "no-store"', catalog)

    def test_openapi_types_have_a_repeatable_drift_gate(self) -> None:
        makefile = (ROOT / "Makefile").read_text()

        for target in ("generate-contracts", "verify-contracts"):
            self.assertIn(f"{target}:", makefile)
        self.assertIn("verify-contracts", makefile.split("verify:", 1)[1])


if __name__ == "__main__":
    unittest.main()
