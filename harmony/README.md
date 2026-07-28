# AIOps — 鸿蒙原生客户端（HarmonyOS NEXT）

与 [aiops-android](https://github.com/sreyun/aiops-android)（Android Compose）**功能 / 信息架构 / 交互闭环**对齐的 ArkTS 工程（`com.aiops.monitor`）。

## 构建与模拟器

路径勿含中文；用 `D:\build\aiops-harmony`：

```bat
set DEVECO=D:\Program Files\deveco\DevEco Studio
set DEVECO_SDK_HOME=%DEVECO%\sdk
set PATH=%DEVECO%\tools\node;%DEVECO%\tools\ohpm\bin;%DEVECO%\tools\hvigor\bin;%PATH%
robocopy "%CD%\harmony" D:\build\aiops-harmony /E /XD build oh_modules node_modules .hvigor dist
cd /d D:\build\aiops-harmony
ohpm install
hvigorw --mode module -p product=default -p buildMode=debug assembleHap --no-daemon
hdc -t 127.0.0.1:5555 install entry\build\default\outputs\default\entry-default-unsigned.hap
hdc -t 127.0.0.1:5555 shell aa start -a EntryAbility -b com.aiops.monitor
```

## 对照 Android 交互验收

模拟器 `127.0.0.1:5555` 已装入最新 debug HAP；登录首屏截图见 [`docs/parity-shots/01-login.jpeg`](docs/parity-shots/01-login.jpeg)、视觉对齐后 [`docs/parity-shots/02-visual-login.jpeg`](docs/parity-shots/02-visual-login.jpeg)。

主题色对齐 Android `Theme.kt`（primary `#4F7FFF`、bg `#080C14`、surface `#111722`）；底栏带图标 + selected pill；登录锁图标卡片；「切换深浅色」会真正切换 Light/Dark tokens。

### 登录
- [x] 首屏仅用户名/密码（无 URL）；右下角连点三下环境切换
- [x] Env：当前标记、切换、删除确认、添加
- [x] MFA 阻塞对话框

### 总览
- [x] WeatherHero（问候+天气+可点在线/严重/警告）
- [x] 顶栏刷新；账号菜单含 MFA 行；切换账号 / 退出分确认
- [x] 快捷入口 8 宫格可编辑；容量热点紧凑行

### 告警
- [x] 副标题 + 刷新；level/status chips 带数量
- [x] AlertCard：严重度条、pills、确认/静默/AI；行 busy + Toast
- [x] AI 带完整告警上下文

### 运维
- [x] OpsOverview KPI 四格可跳转（事件 / 审批 / 工单 / SLO）
- [x] 治理子 Tab：审批 / SLO / 工单；工单指派 + 评论列表
- [x] 事件筛选 + FAB 新建；详情 Sheet 含时间线；诊断气泡 + 终端上下文开关
- [x] 日志：Agent 检索 / 终端审计 双 Tab

### AI / 监控
- [x] 清空、能力 chips（知识库/MCP/Loki/Prometheus）、建议点即发送
- [x] 左右气泡、附件相册选择、麦克风占位、停止生成
- [x] 网络子 Tab 设备/流量/Trap/审计；硬件详情 + AI
- [x] 主机 GPU MetricBar；VM 详情 Sheet；拨测类型 FilterChips + 表单 chips
- [x] 主机详情页 per-GPU 利用率/显存条

保留深色 `AppTheme`；用户文案不出现 hermes。TTS/STT：`VoiceHelper` 已接 CoreSpeechKit（真机效果最佳）。
