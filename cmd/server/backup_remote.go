package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// BackupRemoteConfig uploads dumps to S3-compatible object storage (AWS S3 / Aliyun OSS / MinIO).
type BackupRemoteConfig struct {
	Enabled   bool   `json:"enabled"`
	Endpoint  string `json:"endpoint,omitempty"` // e.g. https://s3.amazonaws.com or https://oss-cn-hangzhou.aliyuncs.com
	Bucket    string `json:"bucket,omitempty"`
	Region    string `json:"region,omitempty"`
	AccessKey string `json:"access_key,omitempty"`
	SecretKey string `json:"secret_key,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	Provider  string `json:"provider,omitempty"` // s3 | oss (both use SigV4 path-style/virtual-host)
}

func (s *Server) uploadBackupRemote(localPath, objectName string) error {
	cfg := s.cfg.BackupCfg().Remote
	if !cfg.Enabled {
		return nil
	}
	cfg.Endpoint = strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	cfg.AccessKey = strings.TrimSpace(cfg.AccessKey)
	cfg.SecretKey = strings.TrimSpace(cfg.SecretKey)
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return fmt.Errorf("remote backup incomplete: endpoint/bucket/access_key/secret_key required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	body, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	key := strings.Trim(cfg.Prefix, "/")
	if key != "" {
		key += "/"
	}
	key += objectName
	return s3PutObject(cfg, key, body)
}

func s3PutObject(cfg BackupRemoteConfig, key string, body []byte) error {
	host, pathStyle := s3HostAndPath(cfg)
	u := &url.URL{Scheme: "https", Host: host, Path: "/" + key}
	if strings.HasPrefix(cfg.Endpoint, "http://") {
		u.Scheme = "http"
	}
	if pathStyle {
		u.Path = "/" + cfg.Bucket + "/" + key
	}
	req, err := http.NewRequest(http.MethodPut, u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	if err := signAWSv4(req, body, cfg.AccessKey, cfg.SecretKey, cfg.Region, "s3"); err != nil {
		return err
	}
	cli := &http.Client{Timeout: 10 * time.Minute}
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("remote upload HTTP %d: %s", resp.StatusCode, truncateRunes(string(b), 200))
	}
	slog.Info("backup uploaded to remote object storage", "bucket", cfg.Bucket, "key", key, "bytes", len(body))
	return nil
}

func s3HostAndPath(cfg BackupRemoteConfig) (host string, pathStyle bool) {
	ep := strings.TrimPrefix(strings.TrimPrefix(cfg.Endpoint, "https://"), "http://")
	ep = strings.TrimRight(ep, "/")
	// Virtual-hosted: bucket.s3.region.amazonaws.com — use path-style for custom endpoints (MinIO/OSS).
	if strings.Contains(ep, "amazonaws.com") && !strings.Contains(ep, "s3.") {
		return cfg.Bucket + "." + ep, false
	}
	if strings.HasPrefix(ep, "s3.") || strings.Contains(ep, ".s3.") {
		return cfg.Bucket + "." + ep, false
	}
	return ep, true
}

func signAWSv4(req *http.Request, payload []byte, accessKey, secretKey, region, service string) error {
	t := time.Now().UTC()
	amzDate := t.Format("20060102T150405Z")
	dateStamp := t.Format("20060102")
	payloadHash := sha256Hex(string(payload))
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if req.Header.Get("Host") == "" {
		req.Header.Set("Host", req.URL.Host)
	}
	canonicalHeaders := "host:" + req.URL.Host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalRequest := strings.Join([]string{
		req.Method, canonicalURI, "", canonicalHeaders, signedHeaders, payloadHash,
	}, "\n")
	credentialScope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", amzDate, credentialScope, sha256Hex(canonicalRequest),
	}, "\n")
	signingKey := awsS3SigningKey(secretKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256Bytes(signingKey, stringToSign))
	req.Header.Set("Authorization",
		"AWS4-HMAC-SHA256 Credential="+accessKey+"/"+credentialScope+
			", SignedHeaders="+signedHeaders+", Signature="+signature)
	return nil
}

func awsS3SigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256Bytes([]byte("AWS4"+secret), dateStamp)
	kRegion := hmacSHA256Bytes(kDate, region)
	kService := hmacSHA256Bytes(kRegion, service)
	return hmacSHA256Bytes(kService, "aws4_request")
}
