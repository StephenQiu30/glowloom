import { defineConfig, globalIgnores } from 'eslint/config';
import nextVitals from 'eslint-config-next/core-web-vitals';
import nextTs from 'eslint-config-next/typescript';

export default defineConfig([
  ...nextVitals,
  ...nextTs,
  {
    files: ['**/*.ts', '**/*.tsx'],
    rules: {
      curly: ['error', 'all'],
      'one-var': ['error', 'never'],
      'no-var': 'error',
      'prefer-const': 'error',
      eqeqeq: ['error', 'always'],
      quotes: [
        'error',
        'single',
        { avoidEscape: true, allowTemplateLiterals: false },
      ],
      'func-style': ['error', 'declaration', { allowArrowFunctions: false }],
      '@typescript-eslint/consistent-type-definitions': ['error', 'interface'],
      'no-restricted-syntax': [
        'error',
        {
          selector:
            'Program > :not(ImportDeclaration):not(ExpressionStatement[directive]) ~ ImportDeclaration',
          message: 'Keep imports before declarations (Google TS).',
        },
        {
          selector: 'TSModuleDeclaration',
          message: 'Use modules instead of namespaces (Google TS).',
        },
        {
          selector: 'TSEnumDeclaration[const=true]',
          message: 'Do not use const enums (Google TS).',
        },
      ],
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/ban-ts-comment': [
        'error',
        {
          'ts-ignore': true,
          'ts-expect-error': true,
          'ts-nocheck': true,
          'ts-check': false,
        },
      ],
    },
  },
  {
    files: [
      'src/runtime/**/*.ts',
      'src/hooks/**/*.ts',
      'tests/**/*.ts',
      'src/components/**/*.tsx',
      'src/content/**/*.ts',
      'src/build/**/*.ts',
      'src/narration/**/*.ts',
      'src/server/**/*.ts',
      'src/workflows/**/*.ts',
      'src/online/components/**/*.ts',
      'src/online/components/**/*.tsx',
    ],
    rules: {
      'no-restricted-exports': [
        'error',
        {
          restrictDefaultExports: {
            direct: true,
            named: true,
            defaultFrom: true,
            namedFrom: true,
            namespaceFrom: true,
          },
        },
      ],
    },
  },
  globalIgnores([
    '.build/**',
    '.local/**',
    'dist/**',
    'content/**',
    'playwright-report/**',
    'test-results/**',
  ]),
]);

// The framework entry files retain the default exports required by Next.js.
// Project-owned UI, runtime and tests use the same TypeScript rules.
