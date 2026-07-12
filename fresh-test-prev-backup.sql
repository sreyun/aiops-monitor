--
-- PostgreSQL database dump
--

\restrict uhefo73TmStSKXNIti6gImwnLkDeKTuU76Xd8Jv8zzicomg3qVc15rswgsD9qdN

-- Dumped from database version 18.4 (Debian 18.4-1.pgdg12+1)
-- Dumped by pg_dump version 18.4 (Debian 18.4-1.pgdg12+1)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: vector; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;


--
-- Name: EXTENSION vector; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION vector IS 'vector data type and ivfflat and hnsw access methods';


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: app_config; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.app_config (
    id integer NOT NULL,
    data jsonb NOT NULL
);


--
-- Name: audit_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audit_log (
    id bigint NOT NULL,
    ts bigint,
    data jsonb NOT NULL
);


--
-- Name: audit_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.audit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: audit_log_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.audit_log_id_seq OWNED BY public.audit_log.id;


--
-- Name: diagnosis_embeddings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.diagnosis_embeddings (
    id bigint NOT NULL,
    incident_id bigint,
    embedding public.vector(1536),
    summary text NOT NULL,
    severity text,
    tags text,
    feedback text DEFAULT ''::text,
    created_at timestamp with time zone DEFAULT now()
);


--
-- Name: diagnosis_embeddings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.diagnosis_embeddings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: diagnosis_embeddings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.diagnosis_embeddings_id_seq OWNED BY public.diagnosis_embeddings.id;


--
-- Name: events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.events (
    id bigint NOT NULL,
    ts bigint,
    data jsonb NOT NULL
);


--
-- Name: events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.events_id_seq OWNED BY public.events.id;


--
-- Name: experience_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.experience_rules (
    id bigint NOT NULL,
    pattern text NOT NULL,
    conclusion text NOT NULL,
    severity text,
    incident_id bigint,
    created_at timestamp with time zone DEFAULT now()
);


--
-- Name: experience_rules_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.experience_rules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: experience_rules_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.experience_rules_id_seq OWNED BY public.experience_rules.id;


--
-- Name: hermes_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.hermes_rules (
    id bigint NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text,
    priority integer DEFAULT 0,
    enabled boolean DEFAULT true,
    config jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: hermes_rules_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.hermes_rules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: hermes_rules_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.hermes_rules_id_seq OWNED BY public.hermes_rules.id;


--
-- Name: hermes_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.hermes_sessions (
    id bigint NOT NULL,
    incident_id bigint DEFAULT 0,
    status text DEFAULT 'active'::text,
    messages jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: hermes_sessions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.hermes_sessions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: hermes_sessions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.hermes_sessions_id_seq OWNED BY public.hermes_sessions.id;


--
-- Name: hermes_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.hermes_templates (
    id bigint NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text,
    content text NOT NULL,
    category text DEFAULT 'system'::text,
    version integer DEFAULT 1,
    active boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: hermes_templates_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.hermes_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: hermes_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.hermes_templates_id_seq OWNED BY public.hermes_templates.id;


--
-- Name: hosts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.hosts (
    id text NOT NULL,
    data jsonb NOT NULL
);


--
-- Name: incidents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.incidents (
    id bigint NOT NULL,
    status text,
    created_at bigint,
    data jsonb NOT NULL
);


--
-- Name: kv_state; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kv_state (
    k text NOT NULL,
    data jsonb NOT NULL
);


--
-- Name: tickets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tickets (
    id bigint NOT NULL,
    status text,
    created_at bigint,
    data jsonb NOT NULL
);


--
-- Name: audit_log id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_log ALTER COLUMN id SET DEFAULT nextval('public.audit_log_id_seq'::regclass);


--
-- Name: diagnosis_embeddings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.diagnosis_embeddings ALTER COLUMN id SET DEFAULT nextval('public.diagnosis_embeddings_id_seq'::regclass);


--
-- Name: events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events ALTER COLUMN id SET DEFAULT nextval('public.events_id_seq'::regclass);


--
-- Name: experience_rules id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.experience_rules ALTER COLUMN id SET DEFAULT nextval('public.experience_rules_id_seq'::regclass);


--
-- Name: hermes_rules id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hermes_rules ALTER COLUMN id SET DEFAULT nextval('public.hermes_rules_id_seq'::regclass);


--
-- Name: hermes_sessions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hermes_sessions ALTER COLUMN id SET DEFAULT nextval('public.hermes_sessions_id_seq'::regclass);


--
-- Name: hermes_templates id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hermes_templates ALTER COLUMN id SET DEFAULT nextval('public.hermes_templates_id_seq'::regclass);


--
-- Data for Name: app_config; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.app_config (id, data) FROM stdin;
1	{"ai": {"model": "", "enabled": false, "endpoint": "", "inspect_interval_min": 0}, "vm": {"url": "http://victoriametrics:8428", "enabled": true}, "slos": [{"id": "13835691", "name": "主机 CPU 可用性目标 99.9%（滚动30天窗口）", "metric": "cpu_percent", "target": 99.9, "enabled": true, "host_id": "0b13099afec7903ff87ac099609d651b", "threshold": 90, "comparator": "<", "created_at": 1783749679, "updated_at": 1783749679, "source_type": "metric", "window_days": 30}], "smtp": {"smtp_host": "", "smtp_port": 0, "smtp_enabled": false, "smtp_use_tls": false, "smtp_username": "", "smtp_from_name": "AIOps Monitor"}, "users": [{"hash": "pbkdf2$sha256$600000$8a4aa8c756ba78e1b9b42ad25539fd338a8384bb22b58d8008860430472bf8bc", "role": "admin", "salt": "767c768064162fbb", "email": "", "username": "admin", "mfa_enabled": false, "display_name": "管理员", "terminal_password_hash": "pbkdf2$sha256$600000$af45e0aa556ce8814ae75648f12db2b724284c12286bcdfa8d6a67d05526d888", "terminal_password_salt": "5341fa677f36540f"}], "checks": [{"id": "80961b3a", "name": "核心交易 API 健康检查（生产环境主入口）", "type": "http", "level": "critical", "target": "https://api.example.com/v1/health/readiness?verbose=true", "enabled": true, "interval_sec": 30}, {"id": "417232de", "name": "PostgreSQL 主库端口探测", "type": "tcp", "level": "critical", "target": "db-primary.internal.example.com:5432", "enabled": true, "interval_sec": 15}, {"id": "e1fb2f2b", "name": "Redis 缓存进程存活", "type": "process", "level": "warning", "target": "0b13099afec7903ff87ac099609d651b/redis-server", "enabled": true, "interval_sec": 60}], "feishu": {"enabled": false, "webhook": ""}, "account": {"hash": "", "salt": "", "email": "", "username": "", "mfa_enabled": false, "display_name": ""}, "dingtalk": {"enabled": false, "webhook": ""}, "categories": {}, "thresholds": {"cpu_crit": 95, "cpu_warn": 80, "gpu_crit": 95, "gpu_warn": 80, "mem_crit": 95, "mem_warn": 85, "disk_crit": 90, "disk_warn": 80, "iops_crit": 100000, "iops_warn": 50000, "load_crit": 8, "load_warn": 4, "proc_warn": 0.5, "diskio_crit": 95, "diskio_warn": 80, "offline_after_sec": 60}, "trust_proxy": false, "mfa_required": false, "postgres_dsn": "postgres://aiops:h3Y7Vmb1CZBOApZM86D@postgres:5432/aiops?sslmode=disable", "install_token": "4936ba85485f23ad4c0b80f96278c940", "require_token": false, "alerts_enabled": true, "custom_webhook": {"url": "", "method": "", "enabled": false, "headers": "", "content_type": "", "body_template": ""}, "forward_listen": "0.0.0.0", "forward_disabled": false, "terminal_disabled": false, "allow_anonymous_agents": false}
\.


--
-- Data for Name: audit_log; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.audit_log (id, ts, data) FROM stdin;
1	1783739769	{"kind": "operation", "actor": "172.19.0.1", "level": "warning", "message": "检测到默认凭据登录，强制用户改密：admin", "timestamp": 1783739769}
2	1783739769	{"kind": "operation", "actor": "172.19.0.1", "level": "info", "message": "登录成功：admin", "timestamp": 1783739769}
3	1783739790	{"kind": "operation", "actor": "172.19.0.1", "level": "warning", "message": "修改登录密码：admin", "timestamp": 1783739790}
4	1783740286	{"kind": "operation", "actor": "172.19.0.1", "level": "warning", "message": "设置终端密码：admin", "timestamp": 1783740286}
5	1783740286	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "warning", "message": "打开远程终端 Eason", "timestamp": 1783740286}
6	1783741193	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "warning", "message": "打开远程终端 Eason", "timestamp": 1783741193}
7	1783741203	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "info", "message": "关闭远程终端 Eason", "timestamp": 1783741203}
8	1783741205	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "warning", "message": "打开远程终端 Eason", "timestamp": 1783741205}
9	1783741208	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "info", "message": "终端命令 [Eason]: ls", "timestamp": 1783741208}
10	1783741213	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "info", "message": "终端命令 [Eason]: ipcocnfig", "timestamp": 1783741213}
11	1783741219	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "info", "message": "终端命令 [Eason]: ipconfig", "timestamp": 1783741219}
12	1783741227	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "info", "message": "关闭远程终端 Eason", "timestamp": 1783741227}
13	1783742279	{"kind": "operation", "actor": "172.19.0.1", "level": "info", "message": "登录成功：admin", "timestamp": 1783742279}
14	1783749679	{"kind": "operation", "actor": "172.19.0.1", "level": "info", "message": "保存自定义监控：核心交易 API 健康检查（生产环境主入口）", "timestamp": 1783749679}
15	1783749679	{"kind": "operation", "actor": "172.19.0.1", "level": "info", "message": "保存自定义监控：PostgreSQL 主库端口探测", "timestamp": 1783749679}
16	1783749679	{"kind": "operation", "actor": "172.19.0.1", "level": "info", "message": "保存自定义监控：Redis 缓存进程存活", "timestamp": 1783749679}
17	1783749679	{"kind": "operation", "actor": "172.19.0.1", "level": "info", "message": "保存自定义监控：官网首页可用性", "timestamp": 1783749679}
18	1783749679	{"kind": "operation", "actor": "admin", "level": "info", "message": "保存 SLO「主机 CPU 可用性目标 99.9%（滚动30天窗口）」", "timestamp": 1783749679}
19	1783749679	{"kind": "operation", "actor": "admin", "level": "info", "message": "保存 SLO「内存使用 SLO」", "timestamp": 1783749679}
20	1783749697	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783749697}
21	1783749712	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783749712}
22	1783749742	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 157 个进程））", "timestamp": 1783749742}
23	1783751195	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783751195}
24	1783751210	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783751210}
25	1783751240	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 157 个进程））", "timestamp": 1783751240}
26	1783751428	{"kind": "operation", "actor": "172.19.0.1", "level": "info", "message": "登录成功：admin", "timestamp": 1783751428}
27	1783752218	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783752218}
28	1783752233	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783752233}
29	1783752259	{"kind": "operation", "actor": "172.19.0.1", "level": "info", "message": "登录成功：admin", "timestamp": 1783752259}
30	1783752262	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 160 个进程））", "timestamp": 1783752262}
31	1783752722	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783752722}
32	1783752738	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783752738}
33	1783752768	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 158 个进程））", "timestamp": 1783752768}
34	1783752778	{"kind": "operation", "actor": "172.19.0.1", "level": "info", "message": "登录成功：admin", "timestamp": 1783752778}
35	1783755241	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783755241}
36	1783755257	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783755257}
37	1783755261	{"kind": "operation", "actor": "172.19.0.1", "level": "info", "message": "登录成功：admin", "timestamp": 1783755261}
38	1783755286	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 159 个进程））", "timestamp": 1783755286}
39	1783755313	{"kind": "operation", "actor": "172.19.0.1", "level": "info", "message": "登录成功：admin", "timestamp": 1783755313}
40	1783755313	{"kind": "operation", "actor": "172.19.0.1", "level": "warning", "message": "删除自定义监控 fc3db5d7", "timestamp": 1783755313}
41	1783755314	{"kind": "operation", "actor": "172.19.0.1", "level": "warning", "message": "删除主机 0b13099a", "timestamp": 1783755314}
42	1783755441	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783755441}
43	1783755456	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783755456}
44	1783755486	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（主机 0b13099a 无进程数据或已离线）", "timestamp": 1783755486}
45	1783770572	{"kind": "operation", "actor": "172.18.0.1", "level": "info", "message": "登录成功：admin", "timestamp": 1783770572}
46	1783770990	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783770990}
47	1783771005	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783771005}
48	1783771035	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 164 个进程））", "timestamp": 1783771035}
49	1783773547	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "warning", "message": "创建端口转发 b18c33ae → Eason:8529 (本地 0.0.0.0:10218)", "timestamp": 1783773547}
50	1783773547	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "info", "message": "关闭端口转发 Eason → :8529", "timestamp": 1783773547}
51	1783773547	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "info", "message": "关闭端口转发 Eason → :8529", "timestamp": 1783773547}
52	1783773954	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "warning", "message": "创建端口转发 28e0c64a → Eason:8529 (本地 0.0.0.0:10297)", "timestamp": 1783773954}
53	1783773957	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "info", "message": "关闭端口转发 Eason → :8529", "timestamp": 1783773957}
54	1783774116	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783774116}
55	1783774132	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783774132}
56	1783774161	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 165 个进程））", "timestamp": 1783774161}
57	1783774972	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783774972}
131	1783825141	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 48478 秒", "timestamp": 1783825141}
58	1783774987	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783774987}
59	1783775017	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 166 个进程））", "timestamp": 1783775017}
60	1783775036	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "warning", "message": "创建端口转发 e5ac44c6 → Eason:8529 (本地 0.0.0.0:10159)", "timestamp": 1783775036}
61	1783775038	{"host": "Eason", "kind": "operation", "actor": "admin", "level": "info", "message": "关闭端口转发 Eason → :8529", "timestamp": 1783775038}
62	1783776727	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 64 秒", "timestamp": 1783776727}
63	1783776977	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 314 秒", "timestamp": 1783776977}
64	1783776992	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783776992}
65	1783777008	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783777008}
66	1783777037	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783777037}
67	1783777147	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 484 秒", "timestamp": 1783777147}
68	1783777162	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783777162}
69	1783777178	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783777178}
70	1783777207	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783777207}
71	1783778270	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 1607 秒", "timestamp": 1783778270}
72	1783778285	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783778285}
73	1783778301	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783778301}
74	1783778330	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783778330}
75	1783778513	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 1850 秒", "timestamp": 1783778513}
76	1783778528	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783778528}
77	1783778543	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783778543}
78	1783778573	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783778573}
79	1783779369	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 2706 秒", "timestamp": 1783779369}
80	1783779384	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783779384}
81	1783779400	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783779400}
82	1783779429	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783779429}
83	1783786222	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 9559 秒", "timestamp": 1783786222}
84	1783786237	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783786237}
85	1783786252	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783786252}
86	1783786282	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783786282}
87	1783787822	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 11159 秒", "timestamp": 1783787822}
88	1783787837	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783787837}
89	1783787852	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783787852}
90	1783787882	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783787882}
91	1783788790	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 12127 秒", "timestamp": 1783788790}
92	1783788805	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783788805}
93	1783788821	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783788821}
94	1783788850	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783788850}
95	1783789282	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 12619 秒", "timestamp": 1783789282}
96	1783789297	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783789297}
97	1783789313	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783789313}
98	1783789342	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783789342}
99	1783789561	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 12898 秒", "timestamp": 1783789561}
100	1783789576	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783789576}
101	1783789592	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783789592}
102	1783789621	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783789621}
103	1783789675	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 13012 秒", "timestamp": 1783789675}
104	1783789690	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783789690}
129	1783824918	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783824918}
105	1783789705	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783789705}
106	1783789735	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783789735}
107	1783789909	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 13246 秒", "timestamp": 1783789909}
108	1783789924	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783789924}
109	1783789940	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783789940}
110	1783789969	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783789969}
111	1783791844	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 15181 秒", "timestamp": 1783791844}
112	1783791859	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783791859}
113	1783791874	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783791874}
114	1783791904	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783791904}
115	1783792234	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 15571 秒", "timestamp": 1783792234}
116	1783792249	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783792249}
117	1783792264	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783792264}
118	1783792294	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783792294}
119	1783824117	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 47454 秒", "timestamp": 1783824117}
120	1783824132	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783824132}
121	1783824147	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783824147}
122	1783824177	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783824177}
123	1783824213	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：admin", "timestamp": 1783824213}
124	1783824225	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：kinney", "timestamp": 1783824225}
125	1783824229	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：admin", "timestamp": 1783824229}
126	1783824858	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 48195 秒", "timestamp": 1783824858}
127	1783824873	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783824873}
128	1783824889	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783824889}
130	1783824991	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：admin", "timestamp": 1783824991}
132	1783825156	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783825156}
133	1783825172	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783825172}
134	1783825201	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783825201}
135	1783825482	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 48819 秒", "timestamp": 1783825482}
136	1783825497	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783825497}
137	1783825513	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783825513}
138	1783825542	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783825542}
139	1783826609	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：admin", "timestamp": 1783826609}
140	1783826725	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：admin", "timestamp": 1783826725}
141	1783826725	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：", "timestamp": 1783826725}
142	1783826732	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：admin", "timestamp": 1783826732}
143	1783828781	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：admin", "timestamp": 1783828781}
144	1783828784	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：admin", "timestamp": 1783828784}
145	1783828794	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：kinney", "timestamp": 1783828794}
146	1783828798	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：Ethan", "timestamp": 1783828798}
147	1783828803	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：Ethan", "timestamp": 1783828803}
148	1783829354	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 52691 秒", "timestamp": 1783829354}
149	1783829369	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783829369}
150	1783829384	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783829384}
151	1783829414	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783829414}
152	1783829554	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：admin", "timestamp": 1783829554}
153	1783829645	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 52982 秒", "timestamp": 1783829645}
154	1783829660	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783829660}
155	1783829676	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783829676}
156	1783829705	{"kind": "system", "actor": "172.18.0.1", "level": "warning", "message": "登录失败：admin", "timestamp": 1783829705}
157	1783829705	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783829705}
158	1783832206	{"host": "Eason", "kind": "system", "actor": "告警引擎", "level": "critical", "message": "告警触发：主机 Eason（192.168.60.11）已失联 55543 秒", "timestamp": 1783832206}
159	1783832221	{"host": "PostgreSQL 主库端口探测", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：PostgreSQL 主库端口探测（连接失败: dial tcp: lookup db-primary.internal.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783832221}
160	1783832237	{"host": "核心交易 API 健康检查（生产环境主入口）", "kind": "system", "actor": "自定义监控", "level": "critical", "message": "自定义监控异常：核心交易 API 健康检查（生产环境主入口）（请求失败: Get \\"https://api.example.com/v1/health/readiness?verbose=true\\": dial tcp: lookup api.example.com on 127.0.0.11:53: no such host）", "timestamp": 1783832237}
161	1783832266	{"host": "Redis 缓存进程存活", "kind": "system", "actor": "自定义监控", "level": "warning", "message": "自定义监控异常：Redis 缓存进程存活（进程 \\"redis-server\\" 未运行（共上报 163 个进程））", "timestamp": 1783832266}
\.


--
-- Data for Name: diagnosis_embeddings; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.diagnosis_embeddings (id, incident_id, embedding, summary, severity, tags, feedback, created_at) FROM stdin;
\.


--
-- Data for Name: events; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.events (id, ts, data) FROM stdin;
\.


--
-- Data for Name: experience_rules; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.experience_rules (id, pattern, conclusion, severity, incident_id, created_at) FROM stdin;
\.


--
-- Data for Name: hermes_rules; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.hermes_rules (id, name, description, priority, enabled, config, created_at, updated_at) FROM stdin;
\.


--
-- Data for Name: hermes_sessions; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.hermes_sessions (id, incident_id, status, messages, created_at, updated_at) FROM stdin;
\.


--
-- Data for Name: hermes_templates; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.hermes_templates (id, name, description, content, category, version, active, created_at, updated_at) FROM stdin;
\.


--
-- Data for Name: hosts; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.hosts (id, data) FROM stdin;
0b13099afec7903ff87ac099609d651b	{"id": "0b13099afec7903ff87ac099609d651b", "ip": "192.168.60.11", "os": "windows", "arch": "amd64", "kernel": "10.0.26200", "latest": {"disks": [{"path": "C:", "used": 221902270464, "total": 321208184832, "percent": 69.1}, {"path": "D:", "used": 354738626560, "total": 701021286400, "percent": 50.6}], "load1": 1.96, "load5": 1.89, "load15": 1.74, "uptime": 77570, "mem_used": 24116924416, "cpu_cores": 22, "disk_used": 221902270464, "mem_total": 33779150848, "net_conns": 139, "swap_used": 18327044096, "timestamp": 1783776663, "disk_total": 321208184832, "proc_count": 509, "swap_total": 68719476736, "cpu_percent": 5.3, "mem_percent": 71.4, "disk_percent": 69.1, "swap_percent": 26.7, "net_recv_rate": 35961.5, "net_sent_rate": 13235.1, "process_names": ["[System Process]", "System", "Secure System", "Registry", "smss.exe", "csrss.exe", "wininit.exe", "services.exe", "LsaIso.exe", "lsass.exe", "svchost.exe", "fontdrvhost.exe", "WUDFHost.exe", "vmms.exe", "Memory Compression", "vmcompute.exe", "spoolsv.exe", "WmiPrvSE.exe", "WUCSProxyService.exe", "NgcIso.exe", "CCB_HDZB_2G_DeviceService.exe", "EsCcbDriverSrv.exe", "AweSun.exe", "HMSCoreService.exe", "HuaweiThpService.exe", "HWDisplayService.exe", "HZ_CommSrv.exe", "HiConnectivityService.exe", "BasicService.exe", "OneApp.IGCC.WinService.exe", "osdservice.exe", "SyncService.exe", "MateBookService.exe", "HWVEAudioService.exe", "IntelAnalyticsService.exe", "ipf_uf.exe", "MsMpEng.exe", "openvpnserv.exe", "openvpnserv2.exe", "MpDefenderCoreService.exe", "D4Ser_CCB.exe", "qoderwake-service.exe", "LCD_Service.exe", "SessionService.exe", "RtkAudUService64.exe", "WMIRegistrationService.exe", "WDKeyMonitorCCB.exe", "tvnserver.exe", "IntelAudioService.exe", "wslservice.exe", "cowork-svc.exe", "D4Mon_CCB.exe", "IntelConnectivityNetworkService.exe", "HMSCoreContainer.exe", "conhost.exe", "unsecapp.exe", "nfsclnt.exe", "AggregatorHost.exe", "qoderwake-bootstrapper.exe", "IDBWMService.exe", "IDBWM.exe", "wlanext.exe", "IntelConnectService.exe", "Intel_PIE_Service.exe", "IntelConnect.exe", "awesun_guard.exe", "NisSrv.exe", "intel_cst_service_standalone.exe", "Lansweeper.OnPremise.AutoUpdate.exe", "winlogon.exe", "dwm.exe", "sihost.exe", "taskhostw.exe", "powershell.exe", "explorer.exe", "ShellHost.exe", "CrossDeviceResume.exe", "HwDistributedMainService.exe", "SearchHost.exe", "StartMenuExperienceHost.exe", "Widgets.exe", "RuntimeBroker.exe", "WidgetService.exe", "msedgewebview2.exe", "chrome.exe", "cmd.exe", "extension-host.exe", "chrome-native-host.exe", "pythonw.exe", "qoderwake-cn.exe", "TextInputHost.exe", "EsCcbTool.exe", "ctfmon.exe", "EsCcbExtUI.exe", "ChsIME.exe", "SogouImeBroker.exe", "SogouCloud.exe", "SGTool.exe", "SOGOUSmartAssistant.exe", "SearchIndexer.exe", "claude.exe", "Taskmgr.exe", "WhatsApp.Root.exe", "M365Copilot.exe", "CCBCertificate.exe", "USBKeyTools.exe", "D4Svr_CCB.exe", "clash-verge.exe", "verge-mihomo.exe", "QoderCN.exe", "WorkBuddy.exe", "node.exe", "mysqlsh.exe", "RemotePC_Service.exe", "OneDrive.Sync.Service.exe", "SecurityHealthService.exe", "wps.exe", "promecefpluginhost.exe", "ShellExperienceHost.exe", "UserOOBEBroker.exe", "Doubao.exe", "aha_doctor.exe", "DynamicDependencyLifetimeManagerShadow.exe", "PushNotificationsLongRunningTask.exe", "WindowsTerminal.exe", "OpenConsole.exe", "msedge.exe", "xl_ext_chrome.exe", "Docker Desktop.exe", "com.docker.backend.exe", "vmwp.exe", "vmmemWSL", "dllhost.exe", "com.docker.build.exe", "wslrelay.exe", "wsl.exe", "wslhost.exe", "msrdc.exe", "HiviewService.exe", "HwMdcCenter.exe", "DFSSearchService.exe", "HwMdcUI.exe", "MBAMessageCenter.exe", "AppGalleryService.exe", "ApplicationFrameHost.exe", "SystemSettings.exe", "AppGalleryAMS.exe", "PcOptimizationCenter.exe", "MessageCenterUI.exe", "WDCertM_CCB.exe", "LockApp.exe", "TabTip.exe", "LogonUI.exe", "rdpclip.exe", "HaoZipWorker.exe", "notepad++.exe", "prevhost.exe", "bash.exe", "python.exe", "pdfext.exe", "agent-browser-win32-x64.exe", "aiops-agent.exe", "SearchProtocolHost.exe"], "disk_read_iops": 0, "disk_read_rate": 0, "disk_write_iops": 0, "disk_write_rate": 0, "disk_io_util_percent": 0}, "category": "测试", "hostname": "Eason", "platform": "Windows 11 (Build 26200)", "last_seen": 1783776663, "first_seen": 1783770573, "fingerprint": "9d4992906160b56e6e6ae05a"}
\.


--
-- Data for Name: incidents; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.incidents (id, status, created_at, data) FROM stdin;
2	resolved	1783749679	{"id": 2, "title": "数据库主节点 CPU 持续超过 95% 且慢查询堆积——疑似索引缺失导致全表扫描，影响核心交易链路", "source": "manual", "status": "resolved", "host_id": "0b13099afec7903ff87ac099609d651b", "hostname": "Eason", "severity": "critical", "timeline": [{"ts": 1783749679, "kind": "created", "text": "数据库主节点 CPU 持续超过 95% 且慢查询堆积——疑似索引缺失导致全表扫描，影响核心交易链路", "actor": "admin"}, {"ts": 1783755314, "kind": "resolved", "text": "", "actor": "admin"}], "created_at": 1783749679, "resolved_at": 1783755314}
3	open	1783749679	{"id": 3, "title": "内存使用率突破 90% 告警阈值，OOM Killer 触发风险升高", "source": "manual", "status": "open", "host_id": "0b13099afec7903ff87ac099609d651b", "hostname": "Eason", "severity": "warning", "timeline": [{"ts": 1783749679, "kind": "created", "text": "内存使用率突破 90% 告警阈值，OOM Killer 触发风险升高", "actor": "admin"}], "created_at": 1783749679}
4	open	1783749679	{"id": 4, "title": "磁盘 /var 分区剩余空间不足 10%，日志轮转与写入可能失败", "source": "manual", "status": "open", "host_id": "0b13099afec7903ff87ac099609d651b", "hostname": "Eason", "severity": "critical", "timeline": [{"ts": 1783749679, "kind": "created", "text": "磁盘 /var 分区剩余空间不足 10%，日志轮转与写入可能失败", "actor": "admin"}], "created_at": 1783749679}
5	open	1783749679	{"id": 5, "title": "网络出口延迟抖动", "source": "manual", "status": "open", "host_id": "0b13099afec7903ff87ac099609d651b", "hostname": "Eason", "severity": "info", "timeline": [{"ts": 1783749679, "kind": "created", "text": "网络出口延迟抖动", "actor": "admin"}], "created_at": 1783749679}
6	open	1783749679	{"id": 6, "title": "TLS 证书 monitor.example.com 将在 7 天内到期，请及时续签以避免面板不可访问", "source": "manual", "status": "open", "severity": "warning", "timeline": [{"ts": 1783749679, "kind": "created", "text": "TLS 证书 monitor.example.com 将在 7 天内到期，请及时续签以避免面板不可访问", "actor": "admin"}], "created_at": 1783749679}
7	open	1783752259	{"id": 7, "title": "��Ϣ�����������������ݿ����ڵ㲻�ɴ�", "source": "manual", "status": "open", "host_id": "0b13099afec7903ff87ac099609d651b", "hostname": "Eason", "severity": "critical", "timeline": [{"ts": 1783752259, "kind": "created", "text": "��Ϣ�����������������ݿ����ڵ㲻�ɴ�", "actor": "admin"}, {"ts": 1783752259, "kind": "note", "text": "【启发式诊断 · 基于规则】\\n\\n根因方向：\\n结合指标趋势与错误日志定位异常起始时间，缩小到具体服务/进程后再处置。\\n\\n采集到的上下文：\\n事件 #7：��Ϣ�����������������ݿ����ڵ㲻�ɴ�（级别 critical，状态 open，来源 manual）\\n主机：Eason\\n当前指标：CPU 6.0% · 内存 62.5% · 磁盘 69.0% · Load 2.09 · 进程 461\\n近 1 小时该主机无 error/warn 日志。\\n\\n提示：在「AI 巡检」页配置 AI Provider 后，可获得智能体级别的根因研判与处置编排。", "actor": "ai-heuristic"}], "created_at": 1783752259}
8	open	1783776727	{"id": 8, "key": "0b13099afec7903ff87ac099609d651b/offline/", "type": "offline", "title": "主机 Eason（192.168.60.11）已失联 64 秒", "source": "alert", "status": "open", "host_id": "0b13099afec7903ff87ac099609d651b", "hostname": "Eason", "severity": "critical", "timeline": [{"ts": 1783776727, "kind": "created", "text": "主机 Eason（192.168.60.11）已失联 64 秒", "actor": "alert"}, {"ts": 1783776727, "kind": "note", "text": "【启发式诊断 · 基于规则】\\n\\n根因方向：\\n检查主机网络连通与 Agent 进程是否存活；确认是否宕机或正在重启。\\n\\n采集到的上下文：\\n事件 #8：主机 Eason（192.168.60.11）已失联 64 秒（级别 critical，状态 open，来源 alert）\\n主机：Eason\\n当前指标：CPU 5.3% · 内存 71.4% · 磁盘 69.1% · Load 1.96 · 进程 509\\n近 1 小时该主机无 error/warn 日志。\\n\\n提示：在「AI 巡检」页配置 AI Provider 后，可获得智能体级别的根因研判与处置编排。", "actor": "ai-heuristic"}], "created_at": 1783776727}
\.


--
-- Data for Name: kv_state; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.kv_state (k, data) FROM stdin;
remediation_runs	[]
slo_burning	{}
logs	[]
alert_states	{}
sessions	{"96221c05f084a27f302262c761dcb176dfe7d2fd357c9163ddf9f1c130748a68": {"user": "admin", "expires": 1784375372}, "9d26c3bba981f1b7e760a0d1976a605d5436618bf6d579217c75e369b8ab4960": {"user": "admin", "expires": 1784357578}, "be40ddd7b63a9f264b5472c4ded03c49fd9c3b68f85fe6da64c53897aeaffd47": {"user": "admin", "expires": 1784360061}, "c54a1e01e8032f3b1e5dfc12ab203abab77d5f83b560f5bb18da02805f4fa0e1": {"user": "admin", "expires": 1784347079}, "ca67141b08322fda0de1fe7945ff8389be4a4d3879d90fc6c9550a76acef7ccd": {"user": "admin", "expires": 1784344590, "terminal_verified": true}, "cede9f7be6b892555713be732b59adf7b3cd39fa172d4625ee204b3a1fc2dc17": {"user": "admin", "expires": 1784360113}, "d45d02cd6f97fcc38d81ae2acf860511ec8c94416979f699c10913b1cfc8559f": {"user": "admin", "expires": 1784357059}, "ff674645fa88640e398db4dc999f76f6021aeac2ef64d7713e7affcffc79fee3": {"user": "admin", "expires": 1784356228}}
messages	[{"id": 1, "ts": 1783752259, "ref": "7", "body": "级别 critical · 来源 manual · 主机 Eason", "read": true, "type": "incident", "view": "sre", "level": "critical", "title": "新事件：��Ϣ�����������������ݿ����ڵ㲻�ɴ�"}, {"id": 2, "ts": 1783752259, "ref": "7", "body": "【启发式诊断 · 基于规则】  根因方向： 结合指标趋势与错误日志定位异常起始时间，缩小到具体服务/进程后再处置。  采集到的上下文： 事件 #7：��Ϣ�����…", "read": true, "type": "ai", "view": "sre", "level": "info", "title": "AI 诊断 · ��Ϣ�����������������ݿ����ڵ㲻�ɴ�"}, {"id": 3, "ts": 1783755261, "ref": "2", "body": "优先级 p2 · 状态 open", "read": true, "type": "ticket", "view": "sre", "level": "info", "title": "新工单：��Ϣ���Ĺ�������"}, {"id": 4, "ts": 1783776727, "ref": "8", "body": "级别 critical · 来源 alert · 主机 Eason", "read": false, "type": "incident", "view": "sre", "level": "critical", "title": "新事件：主机 Eason（192.168.60.11）已失联 64 秒"}, {"id": 5, "ts": 1783776727, "ref": "8", "body": "【启发式诊断 · 基于规则】  根因方向： 检查主机网络连通与 Agent 进程是否存活；确认是否宕机或正在重启。  采集到的上下文： 事件 #8：主机 Eason（192.168.60.11）已�…", "read": false, "type": "ai", "view": "sre", "level": "info", "title": "AI 诊断 · 主机 Eason（192.168.60.11）已失联 64 秒"}, {"id": 6, "ts": 1783776757, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 7, "ts": 1783781169, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 8, "ts": 1783782969, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 9, "ts": 1783784769, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 10, "ts": 1783791709, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 11, "ts": 1783794034, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 12, "ts": 1783795834, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 13, "ts": 1783797634, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 14, "ts": 1783799434, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 15, "ts": 1783801234, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 16, "ts": 1783803033, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 17, "ts": 1783804833, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 18, "ts": 1783806633, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 19, "ts": 1783808433, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 20, "ts": 1783810233, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 21, "ts": 1783812033, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 22, "ts": 1783813833, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 23, "ts": 1783815633, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 24, "ts": 1783817433, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 25, "ts": 1783827282, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 26, "ts": 1783829082, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}, {"id": 27, "ts": 1783831445, "body": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "read": false, "type": "ai", "view": "sre", "level": "critical", "title": "AI 巡检发现 2 项风险"}]
ai_inspections	[{"id": 2, "ts": 1783791709, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 15036 秒", "severity": "critical"}], "duration_ms": 23}, {"id": 3, "ts": 1783794034, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 17371 秒", "severity": "critical"}], "duration_ms": 22}, {"id": 4, "ts": 1783795834, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 19171 秒", "severity": "critical"}], "duration_ms": 18}, {"id": 5, "ts": 1783797634, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 20971 秒", "severity": "critical"}], "duration_ms": 18}, {"id": 6, "ts": 1783799434, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 22771 秒", "severity": "critical"}], "duration_ms": 16}, {"id": 7, "ts": 1783801234, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 24570 秒", "severity": "critical"}], "duration_ms": 22}, {"id": 8, "ts": 1783803033, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 26370 秒", "severity": "critical"}], "duration_ms": 19}, {"id": 9, "ts": 1783804833, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 28170 秒", "severity": "critical"}], "duration_ms": 18}, {"id": 10, "ts": 1783806633, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 29970 秒", "severity": "critical"}], "duration_ms": 17}, {"id": 11, "ts": 1783808433, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 31770 秒", "severity": "critical"}], "duration_ms": 18}, {"id": 12, "ts": 1783810233, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 33570 秒", "severity": "critical"}], "duration_ms": 18}, {"id": 13, "ts": 1783812033, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 35370 秒", "severity": "critical"}], "duration_ms": 19}, {"id": 14, "ts": 1783813833, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 37170 秒", "severity": "critical"}], "duration_ms": 18}, {"id": 15, "ts": 1783815633, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 38970 秒", "severity": "critical"}], "duration_ms": 17}, {"id": 16, "ts": 1783817433, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 40770 秒", "severity": "critical"}], "duration_ms": 19}, {"id": 17, "ts": 1783827282, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 50609 秒", "severity": "critical"}], "duration_ms": 1}, {"id": 18, "ts": 1783829082, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 52419 秒", "severity": "critical"}]}, {"id": 19, "ts": 1783831445, "model": "启发式规则", "source": "heuristic", "context": "巡检范围：在线主机 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 0 项 · 近 30 分钟 error 0/warn 0 条 · 资源高位 0 项。", "summary": "在线 0 台 · 离线 1 台 · firing 告警 1 条 · SLO 超标 0 项 · 近 30 分钟 error 0/warn 0 条。", "trigger": "scheduled", "findings": [{"title": "主机离线：Eason", "detail": "该主机已失联，请检查网络连通与 Agent 进程。", "severity": "critical"}, {"title": "Eason · 主机 Eason（192.168.60.11）已失联 54772 秒", "severity": "critical"}], "duration_ms": 2}]
\.


--
-- Data for Name: tickets; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.tickets (id, status, created_at, data) FROM stdin;
2	open	1783755261	{"id": 2, "title": "��Ϣ���Ĺ�������", "status": "open", "priority": "p2", "reporter": "admin", "created_at": 1783755261, "updated_at": 1783755261}
\.


--
-- Name: audit_log_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.audit_log_id_seq', 161, true);


--
-- Name: diagnosis_embeddings_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.diagnosis_embeddings_id_seq', 1, false);


--
-- Name: events_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.events_id_seq', 1, false);


--
-- Name: experience_rules_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.experience_rules_id_seq', 1, false);


--
-- Name: hermes_rules_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.hermes_rules_id_seq', 1, false);


--
-- Name: hermes_sessions_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.hermes_sessions_id_seq', 1, false);


--
-- Name: hermes_templates_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.hermes_templates_id_seq', 1, false);


--
-- Name: app_config app_config_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_config
    ADD CONSTRAINT app_config_pkey PRIMARY KEY (id);


--
-- Name: audit_log audit_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_log
    ADD CONSTRAINT audit_log_pkey PRIMARY KEY (id);


--
-- Name: diagnosis_embeddings diagnosis_embeddings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.diagnosis_embeddings
    ADD CONSTRAINT diagnosis_embeddings_pkey PRIMARY KEY (id);


--
-- Name: events events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_pkey PRIMARY KEY (id);


--
-- Name: experience_rules experience_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.experience_rules
    ADD CONSTRAINT experience_rules_pkey PRIMARY KEY (id);


--
-- Name: hermes_rules hermes_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hermes_rules
    ADD CONSTRAINT hermes_rules_pkey PRIMARY KEY (id);


--
-- Name: hermes_sessions hermes_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hermes_sessions
    ADD CONSTRAINT hermes_sessions_pkey PRIMARY KEY (id);


--
-- Name: hermes_templates hermes_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hermes_templates
    ADD CONSTRAINT hermes_templates_pkey PRIMARY KEY (id);


--
-- Name: hosts hosts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hosts
    ADD CONSTRAINT hosts_pkey PRIMARY KEY (id);


--
-- Name: incidents incidents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.incidents
    ADD CONSTRAINT incidents_pkey PRIMARY KEY (id);


--
-- Name: kv_state kv_state_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kv_state
    ADD CONSTRAINT kv_state_pkey PRIMARY KEY (k);


--
-- Name: tickets tickets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_pkey PRIMARY KEY (id);


--
-- Name: audit_log_ts; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX audit_log_ts ON public.audit_log USING btree (ts);


--
-- Name: diag_emb_incident; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX diag_emb_incident ON public.diagnosis_embeddings USING btree (incident_id);


--
-- Name: events_ts; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX events_ts ON public.events USING btree (ts);


--
-- Name: hermes_rules_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX hermes_rules_enabled ON public.hermes_rules USING btree (enabled);


--
-- Name: hermes_templates_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX hermes_templates_active ON public.hermes_templates USING btree (active);


--
-- Name: incidents_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX incidents_status ON public.incidents USING btree (status);


--
-- Name: tickets_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tickets_status ON public.tickets USING btree (status);


--
-- PostgreSQL database dump complete
--

\unrestrict uhefo73TmStSKXNIti6gImwnLkDeKTuU76Xd8Jv8zzicomg3qVc15rswgsD9qdN

