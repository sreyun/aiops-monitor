package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Desktop session recording (keyframes) for replay — similar to terminal archive.

type deskRecordFrame struct {
	Ts   int64  `json:"ts"`
	Type string `json:"type"` // meta|jpeg|h264
	Data string `json:"data"` // base64
}

type deskSessionInfo struct {
	ID        string `json:"id"`
	HostID    string `json:"host_id"`
	Hostname  string `json:"hostname"`
	Operator  string `json:"operator"`
	IP        string `json:"ip"`
	CreatedAt int64  `json:"created_at"`
	Frames    int    `json:"frames"`
	Active    bool   `json:"active"`
}

type deskArchive struct {
	Info      deskSessionInfo   `json:"info"`
	Recording []deskRecordFrame `json:"recording"`
}

const deskArchiveCap = 50
const deskRecFrameCap = 3000

func (s *deskSession) recordFrame(typ string, data []byte) {
	s.recMu.Lock()
	defer s.recMu.Unlock()
	if len(s.recording) >= deskRecFrameCap {
		// drop oldest 10%
		s.recording = s.recording[len(s.recording)/10:]
	}
	// Cap single frame size for JPEG/H264 chunk (store up to 512KB)
	if len(data) > 512<<10 {
		data = data[:512<<10]
	}
	s.recording = append(s.recording, deskRecordFrame{
		Ts:   time.Now().UnixMilli(),
		Type: typ,
		Data: base64.StdEncoding.EncodeToString(data),
	})
}

func (m *deskManager) setRecDir(dir string) {
	m.mu.Lock()
	m.recDir = dir
	m.mu.Unlock()
}

func (m *deskManager) persistArchive(a deskArchive) {
	if m.recDir == "" || a.Info.ID == "" || len(a.Recording) == 0 {
		return
	}
	_ = os.MkdirAll(m.recDir, 0o750)
	b, err := json.Marshal(a)
	if err != nil {
		return
	}
	tmp := filepath.Join(m.recDir, a.Info.ID+".json.tmp")
	path := filepath.Join(m.recDir, a.Info.ID+".json")
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, path)
	}
}

func (m *deskManager) archiveSession(s *deskSession) {
	s.recMu.Lock()
	rec := make([]deskRecordFrame, len(s.recording))
	copy(rec, s.recording)
	s.recMu.Unlock()
	if len(rec) == 0 {
		return
	}
	arch := deskArchive{
		Info: deskSessionInfo{
			ID: s.id, HostID: s.hostID, Hostname: s.hostname,
			Operator: s.operator, IP: s.ip, CreatedAt: s.createdAt,
			Frames: len(rec), Active: false,
		},
		Recording: rec,
	}
	m.mu.Lock()
	m.archived = append(m.archived, arch)
	if len(m.archived) > deskArchiveCap {
		m.archived = m.archived[len(m.archived)-deskArchiveCap:]
	}
	m.mu.Unlock()
	go m.persistArchive(arch)
}

func (m *deskManager) listSessions() []deskSessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]deskSessionInfo, 0, len(m.sessions)+len(m.archived))
	for _, s := range m.sessions {
		s.recMu.Lock()
		n := len(s.recording)
		s.recMu.Unlock()
		out = append(out, deskSessionInfo{
			ID: s.id, HostID: s.hostID, Hostname: s.hostname,
			Operator: s.operator, IP: s.ip, CreatedAt: s.createdAt,
			Frames: n, Active: true,
		})
	}
	for i := len(m.archived) - 1; i >= 0; i-- {
		out = append(out, m.archived[i].Info)
	}
	return out
}

func (m *deskManager) getRecording(id string) []deskRecordFrame {
	m.mu.Lock()
	if s := m.sessions[id]; s != nil {
		s.recMu.Lock()
		rec := make([]deskRecordFrame, len(s.recording))
		copy(rec, s.recording)
		s.recMu.Unlock()
		m.mu.Unlock()
		return rec
	}
	for _, a := range m.archived {
		if a.Info.ID == id {
			m.mu.Unlock()
			return a.Recording
		}
	}
	dir := m.recDir
	m.mu.Unlock()
	if dir == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return nil
	}
	var a deskArchive
	if json.Unmarshal(b, &a) != nil {
		return nil
	}
	return a.Recording
}

func (s *Server) handleListDesktopSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.desk.listSessions())
}

func (s *Server) handleDesktopReplay(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	frames := s.desk.getRecording(id)
	if frames == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "common.session_gone")})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "frames": frames})
}
