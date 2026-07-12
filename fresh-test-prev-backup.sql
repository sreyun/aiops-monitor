--
-- PostgreSQL database dump
--

\restrict 1Tc1Em2YoD0YH2MG0A6rZ59oS6hTZkUMZ4M6am5bJDgY1ORUyseeUhCCecWGkM4

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
1	{"ai": {"model": "", "enabled": false, "endpoint": "", "inspect_interval_min": 0}, "vm": {"url": "http://victoriametrics:8428", "enabled": true}, "smtp": {"smtp_host": "", "smtp_port": 0, "smtp_enabled": false, "smtp_use_tls": false, "smtp_username": "", "smtp_from_name": "AIOps Monitor"}, "users": [{"hash": "pbkdf2$sha256$600000$d98dd0e64a978ff00f903efca9454e43fb0cdf31ce87fcce7e549a59c4f8b061", "role": "admin", "salt": "3c4ffca8ac2f359a", "email": "", "username": "admin", "mfa_enabled": false, "display_name": "管理员"}], "checks": null, "feishu": {"enabled": false, "webhook": ""}, "account": {"hash": "", "salt": "", "email": "", "username": "", "mfa_enabled": false, "display_name": ""}, "dingtalk": {"enabled": false, "webhook": ""}, "categories": {}, "thresholds": {"cpu_crit": 95, "cpu_warn": 80, "gpu_crit": 95, "gpu_warn": 80, "mem_crit": 95, "mem_warn": 85, "disk_crit": 90, "disk_warn": 80, "iops_crit": 100000, "iops_warn": 50000, "load_crit": 8, "load_warn": 4, "proc_warn": 0.5, "diskio_crit": 95, "diskio_warn": 80, "offline_after_sec": 60}, "trust_proxy": false, "mfa_required": false, "postgres_dsn": "postgres://aiops:h3Y7Vmb1CZBOApZM86D@postgres:5432/aiops?sslmode=disable", "install_token": "0049789b6f816ef0cf3b310c54ffe43b", "require_token": false, "alerts_enabled": true, "custom_webhook": {"url": "", "method": "", "enabled": false, "headers": "", "content_type": "", "body_template": ""}, "forward_listen": "0.0.0.0", "forward_disabled": false, "terminal_disabled": false, "allow_anonymous_agents": false}
\.


--
-- Data for Name: audit_log; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.audit_log (id, ts, data) FROM stdin;
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
\.


--
-- Data for Name: incidents; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.incidents (id, status, created_at, data) FROM stdin;
\.


--
-- Data for Name: kv_state; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.kv_state (k, data) FROM stdin;
alert_states	{}
sessions	{}
messages	[]
ai_inspections	[]
remediation_runs	[]
slo_burning	{}
logs	[]
\.


--
-- Data for Name: tickets; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.tickets (id, status, created_at, data) FROM stdin;
\.


--
-- Name: audit_log_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.audit_log_id_seq', 1, false);


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

\unrestrict 1Tc1Em2YoD0YH2MG0A6rZ59oS6hTZkUMZ4M6am5bJDgY1ORUyseeUhCCecWGkM4

