package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

func speechResolveVoice(cfg AIConfig, override string) string {
	voice := strings.TrimSpace(override)
	if voice == "" {
		voice = strings.TrimSpace(cfg.SpeechTTSVoice)
	}
	if voice == "" {
		voice = "alloy"
	}
	return voice
}

// synthesizeSpeechTTS calls OpenAI-compatible /audio/speech and returns audio bytes + content-type.
func synthesizeSpeechTTS(ctx context.Context, cfg AIConfig, text, voice string) (audio []byte, contentType string, err error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, "", fmt.Errorf("text 必填")
	}
	if len(text) > 4000 {
		text = text[:4000]
	}
	root := speechAudioRoot(cfg)
	key := speechAPIKey(cfg)
	model := strings.TrimSpace(cfg.SpeechTTSModel)
	if root == "" || key == "" || model == "" {
		return nil, "", fmt.Errorf("未配置语音播报 Endpoint / Key / 模型")
	}
	voice = speechResolveVoice(cfg, voice)
	payload, _ := json.Marshal(map[string]any{
		"model":           model,
		"input":           text,
		"voice":           voice,
		"response_format": "mp3",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, root+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("语音合成上游不可达：%s", err.Error())
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("%s", providerHTTPErrorMsg(resp.StatusCode, string(respBody), cfg))
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" || strings.Contains(ct, "json") {
		ct = "audio/mpeg"
	}
	if len(respBody) == 0 {
		return nil, "", fmt.Errorf("上游未返回音频数据")
	}
	return respBody, ct, nil
}

// transcribeSpeechSTT calls OpenAI-compatible /audio/transcriptions.
func transcribeSpeechSTT(ctx context.Context, cfg AIConfig, raw []byte, filename string) (text string, err error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("音频为空")
	}
	root := speechAudioRoot(cfg)
	key := speechAPIKey(cfg)
	model := strings.TrimSpace(cfg.SpeechSTTModel)
	if root == "" || key == "" || model == "" {
		return "", fmt.Errorf("未配置语音识别 Endpoint / Key / 模型")
	}
	if strings.TrimSpace(filename) == "" {
		filename = "audio.mp3"
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("model", model)
	_ = mw.WriteField("language", "zh")
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("构造请求失败")
	}
	if _, err := fw.Write(raw); err != nil {
		return "", fmt.Errorf("构造请求失败")
	}
	_ = mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, root+"/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("语音识别上游不可达：%s", err.Error())
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s", providerHTTPErrorMsg(resp.StatusCode, string(respBody), cfg))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil || strings.TrimSpace(out.Text) == "" {
		plain := strings.TrimSpace(string(respBody))
		if plain == "" {
			return "", fmt.Errorf("上游未返回识别文本")
		}
		return plain, nil
	}
	return strings.TrimSpace(out.Text), nil
}

func mergeSpeechTestConfig(draft, saved AIConfig) AIConfig {
	c := draft
	c.Enabled = true
	if c.SpeechAPIKey == "" || strings.Contains(c.SpeechAPIKey, "****") {
		c.SpeechAPIKey = saved.SpeechAPIKey
	}
	if c.APIKey == "" || strings.Contains(c.APIKey, "****") {
		c.APIKey = saved.APIKey
	}
	if strings.TrimSpace(c.SpeechEndpoint) == "" {
		c.SpeechEndpoint = saved.SpeechEndpoint
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		c.Endpoint = saved.Endpoint
	}
	// Form values win when present; otherwise fall back to saved models/voice.
	if strings.TrimSpace(c.SpeechSTTModel) == "" {
		c.SpeechSTTModel = saved.SpeechSTTModel
	}
	if strings.TrimSpace(c.SpeechTTSModel) == "" {
		c.SpeechTTSModel = saved.SpeechTTSModel
	}
	if strings.TrimSpace(c.SpeechTTSVoice) == "" {
		c.SpeechTTSVoice = saved.SpeechTTSVoice
	}
	return c
}

func (s *Server) handleAISpeechStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.AIConfig()
	stt, tts := speechReady(cfg)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                  true,
		"prefer_cloud":        cfg.SpeechPreferCloud,
		"stt_ready":           stt,
		"tts_ready":           tts,
		"stt_model":           cfg.SpeechSTTModel,
		"tts_model":           cfg.SpeechTTSModel,
		"tts_voice":           cfg.SpeechTTSVoice,
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
	start := time.Now()
	text, err := transcribeSpeechSTT(r.Context(), cfg, raw, filename)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"text":       text,
		"latency_ms": time.Since(start).Milliseconds(),
		"model":      strings.TrimSpace(cfg.SpeechSTTModel),
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
	audio, ct, err := synthesizeSpeechTTS(r.Context(), cfg, reqBody.Text, reqBody.Voice)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	model := strings.TrimSpace(cfg.SpeechTTSModel)
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Speech-Model", model)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio)
}

// handleTestSpeechConfig verifies draft speech settings before save.
// Flow: TTS synthesize sample → return base64 audio for browser playback;
// if STT model is set, round-trip the same audio through transcription.
// POST /api/v1/ai/test-speech
func (s *Server) handleTestSpeechConfig(w http.ResponseWriter, r *http.Request) {
	var draft AIConfig
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&draft); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	cfg := mergeSpeechTestConfig(draft, s.cfg.AIConfig())
	_, ttsOK := speechReady(cfg)
	if !ttsOK {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": "请先填写 TTS 播报模型，并确保语音 Endpoint / API Key 可用（可留空复用上方 Provider）",
		})
		return
	}

	sample := "语音配置测试成功，这是一段播报样例。"
	voice := speechResolveVoice(cfg, "")
	ttsModel := strings.TrimSpace(cfg.SpeechTTSModel)

	start := time.Now()
	ttsStart := time.Now()
	audio, ct, err := synthesizeSpeechTTS(r.Context(), cfg, sample, voice)
	ttsLatency := time.Since(ttsStart).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":             false,
			"tts_ok":         false,
			"error":          "TTS 失败：" + err.Error(),
			"tts_latency_ms": ttsLatency,
			"latency_ms":     time.Since(start).Milliseconds(),
			"model":          ttsModel,
			"voice":          voice,
		})
		return
	}

	out := map[string]any{
		"ok":             true,
		"tts_ok":         true,
		"tts_latency_ms": ttsLatency,
		"latency_ms":     time.Since(start).Milliseconds(),
		"model":          ttsModel,
		"voice":          voice,
		"text":           sample,
		"audio_base64":   base64.StdEncoding.EncodeToString(audio),
		"content_type":   ct,
		"audio_bytes":    len(audio),
	}

	sttOK, _ := speechReady(cfg)
	if sttOK {
		sttModel := strings.TrimSpace(cfg.SpeechSTTModel)
		out["stt_model"] = sttModel
		filename := "speech-test.mp3"
		if strings.Contains(ct, "wav") {
			filename = "speech-test.wav"
		} else if strings.Contains(ct, "webm") {
			filename = "speech-test.webm"
		} else if strings.Contains(ct, "ogg") {
			filename = "speech-test.ogg"
		}
		sttStart := time.Now()
		transcript, sttErr := transcribeSpeechSTT(r.Context(), cfg, audio, filename)
		sttLatency := time.Since(sttStart).Milliseconds()
		out["stt_latency_ms"] = sttLatency
		out["latency_ms"] = time.Since(start).Milliseconds()
		if sttErr != nil {
			out["stt_ok"] = false
			out["stt_error"] = sttErr.Error()
			// TTS already succeeded — still ok for playback verification.
		} else {
			out["stt_ok"] = true
			out["transcript"] = transcript
		}
	} else {
		out["stt_ok"] = false
		out["stt_skipped"] = true
	}

	writeJSON(w, http.StatusOK, out)
}
