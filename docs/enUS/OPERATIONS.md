# herald-dingtalk Operations Guide

This guide covers production rollout, verification, scaling, and incident triage. See [DEPLOYMENT.md](DEPLOYMENT.md) for DingTalk application setup and the full environment-variable reference.

## Production checklist

- Use a versioned release image such as `ghcr.io/soulteary/herald-dingtalk:v1.2.3`; avoid a mutable `latest` tag in production.
- Inject `DINGTALK_APP_KEY`, `DINGTALK_APP_SECRET`, `DINGTALK_AGENT_ID`, and `API_KEY` from a secret store. Do not put live values in a manifest or repository.
- Keep the service private. Allow only Herald or an authenticated gateway to reach `/v1/send` and `/v1/resolve`.
- Use `/healthz` for liveness and `/readyz` for readiness. Readiness validates local configuration; it does not call DingTalk.
- Allow at least 35 seconds for shutdown. The server stops accepting new connections on `SIGINT` or `SIGTERM` and waits up to 35 seconds for in-flight requests, which is longer than the maximum accepted 30-second request timeout.
- Decide how retries will be deduplicated before using multiple replicas. The idempotency cache is process-local.

## Kubernetes reference manifest

Replace the image tag and secret values before applying this example. The runtime image already uses the unprivileged `herald` user.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: herald-dingtalk
type: Opaque
stringData:
  API_KEY: replace-with-a-random-shared-secret
  DINGTALK_APP_KEY: replace-me
  DINGTALK_APP_SECRET: replace-me
  DINGTALK_AGENT_ID: "123456789"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: herald-dingtalk
spec:
  replicas: 1
  selector:
    matchLabels:
      app: herald-dingtalk
  template:
    metadata:
      labels:
        app: herald-dingtalk
    spec:
      terminationGracePeriodSeconds: 40
      containers:
        - name: herald-dingtalk
          image: ghcr.io/soulteary/herald-dingtalk:v1.2.3
          imagePullPolicy: IfNotPresent
          envFrom:
            - secretRef:
                name: herald-dingtalk
          env:
            - name: PORT
              value: ":8083"
            - name: DINGTALK_LOOKUP_MODE
              value: "none"
            - name: MAX_CONCURRENT_REQUESTS
              value: "32"
            - name: MAX_REQUEST_BODY_BYTES
              value: "65536"
          ports:
            - name: http
              containerPort: 8083
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
            periodSeconds: 10
            timeoutSeconds: 2
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /readyz
              port: http
            periodSeconds: 5
            timeoutSeconds: 2
            failureThreshold: 3
          resources:
            requests:
              cpu: 25m
              memory: 32Mi
            limits:
              cpu: 500m
              memory: 128Mi
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop: ["ALL"]
---
apiVersion: v1
kind: Service
metadata:
  name: herald-dingtalk
spec:
  selector:
    app: herald-dingtalk
  ports:
    - name: http
      port: 8083
      targetPort: http
```

The resource values are starting points, not universal sizing recommendations. Measure latency, memory, and `429` responses under your own traffic before changing them.

Configure Herald with:

```text
HERALD_DINGTALK_API_URL=http://herald-dingtalk:8083
HERALD_DINGTALK_API_KEY=<same value as API_KEY>
```

## Rollout verification

Set a base URL and, when authentication is enabled, an API key in your shell. Omit the `X-API-Key` header in the examples if `API_KEY` is disabled.

```bash
BASE_URL=http://localhost:8083
API_KEY=replace-with-the-configured-api-key

curl -i "$BASE_URL/healthz"
curl -i "$BASE_URL/readyz"
```

Expected results:

- `/healthz` returns `200` whenever the process can serve HTTP.
- `/readyz` returns `200` only when the DingTalk credentials and lookup mode pass local validation. A `200` does not prove DingTalk is reachable or that a recipient is visible to the application.

Send a controlled smoke-test message to a test userid. Reuse the same `SMOKE_KEY` only when repeating this exact logical message; a successful retry should return the cached `message_id` without sending again.

```bash
SMOKE_KEY="rollout-2026-08-26-001"

curl -i "$BASE_URL/v1/send" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -H "X-Request-ID: rollout-check-001" \
  -H "Idempotency-Key: $SMOKE_KEY" \
  --data '{
    "channel": "dingtalk",
    "to": "test-userid",
    "body": "herald-dingtalk rollout check",
    "timeout_seconds": 10
  }'
```

Keep the returned `X-Request-ID` and `message_id` with the rollout record. Search structured logs by `request_id` when a request fails; payloads and credentials are deliberately not logged.

## Capacity and scaling model

- `MAX_CONCURRENT_REQUESTS` is enforced independently by each process. With a value of 32 and two replicas, the theoretical service-wide ceiling is 64 in-flight `/v1` requests before proxy and upstream limits.
- A saturated process returns `429 rate_limited` with `Retry-After: 1`; callers should honor that delay and retry with the same idempotency key.
- The idempotency cache and in-flight coalescing are process-local and bounded. The current service has no configurable shared cache backend. If cross-replica duplicate suppression is required, keep one replica or provide deterministic routing/deduplication outside this service.
- DingTalk access tokens are cached per process. A scale-out or restart can temporarily increase token requests.
- `/readyz` checks configuration only. Monitor real send success rate and latency separately to detect DingTalk outages, permission changes, or recipient visibility problems.

## Incident triage

| HTTP status / code | Meaning | First action |
|---|---|---|
| `400 invalid_request` | Malformed JSON or an invalid request field | Compare the request with [API.md](API.md); do not retry unchanged input. |
| `400 invalid_destination` | Invalid userid/mobile or failed mobile lookup | Verify the identifier, app visibility, and `Contact.User.mobile` permission when enabled. |
| `401 unauthorized` | Missing or mismatched `X-API-Key` | Compare the adapter `API_KEY` with Herald's `HERALD_DINGTALK_API_KEY`. |
| `409 idempotency_conflict` | One key was used for different content | Generate a new key for the new logical message; do not mutate a retried request. |
| `413` | Request exceeds `MAX_REQUEST_BODY_BYTES` | Reduce the payload; raise the limit only after reviewing the 1 MiB maximum and memory impact. |
| `415 unsupported_media_type` | Request is not `application/json` | Set `Content-Type: application/json`. |
| `429 rate_limited` | This process reached its concurrency limit | Honor `Retry-After`, retry with the same idempotency key, and inspect latency/capacity. |
| `502 send_failed` | DingTalk token or send API failed | Correlate by request ID, then check DingTalk status, credentials, permission, and quota. |
| `503 provider_down` | Local DingTalk configuration is invalid | Check `/readyz` and startup logs, then fix configuration and restart. |
| `504 timeout` | Request or DingTalk operation exceeded its deadline | Retry with the same idempotency key; inspect upstream latency before increasing the timeout. |

For symptom-based diagnosis, see [TROUBLESHOOTING.md](TROUBLESHOOTING.md). For credential handling and trust boundaries, see [SECURITY.md](SECURITY.md).
