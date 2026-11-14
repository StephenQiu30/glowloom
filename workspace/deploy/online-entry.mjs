import { readFile } from "node:fs/promises";
import { resolve, dirname } from "node:path";
import { pathToFileURL } from "node:url";
const release = JSON.parse(await readFile("release.json", "utf8"));
if (release.mode !== "online" || !release.entry || release.entry.includes(".."))
  throw new Error("Invalid online release");
const entry = resolve(release.entry);
process.chdir(dirname(entry));
await import(pathToFileURL(entry).href);
