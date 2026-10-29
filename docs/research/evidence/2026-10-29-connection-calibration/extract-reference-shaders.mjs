#!/usr/bin/env node
/**
 * Reproduce the shader-only comparison without executing any third-party code.
 * Usage: node extract-reference-shaders.mjs <worker.js> <product-surface.ts> <output-directory>
 * The exact audited worker is required; its full bundle is not a repository artifact.
 */
import { createHash } from 'node:crypto';
import { readFile, mkdir, writeFile } from 'node:fs/promises';
import { resolve, join } from 'node:path';

const EXPECTED_SHA256 = 'd9970f672832d62c1e90f47f49bc1378771e69b79ced734228ffc78560d73651';
const [workerPath, productPath, outputPath, ...extra] = process.argv.slice(2);
if (!workerPath || !productPath || !outputPath || extra.length) {
  console.error('Usage: node extract-reference-shaders.mjs <worker.js> <product-surface.ts> <output-directory>');
  process.exit(2);
}
const workerBytes = await readFile(resolve(workerPath));
const digest = createHash('sha256').update(workerBytes).digest('hex');
if (digest !== EXPECTED_SHA256) {
  throw new Error(`Worker identity mismatch: expected ${EXPECTED_SHA256}, received ${digest}. No files written.`);
}
const worker = workerBytes.toString('utf8');
const product = await readFile(resolve(productPath), 'utf8');

// Literal string extraction only: no import(), eval(), Function(), vm, or bundle execution.
function templateAfter(source, anchor, start = 0) {
  const anchorAt = source.indexOf(anchor, start);
  if (anchorAt < 0) throw new Error(`Required anchor missing: ${anchor}`);
  const opening = source.indexOf('`', anchorAt);
  const closing = source.indexOf('`', opening + 1);
  if (opening < 0 || closing < 0) throw new Error(`Template incomplete: ${anchor}`);
  return { source: source.slice(opening + 1, closing), end: closing + 1 };
}
const vertex = templateAfter(worker, '`#version 300 es');
const fragment = templateAfter(worker, '`#version 300 es', vertex.end);
// Audited literal values: dark glow rgba(0,142,229,.22), radius/mesh/sweep constants.
const substitutions = new Map([
  ['${Z(7)}', '7.0'],
  ['${Z(.55)}', '0.55'],
  ['${Z(120)}', '120.0'],
  ['${Z(312)}', '312.0'],
  ['${Z(72)}', '72.0'],
  ['${Z(1)}', '1.0'],
  ['${Z(1.5)}', '1.5'],
  ['${Z(r)}', '0.22'],
  ['${Z(a.red/255)}', '0.0'],
  ['${Z(a.green/255)}', String(142 / 255)],
  ['${Z(a.blue/255)}', String(229 / 255)],
]);
function resolveAuditedConstants(template) {
  let result = template;
  for (const [literal, value] of substitutions) result = result.replaceAll(literal, value);
  if (result.includes('${')) throw new Error('Unexpected unprocessed template expression; refusing extraction.');
  if (!result.startsWith('#version 300 es\n')) throw new Error('Unexpected GLSL header.');
  return result;
}
const outputs = {
  'reference-vertex.glsl': resolveAuditedConstants(vertex.source),
  'reference-fragment.glsl': resolveAuditedConstants(fragment.source),
  'product-vertex.glsl': templateAfter(product, 'const vertexShader =').source,
  'product-fragment.glsl': templateAfter(product, 'const fragmentShader =').source,
};
for (const [name, source] of Object.entries(outputs)) {
  if (!source.startsWith('#version 300 es\n') || source.includes('${')) {
    throw new Error(`Unsupported product or reference template: ${name}. No files written.`);
  }
}
const outputDirectory = resolve(outputPath);
await mkdir(outputDirectory, { recursive: true });
for (const [name, source] of Object.entries(outputs)) await writeFile(join(outputDirectory, name), source);
console.log(JSON.stringify({
  workerSha256: digest,
  productSha256: createHash('sha256').update(product).digest('hex'),
  method: 'Static literal extraction and fixed audited constant substitution; no third-party execution',
  outputs: Object.entries(outputs).map(([name, source]) => ({
    path: join(outputDirectory, name),
    bytes: Buffer.byteLength(source),
    sha256: createHash('sha256').update(source).digest('hex'),
  })),
}, null, 2));
