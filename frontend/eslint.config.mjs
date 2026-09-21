import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
import prettier from "eslint-config-prettier/flat";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  prettier,
  {
    files: [
      "src/features/**/*.tsx",
      "src/components/layout/**/*.tsx",
      "src/components/studio/**/*.tsx",
    ],
    rules: {
      "no-restricted-syntax": [
        "error",
        {
          selector:
            "JSXOpeningElement[name.type='JSXIdentifier'][name.name=/^(button|input|textarea|select|table|details)$/]",
          message:
            "交互控件使用 components/ui 的 shadcn/Radix 组件；原生实现集中维护在 UI 基础层。",
        },
      ],
    },
  },
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
    // Legacy declarations remain until each endpoint migrates to generated/.
    "src/api/schema.d.ts",
    "src/api/typings.d.ts",
    "src/api/generated/**",
  ]),
]);

export default eslintConfig;
