# 文档索引

欢迎查阅 herald-dingtalk 的文档。herald-dingtalk 是 [Herald](https://github.com/soulteary/herald) 的钉钉通知适配器。

## 多语言文档

- [English](../enUS/README.md) | [中文](README.md)

## 文档列表

### 核心文档

- **[README.zhCN.md](../../README.zhCN.md)** - 项目概述与快速开始

### 详细文档

- **[API.md](API.md)** - 完整 API 说明
  - Base URL 与认证
  - POST /v1/resolve（OAuth2 auth_code → userid，可选）
  - POST /v1/send 请求/响应（to 支持 userid 或手机号，见 DINGTALK_LOOKUP_MODE）
  - GET /healthz
  - 错误码与 HTTP 状态码
  - 幂等

- **[DEPLOYMENT.md](DEPLOYMENT.md)** - 部署指南
  - 二进制与 Docker 运行
  - 配置项说明（含 DINGTALK_LOOKUP_MODE、Contact.User.mobile）
  - 与 Herald 集成
  - 钉钉应用准备
  - 模板消息不适用于企业内部应用说明

- **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** - 故障排查
  - 收不到消息
  - 503 provider_down
  - 401 unauthorized
  - 目标无效与幂等

- **[SECURITY.md](SECURITY.md)** - 安全实践
  - API Key 使用
  - 凭证管理
  - 生产环境建议

## 快速导航

### 新手入门

1. 阅读 [README.zhCN.md](../../README.zhCN.md) 了解项目
2. 查看 [快速开始](../../README.zhCN.md#快速开始)
3. 参考 [DEPLOYMENT.md](DEPLOYMENT.md) 进行配置与 Herald 集成

### 开发人员

1. 查看 [API.md](API.md) 了解发送协议与错误码
2. 参考 [DEPLOYMENT.md](DEPLOYMENT.md) 了解部署方式

### 运维人员

1. 阅读 [DEPLOYMENT.md](DEPLOYMENT.md) 了解部署与 Herald 侧配置
2. 参考 [SECURITY.md](SECURITY.md) 了解生产实践
3. 排查问题：[TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## 文档结构

```
herald-dingtalk/
├── README.md              # 项目主文档（英文）
├── README.zhCN.md         # 项目主文档（中文）
├── docs/
│   ├── enUS/
│   │   ├── README.md       # 文档索引（英文）
│   │   ├── API.md          # API 文档（英文）
│   │   ├── DEPLOYMENT.md   # 部署指南（英文）
│   │   ├── TROUBLESHOOTING.md # 故障排查（英文）
│   │   └── SECURITY.md     # 安全（英文）
│   └── zhCN/
│       ├── README.md       # 文档索引（中文，本文件）
│       ├── API.md          # API 文档（中文）
│       ├── DEPLOYMENT.md   # 部署指南（中文）
│       ├── TROUBLESHOOTING.md # 故障排查（中文）
│       └── SECURITY.md     # 安全（中文）
└── ...
```

## 按主题查找

- API 端点与认证：[API.md](API.md)
- 配置与 Herald 集成：[DEPLOYMENT.md](DEPLOYMENT.md)
- 常见问题：[TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- 安全：[SECURITY.md](SECURITY.md)
