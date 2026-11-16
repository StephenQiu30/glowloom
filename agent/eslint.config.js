import tseslint from 'typescript-eslint';

export default [
  { ignores: ['node_modules/**', 'dist/**', 'src/database/generated/**'] },
  ...tseslint.configs.recommended,
  {
    files: ['src/business/generated/**/*.ts'],
    linterOptions: { reportUnusedDisableDirectives: 'off' },
    rules: {
      '@typescript-eslint/ban-ts-comment': 'off',
      '@typescript-eslint/no-explicit-any': 'off',
    },
  },
];
