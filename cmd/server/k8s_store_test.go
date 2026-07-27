package main

import (
	"path/filepath"
	"testing"
)

func TestUpsertK8sClusterKeepsSecrets(t *testing.T) {
	cs, err := NewConfigStore(filepath.Join(t.TempDir(), "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := cs.UpsertK8sCluster(K8sClusterConfig{
		Name: "prod", Enabled: true,
		APIServer: "https://192.168.10.81:6443",
		Token:     "real-token-value",
		CACert:    "-----BEGIN CERTIFICATE-----\nABC\n-----END CERTIFICATE-----",
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" {
		t.Fatal("expected id")
	}
	updated, err := cs.UpsertK8sCluster(K8sClusterConfig{
		ID: saved.ID, Name: "prod-renamed", Enabled: true,
		APIServer: "https://192.168.10.81:6443",
		Token:     "****",
		CACert:    "-----BEGIN CERTIFICATE-----\nABC\n-----END CERTIFICATE-----",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := cs.GetK8sCluster(updated.ID)
	if !ok {
		t.Fatal("missing")
	}
	if got.Token != "real-token-value" {
		t.Fatalf("token not preserved: %q", got.Token)
	}
	if got.Name != "prod-renamed" {
		t.Fatalf("name=%q", got.Name)
	}
	masked := maskK8sCluster(got)
	if !masked.HasToken || masked.Token != "****" {
		t.Fatalf("mask=%+v", masked)
	}
}

func TestUpsertK8sClusterKubeconfigOnly(t *testing.T) {
	cs, err := NewConfigStore(filepath.Join(t.TempDir(), "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := cs.UpsertK8sCluster(K8sClusterConfig{
		Name: "kc", Enabled: true,
		KubeconfigYAML: "apiVersion: v1\nkind: Config\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = cs.UpsertK8sCluster(K8sClusterConfig{
		ID: saved.ID, Name: "kc", Enabled: true,
		KubeconfigYAML: "****",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := cs.GetK8sCluster(saved.ID)
	if got.KubeconfigYAML == "" || got.KubeconfigYAML == "****" {
		t.Fatalf("kubeconfig lost: %q", got.KubeconfigYAML)
	}
}

func TestUpsertK8sClusterRejectsEmptyAuth(t *testing.T) {
	cs, err := NewConfigStore(filepath.Join(t.TempDir(), "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cs.UpsertK8sCluster(K8sClusterConfig{Name: "x", Enabled: true}); err == nil {
		t.Fatal("expected auth validation error")
	}
}
