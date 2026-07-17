package main

import (
	"os"
	"testing"
)

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
