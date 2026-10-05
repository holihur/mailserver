# 安全政策 / Security Policy

## 支持的版本

安全修复面向**最新发布版本**与 `main` 分支。请尽量在最新版本上复现问题。

## 报告漏洞

**请勿在公开 issue / PR / 讨论中披露漏洞细节。**

推荐使用 GitHub 的私密报告渠道：

1. 打开仓库的 **Security** 标签页；
2. 点击 **Report a vulnerability**（[GitHub Security Advisories](https://docs.github.com/code-security/security-advisories/guidance-on-reporting-and-writing/privately-reporting-a-security-vulnerability)）；
3. 填写影响版本、复现步骤、影响面与（可选）修复建议。

如无法使用上述渠道，请在公开仓库中开一个**不含任何细节**的 issue，仅说明「需要通过私密渠道联系」，维护者会单独与你联系。

## 我们会做什么

- **确认**：收到报告后 3 个工作日内确认并给出初步评估。
- **修复**：确认的漏洞会尽快修复；严重问题优先发布补丁版本。
- **披露**：修复发布后与你协商公开时间，并在你同意的前提下于 Release 说明中致谢。

## 请一并提供

- 受影响的版本 / commit；
- 复现步骤或 PoC（避免使用真实数据）；
- 影响评估（如：远程未授权、需要登录、仅信息泄露）；
- 相关日志（请脱敏：去除密码、令牌、真实邮箱、IP 等）。

## 范围

本项目关注但不限于：认证/授权绕过、SMTP/IMAP/POP3/JMAP 协议层注入与越权、SSRF、存储型/反射型 XSS、路径穿越、凭证或密钥泄露、DNS/DNSSEC 相关实现缺陷、依赖供应链风险。

以下通常**不属于**安全漏洞：需要已获得管理员权限的操作、仅影响本机且无越权的配置问题、第三方依赖中尚未被评估为可利用的告警。

感谢你帮助保护 Sweetcorn 与它的用户。
