import { useMDXComponents as getThemeComponents } from "nextra-theme-docs";
import type { MDXComponents } from "nextra/mdx-components";
import type { ComponentProps } from "react";

const NativeTable = getThemeComponents().table;

function AccessibleTable(props: ComponentProps<"table">) {
  return <NativeTable {...props} tabIndex={0} />;
}

export function useMDXComponents(components: MDXComponents = {}) {
  return getThemeComponents({ table: AccessibleTable, ...components });
}
