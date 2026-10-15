// @vitest-environment node
import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { expect, it } from "vitest";

it("both themes keep ordinary surfaces, selection and keyboard focus achromatic", async () => {
  const css = await readFile("src/app/globals.css", "utf8");
  const themes = [...css.matchAll(/(?:^|\n)(:root|\.dark)\s*\{([^}]*)\}/g)];
  expect(themes).toHaveLength(2);
  const base = Object.fromEntries(
    [...themes[0][2].matchAll(/(--[\w-]+):\s*([^;]+);/g)].map((m) => [
      m[1],
      m[2],
    ]),
  );
  for (const [, name, body] of themes) {
    const tokens = {
      ...base,
      ...Object.fromEntries(
        [...body.matchAll(/(--[\w-]+):\s*([^;]+);/g)].map((m) => [m[1], m[2]]),
      ),
    };
    const resolve = (token: string): string => {
      const value = tokens[token];
      const reference = /^var\((--[\w-]+)\)$/.exec(value);
      return reference ? resolve(reference[1]) : value;
    };
    for (const token of [
      "--background",
      "--foreground",
      "--primary",
      "--secondary",
      "--muted",
      "--accent",
      "--border",
      "--input",
      "--ring",
      "--sidebar-ring",
    ]) {
      const color = resolve(token);
      const hex = /^#([\da-f]{2})([\da-f]{2})([\da-f]{2})$/i.exec(color);
      const neutral = hex
        ? hex[1] === hex[2] && hex[2] === hex[3]
        : /^oklch\([\d.]+ 0 0\)$/.test(color);
      expect(neutral, `${name} ${token}: ${color}`).toBe(true);
    }
  }
});

it("business UI uses shared colors instead of separate palettes or rich toast colors", async () => {
  const sources = await readdir("src", { recursive: true });
  const violations: string[] = [];
  for (const file of sources) {
    if (!/\.(tsx|css)$/.test(file) || /\.test\./.test(file)) continue;
    let source = await readFile(path.join("src", file), "utf8");
    if (file === "app/globals.css") {
      // The two theme definitions own color values. Page rules consume tokens.
      source = source.replace(/(?:^|\n)(?::root|\.dark)\s*\{[^}]*\}/g, "");
    }
    if (
      /\.css$/.test(file) &&
      /#[\da-f]{3,8}\b|\b(?:rgba?|hsla?|oklch|oklab)\(/i.test(source)
    )
      violations.push(`${file}: literal color`);
    if (
      /\.tsx$/.test(file) &&
      /["'`](?:#[\da-f]{3,8}|(?:rgba?|hsla?|oklch|oklab)\([^)]*\))["'`]/i.test(
        source,
      )
    )
      violations.push(`${file}: inline literal color`);
    if (
      /(?:bg|text|border|ring|outline|fill|stroke|shadow)-(?:blue|sky|cyan|green|emerald|teal|amber|yellow|orange|red|rose|pink|purple|violet|indigo|lime|fuchsia|slate|zinc|neutral|stone|gray)-\d|(?:bg|text|border|ring|outline|fill|stroke)-\[(?:#|rgba?\(|hsla?\(|oklch\()/.test(
        source,
      )
    )
      violations.push(`${file}: independent palette`);
    if (
      /\brichColors\b|var\(--brand\)|(?:bg|text|ring|border)-brand\b/.test(
        source,
      )
    )
      violations.push(`${file}: nonsemantic accent`);
  }
  expect(violations).toEqual([]);
});
