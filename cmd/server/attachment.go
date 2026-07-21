package main

import (
	"strings"
	"unicode/utf8"
)

// Attachment 工单 / 事件评论 / 工单创建时的证据附件（图片 base64 或解析后的文本）。
// 与 AI 聊天的 images/files 载荷对齐，便于 Web / App 复用同一套客户端准备逻辑。
type Attachment struct {
	Name string `json:"name"`
	Mime string `json:"mime,omitempty"`
	Kind string `json:"kind"`           // image | file
	Data string `json:"data,omitempty"` // 图片：无 data: 前缀的 base64
	Text string `json:"text,omitempty"` // 文件：解析/原文文本
}

const (
	maxAttachmentsPerComment = 6
	maxAttachImageBytes      = 2 * 1024 * 1024 // base64 字符串长度上限约 2MB
	maxAttachTextRunes       = 100_000
)

// sanitizeAttachments 裁剪数量与体积，丢弃空条目。
func sanitizeAttachments(in []Attachment) []Attachment {
	if len(in) == 0 {
		return nil
	}
	out := make([]Attachment, 0, len(in))
	for _, a := range in {
		if len(out) >= maxAttachmentsPerComment {
			break
		}
		a.Name = strings.TrimSpace(a.Name)
		a.Mime = strings.TrimSpace(a.Mime)
		a.Kind = strings.ToLower(strings.TrimSpace(a.Kind))
		a.Data = strings.TrimSpace(a.Data)
		a.Text = strings.TrimSpace(a.Text)
		if a.Kind != "image" && a.Kind != "file" {
			if a.Data != "" {
				a.Kind = "image"
			} else {
				a.Kind = "file"
			}
		}
		if a.Kind == "image" {
			if a.Data == "" {
				continue
			}
			if len(a.Data) > maxAttachImageBytes {
				a.Data = a.Data[:maxAttachImageBytes]
			}
			if a.Name == "" {
				a.Name = "image"
			}
			a.Text = "" // 图片不存文本
		} else {
			if a.Text == "" && a.Data == "" {
				continue
			}
			// 文件优先存 Text；若只有 Data（未解析）则丢弃过大的二进制以免撑爆快照
			if a.Text == "" {
				continue
			}
			if utf8.RuneCountInString(a.Text) > maxAttachTextRunes {
				r := []rune(a.Text)
				a.Text = string(r[:maxAttachTextRunes]) + "…"
			}
			a.Data = ""
			if a.Name == "" {
				a.Name = "file.txt"
			}
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
