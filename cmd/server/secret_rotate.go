package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// secretKeyStoreFile persists rotatable config-encryption keys so ops need not
// manually edit AIOPS_SECRET_KEYS_PREV after a rotation.
type secretKeyStoreFile struct {
	Current     secretKeyMaterial   `json:"current"`
	Previous    []secretKeyMaterial `json:"previous,omitempty"`
	RotatedAt   int64               `json:"rotated_at,omitempty"`
	IntervalDay int                 `json:"interval_days,omitempty"` // 0 = disabled auto
}

type secretKeyMaterial struct {
	ID         string `json:"id"`
	Passphrase string `json:"passphrase"`
}

var (
	secretStoreMu   sync.Mutex
	secretStoreOnce sync.Once
)

func secretKeyStorePath() string {
	if p := strings.TrimSpace(os.Getenv("AIOPS_SECRET_KEY_STORE")); p != "" {
		return p
	}
	if d := strings.TrimSpace(os.Getenv("AIOPS_DATA_DIR")); d != "" {
		return filepath.Join(d, "secret_keys.json")
	}
	return filepath.Join(".", "secret_keys.json")
}

func loadSecretKeyStore() (secretKeyStoreFile, error) {
	var st secretKeyStoreFile
	b, err := os.ReadFile(secretKeyStorePath())
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, err
	}
	return st, nil
}

func saveSecretKeyStore(st secretKeyStoreFile) error {
	path := secretKeyStorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil && !os.IsExist(err) {
		// "." has no mkdir need
		_ = err
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// applySecretKeyStoreToEnv injects store keys into process env for loadAllSecretKeys.
func applySecretKeyStoreToEnv(st secretKeyStoreFile) {
	if strings.TrimSpace(st.Current.Passphrase) == "" {
		return
	}
	_ = os.Setenv("AIOPS_SECRET_KEY", st.Current.Passphrase)
	if st.Current.ID != "" {
		_ = os.Setenv("AIOPS_SECRET_KEY_ID", st.Current.ID)
	}
	var parts []string
	for _, p := range st.Previous {
		if p.ID == "" || p.Passphrase == "" {
			continue
		}
		parts = append(parts, p.ID+":"+p.Passphrase)
	}
	if len(parts) > 0 {
		_ = os.Setenv("AIOPS_SECRET_KEYS_PREV", strings.Join(parts, ","))
	}
}

func initSecretKeyStoreFromEnv() {
	secretStoreOnce.Do(func() {
		st, err := loadSecretKeyStore()
		if err == nil && st.Current.Passphrase != "" {
			applySecretKeyStoreToEnv(st)
			slog.Info("已从密钥库加载配置加密密钥", "primary_id", st.Current.ID, "prev", len(st.Previous))
			return
		}
		// Seed store from env if present so future rotations have a file baseline.
		cur := strings.TrimSpace(os.Getenv("AIOPS_SECRET_KEY"))
		if cur == "" {
			return
		}
		st = secretKeyStoreFile{
			Current:     secretKeyMaterial{ID: currentSecretKeyID(), Passphrase: cur},
			IntervalDay: 90,
			RotatedAt:   time.Now().Unix(),
		}
		if prev := strings.TrimSpace(os.Getenv("AIOPS_SECRET_KEYS_PREV")); prev != "" {
			for _, part := range strings.Split(prev, ",") {
				part = strings.TrimSpace(part)
				idx := strings.IndexByte(part, ':')
				if idx <= 0 {
					continue
				}
				st.Previous = append(st.Previous, secretKeyMaterial{
					ID: strings.TrimSpace(part[:idx]), Passphrase: part[idx+1:],
				})
			}
		}
		if err := saveSecretKeyStore(st); err != nil {
			slog.Warn("初始化密钥库失败", "err", err)
		}
	})
}

func generateSecretPassphrase() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// rotateConfigSecretKey generates a new primary key, keeps previous for decrypt,
// applies env, and re-saves config so ciphertext migrates to enc:v2:<new_kid>:…
func (s *Server) rotateConfigSecretKey(actor string, intervalDays int) (map[string]any, error) {
	secretStoreMu.Lock()
	defer secretStoreMu.Unlock()
	initSecretKeyStoreFromEnv()

	st, err := loadSecretKeyStore()
	if err != nil || st.Current.Passphrase == "" {
		cur := strings.TrimSpace(os.Getenv("AIOPS_SECRET_KEY"))
		if cur == "" {
			return nil, fmt.Errorf("未配置 AIOPS_SECRET_KEY / 密钥库，无法轮换")
		}
		st = secretKeyStoreFile{
			Current: secretKeyMaterial{ID: currentSecretKeyID(), Passphrase: cur},
		}
	}
	if intervalDays > 0 {
		st.IntervalDay = intervalDays
	} else if st.IntervalDay <= 0 {
		st.IntervalDay = 90
	}
	pass, err := generateSecretPassphrase()
	if err != nil {
		return nil, err
	}
	newID := "v" + time.Now().UTC().Format("20060102")
	if st.Current.ID == newID {
		newID = newID + "-" + fmt.Sprintf("%d", time.Now().Unix()%1000)
	}
	if st.Current.Passphrase != "" {
		st.Previous = append([]secretKeyMaterial{st.Current}, st.Previous...)
		if len(st.Previous) > 5 {
			st.Previous = st.Previous[:5]
		}
	}
	st.Current = secretKeyMaterial{ID: newID, Passphrase: pass}
	st.RotatedAt = time.Now().Unix()
	applySecretKeyStoreToEnv(st)
	if err := saveSecretKeyStore(st); err != nil {
		return nil, err
	}
	// Trigger full config rewrite under new primary key.
	if err := s.cfg.save(); err != nil {
		return nil, fmt.Errorf("轮换后重加密配置失败: %w", err)
	}
	slog.Info("配置加密密钥已轮换", "actor", actor, "primary_id", newID, "prev", len(st.Previous))
	return map[string]any{
		"ok": true, "primary_id": newID, "previous_count": len(st.Previous),
		"rotated_at": st.RotatedAt, "interval_days": st.IntervalDay,
		"store": secretKeyStorePath(), "keys": secretKeyStatus(),
	}, nil
}

func (s *Server) handleRotateSecretKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm      string `json:"confirm"`
		IntervalDays int    `json:"interval_days"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.Confirm != "ROTATE" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请在 confirm 字段填写 ROTATE"})
		return
	}
	out, err := s.rotateConfigSecretKey(s.actorName(r), in.IntervalDays)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{
		Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("轮换配置加密密钥 → %v", out["primary_id"]),
	})
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSecretRotateStatus(w http.ResponseWriter, r *http.Request) {
	initSecretKeyStoreFromEnv()
	st, err := loadSecretKeyStore()
	out := secretKeyStatus()
	out["store_path"] = secretKeyStorePath()
	if err == nil {
		out["rotated_at"] = st.RotatedAt
		out["interval_days"] = st.IntervalDay
		out["store_loaded"] = st.Current.Passphrase != ""
	} else {
		out["store_loaded"] = false
	}
	writeJSON(w, http.StatusOK, out)
}

var secretRotateOnce sync.Once

func (s *Server) startSecretRotateScheduler() {
	secretRotateOnce.Do(func() {
		go func() {
			initSecretKeyStoreFromEnv()
			t := time.NewTicker(6 * time.Hour)
			defer t.Stop()
			for range t.C {
				st, err := loadSecretKeyStore()
				if err != nil || st.IntervalDay <= 0 || st.Current.Passphrase == "" {
					continue
				}
				due := st.RotatedAt + int64(st.IntervalDay)*86400
				if time.Now().Unix() < due {
					continue
				}
				if _, err := s.rotateConfigSecretKey("scheduler", st.IntervalDay); err != nil {
					slog.Error("scheduled secret key rotation failed", "err", err)
				}
			}
		}()
	})
}
