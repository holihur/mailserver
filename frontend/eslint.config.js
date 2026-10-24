import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import tseslint from 'typescript-eslint'

// 前端 ESLint（flat config）：TS/React 基础规则 + hooks 规则。
// 说明：项目大量使用 any，故关闭 no-explicit-any；未用变量降级为 warning。
export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**', 'coverage/**'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ['**/*.{ts,tsx}'],
    languageOptions: { globals: { ...globals.browser } },
    plugins: { 'react-hooks': reactHooks },
    rules: {
      ...reactHooks.configs.recommended.rules,
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      '@typescript-eslint/no-empty-object-type': 'off',
      '@typescript-eslint/no-unused-expressions': 'off',
      'no-empty': ['warn', { allowEmptyCatch: true }],
      'no-constant-condition': 'warn',
      'no-useless-escape': 'warn',
      'no-control-regex': 'warn',
      'prefer-const': 'warn',
      'no-irregular-whitespace': 'warn',
      // eslint-plugin-react-hooks v7 新增的严格规则：现有代码大量使用「effect 内触发加载」模式，
      // 直接升级会命中 20+ 处。先显式关闭以保持 CI 稳定，后续渐进重构。
      'react-hooks/set-state-in-effect': 'off',
      'react-hooks/refs': 'off',
      'react-hooks/immutability': 'off',
    },
  },
)
