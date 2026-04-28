package ui

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Sahil0114/infrasage1/internal/gitops"
	"github.com/Sahil0114/infrasage1/internal/llm"
	"github.com/Sahil0114/infrasage1/internal/monitor"
	"github.com/Sahil0114/infrasage1/internal/scanner"
	tf "github.com/Sahil0114/infrasage1/internal/terraform"
)

//go:embed assets/*
var assetFS embed.FS

// Config configures the UI server.
type Config struct {
	Addr         string
	GrafanaURL   string
	SystemPrompt string
	AWSAvailable bool
	Mode         string
}

// Server provides the InfraSage UI backend.
type Server struct {
	addr         string
	grafanaURL   string
	systemPrompt string

	mu           sync.Mutex
	clients      map[*wsClient]struct{}
	awsAvailable bool
	mode         string
	busy         bool
}

// NewServer builds a UI server with the given config.
func NewServer(cfg Config) *Server {
	mode := cfg.Mode
	if mode == "" {
		mode = "github"
	}

	return &Server{
		addr:         cfg.Addr,
		grafanaURL:   cfg.GrafanaURL,
		systemPrompt: cfg.SystemPrompt,
		clients:      make(map[*wsClient]struct{}),
		awsAvailable: cfg.AWSAvailable,
		mode:         mode,
	}
}

// ListenAndServe starts the UI HTTP server.
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()

	staticFS, err := fs.Sub(assetFS, "assets")
	if err != nil {
		return fmt.Errorf("ui assets not found: %w", err)
	}

	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/ask", s.handleAsk)
	mux.HandleFunc("/api/drift", s.handleDrift)
	mux.HandleFunc("/api/remediate", s.handleRemediate)
	mux.HandleFunc("/api/mode", s.handleMode)
	mux.HandleFunc("/ws/stream", s.handleWebSocket)
	mux.HandleFunc("/", s.handleIndex)
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(staticFS))))

	server := &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("ui server listening", "addr", s.addr)
	return server.ListenAndServe()
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	file, err := assetFS.Open("assets/index.html")
	if err != nil {
		http.Error(w, "ui assets missing", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.Copy(w, file)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	mode := s.mode
	awsAvailable := s.awsAvailable
	s.mu.Unlock()

	resp := map[string]any{
		"grafanaUrl":   s.grafanaURL,
		"mode":         mode,
		"awsAvailable": awsAvailable,
		"modes":        []string{"github", "aws"},
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	if req.Mode != "github" && req.Mode != "aws" {
		http.Error(w, "mode must be github or aws", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if req.Mode == "aws" && !s.awsAvailable {
		s.mu.Unlock()
		http.Error(w, "aws mode not available — run 'aws configure' and 'infrasage config --aws'", http.StatusBadRequest)
		return
	}
	s.mode = req.Mode
	mode := s.mode
	awsAvailable := s.awsAvailable
	s.mu.Unlock()

	s.broadcast(Event{
		Type:    "mode",
		Payload: ModePayload{Current: mode, AWSAvailable: awsAvailable},
	})

	writeJSON(w, http.StatusOK, map[string]string{"mode": mode})
}

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Prompt string `json:"prompt"`
		Output string `json:"output"`
		NoScan bool   `json:"noScan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	if !s.startJob() {
		http.Error(w, "another job is already running", http.StatusConflict)
		return
	}

	go func() {
		defer s.finishJob()
		s.runAsk(req.Prompt, req.Output, req.NoScan)
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (s *Server) handleDrift(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.startJob() {
		http.Error(w, "another job is already running", http.StatusConflict)
		return
	}

	go func() {
		defer s.finishJob()
		s.runDrift()
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (s *Server) handleRemediate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		File string `json:"file"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	if !s.startJob() {
		http.Error(w, "another job is already running", http.StatusConflict)
		return
	}

	go func() {
		defer s.finishJob()
		s.runRemediate(req.File)
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (s *Server) runAsk(prompt, output string, noScan bool) {
	outFile := strings.TrimSpace(output)
	if outFile == "" {
		outFile = "infra.tf"
	}

	s.pipeline("NL Input")
	s.terminal("➜ " + prompt)

	ollamaURL := getEnvOrDefault("INFRASAGE_OLLAMA_URL", "http://localhost:11434")
	ollamaModel := getEnvOrDefault("INFRASAGE_OLLAMA_MODEL", "qwen2.5-coder:3b")

	client := llm.NewOllamaClient(ollamaURL, ollamaModel)
	if !client.IsHealthy() {
		s.terminalError(fmt.Sprintf("Ollama is not running at %s. Fix: run 'ollama serve' and retry.", ollamaURL))
		monitor.RecordGeneration("error")
		return
	}

	s.pipeline("AI/NLP")
	s.terminal("⏳ Generating Terraform HCL...")
	start := time.Now()
	response, err := client.Generate(s.systemPrompt, prompt)
	if err != nil {
		s.terminalError(fmt.Sprintf("Generation failed: %v", err))
		monitor.RecordGeneration("error")
		return
	}

	hcl := tf.ExtractHCL(response)
	if hcl == "" {
		s.terminalError("Model did not return valid HCL — try rephrasing your prompt.")
		monitor.RecordGeneration("error")
		return
	}

	s.pipeline("IaC Generator")
	if err := os.WriteFile(outFile, []byte(hcl), 0o644); err != nil {
		s.terminalError(fmt.Sprintf("Failed to write %s: %v", outFile, err))
		monitor.RecordGeneration("error")
		return
	}

	elapsed := time.Since(start)
	monitor.RecordGeneration("success")
	monitor.RecordModelLatency(elapsed.Seconds())

	s.terminal(fmt.Sprintf("✅ Wrote %s in %s", outFile, elapsed.Round(10*time.Millisecond)))
	streamLines(s, hcl)

	if noScan {
		s.terminal("⏭️  Scan skipped (--no-scan)")
		return
	}

	s.pipeline("CI/CD")
	s.terminal("🔍 Running security scans...")
	report, err := scanner.RunAllWithCallback(outFile, func(res scanner.ScanResult) {
		s.sendScanResult(res)
	})
	if err != nil {
		s.terminalError(fmt.Sprintf("Scan error: %v", err))
		return
	}

	s.terminal(fmt.Sprintf("✅ Scan complete: %d passed / %d failed", report.TotalPassed(), report.TotalFailed()))
	s.pipeline("Monitoring")
}

func (s *Server) runDrift() {
	dir, err := filepath.Abs(".")
	if err != nil {
		s.terminalError(fmt.Sprintf("Drift: resolving working directory failed: %v", err))
		return
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil || len(matches) == 0 {
		s.terminalError(fmt.Sprintf("Drift: no .tf files found in %s. Generate one first.", dir))
		return
	}

	s.pipeline("Drift")
	s.terminal("🔍 Checking for infrastructure drift...")

	initOutput, err := tf.InitWithOutput(dir)
	if initOutput != "" {
		streamLines(s, initOutput)
	}
	if err != nil {
		s.terminalError(fmt.Sprintf("Terraform init failed: %v", err))
		return
	}

	exitCode, planOutput, err := tf.PlanWithOutput(dir)
	if planOutput != "" {
		streamLines(s, planOutput)
	}
	if err != nil {
		s.terminalError(fmt.Sprintf("Terraform plan failed: %v", err))
		return
	}

	switch exitCode {
	case 0:
		s.sendDrift("clean", "✅ No drift detected.")
	case 2:
		monitor.RecordDrift("unknown")
		s.sendDrift("drift", planOutput)
		s.terminal("⚠️ Drift detected — remediation available.")
	default:
		s.sendDrift("error", planOutput)
	}
}

func (s *Server) runRemediate(file string) {
	tfFile := strings.TrimSpace(file)
	if tfFile == "" {
		matches, err := filepath.Glob("*.tf")
		if err != nil || len(matches) == 0 {
			s.terminalError("Remediate: no .tf files found. Generate one first.")
			return
		}
		tfFile = matches[0]
	}

	s.pipeline("Remediation")
	s.terminal(fmt.Sprintf("🚀 Opening remediation PR for %s...", tfFile))
	s.pipeline("Git Repo")

	base := filepath.Base(tfFile)
	noExt := strings.TrimSuffix(base, filepath.Ext(base))
	ts := time.Now().Format("20060102-150405")
	branch := fmt.Sprintf("infrasage/%s-%s", noExt, ts)
	message := fmt.Sprintf("chore(infrasage): add %s", base)

	if err := gitops.CommitAndPush(tfFile, branch, message); err != nil {
		s.terminalError(fmt.Sprintf("Remediate: git push failed: %v", err))
		return
	}

	s.mu.Lock()
	mode := s.mode
	s.mu.Unlock()

	s.pipeline("CI/CD")
	prBody := gitops.BuildPRBody(tfFile, branch, mode)
	prTitle := fmt.Sprintf("[InfraSage] %s", message)
	prURL, err := gitops.CreatePR(branch, prTitle, prBody)
	if err != nil {
		s.terminalError(fmt.Sprintf("Remediate: creating PR failed: %v", err))
		return
	}

	if mode == "aws" {
		s.pipeline("Cloud")
	}
	s.terminal("✅ Pull request opened: " + prURL)
	s.broadcast(Event{Type: "deploy", Payload: DeployPayload{URL: prURL}})

	// Stream GitHub Actions CI status back to the terminal in the background.
	go func() {
		gitops.PollWorkflowRuns(branch, 3*time.Minute, 6*time.Second, func(msg string) {
			s.terminal(msg)
		})
	}()
}

func (s *Server) sendScanResult(res scanner.ScanResult) {
	status := "PASS"
	if res.Error != nil {
		status = "ERROR"
	} else if res.Failed > 0 {
		status = "WARN"
		for _, finding := range res.Findings {
			if strings.EqualFold(finding.Severity, "CRITICAL") {
				status = "FAIL"
				break
			}
		}
	}

	payload := ScanPayload{
		Tool:     res.Tool,
		Passed:   res.Passed,
		Failed:   res.Failed,
		Status:   status,
		Duration: res.Duration.String(),
	}
	for _, f := range res.Findings {
		payload.Findings = append(payload.Findings, FindingPayload{
			ID:       f.ID,
			Severity: f.Severity,
			Message:  f.Message,
			Resource: f.Resource,
		})
	}

	s.broadcast(Event{Type: "scan", Payload: payload})
}

func (s *Server) sendDrift(status, output string) {
	s.broadcast(Event{
		Type:    "drift",
		Payload: DriftPayload{Status: status, Output: output},
	})
}

func (s *Server) pipeline(step string) {
	s.broadcast(Event{Type: "pipeline", Payload: PipelinePayload{Step: step}})
}

func (s *Server) terminal(line string) {
	s.broadcast(Event{Type: "terminal", Payload: TerminalPayload{Line: line}})
}

func (s *Server) terminalError(line string) {
	s.broadcast(Event{Type: "terminal", Payload: TerminalPayload{Line: line, Level: "error"}})
}

func (s *Server) startJob() bool {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return false
	}
	s.busy = true
	s.mu.Unlock()
	s.broadcast(Event{Type: "busy", Payload: BusyPayload{Busy: true}})
	return true
}

func (s *Server) finishJob() {
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
	s.broadcast(Event{Type: "busy", Payload: BusyPayload{Busy: false}})
}

func (s *Server) broadcast(event Event) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for client := range s.clients {
		client.enqueue(payload)
	}
}

func (s *Server) registerClient(client *wsClient) {
	s.mu.Lock()
	s.clients[client] = struct{}{}
	mode := s.mode
	awsAvailable := s.awsAvailable
	s.mu.Unlock()

	client.enqueue(mustJSON(Event{
		Type:    "mode",
		Payload: ModePayload{Current: mode, AWSAvailable: awsAvailable},
	}))
}

func (s *Server) unregisterClient(client *wsClient) {
	s.mu.Lock()
	delete(s.clients, client)
	s.mu.Unlock()
}

func streamLines(s *Server, content string) {
	trimmed := strings.TrimRight(content, "\n")
	if trimmed == "" {
		return
	}
	for _, line := range strings.Split(trimmed, "\n") {
		s.terminal(line)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func mustJSON(event Event) []byte {
	data, err := json.Marshal(event)
	if err != nil {
		return []byte("{}")
	}
	return data
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
