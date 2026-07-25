package main

import (
	"fmt"
	"strings"
	"time"
)

// K8sClusterConfig is a server-side Kubernetes API endpoint registration.
// Auth: either APIServer+Token(+optional CA), or KubeconfigYAML (current-context).
type K8sClusterConfig struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	APIServer      string `json:"api_server,omitempty"`
	Token          string `json:"token,omitempty"`             // masked / encrypted
	CACert         string `json:"ca_cert,omitempty"`           // PEM
	KubeconfigYAML string `json:"kubeconfig_yaml,omitempty"`   // encrypted
	DefaultNS      string `json:"default_namespace,omitempty"` // empty = all namespaces
	Insecure       bool   `json:"insecure_skip_tls,omitempty"`
	CreatedAt      int64  `json:"created_at,omitempty"`
}

func maskK8sCluster(c K8sClusterConfig) K8sClusterConfig {
	if c.Token != "" {
		c.Token = "****"
	}
	if c.KubeconfigYAML != "" {
		c.KubeconfigYAML = "****"
	}
	return c
}

func (cs *ConfigStore) ListK8sClusters() []K8sClusterConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]K8sClusterConfig, 0, len(cs.cfg.K8sClusters))
	for _, c := range cs.cfg.K8sClusters {
		out = append(out, c)
	}
	return out
}

func (cs *ConfigStore) GetK8sCluster(id string) (K8sClusterConfig, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, c := range cs.cfg.K8sClusters {
		if c.ID == id {
			return c, true
		}
	}
	return K8sClusterConfig{}, false
}

func (cs *ConfigStore) UpsertK8sCluster(in K8sClusterConfig) (K8sClusterConfig, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return K8sClusterConfig{}, fmt.Errorf("cluster name required")
	}
	keepSecret := func(v, prev string) string {
		if v == "" || strings.Contains(v, "****") {
			return prev
		}
		return v
	}
	cs.mu.Lock()
	if in.ID == "" {
		in.ID = termID()[:8]
		in.CreatedAt = time.Now().Unix()
		cs.cfg.K8sClusters = append(cs.cfg.K8sClusters, in)
		cs.mu.Unlock()
		return in, cs.save()
	}
	for i, c := range cs.cfg.K8sClusters {
		if c.ID == in.ID {
			in.CreatedAt = c.CreatedAt
			in.Token = keepSecret(in.Token, c.Token)
			in.KubeconfigYAML = keepSecret(in.KubeconfigYAML, c.KubeconfigYAML)
			cs.cfg.K8sClusters[i] = in
			cs.mu.Unlock()
			return in, cs.save()
		}
	}
	if in.CreatedAt == 0 {
		in.CreatedAt = time.Now().Unix()
	}
	cs.cfg.K8sClusters = append(cs.cfg.K8sClusters, in)
	cs.mu.Unlock()
	return in, cs.save()
}

func (cs *ConfigStore) DeleteK8sCluster(id string) error {
	cs.mu.Lock()
	kept := make([]K8sClusterConfig, 0, len(cs.cfg.K8sClusters))
	found := false
	for _, c := range cs.cfg.K8sClusters {
		if c.ID == id {
			found = true
			continue
		}
		kept = append(kept, c)
	}
	if !found {
		cs.mu.Unlock()
		return fmt.Errorf("cluster not found")
	}
	cs.cfg.K8sClusters = kept
	cs.mu.Unlock()
	return cs.save()
}
