import tseslint from 'typescript-eslint';

export default [
  { ignores: ['node_modules/**', 'dist/**', 'skills/**'] },
  ...tseslint.configs.recommended,
];
