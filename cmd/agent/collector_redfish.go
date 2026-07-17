package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"aiops-monitor/shared"
)

// RedfishTarget is one BMC/iDRAC/iLO endpoint to poll (from config.json).
type RedfishTarget struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	Username      string `json:"username"`
	PasswordEnv   string `json:"password_env"`
	SkipTLSVerify bool   `json:"skip_tls_verify"`
	IntervalSec   int    `json:"interval_sec"`
}

// redfishCollector manages periodic polling of one or more Redfish endpoints.
// Each target runs in its own goroutine with an independent timer.
type redfishCollector struct {
	targets []RedfishTarget
	hostID  string
	fp      string
	httpc   *http.Client

	mu        sync.Mutex
	snapshots []shared.HardwareSnapshot
	lastFW    map[string]int64 // target_name → last firmware collect timestamp
}

func newRedfishCollector(targets []RedfishTarget, hostID, fp string) *redfishCollector {
	return &redfishCollector{
		targets: targets,
		hostID:  hostID,
		fp:      fp,
		httpc: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		},
		lastFW: make(map[string]int64),
	}
}

// run starts one goroutine per target. Called from Agent.Run().
func (rc *redfishCollector) run(reporter func(shared.HardwareReport)) {
	for _, t := range rc.targets {
		go rc.pollLoop(t, reporter)
	}
}

func (rc *redfishCollector) pollLoop(t RedfishTarget, reporter func(shared.HardwareReport)) {
	interval := time.Duration(t.IntervalSec) * time.Second
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}

	slog.Info("Redfish 采集器启动", "target", t.Name, "url", t.URL, "interval", interval)

	failCount := 0
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Collect immediately on start
	snap := rc.collectOne(t)
	if snap.Error != "" {
		failCount++
		slog.Warn("Redfish 采集失败", "target", t.Name, "err", snap.Error)
	} else {
		failCount = 0
		slog.Info("Redfish 采集成功", "target", t.Name, "health", snap.Health)
	}
	rc.storeAndReport(t, snap, reporter)

	for range ticker.C {
		snap := rc.collectOne(t)
		if snap.Error != "" {
			failCount++
			slog.Warn("Redfish 采集失败", "target", t.Name, "err", snap.Error, "consecutive", failCount)
			if failCount >= 3 {
				// Backoff to 5 minutes on consecutive failures
				slog.Error("Redfish 连续失败，退避 5 分钟", "target", t.Name)
				time.Sleep(5 * time.Minute)
				failCount = 0
			}
		} else {
			failCount = 0
		}
		rc.storeAndReport(t, snap, reporter)
	}
}

func (rc *redfishCollector) storeAndReport(t RedfishTarget, snap shared.HardwareSnapshot, reporter func(shared.HardwareReport)) {
	rc.mu.Lock()
	// Update or append snapshot
	found := false
	for i, s := range rc.snapshots {
		if s.TargetName == snap.TargetName && s.TargetURL == snap.TargetURL {
			rc.snapshots[i] = snap
			found = true
			break
		}
	}
	if !found {
		rc.snapshots = append(rc.snapshots, snap)
	}
	all := make([]shared.HardwareSnapshot, len(rc.snapshots))
	copy(all, rc.snapshots)
	rc.mu.Unlock()

	reporter(shared.HardwareReport{
		HostID:      rc.hostID,
		Fingerprint: rc.fp,
		Snapshots:   all,
	})
}

// collectOne does a full sweep of one Redfish target and returns a snapshot.
func (rc *redfishCollector) collectOne(t RedfishTarget) shared.HardwareSnapshot {
	snap := shared.HardwareSnapshot{
		TargetName: t.Name,
		TargetURL:  t.URL,
		Timestamp:  time.Now().Unix(),
	}

	password := ""
	if t.PasswordEnv != "" {
		password = os.Getenv(t.PasswordEnv)
	}

	// Build per-target HTTP client with optional TLS skip
	client := rc.httpc
	if t.SkipTLSVerify {
		client = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	}

	type redfishStatus struct {
		Health string `json:"Health"`
		State  string `json:"State"`
	}

	// 1. System overview
	var sys struct {
		Status redfishStatus `json:"Status"`
		ProcessorSummary struct {
			Count         int    `json:"Count"`
			Model         string `json:"Model"`
			CoreCount     int    `json:"CoreCount"`
			ThreadCount   int    `json:"ThreadCount"`
		} `json:"ProcessorSummary"`
		MemorySummary struct {
			TotalSystemMemoryGiB float64 `json:"TotalSystemMemoryGiB"`
		} `json:"MemorySummary"`
	}
	if err := rc.rfGet(client, t.URL, t.Username, password, "/redfish/v1/Systems/1", &sys); err != nil {
		snap.Error = fmt.Sprintf("Systems/1: %v", err)
		return snap
	}
	snap.Health = sys.Status.Health
	snap.State = sys.Status.State
	if snap.Health == "" {
		snap.Health = "OK"
	}
	snap.Memory.TotalGB = sys.MemorySummary.TotalSystemMemoryGiB

	if sys.ProcessorSummary.Count > 0 {
		snap.CPUs = append(snap.CPUs, shared.RedfishCPU{
			Name:       "CPU Summary",
			Model:      sys.ProcessorSummary.Model,
			Cores:      sys.ProcessorSummary.CoreCount,
			Threads:    sys.ProcessorSummary.ThreadCount,
			Health:     sys.Status.Health,
		})
	}

	// 2. Processors detail
	var procs struct {
		Members []struct {
			ODataID string `json:"@odata.id"`
		} `json:"Members"`
	}
	if rc.rfGet(client, t.URL, t.Username, password, "/redfish/v1/Systems/1/Processors", &procs) == nil {
		snap.CPUs = nil // replace summary with per-processor entries
		for _, m := range procs.Members {
			var p struct {
				Name          string `json:"Name"`
				Model         string `json:"Model"`
				TotalCores    int    `json:"TotalCores"`
				TotalThreads  int    `json:"TotalThreads"`
				MaxSpeedMHz   int    `json:"MaxSpeedMHz"`
				Status        redfishStatus `json:"Status"`
			}
			if rc.rfGetRaw(client, t.URL, t.Username, password, m.ODataID, &p) == nil {
				snap.CPUs = append(snap.CPUs, shared.RedfishCPU{
					Name:       p.Name,
					Model:      p.Model,
					Cores:      p.TotalCores,
					Threads:    p.TotalThreads,
					Health:     p.Status.Health,
					MaxFreqMHz: p.MaxSpeedMHz,
				})
			}
		}
	}

	// 3. Memory DIMMs (lower frequency: every 5 min)
	var mems struct {
		Members []struct {
			ODataID string `json:"@odata.id"`
		} `json:"Members"`
	}
	if rc.rfGet(client, t.URL, t.Username, password, "/redfish/v1/Systems/1/Memory", &mems) == nil {
		for _, m := range mems.Members {
			var dimm struct {
				Name            string `json:"Name"`
				CapacityMiB     float64 `json:"CapacityMiB"`
				MemoryDeviceType string `json:"MemoryDeviceType"`
				OperatingSpeedMhz int  `json:"OperatingSpeedMhz"`
				Status          redfishStatus `json:"Status"`
				Id              string `json:"Id"`
			}
			if rc.rfGetRaw(client, t.URL, t.Username, password, m.ODataID, &dimm) == nil {
				snap.Memory.DIMMs = append(snap.Memory.DIMMs, shared.MemoryDIMM{
					Name:       dimm.Name,
					CapacityGB: dimm.CapacityMiB / 1024,
					Type:       dimm.MemoryDeviceType,
					SpeedMHz:   dimm.OperatingSpeedMhz,
					Health:     dimm.Status.Health,
					Slot:       dimm.Id,
				})
			}
		}
	}

	// 4. Storage (every 2 min)
	var storages struct {
		Members []struct {
			ODataID string `json:"@odata.id"`
		} `json:"Members"`
	}
	if rc.rfGet(client, t.URL, t.Username, password, "/redfish/v1/Systems/1/Storage", &storages) == nil {
		for _, sm := range storages.Members {
			var st struct {
				Drives []struct {
					ODataID string `json:"@odata.id"`
				} `json:"Drives"`
			}
			if rc.rfGetRaw(client, t.URL, t.Username, password, sm.ODataID, &st) == nil {
				for _, d := range st.Drives {
					var drv struct {
						Name         string  `json:"Name"`
						Model        string  `json:"Model"`
						CapacityBytes uint64 `json:"CapacityBytes"`
						Status       redfishStatus `json:"Status"`
						MediaType    string `json:"MediaType"`
						Protocol     string `json:"Protocol"`
					}
					if rc.rfGetRaw(client, t.URL, t.Username, password, d.ODataID, &drv) == nil {
						snap.Storage = append(snap.Storage, shared.RedfishStorage{
							Name:       drv.Name,
							Model:      drv.Model,
							CapacityGB: float64(drv.CapacityBytes) / (1024 * 1024 * 1024),
							Health:     drv.Status.Health,
							MediaType:  drv.MediaType,
							Protocol:   drv.Protocol,
							Status:     drv.Status.Health,
						})
					}
				}
			}
		}
	}

	// 5. Thermal (temperatures + fans)
	var thermal struct {
		Temperatures []struct {
			Name                string  `json:"Name"`
			ReadingCelsius      float64 `json:"ReadingCelsius"`
			Status              redfishStatus `json:"Status"`
			UpperThresholdCaution  float64 `json:"UpperThresholdCaution"`
			UpperThresholdCritical float64 `json:"UpperThresholdCritical"`
		} `json:"Temperatures"`
		Fans []struct {
			Name         string `json:"Name"`
			Reading      int    `json:"Reading"`
			ReadingUnits string `json:"ReadingUnits"`
			Status       redfishStatus `json:"Status"`
		} `json:"Fans"`
	}
	if rc.rfGet(client, t.URL, t.Username, password, "/redfish/v1/Chassis/1/Thermal", &thermal) == nil {
		for _, t := range thermal.Temperatures {
			snap.Temps = append(snap.Temps, shared.SensorReading{
				Name:          t.Name,
				Reading:       t.ReadingCelsius,
				Unit:          "Celsius",
				Status:        t.Status.Health,
				UpperCaution:  t.UpperThresholdCaution,
				UpperCritical: t.UpperThresholdCritical,
			})
		}
		for _, f := range thermal.Fans {
			snap.Fans = append(snap.Fans, shared.FanReading{
				Name:   f.Name,
				RPM:    f.Reading,
				Health: f.Status.Health,
				Status: f.Status.State,
			})
		}
	}

	// 6. Power (PSU + watts)
	var power struct {
		PowerControl []struct {
			Name             string  `json:"Name"`
			PowerConsumedWatts float64 `json:"PowerConsumedWatts"`
		} `json:"PowerControl"`
		PowerSupply []struct {
			Name             string  `json:"Name"`
			PowerInputWatts  float64 `json:"PowerInputWatts"`
			PowerOutputWatts float64 `json:"PowerOutputWatts"`
			Status           redfishStatus `json:"Status"`
		} `json:"PowerSupply"`
		PowerSupplyRedundancy []struct {
			Mode string `json:"Mode"`
		} `json:"PowerSupplyRedundancy"`
	}
	if rc.rfGet(client, t.URL, t.Username, password, "/redfish/v1/Chassis/1/Power", &power) == nil {
		if len(power.PowerSupplyRedundancy) > 0 {
			snap.Power.Redundancy = power.PowerSupplyRedundancy[0].Mode
		}
		for _, pc := range power.PowerControl {
			snap.Power.TotalWatts += pc.PowerConsumedWatts
		}
		for _, ps := range power.PowerSupply {
			snap.Power.PSUs = append(snap.Power.PSUs, shared.PSUReading{
				Name:        ps.Name,
				InputWatts:  ps.PowerInputWatts,
				OutputWatts: ps.PowerOutputWatts,
				Health:      ps.Status.Health,
				State:       ps.Status.State,
			})
		}
	}

	// 7. Firmware (low frequency: every hour)
	now := time.Now().Unix()
	rc.mu.Lock()
	lastFW := rc.lastFW[t.Name]
	rc.mu.Unlock()
	if now-lastFW >= 3600 {
		var fw struct {
			Members []struct {
				ODataID string `json:"@odata.id"`
			} `json:"Members"`
		}
		if rc.rfGet(client, t.URL, t.Username, password, "/redfish/v1/UpdateService/FirmwareInventory", &fw) == nil {
			for _, m := range fw.Members {
				var f struct {
					Name    string `json:"Name"`
					Version string `json:"Version"`
				}
				if rc.rfGetRaw(client, t.URL, t.Username, password, m.ODataID, &f) == nil {
					snap.Firmware = append(snap.Firmware, shared.FirmwareInfo{
						Name:    f.Name,
						Version: f.Version,
					})
				}
			}
			rc.mu.Lock()
			rc.lastFW[t.Name] = now
			rc.mu.Unlock()
		}
	}

	return snap
}

// rfGet fetches a Redfish endpoint relative to the target base URL.
func (rc *redfishCollector) rfGet(client *http.Client, base, user, pass, path string, dst any) error {
	return rc.rfGetRaw(client, base, user, pass, path, dst)
}

// rfGetRaw fetches an arbitrary Redfish path (may be @odata.id from collection members).
func (rc *redfishCollector) rfGetRaw(client *http.Client, base, user, pass, path string, dst any) error {
	url := base
	if len(path) > 0 && path[0] == '/' {
		url = base + path
	} else {
		// path is an @odata.id, already absolute on the BMC
		url = base + path
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}
