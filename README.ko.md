# AIOps

> **엔터프라이즈급 호스트 모니터링 및 SRE 운영 플랫폼 · 100% 오픈소스 · 프라이빗 자체 호스팅 · 데이터 영구 자체 보유**
>
> 단일 Go 바이너리 + 의존성 없는 Agent가 메트릭 수집, 지능형 알림, 원격 터미널, 자동 자가 치유부터 SRE 클로즈드루프, AI 점검 진단, Android / HarmonyOS 모바일 콘솔까지의 운영 전체 라이프사이클을 커버합니다. PostgreSQL + VictoriaMetrics 이중 스토리지로, 단 한 줄의 명령으로 배포하고 3분 내에 가동됩니다.
>
> **English:** AIOps is an open-source, self-hosted enterprise host-monitoring & SRE platform. One Go binary plus a zero-dependency agent covers the full ops loop — metrics, alerting, remote terminal, auto-remediation, SRE closure, AI diagnosis, and a native mobile console — on unified PostgreSQL + VictoriaMetrics storage. MIT licensed.

**언어 / Languages：** [简体中文](README.md) · [English](README_EN.md) · [日本語](README.ja.md) · [한국어](README.ko.md)

---

## 목차

- [프로젝트 개요](#프로젝트-개요)
- [기능 특성](#기능-특성)
- [설치 단계](#설치-단계)
- [사용 방법](#사용-방법)
- [설정 파라미터 설명](#설정-파라미터-설명)
- [기술 아키텍처 개요](#기술-아키텍처-개요)
- [기여 가이드](#기여-가이드)
- [라이선스](#라이선스)

---

## 프로젝트 개요

AIOps는 **엔터프라이즈급 호스트 모니터링 및 SRE 운영 플랫폼**으로, "Go 네이티브 수집 + Python 플러그인 레이어 + 실시간 패널"의 하이브리드 아키텍처를 채택하여 크로스 플랫폼(Linux / Windows / macOS / Kylin 등 국산 OS) 메트릭 수집, GPU 모니터링, 커스텀 헬스 체크, 원격 터미널, 자동화 플레이북, SRE 허브(인시던트 / 자동 복구 / SLO / 티켓), 로그 수집·검색, AI 점검 진단 등의 기능을 제공합니다.

통합 스토리지는 **PostgreSQL(모든 관계형 데이터) + VictoriaMetrics(모든 시계열 데이터)** 로 통일되었으며, 내장된 `aiops.db`는 폐기되었습니다. 새로 구성 키의 AES-256-GCM 저장 암호화, 선택적 TLS 전송 암호화, 최초 로그인 시 강제 보안 초기화, 크로스 플랫폼 부팅 자동 시작 및 프로세스 유지(keepalive)가 추가되었습니다.

**대상 사용자**

- **운영 엔지니어 및 SRE**: 호스트 및 비즈니스 가용성 모니터링, 알림 거버넌스, SLO와 인시던트 클로즈드루프, 자동 복구 및 티켓 흐름.
- **플랫폼 및 개발**: API 모니터링, 포트 포워딩 및 HTTP 프록시, 원격 터미널, 로그 검색, AI 보조 진단.
- **보안 및 컴플라이언스**: RBAC, MFA, 감사, SSRF 방어, TLS 및 저장 암호화.

**기존 모니터링 방식 대비 차별화 우위**

- 단일 바이너리 서버 + 의존성 없는 Agent, 3개 플랫폼 네이티브 수집 및 GPU 지원.
- Server-Agent 분리 배포, Agent가 **역방향 연결**되어 인바운드 포트를 열 필요 없음.
- PG + VM 통합 스토리지로 관계형과 시계열 데이터가 각자 역할을 수행.
- 내장 AI 점검 및 RAG 메모리 라이브러리, pgvector 기반 유사 사례 검색.
- 알림 거버넌스(사일런스 / 억제 / 라우팅)와 통합 메시지 센터로 노이즈를 줄이고 실행 가능성 향상.

> 위치 선정 측면에서 AIOps는 완전히 통제 가능한 하나의 플랫폼으로 관측성, 알림, 자동화, AI 진단, SRE 클로즈드루프, 모바일을 수렴하여 여러 분산 도구를 유지보수하는 부담을 줄이는 것을 지향합니다.

---

## 기능 특성

### 호스트 및 리소스 모니터링

- **크로스 플랫폼 네이티브 수집**: Linux / Windows / macOS / Kylin 등, 제3자 의존성 없음.
- **메트릭 차원**: CPU, 메모리, 스왑, 디스크, 프로세스, 포트, 네트워크, DiskIO, IOPS, GPU, 로드, 업타임.
- **GPU 모니터링**: best-effort로 VRAM, 사용률, 온도 등 수집.
- **외부 수집기(대상 장치에 Agent 설치 불필요)**:
  - **Redfish**: BMC / iDRAC / iLO 폴링으로 CPU / 메모리 / 스토리지 / 온도 / 팬 / 전원 / 펌웨어 수집.
  - **NetFlow**: UDP로 스위치 / 방화벽 Flow 수신(v5 / v9 템플릿).
  - **Huawei OceanStor**: DeviceManager REST로 스토리지 / 디스크 인클로저 상태 수집.
  - **SNMP**: 자체 구현 프로토콜 스택(v2c / v3 USM + Trap)으로 스위치 / 라우터 관리.
  - **5-tuple 패킷 / 페이로드**, **SNI / DNS 및 평문 HTTP 콘텐츠 감사**(컴플라이언스 제어 가능, 기본 비활성).
  - **컨테이너 인벤토리**(Docker / Podman 자동 탐지), **Hyper-V** 가상 머신 수집.

### 로그 및 관측성

- **로그 수집**: 증분 tail, 선택적 gzip + AES-256-GCM 암호화 전송.
- **전문 검색**: 로그 중앙 집중 저장 및 조회.
- **메트릭 추세**: VictoriaMetrics가 담당, 원시 정밀도를 장기 보관, 필요에 따라 집계 조회, 장기 추적에 무리 없음.

### 헬스 체크 및 API 모니터링

- **커스텀 헬스 체크**: Ping / TCP / HTTP / 프로세스 생존.
- **API 비즈니스 모니터링**: 가용률 / P95 / 처리량, 분산 다지점 탐지(WebSocket 프로브).
- 결과는 통합 시계열 DB에 기록되어 알림 및 SLO 계산을 트리거.

### 알림 및 거버넌스

- **임계값 엔진**: CPU / 메모리 / 디스크 / IO / IOPS / GPU / 로드 / 프로세스 / 연결 수 / 오프라인 판정.
- **알림 거버넌스**: 중복 제거, 사일런스(시간대 / 요일), 억제(근인이 파생 억제), 라우팅(채널 분류).
- **다채널 알림**: Feishu, DingTalk, 이메일, SMS, 음성 전화.
- **복구 알림** 및 알림 라이프사이클 관리.

### 원격 터미널 및 포트 포워딩

- **웹 터미널**: 브라우저 ↔ 서버 ↔ Agent 역방향 채널(wait / rx / tx), 멀티 탭, 세션 녹화·재생, 읽기 전용 관찰, 명령 감사, 2차 비밀번호 인증.
- **포트 포워딩**: TCP / UDP 매핑, HTTP 리버스 프록시(`/proxy/{hostID}/{port}/{path}`, WebSocket 지원), SSRF 아웃바운드 방어, 전역 비활성화 가능.
- **머신 핑거프린트 인증**: Token 교체가 이미 설치된 Agent에 영향 없음.

### 자동화 및 SRE 허브

- **자동화 플레이북(Playbook)**: 다단계 명령, 대상 선택(전체 / 분류 / 시스템 / 호스트), 병렬 실행, 이력 리포트.
- **자동 복구**: 가드레일 + 승인 흐름으로 위험 작업 방지.
- **인시던트 관리**: 집계, 클로즈드루프, 사례 내보내기.
- **SLO**: 에러 예산 관리.
- **티켓 흐름**: 인시던트 / 자동 복구와 연동.
- **통합 메시지 센터**: 인시던트 / 알림 / SLO / 자동 복구 / AI / 티켓 통합 받은편지함.

### AI 운영 역량

- **AI 점검**: 정기 자동 점검 및 리포트 산출.
- **근본 원인 분석**: 컨텍스트를 결합하여 진단 제안.
- **RAG 메모리 라이브러리**: pgvector 기반 유사 사례 검색(diagnosis_embeddings / ai_memory_embeddings).
- **자율 Agent**: Function Calling으로 운영 동작 실행.
- **AI Copilot / 음성**: 대화형 어시스턴트 및 음성 상호작용(TTS / STT).

### 보안 및 컴플라이언스

- **인증**: 세션 Cookie + RBAC(admin / operator / viewer).
- **MFA**: TOTP 동적 OTP(1회용).
- **터미널 2차 비밀번호**: 속도 제한 + 잠금 기간.
- **Agent 머신 핑거프린트**: `X-Agent-Fingerprint`(machine-id + MAC).
- **설치 토큰**: 교체 + 7일 유예 기간.
- **중계 키**: `AIOPS_RELAY_SECRET` 출처 검증.
- **저장 암호화**: 구성 키 AES-256-GCM.
- **전송 암호화**: 선택적 TLS; 최초 로그인 시 강제 보안 초기화.

### 모바일

- **네이티브 Android App**(Kotlin + Jetpack Compose): 29개 네비게이션 라우트 / 20+ 화면. SRE 운전석, 호스트 상세(네이티브 Canvas 차트), 알림(레벨 / 상태 2차원 필터 + AI 진단), 엔터프라이즈급 VT 터미널, 운영 센터(인시던트 / 승인 / SLO / 티켓), 모니터링 헬스 체크, AI 어시스턴트(SSE 스트리밍), 하드웨어 / NetFlow / Hyper-V, 터미널 재생, 메시지 센터 등. DataStore 영속 세션, MFA 팝업, 자체 구축 `/ws/push` 장기 연결 푸시.
- **HarmonyOS NEXT App**(ArkTS): Android와 "기능 / 정보 아키텍처 / 상호작용 클로즈드루프"가 정렬된 네이티브 HarmonyOS 콘솔.

### 배포 및 경험

- **웹 패널**: 서버 내장 Dashboard, 듀얼 테마(다크 / 라이트), 3개 언어 전환(简中 / 繁中 / English). (마케팅 사이트 `website/`는 简中 / 繁中 / English / 日本語 / 한국어 5개 언어 지원)
- **다중 서버 브로드캐스트**: 한 번 수집하여 여러 서버에 동시 보고(크로스 IDC 재해 복구).
- **게이트웨이 중계**: 내부망에서 인터넷에 연결된 한 대의 머신만이 모든 보고를 클라우드로 프록시.

---

## 설치 단계

> 서버는 PostgreSQL과 VictoriaMetrics를 **강하게 의존**하며, 둘 중 하나라도 누락되면 시작을 거부합니다.

### 방식 1: Docker Compose(권장)

```bash
# 1. 구성 준비(필요에 따라 compose의 환경 변수 / 비밀번호 수정)
cp docker-compose.yml docker-compose.override.yml   # 선택

# 2. server + victoriametrics + postgres(pgvector) 한 번에 기동
docker compose up -d

# 3. http://localhost:8529 접속, 안내에 따라 최초 보안 초기화 완료
```

이미지는 기본적으로 Huawei Cloud SWR(`swr.cn-east-3.myhuaweicloud.com/sreyun/...`)을 사용하며, 자체 빌드 이미지로 교체 가능.

### 방식 2: 바이너리 배포

```bash
# 서버: 필수 환경 변수 설정 후 직접 실행
export AIOPS_POSTGRES_DSN="postgres://aiops:비밀번호@localhost:5432/aiops?sslmode=disable"
export AIOPS_VM_URL="http://localhost:8428"
export AIOPS_LISTEN=":8529"            # 선택, 기본 :8529
./aiops-server

# Agent: 구성 복사 후 시작
cp config.example.yaml config.yaml
./aiops-agent --config config.yaml
```

### 방식 3: 소스 빌드

```bash
# Go 1.26+ 필요(go.mod 참고)
go build ./cmd/server ./cmd/agent

# 또는 저장소 스크립트 사용(Windows는 build.ps1, 교차 컴파일 포함; Makefile은 보안 게이트 포함: vet/test/govulncheck/gosec/staticcheck/sbom)
make build          # Linux/macOS
./build.ps1         # Windows
```

설치 완료 후, 웹 "설치 명령" 페이지에서 Agent 설치 명령을 생성(Token 자동 주입)하여 대상 호스트에 붙여넣으면 관리 대상으로 등록됩니다.

자세한 설치 및 구성은 [INSTALL.md](INSTALL.md)와 [DEPLOY_GUIDE.md](DEPLOY_GUIDE.md)를 참고하세요(영문판은 [INSTALL_EN.md](INSTALL_EN.md) / [DEPLOY_GUIDE_EN.md](DEPLOY_GUIDE_EN.md)).

---

## 사용 방법

1. **최초 로그인**: `http://<서버>:8529` 접속, 최초 로그인 시 사용자 이름 + 비밀번호 강제 변경; 관리자 계정에 **MFA** 활성화 권장.
2. **호스트 관리**: "설치 명령" 페이지에서 명령 생성 → 대상 호스트 실행 → Agent 역방향 연결 및 자동 등록; `category`(프로덕션 / 테스트 / DB / 사무)로 그룹화 가능.
3. **모니터링 구성**:
   - "알림" 페이지에서 임계값 및 거버넌스 규칙(사일런스 / 억제 / 라우팅) 설정.
   - "헬스 체크" 페이지에서 Ping / TCP / HTTP / 프로세스 생존 작업 추가; "API 모니터링" 페이지에서 비즈니스 인터페이스 연동.
   - "플레이북" 페이지에서 자동 복구 단계 오케스트레이션, 필요시 승인 가드레일 활성화.
4. **원격 운영**: 호스트 카드에서 "터미널" 열어 실시간 트러블슈팅(녹화·재생 및 2차 비밀번호 지원); "포워딩" 페이지에서 포트 포워딩 / HTTP 프록시 규칙 생성.
5. **SRE 클로즈드루프**: 인시던트, SLO, 티켓이 "운영 센터"에서 연동; AI 점검 및 진단은 "AI 어시스턴트"에서 확인.
6. **모바일**: Android / HarmonyOS App 설치, 자체 구축 서버 주소와 계정 입력, DingTalk / Feishu와 같은 푸시가 자체 구축 `/ws/push` 장기 연결로 전달.
7. **외부 장치**: Agent `config.yaml`에 `redfish_targets` / `oceanstor_targets` / `netflow` / `snmp` 등 기재, 대상 장치에 Agent 설치 없이 관리 대상으로 편입.

---

## 설정 파라미터 설명

### 서버(환경 변수 오버라이드)

| 변수 | 설명 | 필수 |
|---|---|---|
| `AIOPS_POSTGRES_DSN` | PostgreSQL 연결 문자열(관계형 데이터 + 감사 + 인시던트 + 티켓 + pgvector RAG) | 예 |
| `AIOPS_VM_URL` | VictoriaMetrics 주소(시계열 데이터) | 예 |
| `AIOPS_LISTEN` | 서버 리스닝 주소, 기본 `:8529` | 아니오 |
| `AIOPS_SECRET_KEY` | 구성 키 저장 암호화 키(AES-256-GCM) | 아니오(프로덕션 설정 권장) |
| `AIOPS_TLS_CERT` / `AIOPS_TLS_KEY` | TLS 인증서 / 개인키(HTTPS 활성화) | 아니오(프로덕션 설정 권장) |
| `AIOPS_RELAY_SECRET` | 중계 출처 검증 키 | 아니오 |
| `AIOPS_TERMINAL_DISABLED` | 전역 웹 터미널 비활성화 | 아니오 |
| `AIOPS_ALLOW_ANONYMOUS_AGENTS` | 미인증 Agent 접속 허용(디버그 전용) | 아니오 |
| `AIOPS_TRUST_PROXY` | 프론트 프록시 신뢰(X-Forwarded-*) | 아니오 |
| `AIOPS_REQUIRE_TOKEN` | Agent에 설치 Token 요구 | 아니오 |
| `AIOPS_FORWARD_*` | 포트 포워딩 관련(리스닝 주소 / 포트 범위 등) | 아니오 |

서버에는 추가로 `cmd/server/config_example.yaml`(알림 webhook, 임계값 단계, 분류, install_token, 포워딩, 계정, checks 등)이 있음.

### Agent(`config.yaml` / `config.json`)

| 그룹 | 주요 파라미터 | 설명 |
|---|---|---|
| 기본 | `server` / `token` / `category` | 서버 주소, 설치 Token, 호스트 분류 라벨 |
| 보고 | `report_interval` / `plugin_interval` | 메트릭 보고 간격(기본 30s) / 플러그인 주기(기본 60s) |
| 다중 서버 | `servers[]` | 비어 있지 않으면 단일 server를 **덮어씀**: 한 번 수집하여 모든 서버에 동시 보고 |
| 로그 | `log_paths` / `log_encrypt` | 수집 경로(와일드카드 지원) / gzip+AES 암호화 보고 여부(기본 true) |
| TLS | `tls_skip_verify` / `ca_cert` | 인증서 검증 건너뛰기(비안전) / 자체 서명 CA 지정 |
| 중계 | `relay` / `listen` / `relay_secret` | 게이트웨이 모드: 내부망 머신이 본 게이트웨이를 경유해 보고 |
| 하드웨어 | `redfish_targets` / `oceanstor_targets` | BMC / Huawei 스토리지 대역외 수집(대상에 Agent 불필요) |
| 네트워크 | `netflow` / `snmp` / `packet_capture` / `sni_dns_capture` | NetFlow / SNMP(v2c/v3+Trap) / 5-tuple / SNI-DNS 및 콘텐츠 감사 |
| 가상화 | `hyperv_*` / `container_*` | Hyper-V와 컨테이너 인벤토리(보통 자동 탐지) |

비밀번호 계열 필드는 항상 `*_env` 환경 변수(예: `REDFISH_DELL_PASSWORD`)를 우선 사용하여 평문 저장을 피하세요. 전체 예제는 [config.example.yaml](config.example.yaml)과 [cmd/agent/config_example.yaml](cmd/agent/config_example.yaml)을 참고하세요.

---

## 기술 아키텍처 개요

AIOps는 **Server-Agent 분리** 아키텍처를 채택하며, **Go + Python 하이브리드 설계 원칙**을 결합합니다: 고빈도·성능 민감한 기본 메트릭 수집은 Go(단일 바이너리, 제로 의존성)로, 가변적이거나 AI에 의존하는 커스텀 로직은 Python 플러그인으로 처리합니다.

```mermaid
graph TB
  subgraph Browser["브라우저 / 모바일"]
    UI["웹 패널 · Android · HarmonyOS"]
  end
  subgraph Server["서버 (Go)"]
    MW["미들웨어: 인증/CORS/gzip/보안 헤더"]
    API["HTTP API 라우트"]
    WS["WebSocket 푸시 / 터미널"]
    SRE["알림·헬스체크·플레이북·SRE·AI·메시지"]
    PG[(PostgreSQL)]
    VM[(VictoriaMetrics)]
  end
  subgraph Agent["Agent (Go)"]
    COL["네이티브 수집기 (고빈도)"]
    EXT["외부 수집기 (Redfish/NetFlow/OceanStor/SNMP)"]
    PLG["Python 플러그인 러너"]
    LOG["로그 수집"]
    CH["역방향 채널 (터미널/포워딩)"]
  end
  subgraph Ext["외부 장치"]
    BMC["BMC/iDRAC/iLO"]
    SW["스위치/방화벽"]
    STOR["OceanStor"]
  end
  UI --> MW --> API
  API --> SRE --> PG & VM
  API --> WS
  COL --> API
  EXT --> BMC & SW & STOR
  PLG --> API
  LOG --> API
  CH <--> API
```

**핵심 설계**

- **통신 프로토콜**: HTTP REST(Agent 등록 / 보고 / 로그 / 관리 API); WebSocket(브라우저 실시간 푸시 및 원격 터미널); Agent 능동 롱 폴링으로 터미널 / 포워딩 역방향 채널 구축.
- **데이터 흐름**: Agent 수집 → 서버 보고 → 관계형 데이터는 PG에, 시계열 데이터는 VM에 배치 기록; 브라우저는 REST / WebSocket로 조회 및 하달.
- **이중 스토리지 강제**: PG(관계형 + 감사 + 인시던트 + 티켓 + 세션 + 벡터 메모리)와 VM(메트릭 / 추세), 둘 중 하나라도 누락되면 시작 거부.
- **외부 수집기**: 표준 프로토콜 원격 폴링, 대상 장치 무침투, 네트워크 연결성만 필요.
- **다중 서버 브로드캐스트**: 단일 Agent가 한 번 수집하여 여러 서버에 동시 보고, 약한 네트워크에서 재시도 / 서킷 브레이커 / gzip 폴백.
- **AI / RAG**: 플러그인 가능 LLM + pgvector 유사 사례 검색.

보다 완전한 아키텍처, 데이터 흐름, 성능 및 문제 해결은 [.qoder/repowiki/zh](.qoder/repowiki/zh/content)의 "아키텍처 설계" "핵심 기능" "문제 해결" 문서를 참고하세요.

---

## 기여 가이드

코드, 문서, 번역 기여를 환영합니다!

1. **Issue 등록**: 버그, 기능 제안, 문서 수정 모두 환영하며, 재현 단계와 환경 정보를 최대한 제공해 주세요.
2. **개발 환경**: Go 1.26+, Python 3(플러그인 SDK). `make build`로 빌드, `make audit`로 보안 게이트 실행(vet / test / govulncheck / gosec / staticcheck / sbom).
3. **코드 규칙**:
   - Go 코드는 `gofmt` / `go vet` 사용; 새 로직에는 테스트를 동반하세요.
   - 서버는 제로 프레임워크, 제로 CGO; 단일 바이너리, 제3자 의존성 없는 Agent 원칙 유지.
   - 커밋 메시지는 "왜"를 명확히 표현.
4. **국제화**: 마케팅 사이트는 简中 / 繁中 / English / 日本語 / 한국어 5개 언어, 관리 패널은 简中 / 繁中 / English 3개 언어를 지원합니다. 새 문구는 해당 언어 사전에 동기화하세요(마케팅 사이트: `website/js/i18n.js`와 `website/js/i18n-extra.js`; 관리 패널: `cmd/server/web/` 하위 `i18n-dashboard*.js`. 패널 흐름: 권위 사전 수정 → 영어 보완 → `build_en` / `build_tw` 실행 → parity 검증 → `go build` 재임베드).
5. **PR 제출**: Fork → 브랜치 개발 → 변경 및 테스트 설명 → CI와 리뷰 대기.
6. **보안 취약점**: 공개 Issue로 등록하지 말고 비공개 메시지 / 보안 채널로 보고해 주세요. 우선 처리하겠습니다.
7. **개발자 상세 규정**은 [.qoder/repowiki/zh/content/开发者指南](.qoder/repowiki/zh/content/开发者指南) 참고.

---

## 라이선스

본 프로젝트는 **MIT 라이선스**로 오픈소스이며, 자세한 내용은 [LICENSE](LICENSE)를 참고하세요.

- 코드는 GitHub에 호스팅되어 투명하고 신뢰할 수 있습니다. 호스트 수 제한 없음, 기능 축소 없음, "엔터프라이즈 에디션" 술책 없음.
- 제3자 의존성(예: `vendor/` 하의 `lib/pq`, `go-qrcode`, `ledongthuc/pdf`, 그리고 `harmony/`의 ohpm 의존성)은 각자의 라이선스를 따르며 원저자에게 귀속됩니다.

---

<p align="center">
  <b>AIOps · 운영의 복잡성을, 당신이 완전히 통제하는 하나의 플랫폼으로 수렴하세요.</b>
</p>
