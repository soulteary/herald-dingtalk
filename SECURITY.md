# Security

Security practices for herald-dingtalk are documented in the docs:

- **[English](docs/enUS/SECURITY.md)** – API Key usage, DingTalk credential management, production recommendations
- **[中文](docs/zhCN/SECURITY.md)** – API Key 使用、钉钉凭证管理、生产环境建议

**Summary**: Use `API_KEY` in production and keep it secret; store DingTalk credentials in environment variables or a secret manager (never in code or committed config); prefer private network and HTTPS in front of herald-dingtalk.

## Supported versions

Beginning with the v1.0.0 release, security fixes are provided for the current v1.x line. Pre-1.0 releases are no longer supported after v1.0.0 is published.

| Version | Supported |
|---|---|
| 1.x | Yes, beginning with v1.0.0 |
| 0.x | No, after v1.0.0 is published |

To report a security vulnerability, please open a private security advisory or contact the maintainers directly.
