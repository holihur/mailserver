# 贡献指南 / Contributing

感谢你愿意为 **Sweetcorn** 做出贡献！本文说明如何搭建环境、提交改动与通过 CI。

## 开发环境

- **Go** 1.22+（后端，`backend/`）
- **Node.js** 22 + **pnpm** 11（前端，`frontend/`）
- **PostgreSQL** 16（后端测试与运行）
- **Redis** 7（发信队列、限流）

### 后端

```bash
cd backend
go mod download
go build ./...
go vet ./...
go test ./...                     # 需要 TEST_DATABASE_URL 才会跑 DB 集成测试
TEST_DATABASE_URL="postgres://mailserver:mailserver@localhost:5432/mailserver_test?sslmode=disable" go test ./...
```

### 前端

```bash
cd frontend
pnpm install
pnpm run typecheck
pnpm run lint
pnpm run test
pnpm run build
```

## 提交规范

- 分支从 `main` 切出，命名如 `fix/imap-idle`、`feat/dane`。
- 提交信息简洁描述**做了什么、为什么**；一个 PR 聚焦一件事。
- PR 务必通过 CI（`go vet` / `golangci-lint` / `govulncheck` / 单测与覆盖率 / 前端 lint + test + build / `docker build` / `shellcheck`）。
- 涉及行为变更请更新 `backend/CHANGELOG.md`（`[未发布]` 段）与相关文档（如 `README.md`、`DNSSEC.md`）。
- 新增/修复逻辑请**附带测试**；核心包覆盖率要求见 CI。

## 代码风格

- Go：`gofmt` / `golangci-lint`；错误信息与注释使用中文，保持与现有代码一致。
- 前端：TypeScript + React，ESLint 需通过；UI 文案走 `frontend/src/lib/i18n.tsx`（中英双语）。
- 不要在日志、错误信息、测试样例中出现真实密码、令牌、邮箱或域名。

## 安全

请勿在公开 issue/PR 中披露漏洞细节，改用 [SECURITY.md](./SECURITY.md) 的私密渠道。

## 许可

除非另有说明，你的贡献将按本仓库的许可证发布。
