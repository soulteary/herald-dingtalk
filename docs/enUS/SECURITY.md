# herald-dingtalk Security Practices

This document describes security considerations and recommendations for herald-dingtalk.

## API Key

- When `API_KEY` is set, herald-dingtalk requires the `X-API-Key` header to match. Use a strong, unique value and keep it secret.
- Herald must be configured with the same value as `HERALD_DINGTALK_API_KEY` so that it sends the key on every request to herald-dingtalk.
- Do not log or expose the API key. Prefer environment variables or a secret manager over config files committed to source control.

## DingTalk Credentials

- **AppKey**, **AppSecret**, and **AgentID** must never be hardcoded or committed to the repository.
- Store them in environment variables or a secret manager (e.g. Kubernetes Secrets, HashiCorp Vault). Use `.env` only for local development and ensure `.env` is in `.gitignore`.
- Rotate AppSecret periodically in the DingTalk open platform and update herald-dingtalk configuration accordingly.

## Production Recommendations

- **Network**: Run herald-dingtalk in a private network. Only Herald (or your gateway) should call it; do not expose herald-dingtalk directly to the public internet unless behind HTTPS and strict access control.
- **HTTPS**: If herald-dingtalk is reachable over the internet or across untrusted networks, put it behind a reverse proxy (e.g. Traefik, nginx) with TLS. Herald should use `https://` for `HERALD_DINGTALK_API_URL` in that case.
- **Least privilege**: Run the process with a non-root user; in Docker, use a non-root user in the image if possible.
- **Logging**: Avoid logging request bodies or headers that may contain secrets. Structured logs (e.g. `to`, `message_id`, error codes) are sufficient for operations and troubleshooting.

## Summary

- Use `API_KEY` in production and keep it secret; configure Herald with `HERALD_DINGTALK_API_KEY` to match.
- Store DingTalk credentials in env or a secret manager; never in code or committed config.
- Prefer private network and HTTPS in front of herald-dingtalk; do not expose it publicly without protection.
