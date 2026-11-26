import { existsSync, readdirSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const WORKSPACE_DIR = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '..',
);
const REPO_ROOT = path.resolve(WORKSPACE_DIR, '..');
const CONTENT_DIR = path.join(WORKSPACE_DIR, 'content');
const CATEGORIES = [
  'requirement',
  'design',
  'prd',
  'plan',
  'research',
  'operations',
  'templates',
];
const ROOT_DOCS = [
  'AGENTS.md',
  'BACKLOG.md',
  'README.md',
  'workspace/AGENTS.md',
  'workspace/ARCHITECTURE.md',
  'workspace/PROJECT.md',
  'workspace/README.md',
];

const LINK_RE = /\[[^\]]*\]\(([^)]+)\)/g;
const errors = [];

function checkLinks(filePath) {
  const text = readFileSync(filePath, 'utf8');
  const baseDir = path.dirname(filePath);
  const relFile = path.relative(REPO_ROOT, filePath);

  for (const match of text.matchAll(LINK_RE)) {
    const rawHref = match[1].trim().split(/\s+/)[0];
    if (!rawHref) continue;
    if (/^plugin:\/\//i.test(rawHref)) {
      errors.push(`[private-link] ${relFile} -> ${rawHref}`);
      continue;
    }
    if (/^(?:https?:|mailto:|data:|#|\/)/i.test(rawHref)) continue;
    const targetPath = rawHref.split('#')[0].split('?')[0];
    if (!targetPath) continue;
    const resolved = path.resolve(baseDir, decodeURI(targetPath));
    if (!existsSync(resolved)) {
      errors.push(`[broken-link] ${relFile} -> ${rawHref}`);
    }
  }
}

for (const category of CATEGORIES) {
  const categoryDir = path.join(CONTENT_DIR, category);
  if (!existsSync(categoryDir)) {
    errors.push(`[missing-category] workspace/content/${category}`);
    continue;
  }

  const mdFiles = readdirSync(categoryDir)
    .filter((name) => name.endsWith('.md'))
    .sort();
  const seenNumbers = new Map();
  let navFile = null;

  for (const fileName of mdFiles) {
    const fullPath = path.join(categoryDir, fileName);
    const relFile = path.relative(REPO_ROOT, fullPath);
    const numMatch = /^(\d{2})-/.exec(fileName);

    if (!numMatch) {
      errors.push(`[naming] ${relFile} must start with NN-`);
    } else {
      const num = numMatch[1];
      if (seenNumbers.has(num)) {
        errors.push(
          `[duplicate-number] ${relFile} conflicts with ${seenNumbers.get(num)}`,
        );
      } else {
        seenNumbers.set(num, fileName);
      }
      if (num === '00') {
        navFile = fileName;
      }
    }

    const content = readFileSync(fullPath, 'utf8');
    if (!/^#\s+/m.test(content) && !/^title:/m.test(content)) {
      errors.push(`[missing-title] ${relFile} has no H1 or frontmatter title`);
    }

    if (numMatch?.[1] !== '00') {
      const header = content.slice(0, 1500);
      if (category === 'prd' && !/\.\.\/design\//.test(header)) {
        errors.push(`[traceability] ${relFile} missing ../design/ source link`);
      }
      if (category === 'plan' && !/\.\.\/prd\//.test(header)) {
        errors.push(`[traceability] ${relFile} missing ../prd/ source link`);
      }
    }

    checkLinks(fullPath);
  }

  if (!navFile) {
    errors.push(`[missing-nav] workspace/content/${category} has no 00-* file`);
    continue;
  }

  const navContent = readFileSync(path.join(categoryDir, navFile), 'utf8');
  for (const fileName of mdFiles) {
    if (fileName === navFile) continue;
    if (!navContent.includes(fileName)) {
      errors.push(
        `[orphan-doc] workspace/content/${category}/${fileName} not listed in ${navFile}`,
      );
    }
  }
}

const indexPath = path.join(CONTENT_DIR, 'index.mdx');
if (existsSync(indexPath)) {
  checkLinks(indexPath);
}

for (const relDoc of ROOT_DOCS) {
  const fullPath = path.join(REPO_ROOT, relDoc);
  if (existsSync(fullPath)) {
    checkLinks(fullPath);
  } else if (
    relDoc === 'workspace/ARCHITECTURE.md' ||
    relDoc === 'workspace/PROJECT.md'
  ) {
    errors.push(`[missing-project-doc] ${relDoc}`);
  }
}

if (errors.length > 0) {
  console.error(`docs:check failed with ${errors.length} issue(s):`);
  for (const err of errors) {
    console.error(`  - ${err}`);
  }
  process.exit(1);
}

console.log(
  `docs:check passed (${CATEGORIES.length} categories, root specs & links verified).`,
);
