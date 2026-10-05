import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: { port: 5173, proxy: { '/api': 'http://localhost:8080' } },
  build: {
    outDir: 'dist',
    chunkSizeWarningLimit: 600,
    // 路由级懒加载已按页面分包；把 React 运行时拆成独立 chunk 以利长期缓存。
    rollupOptions: {
      output: {
        manualChunks: { react: ['react', 'react-dom', 'react-router-dom'] },
      },
    },
  },
  test: {
    environment: 'node',
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/**/*.test.*', 'src/main.tsx', 'src/**/*.d.ts'],
      // 核心文件覆盖率门（#20）：api/client 与 lib/utils 必须有单测兜底。
      thresholds: {
        'src/api/client.ts': { statements: 90, branches: 80, lines: 90 },
        'src/lib/utils.ts': { statements: 80, branches: 70, functions: 80, lines: 80 },
      },
    },
  },
})
