package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// speechAudioRoot derives OpenAI-compatible /v1 root for audio APIs.
// Accepts: full chat endpoint, /v1 root, or host-only URL.
func speechAudioRoot(cfg AIConfig) string {
	ep := strings.TrimSpace(cfg.SpeechEndpoint)
	if ep == "" {
		ep = strings.TrimSpace(cfg.Endpoint)
	}
	ep = strings.TrimRight(ep, "/")
	if ep == "" {
		return ""
	}
	for _, suf := range []string{"/chat/completions", "/messages", "/embeddings", "/audio/speech", "/audio/transcriptions"} {
		if i := strings.LastIndex(ep, suf); i > 0 {
			ep = ep[:i]
			break
		}
	}
	ep = strings.TrimRight(ep, "/")
	if !strings.HasSuffix(ep, "/v1") && !strings.Contains(ep, "/compatible-mode/v1") {
		// Bailian compatible-mode already includes /v1; plain hosts get /v1.
		if strings.Contains(ep, "compatible-mode") {
			if !strings.HasSuffix(ep, "/v1") {
				ep += "/v1"
			}
		} else if !strings.Contains(ep, "/v1/") && !strings.HasSuffix(ep, "/v1") {
			ep += "/v1"
		}
	}
	return ep
}

func speechAPIKey(cfg AIConfig) string {
	key := strings.TrimSpace(cfg.SpeechAPIKey)
	if key == "" {
		key = strings.TrimSpace(cfg.APIKey)
	}
	return key
}

func speechReady(cfg AIConfig) (stt, tts bool) {
	if !cfg.Enabled {
		return false, false
	}
	root := speechAudioRoot(cfg)
	key := speechAPIKey(cfg)
	if root == "" || key == "" {
		return false, false
	}
	stt = strings.TrimSpace(cfg.SpeechSTTModel) != ""
	tts = strings.TrimSpace(cfg.SpeechTTSModel) != ""
	return stt, tts
}

func (s *Server) handleAISpeechStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.AIConfig()
	stt, tts := speechReady(cfg)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                 true,
		"prefer_cloud":       cfg.SpeechPreferCloud,
		"stt_ready":          stt,
		"tts_ready":          tts,
		"stt_model":          cfg.SpeechSTTModel,
		"tts_model":          cfg.SpeechTTSModel,
		"tts_voice":          cfg.SpeechTTSVoice,
		"endpoint_configured": speechAudioRoot(cfg) != "",
	})
}

// handleAISpeechSTT proxies multipart audio to OpenAI-compatible /audio/transcriptions.
func (s *Server) handleAISpeechSTT(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.AIConfig()
	sttOK, _ := speechReady(cfg)
	if !sttOK {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "未配置语音识别模型（AI 设置 → 语音）"})
		return
	}
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "音频过大或格式无效"})
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		file, hdr, err = r.FormFile("audio")
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少音频文件字段 file/audio"})
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 10<<20))
	if err != nil || len(raw) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "读取音频失败"})
		return
	}
	filename := "audio.webm"
	if hdr != nil && strings.TrimSpace(hdr.Filename) != "" {
		filename = hdr.Filename
	}
	root := speechAudioRoot(cfg)
	key := speechAPIKey(cfg)
	model := strings.TrimSpace(cfg.SpeechSTTModel)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("model", model)
	_ = mw.WriteField("language", "zh")
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "构造请求失败"})
		return
	}
	if _, err := fw.Write(raw); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "构造请求失败"})
		return
	}
	_ = mw.Close()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, root+"/audio/transcriptions", &body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "语音识别上游不可达：" + err.Error()})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": providerHTTPErrorMsg(resp.StatusCode, string(respBody), cfg),
		})
		return
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil || strings.TrimSpace(out.Text) == "" {
		// Some gateways return plain text
		text := strings.TrimSpace(string(respBody))
		if text == "" {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "上游未返回识别文本"})
			return
		}
		out.Text = text
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"text":       out.Text,
		"latency_ms": time.Since(start).Milliseconds(),
		"model":      model,
	})
}

// handleAISpeechTTS proxies text to OpenAI-compatible /audio/speech and returns audio bytes.
func (s *Server) handleAISpeechTTS(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.AIConfig()
	_, ttsOK := speechReady(cfg)
	if !ttsOK {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "未配置语音播报模型（AI 设置 → 语音）"})
		return
	}
	var reqBody struct {
		Text  string `json:"text"`
		Voice string `json:"voice"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&reqBody); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效 JSON"})
		return
	}
	text := strings.TrimSpace(reqBody.Text)
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text 必填"})
		return
	}
	if len(text) > 4000 {
		text = text[:4000]
	}
	voice := strings.TrimSpace(reqBody.Voice)
	if voice == "" {
		voice = strings.TrimSpace(cfg.SpeechTTSVoice)
	}
	if voice == "" {
		voice = "alloy"
	}
	root := speechAudioRoot(cfg)
	key := speechAPIKey(cfg)
	model := strings.TrimSpace(cfg.SpeechTTSModel)
	payload, _ := json.Marshal(map[string]any{
		"model":           model,
		"input":           text,
		"voice":           voice,
		"response_format": "mp3",
	})
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, root+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "语音合成上游不可达：" + err.Error()})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": providerHTTPErrorMsg(resp.StatusCode, string(respBody), cfg),
		})
		return
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" || strings.Contains(ct, "json") {
		ct = "audio/mpeg"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Speech-Model", model)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBody)
}
