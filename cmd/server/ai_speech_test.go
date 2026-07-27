package main

import (
	"strings"
	"testing"
)

func TestSpeechAudioRoot(t *testing.T) {
	cases := []struct {
		ep   string
		want string
	}{
		{"https://api.openai.com/v1/chat/completions", "https://api.openai.com/v1"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1"},
		{"https://dashscope.aliyuncs.com/compatible-mode/v1", "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{"https://api.openai.com/v1/audio/speech", "https://api.openai.com/v1"},
	}
	for _, c := range cases {
		got := speechAudioRoot(AIConfig{SpeechEndpoint: c.ep})
		if got != c.want {
			t.Fatalf("speechAudioRoot(%q)=%q want %q", c.ep, got, c.want)
		}
	}
	got := speechAudioRoot(AIConfig{Endpoint: "https://api.openai.com/v1/chat/completions"})
	if got != "https://api.openai.com/v1" {
		t.Fatalf("fallback root=%q", got)
	}
}

func TestSpeechReady(t *testing.T) {
	cfg := AIConfig{
		Enabled:        true,
		Endpoint:       "https://api.openai.com/v1/chat/completions",
		APIKey:         "sk-test",
		SpeechSTTModel: "whisper-1",
		SpeechTTSModel: "tts-1",
	}
	stt, tts := speechReady(cfg)
	if !stt || !tts {
		t.Fatalf("expected both ready, got stt=%v tts=%v", stt, tts)
	}
	cfg.SpeechSTTModel = ""
	stt, tts = speechReady(cfg)
	if stt || !tts {
		t.Fatalf("expected only tts, got stt=%v tts=%v", stt, tts)
	}
}

func TestListDashboardsUIActions(t *testing.T) {
	// Smoke: openDashboardAction shape used by list_dashboards.
	act := openDashboardAction("d1", "CPU 看板")
	if act["type"] != "open_dashboard" || act["id"] != "d1" {
		t.Fatalf("bad action: %#v", act)
	}
	label, _ := act["label"].(string)
	if !strings.Contains(label, "CPU") {
		t.Fatalf("label=%q", label)
	}
}
