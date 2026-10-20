import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

const apiSyntaxRules = [
  {
    selector:
      "ImportDeclaration[source.value='axios'], ImportDeclaration[source.value='@/lib/request'] > ImportDefaultSpecifier, ImportDeclaration[source.value='@/lib/request'] > ImportNamespaceSpecifier",
    message: "Axios 只能用于 src/lib/request.ts；业务请求必须调用生成的 API。",
  },
  {
    selector: "ImportSpecifier[imported.name='request']",
    message: "业务接口必须从 @/api 导入生成函数，禁止手写请求。",
  },
  {
    selector:
      "CallExpression[callee.name='fetch'], CallExpression[callee.property.name='fetch'], NewExpression[callee.name='XMLHttpRequest']",
    message: "业务接口必须使用 Swagger 生成的 @/api 客户端。",
  },
];

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
    ignores: ["src/app/api/**", "src/lib/request.ts"],
    rules: {
      "no-restricted-syntax": ["error", ...apiSyntaxRules],
    },
  },
  {
    files: ["src/**/*.tsx"],
    ignores: ["src/app/api/**", "src/lib/request.ts", "src/components/ui/**"],
    rules: {
      "no-restricted-syntax": [
        "error",
        ...apiSyntaxRules,
        {
          selector:
            "JSXOpeningElement[name.type='JSXIdentifier'][name.name=/^(button|input|select|option|optgroup|textarea|label|fieldset|legend|details|summary|table|thead|tbody|tfoot|tr|th|td|caption|hr|progress|meter)$/]",
          message:
            "业务界面的交互控件必须使用官方 shadcn/ui 组件；form、main 等语义容器可以保留。",
        },
        {
          selector:
            "JSXAttribute[name.name='role'][value.value=/^(listbox|option|menu|menuitem|tab|tablist|combobox|checkbox|switch|dialog)$/]",
          message: "使用官方组件提供的交互语义，不在业务页面手写 ARIA 控件。",
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
