// Run: node prepare-harness.mjs <output-directory> <downloaded-reference-worker.js>
// Produces an isolated local research page; never executes the reference bundle.
import { readFile, writeFile, mkdir, copyFile } from 'node:fs/promises';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';
const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, '../../../..');
const out = process.argv[2] && resolve(process.argv[2]);
const reference = process.argv[3] && resolve(process.argv[3]);
if (!out || !reference) throw new Error('Provide an output directory and the audited reference worker file.');
await mkdir(out, { recursive: true });
const require = createRequire(resolve(root, 'frontend/package.json'));
const ts = require('typescript');
const source = resolve(root, 'frontend/src/components/canvas/connections');
for (const name of ['connection-flow-model','connection-flow-surface']) {
  const input = await readFile(resolve(source, name + '.ts'), 'utf8');
  const output = ts.transpileModule(input, {compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ES2022}}).outputText;
  await writeFile(resolve(out, name + '.js'), output.replace('"./connection-flow-model"','"./connection-flow-model.js"'));
}
for (const [from,to] of [
  ['isolated-harness.html','isolated.html'],
  ['shader-parity.html','shader-parity.html'],
  ['isolated-geometry.json','isolated-geometry.json'],
  ['isolated-background.png','isolated-background.png'],
  ['isolated-flow-07.png','isolated-reference.png'],
]) await copyFile(resolve(here,from),resolve(out,to));
execFileSync(process.execPath, [resolve(here,'extract-reference-shaders.mjs'),reference,resolve(source,'connection-flow-surface.ts'),out], {stdio:'inherit'});
console.log('Ready. Serve this directory on 127.0.0.1 and open /shader-parity.html or /isolated.html.');
