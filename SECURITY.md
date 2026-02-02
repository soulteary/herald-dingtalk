# Security

Security practices for herald-dingtalk are documented in the docs:

- **[English](docs/enUS/SECURITY.md)** – API Key usage, DingTalk credential management, production recommendations
- **[中文](docs/zhCN/SECURITY.md)** – API Key 使用、钉钉凭证管理、生产环境建议

**Summary**: Use `API_KEY` in production and keep it secret; store DingTalk credentials in environment variables or a secret manager (never in code or committed config); prefer private network and HTTPS in front of herald-dingtalk.

To report a security vulnerability, please open a private security advisory or contact the maintainers directly.
