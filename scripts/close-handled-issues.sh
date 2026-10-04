#!/usr/bin/env bash
# 关闭已处理的安全 / bug / enhancement issue（#1–#20）。
# 需要先鉴权：`gh auth login`，或设置环境变量 GH_TOKEN / GITHUB_TOKEN。
#
# 用法：scripts/close-handled-issues.sh
set -euo pipefail

REPO="${REPO:-holihur/mailserver}"
ISSUES=(1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20)
MSG="已在 main 分支处理并发布（详见 CHANGELOG「未发布」）。相关提交：5da2c38、3fac975、ef0eb90、0ee04e5、5ef7127、30b4117。"

command -v gh >/dev/null 2>&1 || { echo "未安装 gh CLI：https://cli.github.com/"; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "未登录 gh：请先 gh auth login 或设置 GH_TOKEN"; exit 1; }

for n in "${ISSUES[@]}"; do
  if gh issue close "$n" --repo "$REPO" --comment "$MSG"; then
    echo "closed #$n"
  else
    echo "skip #$n（可能已关闭或不存在）"
  fi
done
