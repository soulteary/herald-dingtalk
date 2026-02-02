# Documentation Index

Welcome to the herald-dingtalk documentation. herald-dingtalk is the DingTalk notification adapter for [Herald](https://github.com/soulteary/herald).

## Multi-language Documentation

- [English](README.md) | [中文](../zhCN/README.md)

## Document List

### Core Documents

- **[README.md](../../README.md)** - Project overview and quick start guide

### Detailed Documents

- **[API.md](API.md)** - Complete API reference
  - Base URL and authentication
  - POST /v1/resolve (OAuth2 auth_code → userid, optional)
  - POST /v1/send request/response (to supports userid or mobile per DINGTALK_LOOKUP_MODE)
  - GET /healthz
  - Error codes and HTTP status codes
  - Idempotency

- **[DEPLOYMENT.md](DEPLOYMENT.md)** - Deployment guide
  - Binary and Docker run
  - Configuration options (including DINGTALK_LOOKUP_MODE, Contact.User.mobile)
  - Integration with Herald
  - DingTalk app setup
  - Template messages not for enterprise internal apps

- **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** - Troubleshooting guide
  - Messages not received
  - 503 provider_down
  - 401 unauthorized
  - Invalid destination and idempotency

- **[SECURITY.md](SECURITY.md)** - Security practices
  - API Key usage
  - Credential management
  - Production recommendations

## Quick Navigation

### Getting Started

1. Read [README.md](../../README.md) to understand the project
2. Check the [Quick Start](../../README.md#quick-start) section
3. Refer to [DEPLOYMENT.md](DEPLOYMENT.md) for configuration and Herald integration

### Developers

1. Check [API.md](API.md) for the send contract and error codes
2. Review [DEPLOYMENT.md](DEPLOYMENT.md) for deployment options

### Operations

1. Read [DEPLOYMENT.md](DEPLOYMENT.md) for deployment and Herald side config
2. Refer to [SECURITY.md](SECURITY.md) for production practices
3. Troubleshoot issues: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## Document Structure

```
herald-dingtalk/
├── README.md              # Main project document (English)
├── README.zhCN.md         # Main project document (Chinese)
├── docs/
│   ├── enUS/
│   │   ├── README.md       # Documentation index (this file)
│   │   ├── API.md          # API reference
│   │   ├── DEPLOYMENT.md   # Deployment guide
│   │   ├── TROUBLESHOOTING.md # Troubleshooting guide
│   │   └── SECURITY.md     # Security practices
│   └── zhCN/
│       ├── README.md       # Documentation index (Chinese)
│       ├── API.md          # API reference (Chinese)
│       ├── DEPLOYMENT.md   # Deployment guide (Chinese)
│       ├── TROUBLESHOOTING.md # Troubleshooting guide (Chinese)
│       └── SECURITY.md     # Security practices (Chinese)
└── ...
```

## Find by Topic

- API endpoints and auth: [API.md](API.md)
- Configuration and Herald integration: [DEPLOYMENT.md](DEPLOYMENT.md)
- Common issues: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- Security: [SECURITY.md](SECURITY.md)
