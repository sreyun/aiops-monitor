---
kind: external_dependency
name: 华为云短信与语音通知 - X-WSSE 认证
slug: huawei-sms-voice
category: external_dependency
category_hints:
    - auth_protocol
scope:
    - '**'
---

### 华为云短信/语音服务
- **角色**：告警通知渠道之一，提供短信和 TTS 语音电话功能
- **鉴权协议**：X-WSSE（UsernameToken）认证，需在请求头中携带 WSSE 安全令牌
- **API端点**：短信 `smsapi.cn-north-4.myhuaweicloud.com`，语音 `rtc-api.myhuaweicloud.com`
- **配置要求**：需要填写 `app_id` = project_id（华为云控制台「我的凭证」获取）
- **版本**：使用 API v2（最新版本）