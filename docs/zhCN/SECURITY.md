# herald-dingtalk 安全实践

本文说明 herald-dingtalk 的安全注意事项与推荐做法。

## API Key

- 配置 `API_KEY` 后，herald-dingtalk 会要求请求头 `X-API-Key` 与之一致。请使用足够强且唯一的密钥并妥善保管。
- Herald 侧需配置相同的 `HERALD_DINGTALK_API_KEY`，以便在请求 herald-dingtalk 时携带该密钥。
- 不要将 API Key 写入日志或对外暴露。优先使用环境变量或密钥管理服务，避免将密钥写入并提交到仓库的配置文件中。

## 钉钉凭证

- **AppKey**、**AppSecret**、**AgentID** 不得硬编码或提交到代码库。
- 应通过环境变量或密钥管理服务（如 Kubernetes Secrets、HashiCorp Vault）注入。本地开发可使用 `.env`，并确保 `.env` 已加入 `.gitignore`。
- 建议在钉钉开放平台定期轮换 AppSecret，并同步更新 herald-dingtalk 的配置。

## 生产环境建议

- **网络**：将 herald-dingtalk 部署在内网或私有网络中，仅允许 Herald（或统一网关）访问；不要将 herald-dingtalk 直接暴露到公网，除非在 HTTPS 与严格访问控制之后。
- **HTTPS**：若 herald-dingtalk 会经过公网或不可信网络被访问，应在其前增加带 TLS 的反向代理（如 Traefik、nginx）。此时 Herald 的 `HERALD_DINGTALK_API_URL` 应使用 `https://`。
- **最小权限**：使用非 root 用户运行进程；在 Docker 中尽量使用非 root 用户镜像。
- **日志**：避免记录可能包含敏感信息的请求体或请求头；仅记录运维与排查所需字段（如 `to`、`message_id`、错误码）即可。

## 小结

- 生产环境建议配置 `API_KEY` 并严格保密；Herald 侧配置 `HERALD_DINGTALK_API_KEY` 与之一致。
- 钉钉凭证仅通过环境变量或密钥管理服务注入，不写入代码或提交的配置。
- 尽量在内网部署 herald-dingtalk，对外暴露时使用 HTTPS 与访问控制。
