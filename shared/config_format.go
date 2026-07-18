package shared

// 配置文件格式：同时支持 JSON 与 YAML/YML，按文件扩展名自动识别。YAML 经「解析成
// map → json.Marshal → json.Unmarshal」中转，从而复用现有结构体的 `json:` tag，无需给
// 每个结构再加一套 `yaml:` tag，也无需改动任何已有配置结构。

import (
	"encoding/json"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// IsYAMLPath 判断路径是否为 YAML/YML（大小写不敏感）。
func IsYAMLPath(p string) bool {
	lp := strings.ToLower(p)
	return strings.HasSuffix(lp, ".yaml") || strings.HasSuffix(lp, ".yml")
}

// DecodeConfig 按扩展名把 data 解码进 v：.yaml/.yml 走 YAML，其余走 JSON。
// YAML 分支中转 JSON 以复用 `json:` tag（yaml.v3 的 mapping 解成 map[string]any，
// json.Marshal 可直接序列化）。
func DecodeConfig(path string, data []byte, v any) error {
	if IsYAMLPath(path) {
		var m any
		if err := yaml.Unmarshal(data, &m); err != nil {
			return err
		}
		jb, err := json.Marshal(m)
		if err != nil {
			return err
		}
		return json.Unmarshal(jb, v)
	}
	return json.Unmarshal(data, v)
}

// ResolveConfigPath 在候选路径中返回第一个存在的文件；都不存在时返回第一个候选
// （供调用方作默认路径用于错误提示）。用于自动探测 config.json / config.yaml / config.yml。
func ResolveConfigPath(candidates ...string) string {
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}
