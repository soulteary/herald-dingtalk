# herald-dingtalk 运维指南

本文覆盖生产上线、验证、扩缩容与故障分流。钉钉应用准备和完整环境变量说明见 [DEPLOYMENT.md](DEPLOYMENT.md)。

## 生产检查清单

- 使用带版本的发布镜像 `ghcr.io/soulteary/herald-dingtalk:v1.0.0`；生产环境不要依赖可变的 `latest` 标签。
- 通过密钥管理系统注入 `DINGTALK_APP_KEY`、`DINGTALK_APP_SECRET`、`DINGTALK_AGENT_ID` 和 `API_KEY`，不要把真实值写入清单或代码库。
- 将服务放在私有网络中，只允许 Herald 或经过认证的网关访问 `/v1/send` 和 `/v1/resolve`。
- `/healthz` 用于存活探测，`/readyz` 用于就绪探测。就绪探针只校验本地配置，不会调用钉钉。
- 为编排终止过程预留至少 40 秒。进程收到 `SIGINT` 或 `SIGTERM` 后停止接收新连接，并等待在途请求，最长 35 秒，高于请求允许的 30 秒最大超时。
- 使用多副本前先确定重试去重方案；幂等缓存仅位于单个进程内。

## Kubernetes 参考清单

应用前请替换镜像标签和密钥值。运行时镜像已经使用非特权 `herald` 用户。

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
  # 幂等状态仅位于单个进程。Recreate 可避免滚动更新短暂运行两个 Pod，
  # 从而绕过单进程重复抑制。
  strategy:
    type: Recreate
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
          image: ghcr.io/soulteary/herald-dingtalk:v1.0.0
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

资源值只是起点，并非通用容量建议。调整前应在实际流量下观测延迟、内存和 `429` 响应。

Herald 侧配置：

```text
HERALD_DINGTALK_API_URL=http://herald-dingtalk:8083
HERALD_DINGTALK_API_KEY=<与 API_KEY 相同的值>
```

## 上线验证

在 shell 中设置 Base URL；启用认证时再设置 API Key。若未配置 `API_KEY`，请从示例中删除 `X-API-Key` 请求头。

```bash
BASE_URL=http://localhost:8083
API_KEY=replace-with-the-configured-api-key

curl -i "$BASE_URL/healthz"
curl -i "$BASE_URL/readyz"
```

预期结果：

- 只要进程能够提供 HTTP 服务，`/healthz` 就返回 `200`。
- 只有钉钉凭证和查询模式通过本地校验时，`/readyz` 才返回 `200`。该结果不代表钉钉网络可达，也不代表目标用户位于应用可见范围内。

向测试 userid 发送一条受控的冒烟消息。仅在重试同一条逻辑消息时复用 `SMOKE_KEY`；首次发送成功后，重试应直接返回缓存的 `message_id`，不会再次发送。

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

将返回的 `X-Request-ID` 和 `message_id` 记录到上线记录中。请求失败时按 `request_id` 检索结构化日志；服务刻意不记录负载和凭证。

## 容量与扩缩容模型

- `MAX_CONCURRENT_REQUESTS` 由每个进程独立执行。该值为 32 且运行两个副本时，不考虑代理和上游限制，理论上的服务级上限为 64 个在途 `/v1` 请求。
- 单个进程饱和时返回 `429 rate_limited` 和 `Retry-After: 1`；调用方应遵守等待时间，并使用相同幂等键重试。
- 幂等缓存和在途请求合并仅在单个进程内生效，且容量有限。当前服务没有可配置的共享缓存后端。若必须跨副本去重，请保持单副本，或在服务外提供确定性路由/去重能力。
- 钉钉 access token 由每个进程分别缓存；扩容或重启会在短时间内增加 token 请求。
- `/readyz` 仅校验配置。还应单独监控实际发送成功率和延迟，才能发现钉钉故障、权限变化或应用可见范围问题。

## 故障分流

| HTTP 状态 / 错误码 | 含义 | 首要处理 |
|---|---|---|
| `400 invalid_request` | JSON 非法或请求字段无效 | 对照 [API.md](API.md) 检查请求；不要原样重试。 |
| `400 invalid_destination` | userid/手机号无效，或手机号查询失败 | 检查标识、应用可见范围；启用手机号查询时检查 `Contact.User.mobile` 权限。 |
| `401 unauthorized` | `X-API-Key` 缺失或不匹配 | 对比适配器的 `API_KEY` 与 Herald 的 `HERALD_DINGTALK_API_KEY`。 |
| `409 idempotency_conflict` | 同一个 key 被用于不同内容 | 为新的逻辑消息生成新 key；重试时不要修改原请求。 |
| `413` | 请求超过 `MAX_REQUEST_BODY_BYTES` | 缩小负载；只有评估 1 MiB 上限和内存影响后才提高限制。 |
| `415 unsupported_media_type` | 请求不是 `application/json` | 设置 `Content-Type: application/json`。 |
| `429 rate_limited` | 当前进程达到并发上限，或钉钉对操作实施限流 | 存在 `Retry-After` 时遵守等待时间，使用相同幂等键重试，并检查本地容量与钉钉配额。 |
| `502 send_failed` / `resolve_failed` | 钉钉上游 API 调用失败 | 用请求 ID 关联日志，再检查钉钉状态、权限和配额；凭证被拒绝会单独返回 `503 provider_down`。 |
| `503 provider_down` | 本地钉钉配置无效 | 检查 `/readyz` 和启动日志，修正配置后重启。 |
| `504 timeout` | 请求或钉钉操作超过截止时间；钉钉可能已经接收发送请求 | 将交付状态视为不确定。操作已超时结束后，复用相同幂等键不能保证去重；仅在允许重复发送或具备外部去重/核对机制时重试。 |

按现象排查见 [TROUBLESHOOTING.md](TROUBLESHOOTING.md)，凭证处理和信任边界见 [SECURITY.md](SECURITY.md)。
