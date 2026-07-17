---
kind: external_dependency
name: 腾讯云短信与语音通知 - TC3-HMAC-SHA256签名
slug: tencent-sms-voice
category: external_dependency
category_hints:
    - auth_protocol
scope:
    - '**'
---

### 腾讯云短信/语音服务
- **角色**：告警通知渠道之一，提供短信和 TTS 语音电话功能
- **鉴权协议**：TC3-HMAC-SHA256 签名算法
- **API端点**：短信 `sms.tencentcloudapi.com`（2021-01-11版本），语音 `vms.tencentcloudapi.com`（2020-02-24版本）
- **配置要求**：短信需要 `SmsSdkAppId`，语音需要 `VoiceSdkAppId`，分别对应应用管理中的 SDK AppID