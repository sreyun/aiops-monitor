package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
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
// AppCode 从环境变量 AIOPS_WEATHER_APPCODE 读取；未配置则天气功能关闭（App 优雅降级）。
// AIOPS_WEATHER_DEFAULT_IP 可选：客户端为内网/环回 IP（无法定位）时用它兜底。
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
	TodayHigh int    `json:"today_high,omitempty"`
	TodayLow  int    `json:"today_low,omitempty"`
	Reason    string `json:"reason,omitempty"` // ok=false 时说明原因
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
	if ip == "" {
		ip = s.clientIP(r)
	}
	// 内网/环回 IP 无法定位，用兜底 IP（如配置）
	if isPrivateOrLoopback(ip) {
		if def := strings.TrimSpace(os.Getenv("AIOPS_WEATHER_DEFAULT_IP")); def != "" {
			ip = def
		} else {
			writeJSON(w, http.StatusOK, weatherResult{OK: false, Reason: "client ip not routable"})
			return
		}
	}

	// 缓存：天气不必秒级刷新，同一 IP 20 分钟内复用，省调用配额。
	weatherMu.Lock()
	if e, ok := weatherCache[ip]; ok && time.Since(e.at) < weatherCacheTTL {
		weatherMu.Unlock()
		writeJSON(w, http.StatusOK, e.res)
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
	writeJSON(w, http.StatusOK, res)
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
		Location:  cityCN(up.Body.CityInfo.C2),
		TodayHigh: weatherAtoi(up.Body.F1.DayTemp),
		TodayLow:  weatherAtoi(up.Body.F1.NightTemp),
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
