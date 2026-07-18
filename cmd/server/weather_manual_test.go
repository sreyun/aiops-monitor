package main

import (
	"os"
	"testing"
)

// 手动联网验证出口公网 IP 自动探测：设置 AIOPS_EGRESS_TEST=1 才跑。
// 验证 serverEgressIP 能从国内回显服务拿到一个**公网** IPv4（即用户园区出口）。
func TestServerEgressIPManual(t *testing.T) {
	if os.Getenv("AIOPS_EGRESS_TEST") != "1" {
		t.Skip("需设置 AIOPS_EGRESS_TEST=1 才运行")
	}
	ip := serverEgressIP()
	if ip == "" {
		t.Fatalf("未能探测到出口公网 IP（国内回显服务均不可达？）")
	}
	if isPrivateOrLoopback(ip) {
		t.Fatalf("探测到的不是公网 IP: %s", ip)
	}
	t.Logf("出口公网 IP = %s（将据此定位城市）", ip)

	// 若同时配了 AppCode，顺带打通整条链，打印解析出的城市（不做断言，纯观测）。
	if code := os.Getenv("AIOPS_WEATHER_APPCODE"); code != "" {
		if res, err := fetchWeather(code, ip); err == nil {
			t.Logf("→ 城市=%q 温度=%d 天气=%q", res.Location, res.TempC, res.Text)
		}
	}
}

// 手动联网验证：AIOPS_WEATHER_APPCODE + AIOPS_WEATHER_TEST_IP 都设置时才跑，
// 验证上游解析 + 乱码清洗 + code 映射是否正确。平时（CI）自动跳过。
func TestFetchWeatherManual(t *testing.T) {
	code := os.Getenv("AIOPS_WEATHER_APPCODE")
	ip := os.Getenv("AIOPS_WEATHER_TEST_IP")
	if code == "" || ip == "" {
		t.Skip("需设置 AIOPS_WEATHER_APPCODE 和 AIOPS_WEATHER_TEST_IP 才运行")
	}
	res, err := fetchWeather(code, ip)
	if err != nil {
		t.Fatalf("fetchWeather 失败: %v", err)
	}
	t.Logf("归一化结果: location=%q temp=%d text=%q humidity=%q aqi=%q high=%d low=%d",
		res.Location, res.TempC, res.Text, res.Humidity, res.AQI, res.TodayHigh, res.TodayLow)
	if !res.OK {
		t.Errorf("ok=false")
	}
	if res.Text == "未知" {
		t.Logf("警告：weather_code 未命中映射表")
	}
}
