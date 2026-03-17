package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/scanray/pupp/internal/config"
	"github.com/scanray/pupp/internal/ws"
)

type Agent struct {
	cfg     *config.Config
	client  *ws.Client
	scanner *Scanner
	done    chan struct{}
}

func New(cfg *config.Config) *Agent {
	return &Agent{
		cfg:     cfg,
		client:  ws.NewClient(cfg.ConsoleURL, cfg.AuthToken),
		scanner: NewScanner(cfg.ScanrayBin, cfg.NucleiBin, cfg.DataDir),
	}
}

func (a *Agent) Run() {
	for {
		a.done = make(chan struct{})
		a.client.ConnectWithRetry()
		a.client.OnMessage = a.handleMessage
		a.sendRegistration()
		go a.heartbeatLoop()
		a.client.ReadLoop()
		close(a.done)
		log.Println("[agent] Disconnected, will reconnect...")
		time.Sleep(2 * time.Second)
	}
}

func (a *Agent) sendRegistration() {
	scanrayVer := GetBinaryVersion(a.cfg.ScanrayBin)
	nucleiVer := GetBinaryVersion(a.cfg.NucleiBin)
	sysInfo := CollectSystemInfo()

	msg := map[string]interface{}{
		"type": "register",
		"payload": map[string]interface{}{
			"system_info": map[string]interface{}{
				"os":       sysInfo.OS,
				"arch":     sysInfo.Arch,
				"hostname": sysInfo.Hostname,
			},
			"scanray_version": scanrayVer,
			"nuclei_version":  nucleiVer,
		},
	}
	if err := a.client.SendJSON(msg); err != nil {
		log.Printf("[agent] Failed to send registration: %v", err)
	}
}

func (a *Agent) heartbeatLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics := CollectHealthMetrics()
			msg := map[string]interface{}{
				"type": "heartbeat",
				"payload": map[string]interface{}{
					"status": "online",
					"metrics": map[string]interface{}{
						"cpu_percent":    metrics.CPUPercent,
						"mem_percent":    metrics.MemPercent,
						"disk_percent":   metrics.DiskPercent,
						"uptime_seconds": metrics.Uptime,
					},
				},
			}
			if err := a.client.SendJSON(msg); err != nil {
				log.Printf("[agent] Heartbeat send failed: %v", err)
				return
			}
		case <-a.done:
			return
		}
	}
}

func (a *Agent) handleMessage(raw []byte) {
	var msg struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		log.Printf("[agent] Invalid message: %v", err)
		return
	}

	switch msg.Type {
	case "start_scan":
		var req ScanRequest
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			log.Printf("[agent] Invalid scan request: %v", err)
			return
		}
		go a.executeScan(req)

	case "cancel_scan":
		a.scanner.Cancel()

	case "update_templates":
		go a.updateTemplates()

	case "ping":
		a.client.SendJSON(map[string]interface{}{
			"type":    "heartbeat",
			"payload": map[string]interface{}{"status": "online"},
		})
	}
}

func (a *Agent) executeScan(req ScanRequest) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[agent] PANIC in scan goroutine (recovered): %v", r)
			a.client.SendJSON(map[string]interface{}{
				"type":    "scan_status",
				"payload": map[string]interface{}{"status": "failed", "scan_run_id": req.ScanRunID, "error": fmt.Sprintf("internal panic: %v", r)},
			})
		}
	}()

	log.Printf("[agent] Executing scan: type=%s run_id=%s targets=%d", req.ScanType, req.ScanRunID, len(req.Targets))

	if err := a.client.SendJSON(map[string]interface{}{
		"type":    "scan_status",
		"payload": map[string]interface{}{"status": "running", "scan_run_id": req.ScanRunID, "scan_type": req.ScanType},
	}); err != nil {
		log.Printf("[agent] Failed to send running status: %v", err)
		return
	}

	if req.ScanType == "vulnerability" {
		a.runVulnScan(req)
	} else {
		a.runAssetScan(req)
	}
}

func (a *Agent) runAssetScan(req ScanRequest) {
	result, err := a.scanner.RunAssetScan(req)
	if err != nil {
		log.Printf("[agent] Asset scan failed: %v", err)
		a.client.SendJSON(map[string]interface{}{
			"type":    "scan_status",
			"payload": map[string]interface{}{"status": "failed", "scan_run_id": req.ScanRunID, "error": err.Error()},
		})
		return
	}

	hosts, _ := result["hosts"].([]interface{})
	log.Printf("[agent] Asset scan complete: %d hosts found, sending results", len(hosts))

	result["scan_run_id"] = req.ScanRunID
	if err := a.client.SendJSON(map[string]interface{}{
		"type":    "scan_result_asset",
		"payload": result,
	}); err != nil {
		log.Printf("[agent] Failed to send asset results: %v", err)
		return
	}

	if err := a.client.SendJSON(map[string]interface{}{
		"type":    "scan_status",
		"payload": map[string]interface{}{"status": "completed", "scan_run_id": req.ScanRunID},
	}); err != nil {
		log.Printf("[agent] Failed to send completion status: %v", err)
	}
	log.Printf("[agent] Asset scan run_id=%s finished and reported", req.ScanRunID)
}

func (a *Agent) runVulnScan(req ScanRequest) {
	var findings []map[string]interface{}
	batchSize := 50

	err := a.scanner.RunVulnScan(req, func(finding map[string]interface{}) {
		findings = append(findings, finding)
		if len(findings) >= batchSize {
			a.client.SendJSON(map[string]interface{}{
				"type":    "scan_result_vuln",
				"payload": map[string]interface{}{"findings": findings, "scan_run_id": req.ScanRunID},
			})
			findings = nil
		}
	})

	if len(findings) > 0 {
		a.client.SendJSON(map[string]interface{}{
			"type":    "scan_result_vuln",
			"payload": map[string]interface{}{"findings": findings, "scan_run_id": req.ScanRunID},
		})
	}

	if err != nil {
		log.Printf("[agent] Vuln scan failed: %v", err)
		a.client.SendJSON(map[string]interface{}{
			"type":    "scan_status",
			"payload": map[string]interface{}{"status": "failed", "scan_run_id": req.ScanRunID, "error": err.Error()},
		})
		return
	}

	a.client.SendJSON(map[string]interface{}{
		"type":    "scan_status",
		"payload": map[string]interface{}{"status": "completed", "scan_run_id": req.ScanRunID},
	})
}

func (a *Agent) updateTemplates() {
	log.Println("[agent] Updating Nuclei templates...")
	cmd := exec.Command(a.cfg.NucleiBin, "-update-templates")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("[agent] Template update failed: %v", err)
	} else {
		log.Println("[agent] Templates updated successfully")
	}
}
