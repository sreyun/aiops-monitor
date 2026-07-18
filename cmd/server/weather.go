package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// 天气代理（供 App 首页天气头用）
//
// 为什么走服务端代理而不是 App 直连：
//  1. AppCode 是凭据，放服务端不随 APK 分发（APK 可被反编译提取）。
//  2. 这个 ip-to-weather 接口按**调用方传入的 IP** 定位；服务端天然知道客户端 IP，
//     直连的话 App 还得先查自己的公网 IP，多一次外部请求。
//  3. 上游返回的**中文城市/天气是双重编码乱码**（GBK 当 UTF-16），必须服务端清洗：
//     只取干净的数字字段（温度、weather_code、湿度、AQI），中文由我们按 code 映射。
//
// 城市定位优先级（自动识别用户所在城市，不再写死杭州）：
//  1. ?ip= 显式覆盖（调试用）。
//  2. 客户端公网 IP：用户走公网访问时，clientIP 即其真实公网 IP → 其所在城市。
//  3. 客户端是内网/环回 IP（本产品服务端与用户同处一个局域网/园区最常见）：
//     自动探测**服务端出口公网 IP**——出口 IP 即园区所在城市的公网 IP，
//     等价于用户所在城市。结果缓存，出口 IP 基本不变。
//  4. 出口探测失败时才回退到 AIOPS_WEATHER_DEFAULT_IP（如配置，纯兜底/强制指定城市）。
//
// AppCode 从环境变量 AIOPS_WEATHER_APPCODE 读取；未配置则天气功能关闭（App 优雅降级）。
// AIOPS_WEATHER_DEFAULT_IP 可选：仅当出口探测失败时兜底，或云上部署强制指定城市时使用。
// ---------------------------------------------------------------------------

const weatherUpstream = "https://weather01.market.alicloudapi.com/ip-to-weather"
const weatherCacheTTL = 20 * time.Minute

type weatherResult struct {
	OK        bool   `json:"ok"`
	Location  string `json:"location"`
	TempC     int    `json:"temp_c"`
	Text      string `json:"text"`
	Humidity  string `json:"humidity,omitempty"`
	AQI       string `json:"aqi,omitempty"`
	TodayHigh    int    `json:"today_high,omitempty"`
	TodayLow     int    `json:"today_low,omitempty"`
	TomorrowHigh int    `json:"tomorrow_high,omitempty"`
	TomorrowLow  int    `json:"tomorrow_low,omitempty"`
	TomorrowText string `json:"tomorrow_text,omitempty"` // 明天白天天气（按 code 映射，避免上游中文乱码）
	Reason       string `json:"reason,omitempty"`        // ok=false 时说明原因
	Source       string `json:"source,omitempty"`        // 定位 IP 来源：client/egress/default-env/override（调试用）
}

type weatherCacheEntry struct {
	res weatherResult
	at  time.Time
}

var (
	weatherMu    sync.Mutex
	weatherCache = map[string]weatherCacheEntry{}
	weatherHTTP  = &http.Client{Timeout: 8 * time.Second}
)

// handleWeather returns normalized current weather for the caller's location.
func (s *Server) handleWeather(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(r); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	appCode := strings.TrimSpace(os.Getenv("AIOPS_WEATHER_APPCODE"))
	if appCode == "" {
		writeJSON(w, http.StatusOK, weatherResult{OK: false, Reason: "weather not configured"})
		return
	}

	ip := r.URL.Query().Get("ip") // 允许显式覆盖（调试用）
	source := "override"
	if ip == "" {
		ip = s.clientIP(r)
		source = "client"
	}
	// 客户端是内网/环回 IP（服务端与用户同处局域网时最常见）：无法用它定位。
	// 改为自动探测**服务端出口公网 IP**（园区出口=用户所在城市）；探测失败才回退默认 IP。
	if isPrivateOrLoopback(ip) {
		if eip := serverEgressIP(); eip != "" {
			ip = eip
			source = "egress"
		} else if def := strings.TrimSpace(os.Getenv("AIOPS_WEATHER_DEFAULT_IP")); def != "" {
			ip = def
			source = "default-env"
		} else {
			writeJSON(w, http.StatusOK, weatherResult{OK: false, Reason: "client ip not routable; egress detect failed"})
			return
		}
	}

	// 缓存：天气不必秒级刷新，同一 IP 20 分钟内复用，省调用配额。
	weatherMu.Lock()
	if e, ok := weatherCache[ip]; ok && time.Since(e.at) < weatherCacheTTL {
		weatherMu.Unlock()
		res := e.res
		res.Source = source
		writeJSON(w, http.StatusOK, res)
		return
	}
	weatherMu.Unlock()

	res, err := fetchWeather(appCode, ip)
	if err != nil {
		writeJSON(w, http.StatusOK, weatherResult{OK: false, Reason: err.Error()})
		return
	}
	weatherMu.Lock()
	weatherCache[ip] = weatherCacheEntry{res: res, at: time.Now()}
	weatherMu.Unlock()
	res.Source = source
	writeJSON(w, http.StatusOK, res)
}

// ---------------------------------------------------------------------------
// 服务端出口公网 IP 探测
//
// 本产品多为内网/园区部署，服务端与用户同处一个局域网，出口公网 IP 即用户所在
// 城市的公网 IP。依次尝试几个**国内可达**的 IP 回显服务，取第一个成功返回的公网
// IPv4。结果缓存 6 小时（出口 IP 基本不变）；失败短暂缓存 1 分钟，避免离线时反复打。
// ---------------------------------------------------------------------------

const (
	egressTTL     = 6 * time.Hour
	egressFailTTL = 1 * time.Minute
)

var (
	egressMu   sync.Mutex
	egressIP   string
	egressAt   time.Time
	egressOK   bool
	egressHTTP = &http.Client{Timeout: 4 * time.Second}
	ipv4Re     = regexp.MustCompile(`(?:\d{1,3}\.){3}\d{1,3}`)

	// 国内可达、返回体里含**调用方公网 IP** 的回显服务，任一成功即可。全部为国内节点：
	// 国际线路（如 ipify）在国内常经不同出口，会解析出另一个 IP → 错误城市，故不用。
	egressProviders = []string{
		"https://myip.ipip.net",         // 文本：当前 IP：1.2.3.4 来自于：中国 上海 ...
		"https://ip.3322.net",           // 纯文本 IP：1.2.3.4
		"https://ddns.oray.com/checkip", // 文本：Current IP Address: 1.2.3.4
		"https://cip.cc",                // 文本：IP : 1.2.3.4  地址 : 中国 ...
	}
)

// serverEgressIP returns the server's public egress IPv4, cached. Empty on failure.
func serverEgressIP() string {
	egressMu.Lock()
	if egressIP != "" && time.Since(egressAt) < egressTTL {
		ip := egressIP
		egressMu.Unlock()
		return ip
	}
	if !egressOK && egressIP == "" && time.Since(egressAt) < egressFailTTL {
		egressMu.Unlock() // 近期刚失败，先别重试
		return ""
	}
	egressMu.Unlock()

	found := probeEgressIP()

	egressMu.Lock()
	egressAt = time.Now()
	if found != "" {
		egressIP = found
		egressOK = true
	} else {
		egressOK = false
	}
	egressMu.Unlock()
	return found
}

func probeEgressIP() string {
	for _, url := range egressProviders {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "curl/8.0") // 部分回显服务对无 UA 返回 HTML
		resp, err := egressHTTP.Do(req)
		if err != nil {
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		if resp.StatusCode != 200 {
			continue
		}
		// 取返回体里第一个**公网** IPv4（跳过内网/环回，防回显服务把内网地址塞进来）。
		for _, cand := range ipv4Re.FindAllString(string(raw), -1) {
			if net.ParseIP(cand) != nil && !isPrivateOrLoopback(cand) {
				return cand
			}
		}
	}
	return ""
}

func fetchWeather(appCode, ip string) (weatherResult, error) {
	req, _ := http.NewRequest("GET", weatherUpstream+"?ip="+ip, nil)
	req.Header.Set("Authorization", "APPCODE "+appCode)
	resp, err := weatherHTTP.Do(req)
	if err != nil {
		return weatherResult{}, fmt.Errorf("weather upstream: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return weatherResult{}, fmt.Errorf("weather upstream HTTP %d", resp.StatusCode)
	}

	// 只解析**干净**字段。中文城市/天气名在上游是乱码，一律不取，改由 code 映射。
	var up struct {
		Code int `json:"showapi_res_code"`
		Body struct {
			CityInfo struct {
				C2 string `json:"c2"` // 城市拼音（干净）
			} `json:"cityInfo"`
			Now struct {
				Temperature string `json:"temperature"`
				WeatherCode string `json:"weather_code"`
				SD          string `json:"sd"`
				AQI         string `json:"aqi"`
			} `json:"now"`
			F1 struct {
				DayTemp   string `json:"day_air_temperature"`
				NightTemp string `json:"night_air_temperature"`
			} `json:"f1"`
			F2 struct {
				DayTemp   string `json:"day_air_temperature"`
				NightTemp string `json:"night_air_temperature"`
				DayCode   string `json:"day_weather_code"`
			} `json:"f2"`
		} `json:"showapi_res_body"`
	}
	if err := json.Unmarshal(raw, &up); err != nil {
		return weatherResult{}, fmt.Errorf("weather parse: %w", err)
	}
	if up.Code != 0 {
		return weatherResult{}, fmt.Errorf("weather upstream code %d", up.Code)
	}

	res := weatherResult{
		OK:        true,
		TempC:     weatherAtoi(up.Body.Now.Temperature),
		Text:      weatherCodeText(up.Body.Now.WeatherCode),
		Humidity:  up.Body.Now.SD,
		AQI:       up.Body.Now.AQI,
		Location:     cityCN(up.Body.CityInfo.C2),
		TodayHigh:    weatherAtoi(up.Body.F1.DayTemp),
		TodayLow:     weatherAtoi(up.Body.F1.NightTemp),
		TomorrowHigh: weatherAtoi(up.Body.F2.DayTemp),
		TomorrowLow:  weatherAtoi(up.Body.F2.NightTemp),
		TomorrowText: weatherCodeText(up.Body.F2.DayCode),
	}
	return res, nil
}

func weatherAtoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func isPrivateOrLoopback(ipStr string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

// weatherCodeText maps ShowAPI weather codes to Chinese. Covers the common set;
// unknown codes fall back to a neutral label rather than the upstream mojibake.
func weatherCodeText(code string) string {
	switch strings.TrimSpace(code) {
	case "00":
		return "晴"
	case "01":
		return "多云"
	case "02":
		return "阴"
	case "03":
		return "阵雨"
	case "04":
		return "雷阵雨"
	case "05":
		return "雷阵雨伴冰雹"
	case "06":
		return "雨夹雪"
	case "07":
		return "小雨"
	case "08":
		return "中雨"
	case "09":
		return "大雨"
	case "10":
		return "暴雨"
	case "11":
		return "大暴雨"
	case "12":
		return "特大暴雨"
	case "13":
		return "阵雪"
	case "14":
		return "小雪"
	case "15":
		return "中雪"
	case "16":
		return "大雪"
	case "17":
		return "暴雪"
	case "18":
		return "雾"
	case "19":
		return "冻雨"
	case "20":
		return "沙尘暴"
	case "29":
		return "浮尘"
	case "30":
		return "扬沙"
	case "32":
		return "浓雾"
	case "49", "53", "54", "55", "56", "57":
		return "霾"
	default:
		return "未知"
	}
}

// cityCN maps a handful of common city pinyin to Chinese; falls back to
// Title-cased pinyin so we never surface the upstream mojibake.
func cityCN(pinyin string) string {
	p := strings.ToLower(strings.TrimSpace(pinyin))
	m := map[string]string{
		"shanghai": "上海", "beijing": "北京", "guangzhou": "广州", "shenzhen": "深圳",
		"nanjing": "南京", "hangzhou": "杭州", "suzhou": "苏州", "chengdu": "成都",
		"wuhan": "武汉", "xian": "西安", "chongqing": "重庆", "tianjin": "天津",
		"hefei": "合肥", "zhengzhou": "郑州", "changsha": "长沙", "qingdao": "青岛",
		"fengxian": "奉贤", "shenyang": "沈阳", "dalian": "大连", "xiamen": "厦门",
	}
	if cn, ok := m[p]; ok {
		return cn
	}
	if p == "" {
		return ""
	}
	return strings.ToUpper(p[:1]) + p[1:]
}
