---
kind: external_dependency
name: 阿里云短信与语音通知 - ACS3-HMAC-SHA256 V3签名
slug: aliyun-sms-voice
category: external_dependency
category_hints:
    - auth_protocol
    - sdk_real_api
scope:
    - '**'
---

### 阿里云短信/语音服务
- **角色**：告警通知渠道之一，支持短信发送和 TTS 语音电话通知
- **鉴权协议**：ACS3-HMAC-SHA256 签名 V3（官方推荐），比旧版 HMAC-SHA1 更安全
- **API端点**：短信 `dysmsapi.aliyuncs.com`，语音 `dyvmsapi.aliyuncs.com`，均为 POST 请求
- **签名特征**：Authorization 头包含 `Credential=<AccessKeyId>,SignedHeaders=...`，时间戳通过 `x-acs-date` 传递
- **模板参数**：支持自定义 JSON 模板参数（`template_param` 字段），留空时回退到默认的 `{"message":"告警文本"}`