package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

// ============================================================================
// AI 对话·文件/URL 识别（②）
//
// 前端把上传的文件(base64)或一个 URL 交给 POST /hermes/parse，服务端解析成纯文本
// 注入多轮对话上下文。支持：
//   · md/txt/csv/json/log/xml/yaml… → 直读文本
//   · docx / xlsx                    → 标准库 archive/zip + encoding/xml 提取（零依赖）
//   · pdf                            → 轻量纯 Go 库 ledongthuc/pdf 提取
//   · URL                            → 走 SSRF 守卫客户端抓取 + HTML 正文提取
// ============================================================================

const maxExtractChars = 200000 // 注入上下文的文本上限（约 200KB，防止拖垮模型上下文）

// handleSreyunParse 解析上传文件/URL 为纯文本。
func (s *Server) handleSreyunParse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Mime string `json:"mime"`
		Data string `json:"data"` // base64 编码的文件内容
		URL  string `json:"url"`  // 或：抓取一个 URL
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 40<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效请求"})
		return
	}

	var text, kind string
	var err error

	if strings.TrimSpace(req.URL) != "" {
		kind = "url"
		text, err = fetchURLText(req.URL)
	} else {
		raw, e := base64.StdEncoding.DecodeString(strings.TrimSpace(req.Data))
		if e != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "文件解码失败"})
			return
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(req.Name), "."))
		kind = ext
		switch ext {
		case "docx":
			text, err = extractDocx(raw)
		case "xlsx":
			text, err = extractXlsx(raw)
		case "pdf":
			text, err = extractPDF(raw)
		case "md", "markdown", "txt", "text", "csv", "tsv", "json", "log", "xml",
			"yaml", "yml", "ini", "conf", "cfg", "properties", "sql", "sh", "py", "go", "js", "ts", "html", "htm":
			text = string(raw)
		default:
			if looksBinary(raw) {
				writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "暂不支持的文件类型：." + ext})
				return
			}
			text = string(raw) // 未知但看起来是文本 → 当纯文本
		}
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error(), "kind": kind})
		return
	}

	text = strings.TrimSpace(text)
	truncated := false
	if len(text) > maxExtractChars {
		text = text[:maxExtractChars]
		truncated = true
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": text, "kind": kind, "chars": len(text), "truncated": truncated})
}

// ---- docx（WordprocessingML）----

func extractDocx(b []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", fmt.Errorf("docx 不是有效的 zip：%v", err)
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()
			raw, _ := io.ReadAll(io.LimitReader(rc, 30<<20))
			return docxXMLToText(raw), nil
		}
	}
	return "", fmt.Errorf("docx 缺少 word/document.xml")
}

// docxXMLToText：<w:t> 是文字，</w:p> 段落换行，<w:tab/> 制表，<w:br>/<w:cr> 硬换行。
func docxXMLToText(raw []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var sb strings.Builder
	inText := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "tab":
				sb.WriteByte('\t')
			case "br", "cr":
				sb.WriteByte('\n')
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				sb.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				sb.Write(t)
			}
		}
	}
	return sb.String()
}

// ---- xlsx（SpreadsheetML）----

func extractXlsx(b []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", fmt.Errorf("xlsx 不是有效的 zip：%v", err)
	}
	var shared []string
	for _, f := range zr.File {
		if f.Name == "xl/sharedStrings.xml" {
			rc, e := f.Open()
			if e == nil {
				raw, _ := io.ReadAll(io.LimitReader(rc, 30<<20))
				rc.Close()
				shared = parseSharedStrings(raw)
			}
			break
		}
	}
	var sb strings.Builder
	got := false
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "xl/worksheets/sheet") && strings.HasSuffix(f.Name, ".xml") {
			rc, e := f.Open()
			if e != nil {
				continue
			}
			raw, _ := io.ReadAll(io.LimitReader(rc, 30<<20))
			rc.Close()
			sb.WriteString(parseSheet(raw, shared))
			sb.WriteByte('\n')
			got = true
		}
	}
	if !got {
		return "", fmt.Errorf("xlsx 缺少 worksheet")
	}
	return sb.String(), nil
}

// parseSharedStrings：每个 <si> 是一个共享字符串（富文本可能含多个 <t>）。
func parseSharedStrings(raw []byte) []string {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var out []string
	var cur strings.Builder
	inSi, inT := false, false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "si" {
				inSi = true
				cur.Reset()
			} else if t.Name.Local == "t" && inSi {
				inT = true
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inT = false
			} else if t.Name.Local == "si" {
				inSi = false
				out = append(out, cur.String())
			}
		case xml.CharData:
			if inT {
				cur.Write(t)
			}
		}
	}
	return out
}

// parseSheet：逐行逐格，t="s" 的格值是共享字符串下标，其余是直接值；制表符分隔、行末换行。
func parseSheet(raw []byte, shared []string) string {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var sb strings.Builder
	cellType := ""
	inV := false
	var vbuf strings.Builder
	firstCell := true
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				firstCell = true
			case "c":
				cellType = ""
				for _, a := range t.Attr {
					if a.Name.Local == "t" {
						cellType = a.Value
					}
				}
			case "v":
				inV = true
				vbuf.Reset()
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				inV = false
				val := vbuf.String()
				if cellType == "s" {
					if idx, ok := atoiSafe(val); ok && idx >= 0 && idx < len(shared) {
						val = shared[idx]
					}
				}
				if !firstCell {
					sb.WriteByte('\t')
				}
				sb.WriteString(val)
				firstCell = false
			case "row":
				sb.WriteByte('\n')
			}
		case xml.CharData:
			if inV {
				vbuf.Write(t)
			}
		}
	}
	return sb.String()
}

// ---- pdf ----

// extractPDF 用 ledongthuc/pdf 提取纯文本。该库对畸形 PDF 可能 panic，用 recover 兜住不拖垮服务。
func extractPDF(b []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pdf 解析异常：%v", r)
			text = ""
		}
	}()
	rd, e := pdf.NewReader(bytes.NewReader(b), int64(len(b)))
	if e != nil {
		return "", fmt.Errorf("pdf 打开失败：%v", e)
	}
	tr, e := rd.GetPlainText()
	if e != nil {
		return "", fmt.Errorf("pdf 文本提取失败：%v", e)
	}
	out, _ := io.ReadAll(io.LimitReader(tr, 30<<20))
	return string(out), nil
}

// ---- URL ----

var (
	reScriptStyle = regexp.MustCompile(`(?is)<(script|style|noscript)[^>]*>.*?</(script|style|noscript)>`)
	reTag         = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpaces      = regexp.MustCompile(`[ \t]+`)
	reBlankLines  = regexp.MustCompile(`\n{3,}`)
)

// fetchURLText 用 SSRF 守卫客户端抓取 URL 并提取正文文本。
func fetchURLText(rawurl string) (string, error) {
	rawurl = strings.TrimSpace(rawurl)
	if !strings.HasPrefix(rawurl, "http://") && !strings.HasPrefix(rawurl, "https://") {
		rawurl = "https://" + rawurl
	}
	client := newGuardedHTTPClient(15 * time.Second) // SSRF 守卫：拒云元数据/链路本地
	resp, err := client.Get(rawurl)
	if err != nil {
		return "", fmt.Errorf("抓取失败：%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("抓取返回 HTTP %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "html") || bytes.Contains(bytes.ToLower(raw), []byte("<html")) || bytes.Contains(bytes.ToLower(raw), []byte("<body")) {
		return htmlToText(string(raw)), nil
	}
	return string(raw), nil
}

// htmlToText：去 script/style、去标签、解实体、压空白。
func htmlToText(h string) string {
	h = reScriptStyle.ReplaceAllString(h, " ")
	h = reTag.ReplaceAllString(h, " ")
	h = html.UnescapeString(h)
	h = reSpaces.ReplaceAllString(h, " ")
	h = reBlankLines.ReplaceAllString(h, "\n\n")
	return strings.TrimSpace(h)
}

// looksBinary 粗判是否二进制（含 NUL 或大量不可打印字符）。
func looksBinary(b []byte) bool {
	n := len(b)
	if n > 4096 {
		n = 4096
	}
	nonPrint := 0
	for i := 0; i < n; i++ {
		c := b[i]
		if c == 0 {
			return true
		}
		if c < 9 || (c > 13 && c < 32) {
			nonPrint++
		}
	}
	return n > 0 && nonPrint*100/n > 30
}
