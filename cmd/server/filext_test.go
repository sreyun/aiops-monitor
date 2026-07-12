package main

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// zipBytes 打包若干 name→content 为 zip 字节（用于合成 docx/xlsx）。
func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractDocx(t *testing.T) {
	doc := `<?xml version="1.0"?><w:document xmlns:w="x"><w:body>` +
		`<w:p><w:r><w:t>第一段落</w:t></w:r><w:r><w:tab/><w:t>制表后</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>第二段落</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	b := zipBytes(t, map[string]string{"word/document.xml": doc, "[Content_Types].xml": "<x/>"})
	got, err := extractDocx(b)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "第一段落") || !strings.Contains(got, "制表后") || !strings.Contains(got, "第二段落") {
		t.Fatalf("docx 抽取缺文本: %q", got)
	}
	if !strings.Contains(got, "第一段落\t制表后") {
		t.Fatalf("制表符未保留: %q", got)
	}
	// 段落换行
	if !strings.Contains(got, "\n") {
		t.Fatalf("段落换行缺失: %q", got)
	}
}

func TestExtractXlsx(t *testing.T) {
	shared := `<sst><si><t>主机名</t></si><si><t>CPU</t></si><si><t>web-01</t></si></sst>`
	// 行1: 主机名(s0) | CPU(s1)  行2: web-01(s2) | 85（直接数值）
	sheet := `<worksheet><sheetData>` +
		`<row><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>` +
		`<row><c r="A2" t="s"><v>2</v></c><c r="B2"><v>85</v></c></row>` +
		`</sheetData></worksheet>`
	b := zipBytes(t, map[string]string{"xl/sharedStrings.xml": shared, "xl/worksheets/sheet1.xml": sheet})
	got, err := extractXlsx(b)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "主机名\tCPU") {
		t.Fatalf("表头共享字符串未解析: %q", got)
	}
	if !strings.Contains(got, "web-01\t85") {
		t.Fatalf("单元格值(共享+直接)未解析: %q", got)
	}
}

func TestHTMLToText(t *testing.T) {
	h := `<html><head><style>.x{color:red}</style><script>alert(1)</script></head>` +
		`<body><h1>标题</h1><p>正文&amp;内容 &lt;ok&gt;</p></body></html>`
	got := htmlToText(h)
	if strings.Contains(got, "alert") || strings.Contains(got, "color:red") {
		t.Fatalf("script/style 未剥离: %q", got)
	}
	if !strings.Contains(got, "标题") || !strings.Contains(got, "正文&内容 <ok>") {
		t.Fatalf("正文/实体解码错误: %q", got)
	}
}

func TestFetchURLText(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><script>x()</script><p>抓取到的正文</p></body></html>`))
	}))
	defer ts.Close()
	got, err := fetchURLText(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "抓取到的正文") || strings.Contains(got, "x()") {
		t.Fatalf("URL 正文提取错误: %q", got)
	}
}

func TestLooksBinary(t *testing.T) {
	if looksBinary([]byte("正常的一段中文文本 with english")) {
		t.Fatal("文本被误判为二进制")
	}
	if !looksBinary([]byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0x00, 0x03}) {
		t.Fatal("二进制未被识别")
	}
}
