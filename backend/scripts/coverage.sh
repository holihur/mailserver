#!/usr/bin/env bash
# 核心包单元测试覆盖率门禁（>=90%）。在 backend/ 目录下运行：bash scripts/coverage.sh
set -euo pipefail
cd "$(dirname "$0")/.."
export CGO_ENABLED=0
python3 - <<'PY'
import subprocess, sys
core = {
    'mailserver/internal/auth', 'mailserver/internal/config',
    'mailserver/internal/certstore', 'mailserver/internal/dkim',
    'mailserver/internal/provider', 'mailserver/internal/runtimecfg',
    'mailserver/internal/secret',
}
r = subprocess.run(['go', 'test', '-cover', './...'], capture_output=True, text=True)
sys.stdout.write(r.stdout)
sys.stderr.write(r.stderr)
if r.returncode != 0:
    print('FAIL: go test 失败')
    sys.exit(1)
bad = []
for line in r.stdout.splitlines():
    if 'coverage:' not in line or not line.strip().startswith(('ok', 'FAIL')):
        continue
    pkg = line.split()[1]
    pct = float(line.split('coverage:')[1].split('%')[0])
    if pkg in core and pct < 90:
        bad.append(f'{pkg} {pct}%')
if bad:
    print('FAIL: 核心包覆盖率 <90%:', bad)
    sys.exit(1)
print('OK: 核心包覆盖率均 >=90%')
PY
