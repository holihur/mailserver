# Mailserver — 轻量全栈邮件系统

前端 React + TailwindCSS + shadcn 风格 · 后端 Go + GORM · 单文件 SQLite

> 低内存设计：后端单二进制 + SQLite（无 Postgres/Redis），前端 Vite 构建后仅静态文件。
> 内存占用：后端常驻约 15~30MB。

## 目录

```
mailserver/
├── backend/          # Go (stdlib net/http + GORM, 无 Gin)
│   ├── main.go
│   ├── internal/
│   │   ├── config/config.go
│   │   ├── model/models.go
│   │   ├── db/db.go
│   │   ├── auth/jwt.go
│   │   ├── handler/auth.go
│   │   ├── handler/mail.go
│   │   └── smtp/server.go
│   ├── go.mod
│   └── .env.example
├── frontend/         # React + Vite + Tailwind + shadcn 风格 ui/
│   ├── package.json
│   ├── vite.config.js
│   ├── tailwind.config.js
│   ├── index.html
│   └── src/
└── docker-compose.yml
```

## 快速启动（低内存）

```bash
# 后端 (~20MB)
cd backend
cp .env.example .env
go mod tidy          # 仅首次，需网络
go run .             # 或 go build -ldflags="-s -w" mailserver && ./mailserver

# 前端 dev
cd frontend
npm i --no-audit --no-fund   # 内存小就加 --max-old-space-size=256
npm run dev

# 或 docker（仅 2 容器，总 <150MB）
docker compose up -d --build
```

API 默认 `:8080`，SMTP 入站 `:2525`，前端 `:5173`。

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/register | 注册 |
| POST | /api/login | 登录 -> JWT |
| GET | /api/me | 当前用户 |
| GET | /api/mails?folder=inbox\|sent&page=1 | 邮件列表 |
| GET | /api/mails/:id | 详情（自动标已读） |
| POST | /api/mails | 发件/存草稿 `{to,subject,body,folder}` |
| PATCH | /api/mails/:id | 星标/已读/移动文件夹 |
| DELETE | /api/mails/:id | 移入 trash |
