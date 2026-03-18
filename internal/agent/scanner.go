package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

type ScanRequest struct {
	ScanRunID string   `json:"scan_run_id"`
	ScanType  string   `json:"scan_type"` // quick, basic, vulnerability
	Targets   []string `json:"targets"`
	RateLimit int      `json:"rate_limit"`
}

type Scanner struct {
	ScanrayBin string
	NucleiBin  string
	DataDir    string

	mu         sync.Mutex
	activeProc *os.Process
}

func NewScanner(scanrayBin, nucleiBin, dataDir string) *Scanner {
	os.MkdirAll(dataDir, 0775)
	return &Scanner{
		ScanrayBin: scanrayBin,
		NucleiBin:  nucleiBin,
		DataDir:    dataDir,
	}
}

// RunAssetScan runs Scanray and returns the JSON output as a map.
// scan_type "quick" = ping-only sweep; "basic" = full SYN port scan.
func (s *Scanner) RunAssetScan(req ScanRequest) (map[string]interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	targetsFile := filepath.Join(s.DataDir, fmt.Sprintf("targets_%s.txt", req.ScanRunID))
	outputFile := filepath.Join(s.DataDir, fmt.Sprintf("output_%s.json", req.ScanRunID))

	f, err := os.Create(targetsFile)
	if err != nil {
		return nil, fmt.Errorf("create targets file: %w", err)
	}
	for _, t := range req.Targets {
		fmt.Fprintln(f, t)
	}
	f.Close()
	defer os.Remove(targetsFile)
	defer os.Remove(outputFile)

	args := []string{
		"-f", targetsFile,
		"-o", outputFile,
	}

	if req.ScanType == "quick" {
		args = append(args, "--ping-only")
	} else {
		args = append(args, "--syn", "-t", "500", "--timeout", "1500")
	}

	if req.RateLimit > 0 {
		args = append(args, "--host-rate", fmt.Sprintf("%d", req.RateLimit))
	}

	cmd := exec.Command(s.ScanrayBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	s.activeProc = cmd.Process

	log.Printf("[scanner] Starting %s asset scan: %s %v", req.ScanType, s.ScanrayBin, args)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("scanray exited: %w", err)
	}

	data, err := os.ReadFile(outputFile)
	if err != nil {
		return nil, fmt.Errorf("read output: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse output: %w", err)
	}
	return result, nil
}

// RunVulnScan runs Nuclei and streams findings back via the callback.
func (s *Scanner) RunVulnScan(req ScanRequest, onFinding func(map[string]interface{})) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	targetsFile := filepath.Join(s.DataDir, fmt.Sprintf("targets_%s.txt", req.ScanRunID))
	f, err := os.Create(targetsFile)
	if err != nil {
		return fmt.Errorf("create targets file: %w", err)
	}
	for _, t := range req.Targets {
		fmt.Fprintln(f, t)
	}
	f.Close()
	defer os.Remove(targetsFile)

	args := []string{
		"-l", targetsFile,
		"-jsonl",
		"-rate-limit", fmt.Sprintf("%d", req.RateLimit),
		"-bulk-size", "25",
		"-concurrency", "25",
		"-timeout", "5",
		"-no-color",
	}

	cmd := exec.Command(s.NucleiBin, args...)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	log.Printf("[scanner] Starting vuln scan: %s %v", s.NucleiBin, args)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("nuclei start: %w", err)
	}
	s.activeProc = cmd.Process

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var finding map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &finding); err == nil {
			onFinding(finding)
		}
	}

	return cmd.Wait()
}

func (s *Scanner) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeProc != nil {
		s.activeProc.Kill()
		s.activeProc = nil
		log.Println("[scanner] Scan process cancelled")
	}
}
