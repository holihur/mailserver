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
})
