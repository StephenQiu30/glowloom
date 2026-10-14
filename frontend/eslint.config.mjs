import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

export default defineConfig([
  ...nextVitals,
  ...nextTs,
  {
    files: ["src/components/**/*.{ts,tsx}"],
    rules: {
      "no-restricted-imports": [
        "error",
        {
          patterns: [
            {
              group: ["@/app/**", "**/app/**"],
              message: "复用组件不得反向导入路由页面或页面私有组件。",
            },
          ],
        },
      ],
    },
  },
  {
    files: ["src/**/*.{ts,tsx}"],
    ignores: ["src/**/*.test.{ts,tsx}", "src/app/api/**", "src/lib/request.ts"],
    rules: {
      "no-restricted-syntax": [
        "error",
        {
          selector: "ImportSpecifier[imported.name='request']",
          message: "业务接口必须从 @/api 导入生成函数，禁止手写请求。",
        },
        {
          selector:
            "CallExpression[callee.name='fetch'], CallExpression[callee.property.name='fetch'], NewExpression[callee.name='XMLHttpRequest']",
          message: "业务接口必须使用 Swagger 生成的 @/api 客户端。",
        },
      ],
    },
  },
  globalIgnores([
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
    "src/api/**",
  ]),
]);
