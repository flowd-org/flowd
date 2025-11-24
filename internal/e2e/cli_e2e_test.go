//go:build !windows

// SPDX-License-Identifier: AGPL-3.0-or-later
// SC-REF: SC701 (Phase 7 — Usability via aliases/completion)
// SC-REF: SC703 (Phase 7 — Completion latency thresholds)
// Non-functional traceability tags for reviewer mapping.
package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

var flowdBinary string
var cliIdempotencySeq uint64

func TestMain(m *testing.M) {
	bin, err := buildFlowdBinary()
	if err != nil {
		panic(err)
	}
	flowdBinary = bin
	code := m.Run()
	_ = os.Remove(flowdBinary)
	os.Exit(code)
}

func TestCLIJobsJSON(t *testing.T) {
	workspace := setupWorkspace(t)
	out := runCommand(t, workspace, flowdBinary, ":jobs", "--json")

	var payload struct {
		Jobs []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Summary string `json:"summary"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("failed to decode jobs JSON: %v\n%s", err, out)
	}
	if len(payload.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d (%v)", len(payload.Jobs), payload.Jobs)
	}
	job := payload.Jobs[0]
	if job.ID != "demo" {
		t.Fatalf("expected job id demo, got %v", job.ID)
	}
	if job.Name != "Demo Job" {
		t.Fatalf("expected job name Demo Job, got %v", job.Name)
	}
}

func TestCLIJobsTableTTY(t *testing.T) {
	workspace := setupWorkspace(t)
	cmd := exec.Command(flowdBinary, ":jobs")
	cmd.Dir = workspace
	dataDir := workspaceDataDir(workspace)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("create data dir: %v", err)
	}
	cmd.Env = append(os.Environ(), "FLWD_PROFILE=secure", "DATA_DIR="+dataDir)

	master, err := startPTYCommand(cmd)
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) || isInvalidCTTY(err) {
			t.Skipf("pty unavailable: %v", err)
		}
		t.Fatalf("failed to start PTY: %v", err)
	}
	defer func() { _ = master.Close() }()

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&buf, master)
		done <- err
	}()

	if err := cmd.Wait(); err != nil {
		t.Fatalf(":jobs exited with error: %v\n%s", err, buf.String())
	}
	<-done

	normalized := normalizeTableOutput(buf.String())
	golden := loadGolden(t, "jobs_table.golden")
	if normalized != golden {
		t.Fatalf("table output mismatch\nexpected:\n%q\nactual:\n%q", golden, normalized)
	}
}

func TestCLIPlanJSON(t *testing.T) {
	workspace := setupWorkspace(t)
	out := runCommand(t, workspace, flowdBinary, ":plan", "demo", "--json", "--profile", "permissive")

	var plan map[string]any
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("failed to decode plan JSON: %v\n%s", err, out)
	}
	jobID, _ := plan["job_id"].(string)
	if !strings.HasSuffix(jobID, "demo") {
		t.Fatalf("expected job_id to end with demo, got %v", plan["job_id"])
	}
	if profile := plan["security_profile"]; profile != "permissive" {
		t.Fatalf("expected security_profile permissive, got %v", profile)
	}
	eff, ok := plan["effective_argspec"].(map[string]any)
	if !ok || len(eff) == 0 {
		t.Fatalf("expected effective_argspec to be populated, got %v", plan["effective_argspec"])
	}
}

func TestCLINonContainerDemos(t *testing.T) {
	workspace := setupWorkspaceWithScripts(t, "demo")
	createNonContainerDagDemo(t, workspace)

	out := runCommand(t, workspace, flowdBinary, "demo", "--name", "Casey")
	if !strings.Contains(out, "demo: hello Casey") {
		t.Fatalf("expected greeting for Casey, got %s", out)
	}
	runDir := latestRunDir(t, workspace)
	stdout := mustReadFile(t, filepath.Join(runDir, "stdout"))
	if !strings.Contains(stdout, "demo: completed") {
		t.Fatalf("expected run stdout to include completion marker, got %s", stdout)
	}

	runCommand(t, workspace, flowdBinary, "demo-dag")
	runDir = latestRunDir(t, workspace)
	stdout = mustReadFile(t, filepath.Join(runDir, "stdout"))
	if !strings.Contains(stdout, "second_step") || !strings.Contains(stdout, "first_step") {
		t.Fatalf("expected stdout to include both step outputs, got %s", stdout)
	}
}

func TestCLIContainerDemos(t *testing.T) {
	workspace := setupWorkspaceWithScripts(t, "demo-container", "demo-dag-container")
	stubPath := addContainerRuntimeStub(t, workspace)
	pathEnv := "PATH=" + stubPath + string(os.PathListSeparator) + os.Getenv("PATH")

	out := runCommandWithEnv(t, workspace, flowdBinary, []string{pathEnv}, "demo-container", "--name", "Morgan")
	if !strings.Contains(out, "Hello Morgan from inside a container!") {
		t.Fatalf("expected container greeting, got %s", out)
	}
	runDir := latestRunDir(t, workspace)
	stdout := mustReadFile(t, filepath.Join(runDir, "stdout"))
	if !strings.Contains(stdout, "Hello Morgan") {
		t.Fatalf("expected stdout to include greeting, got %s", stdout)
	}

	runCommandWithEnv(t, workspace, flowdBinary, []string{pathEnv}, "demo-dag-container")
	runDir = latestRunDir(t, workspace)
	state := mustReadFile(t, filepath.Join(runDir, "state.txt"))
	if strings.TrimSpace(state) != "step1\nstep2" {
		t.Fatalf("unexpected state.txt contents: %q", state)
	}
	stdout = mustReadFile(t, filepath.Join(runDir, "stdout"))
	if !strings.Contains(stdout, "step2") {
		t.Fatalf("expected stdout to include step output, got %s", stdout)
	}
}

func TestCLIServeMode(t *testing.T) {
	workspace := setupWorkspaceWithScripts(t, "demo")
	stubPath := addContainerRuntimeStub(t, workspace)
	addr := listenAddress(t)

	cmd := exec.Command(flowdBinary, ":serve", "--bind", addr, "--dev", "--log", "json")
	cmd.Dir = workspace
	dataDir := workspaceDataDir(workspace)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("create data dir: %v", err)
	}
	env := append(os.Environ(),
		"FLWD_PROFILE=secure",
		"DATA_DIR="+dataDir,
		fmt.Sprintf("PATH=%s%s%s", stubPath, string(os.PathListSeparator), os.Getenv("PATH")),
	)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start :serve: %v", err)
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()
	var serveExited bool
	var serveErr error
	stop := func() {
		if serveExited {
			return
		}
		if cmd.Process == nil {
			return
		}
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case err := <-waitCh:
			serveExited = true
			serveErr = err
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			err := <-waitCh
			serveExited = true
			serveErr = err
		}
	}
	t.Cleanup(func() {
		stop()
		if t.Failed() {
			t.Logf(":serve stdout:\n%s", stdout.String())
			t.Logf(":serve stderr:\n%s", stderr.String())
			if serveErr != nil {
				t.Logf(":serve error: %v", serveErr)
			}
		}
	})

	waitForServe(t, cmd, addr, waitCh, &serveExited, &serveErr, &stderr)

	client := &http.Client{Timeout: 2 * time.Second}

	// GET /jobs
	jobsResp := httpGet(t, client, addr, "/jobs")
	defer jobsResp.Body.Close()
	if jobsResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(jobsResp.Body)
		t.Fatalf("GET /jobs status=%d body=%s", jobsResp.StatusCode, string(body))
	}
	var jobs []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(jobsResp.Body).Decode(&jobs); err != nil {
		t.Fatalf("decode jobs: %v", err)
	}
	foundDemo := false
	for _, job := range jobs {
		if job.ID == "demo" {
			foundDemo = true
			break
		}
	}
	if !foundDemo {
		t.Fatalf("expected demo job in jobs listing, got %#v", jobs)
	}

	healthResp := httpGet(t, client, addr, "/health/storage")
	defer healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(healthResp.Body)
		t.Fatalf("GET /health/storage status=%d body=%s", healthResp.StatusCode, string(body))
	}
	var storageHealth map[string]any
	if err := json.NewDecoder(healthResp.Body).Decode(&storageHealth); err != nil {
		t.Fatalf("decode storage health: %v", err)
	}
	if ok, _ := storageHealth["ok"].(bool); !ok {
		t.Fatalf("expected storage health ok=true, got %v", storageHealth)
	}

	// POST /plans
	planBody := `{"job_id":"demo","args":{"name":"Avery"}}`
	planResp := httpPost(t, client, addr, "/plans", planBody)
	defer planResp.Body.Close()
	if planResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(planResp.Body)
		t.Fatalf("POST /plans status=%d body=%s", planResp.StatusCode, string(body))
	}
	var plan struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(planResp.Body).Decode(&plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if plan.JobID != "demo" {
		t.Fatalf("expected plan job_id \"demo\", got %q", plan.JobID)
	}

	// POST /runs
	runResp := httpPost(t, client, addr, "/runs", `{"job_id":"demo","args":{"name":"Jordan"}}`)
	defer runResp.Body.Close()
	if runResp.StatusCode != http.StatusAccepted && runResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(runResp.Body)
		t.Fatalf("POST /runs status=%d body=%s", runResp.StatusCode, string(body))
	}
	var runPayload struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(runResp.Body).Decode(&runPayload); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if runPayload.ID == "" {
		t.Fatalf("expected run id in response")
	}

	runStatus := awaitRunCompletion(t, client, addr, runPayload.ID)
	if runStatus != "completed" {
		t.Fatalf("expected run status completed, got %s", runStatus)
	}

	// SSE resume with expired cursor should yield 410 cursor-expired
	req410, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/runs/%s/events", addr, runPayload.ID), nil)
	if err != nil {
		t.Fatalf("build SSE resume request: %v", err)
	}
	req410.Header.Set("Last-Event-ID", "0")
	resp410, err := client.Do(req410)
	if err != nil {
		t.Fatalf("GET /runs/%s/events: %v", runPayload.ID, err)
	}
	defer resp410.Body.Close()
	if resp410.StatusCode != http.StatusGone {
		body, _ := io.ReadAll(resp410.Body)
		t.Fatalf("expected 410 cursor expired, got %d (%s)", resp410.StatusCode, string(body))
	}
	var problem410 map[string]any
	if err := json.NewDecoder(resp410.Body).Decode(&problem410); err != nil {
		t.Fatalf("decode 410 problem: %v", err)
	}
	if problem410["type"] != "https://flowd.dev/problems/cursor-expired" {
		t.Fatalf("expected cursor-expired type, got %v", problem410["type"])
	}

	// Idempotency replay should succeed; conflict on body mismatch should return 409
	idemURL := fmt.Sprintf("http://%s/runs", addr)
	bodyGood := `{"job_id":"demo","args":{"name":"Casey"}}`
	reqIdem, err := http.NewRequest(http.MethodPost, idemURL, strings.NewReader(bodyGood))
	if err != nil {
		t.Fatalf("build idempotent request: %v", err)
	}
	reqIdem.Header.Set("Content-Type", "application/json")
	if err := addSpecificIdempotencyHeader(reqIdem, "cli-e2e-idem", bodyGood); err != nil {
		t.Fatalf("set idempotency header: %v", err)
	}
	firstIdem := httpDo(t, client, reqIdem)
	if firstIdem.StatusCode != http.StatusAccepted && firstIdem.StatusCode != http.StatusCreated && firstIdem.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(firstIdem.Body)
		firstIdem.Body.Close()
		t.Fatalf("expected success for first idempotent request, got %d (%s)", firstIdem.StatusCode, string(body))
	}
	firstIdem.Body.Close()

	// Reuse key with different body triggers 409 conflict
	bodyConflict := `{"job_id":"demo","args":{"name":"Taylor"}}`
	reqConflict, err := http.NewRequest(http.MethodPost, idemURL, strings.NewReader(bodyConflict))
	if err != nil {
		t.Fatalf("build conflict request: %v", err)
	}
	reqConflict.Header.Set("Content-Type", "application/json")
	if err := addSpecificIdempotencyHeader(reqConflict, "cli-e2e-idem", bodyConflict); err != nil {
		t.Fatalf("set conflict headers: %v", err)
	}
	respConflict := httpDo(t, client, reqConflict)
	defer respConflict.Body.Close()
	if respConflict.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(respConflict.Body)
		t.Fatalf("expected 409 conflict, got %d (%s)", respConflict.StatusCode, string(body))
	}
	var prob409 map[string]any
	if err := json.NewDecoder(respConflict.Body).Decode(&prob409); err != nil {
		t.Fatalf("decode conflict problem: %v", err)
	}
	if prob409["type"] != "https://flowd.dev/problems/idempotency-key-conflict" {
		t.Fatalf("expected idempotency conflict type, got %v", prob409["type"])
	}
}

func TestCLIServeConformanceErrors(t *testing.T) {
	workspace := setupWorkspace(t)
	addr := listenAddress(t)

	cmd := exec.Command(flowdBinary, ":serve", "--bind", addr, "--log", "json")
	cmd.Dir = workspace
	dataDir := workspaceDataDir(workspace)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("create data dir: %v", err)
	}
	cmd.Env = append(os.Environ(), "FLWD_PROFILE=secure", "DATA_DIR="+dataDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start :serve: %v", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	serveExited := false
	var serveErr error
	stop := func() {
		if serveExited {
			return
		}
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case err := <-waitCh:
			serveExited = true
			serveErr = err
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			err := <-waitCh
			serveExited = true
			serveErr = err
		}
	}

	t.Cleanup(func() {
		stop()
		if t.Failed() {
			t.Logf(":serve stdout:\n%s", stdout.String())
			t.Logf(":serve stderr:\n%s", stderr.String())
			if serveErr != nil {
				t.Logf(":serve error: %v", serveErr)
			}
		}
	})

	waitForServe(t, cmd, addr, waitCh, &serveExited, &serveErr, &stderr)

	client := &http.Client{Timeout: 2 * time.Second}

	readProblem := func(resp *http.Response) map[string]any {
		t.Helper()
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var body map[string]any
		if err := json.Unmarshal(data, &body); err != nil {
			t.Fatalf("decode problem: %v\n%s", err, string(data))
		}
		return body
	}

	// 401 — missing bearer token on protected read.
	unauth := httpGet(t, client, addr, "/jobs")
	if unauth.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(unauth.Body)
		unauth.Body.Close()
		t.Fatalf("expected 401 unauthorized, got %d (%s)", unauth.StatusCode, string(body))
	}
	if challenge := unauth.Header.Get("WWW-Authenticate"); !strings.Contains(strings.ToLower(challenge), "bearer") {
		t.Fatalf("expected WWW-Authenticate Bearer challenge, got %q", challenge)
	}
	unauthProb := readProblem(unauth)
	if title, _ := unauthProb["title"].(string); title != "unauthorized" {
		t.Fatalf("expected unauthorized title, got %q", title)
	}

	// 403 — insufficient scope on runs read.
	req403, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/runs", addr), nil)
	if err != nil {
		t.Fatalf("build 403 request: %v", err)
	}
	req403.Header.Set("Authorization", "Bearer jobs:read")
	resp403, err := client.Do(req403)
	if err != nil {
		t.Fatalf("GET /runs: %v", err)
	}
	if resp403.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp403.Body)
		resp403.Body.Close()
		t.Fatalf("expected 403 forbidden, got %d (%s)", resp403.StatusCode, string(body))
	}
	prob403 := readProblem(resp403)
	if detail, _ := prob403["detail"].(string); detail != "missing required scope" {
		t.Fatalf("expected missing scope detail, got %q", detail)
	}

	authHeader := "Bearer runs:write runs:read jobs:read events:read"

	postJSON := func(path, body string) *http.Request {
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s%s", addr, path), strings.NewReader(body))
		if err != nil {
			t.Fatalf("build POST %s: %v", path, err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		return req
	}

	setIdem := func(req *http.Request, key string, body string) {
		canonical, err := canonicalizeJSONBody(body)
		if err != nil {
			t.Fatalf("canonicalize body: %v", err)
		}
		sum := sha256.Sum256(canonical)
		req.Header.Set("Idempotency-Key", key)
		req.Header.Set("Idempotency-SHA256", hex.EncodeToString(sum[:]))
	}

	// 400 — missing Idempotency-Key header.
	body400 := `{"job_id":"demo","args":{"name":"Casey"}}`
	req400 := postJSON("/runs", body400)
	resp400, err := client.Do(req400)
	if err != nil {
		t.Fatalf("POST /runs missing idem: %v", err)
	}
	if resp400.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp400.Body)
		resp400.Body.Close()
		t.Fatalf("expected 400 for missing idempotency, got %d (%s)", resp400.StatusCode, string(body))
	}
	prob400 := readProblem(resp400)
	if title, _ := prob400["title"].(string); title != "Idempotency-Key header required" {
		t.Fatalf("expected missing Idempotency-Key title, got %q", title)
	}

	// Successful request to seed idempotency store.
	bodyGood := `{"job_id":"demo","args":{"name":"Jordan"}}`
	conflictKey := fmt.Sprintf("cli-conflict-%d", time.Now().UnixNano())
	reqRun := postJSON("/runs", bodyGood)
	setIdem(reqRun, conflictKey, bodyGood)
	respRun, err := client.Do(reqRun)
	if err != nil {
		t.Fatalf("POST /runs success: %v", err)
	}
	if respRun.StatusCode != http.StatusAccepted && respRun.StatusCode != http.StatusCreated && respRun.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respRun.Body)
		respRun.Body.Close()
		t.Fatalf("expected 2xx for initial run, got %d (%s)", respRun.StatusCode, string(body))
	}
	var runPayload struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(respRun.Body).Decode(&runPayload); err != nil {
		respRun.Body.Close()
		t.Fatalf("decode run payload: %v", err)
	}
	respRun.Body.Close()
	if runPayload.ID == "" {
		t.Fatalf("expected run id in response")
	}

	// 409 — same key, different body.
	bodyConflict := `{"job_id":"demo","args":{"name":"Taylor"}}`
	reqConflict := postJSON("/runs", bodyConflict)
	setIdem(reqConflict, conflictKey, bodyConflict)
	respConflict, err := client.Do(reqConflict)
	if err != nil {
		t.Fatalf("POST /runs conflict: %v", err)
	}
	if respConflict.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(respConflict.Body)
		respConflict.Body.Close()
		t.Fatalf("expected 409 conflict, got %d (%s)", respConflict.StatusCode, string(body))
	}
	prob409 := readProblem(respConflict)
	if typ, _ := prob409["type"].(string); typ != "https://flowd.dev/problems/idempotency-key-conflict" {
		t.Fatalf("expected idempotency conflict type, got %q", typ)
	}

	status := awaitRunCompletionWithAuth(t, client, addr, runPayload.ID, authHeader)
	if status != "completed" {
		t.Fatalf("expected completed status, got %s", status)
	}

	// 422 — argspec validation failure.
	body422 := `{"job_id":"demo","args":{}}`
	req422 := postJSON("/runs", body422)
	setIdem(req422, fmt.Sprintf("cli-unprocessable-%d", time.Now().UnixNano()), body422)
	resp422, err := client.Do(req422)
	if err != nil {
		t.Fatalf("POST /runs arg validation: %v", err)
	}
	if resp422.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp422.Body)
		resp422.Body.Close()
		t.Fatalf("expected 422, got %d (%s)", resp422.StatusCode, string(body))
	}
	prob422 := readProblem(resp422)
	if title, _ := prob422["title"].(string); title != "argument validation failed" {
		t.Fatalf("expected argument validation failed title, got %q", title)
	}

	// 410 — resume with expired cursor.
	req410, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/runs/%s/events", addr, runPayload.ID), nil)
	if err != nil {
		t.Fatalf("build events request: %v", err)
	}
	req410.Header.Set("Authorization", authHeader)
	req410.Header.Set("Last-Event-ID", "0")
	resp410, err := client.Do(req410)
	if err != nil {
		t.Fatalf("GET /runs/%s/events: %v", runPayload.ID, err)
	}
	if resp410.StatusCode != http.StatusGone {
		body, _ := io.ReadAll(resp410.Body)
		resp410.Body.Close()
		t.Fatalf("expected 410 cursor expired, got %d (%s)", resp410.StatusCode, string(body))
	}
	prob410 := readProblem(resp410)
	if typ, _ := prob410["type"].(string); typ != "https://flowd.dev/problems/cursor-expired" {
		t.Fatalf("expected cursor-expired type, got %q", typ)
	}
}

func TestCompletionEntrypointContract(t *testing.T) {
	workspace := setupCompletionWorkspace(t)

	out := runCommand(t, workspace, flowdBinary, "__complete", "1")
	cands := parseCompletionCandidates(t, out)
	assertCandidate(t, cands, "segment", "alpha")
	assertCandidate(t, cands, "segment", "ship")
	rejectCandidate(t, cands, "segment", ":plan")

	out = runCommand(t, workspace, flowdBinary, "__complete", "2", "sh")
	cands = parseCompletionCandidates(t, out)
	assertCandidate(t, cands, "segment", "ship")

	out = runCommand(t, workspace, flowdBinary, "__complete", "2", ":")
	cands = parseCompletionCandidates(t, out)
	assertCandidate(t, cands, "segment", ":plan")

	out = runCommand(t, workspace, flowdBinary, "__complete", "4", "alpha", "deploy", "")
	cands = parseCompletionCandidates(t, out)
	assertCandidate(t, cands, "flag", "--env")
	assertCandidate(t, cands, "flag", "--strategy")

	out = runCommand(t, workspace, flowdBinary, "__complete", "4", "alpha", "deploy", "--", "--e")
	cands = parseCompletionCandidates(t, out)
	assertCandidate(t, cands, "flag", "--env")

	out = runCommand(t, workspace, flowdBinary, "__complete", "5", "alpha", "deploy", "--", "--env", "")
	cands = parseCompletionCandidates(t, out)
	assertCandidate(t, cands, "value", "prod")
	assertCandidate(t, cands, "value", "staging")

	out = runCommand(t, workspace, flowdBinary, "__complete", "5", "alpha", "deploy", "--", "--env", "p")
	cands = parseCompletionCandidates(t, out)
	if !hasCandidate(cands, "value", "prod") {
		t.Fatalf("expected prod candidate, got %#v", cands)
	}
	if len(cands) != 1 {
		t.Fatalf("expected only prod candidate, got %#v", cands)
	}
}

// Helpers --------------------------------------------------------------------

func buildFlowdBinary() (string, error) {
	root := repoRoot()
	binDir, err := os.MkdirTemp("", "flowd-bin")
	if err != nil {
		return "", err
	}
	binPath := filepath.Join(binDir, "flowd-e2e")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/flowd")
	cmd.Dir = root
	cacheDir := filepath.Join(binDir, "gocache")
	modCache := filepath.Join(binDir, "gomodcache")
	if err := os.Mkdir(cacheDir, 0o755); err != nil {
		return "", err
	}
	if err := os.Mkdir(modCache, 0o755); err != nil {
		return "", err
	}
	cmd.Env = append(os.Environ(), "GOCACHE="+cacheDir, "GOMODCACHE="+modCache)
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return binPath, nil
}

func workspaceDataDir(dir string) string {
	return filepath.Join(dir, ".flowd")
}

func setupWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	scriptDir := filepath.Join(dir, "scripts", "demo")
	if err := os.MkdirAll(filepath.Join(scriptDir, "config.d"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}

	config := `version: v1
job:
  id: demo
  name: Demo Job
  summary: Prints greeting
argspec:
  args:
    - name: name
      type: string
      required: true
steps:
  - id: greet
    script: scripts/demo/hello.sh
`
	configPath := filepath.Join(scriptDir, "config.d", "config.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	hello := "#!/usr/bin/env bash\necho \"Hello ${name:-world}\" > \"$FLWD_RUN_DIR/hello.txt\"\n"
	scriptPath := filepath.Join(scriptDir, "hello.sh")
	if err := os.WriteFile(scriptPath, []byte(hello), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	return dir
}

func runCommand(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	dataDir := workspaceDataDir(dir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("create data dir: %v", err)
	}
	cmd.Env = append(os.Environ(), "FLWD_PROFILE=secure", "DATA_DIR="+dataDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func runCommandWithEnv(t *testing.T, dir string, name string, extraEnv []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	dataDir := workspaceDataDir(dir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("create data dir: %v", err)
	}
	env := append(os.Environ(), "FLWD_PROFILE=secure", "DATA_DIR="+dataDir)
	env = append(env, extraEnv...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func normalizeTableOutput(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		normalized = append(normalized, strings.Join(fields, "\t"))
	}
	return strings.Join(normalized, "\n")
}

func loadGolden(t *testing.T, name string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return strings.TrimSpace(string(data))
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func setupWorkspaceWithScripts(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir scripts dir: %v", err)
	}
	root := repoRoot()
	for _, name := range names {
		src := filepath.Join(root, "scripts", name)
		if _, err := os.Stat(src); err != nil {
			t.Fatalf("missing script source %s: %v", src, err)
		}
		dst := filepath.Join(scriptsDir, name)
		copyDir(t, src, dst)
	}
	return dir
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm())
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
	if err != nil {
		t.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
}

func setupCompletionWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir scripts dir: %v", err)
	}

	buildConfigDir := filepath.Join(scriptsDir, "alpha", "build", "config.d")
	deployConfigDir := filepath.Join(scriptsDir, "alpha", "deploy", "config.d")
	for _, path := range []string{buildConfigDir, deployConfigDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}

	buildConfig := `version: v1
job:
  id: alpha.build
  name: Alpha Build
argspec:
  args:
    - name: target
      type: string
      enum:
        - debug
        - release
steps:
  - id: build
    script: scripts/alpha/build/run.sh
`
	if err := os.WriteFile(filepath.Join(buildConfigDir, "config.yaml"), []byte(buildConfig), 0o644); err != nil {
		t.Fatalf("write build config: %v", err)
	}

	deployConfig := `version: v1
job:
  id: alpha.deploy
  name: Alpha Deploy
argspec:
  args:
    - name: env
      type: string
      required: true
      enum:
        - prod
        - staging
    - name: strategy
      type: string
      enum:
        - rolling
        - blue-green
steps:
  - id: deploy
    script: scripts/alpha/deploy/run.sh
`
	if err := os.WriteFile(filepath.Join(deployConfigDir, "config.yaml"), []byte(deployConfig), 0o644); err != nil {
		t.Fatalf("write deploy config: %v", err)
	}

	buildScript := "#!/usr/bin/env bash\necho build \"$target\" > \"$FLWD_RUN_DIR/build.log\"\n"
	if err := os.WriteFile(filepath.Join(scriptsDir, "alpha", "build", "run.sh"), []byte(buildScript), 0o755); err != nil {
		t.Fatalf("write build script: %v", err)
	}
	deployScript := "#!/usr/bin/env bash\necho deploy \"$env\" \"$strategy\" > \"$FLWD_RUN_DIR/deploy.log\"\n"
	if err := os.WriteFile(filepath.Join(scriptsDir, "alpha", "deploy", "run.sh"), []byte(deployScript), 0o755); err != nil {
		t.Fatalf("write deploy script: %v", err)
	}

	aliasConfig := `aliases:
  - from: alpha/deploy
    to: ship
    description: Shortcut deploy alias
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "flwd.yaml"), []byte(aliasConfig), 0o644); err != nil {
		t.Fatalf("write flwd.yaml: %v", err)
	}

	return dir
}

func createNonContainerDagDemo(t *testing.T, workspace string) {
	t.Helper()
	base := filepath.Join(workspace, "scripts", "demo-dag")
	configDir := filepath.Join(base, "config.d")
	stepsDir := filepath.Join(base, "steps")
	for _, dir := range []string{configDir, stepsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	config := `version: v1
job:
  id: demo-dag
  name: Demo DAG job
composition: steps
executor: proc
interpreter: "/usr/bin/env bash"
steps:
  - id: first
    script: steps/first.sh
  - id: second
    script: steps/second.sh
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write dag config: %v", err)
	}

	step1 := "#!/usr/bin/env bash\nset -euo pipefail\necho first_step\n"
	if err := os.WriteFile(filepath.Join(stepsDir, "first.sh"), []byte(step1), 0o755); err != nil {
		t.Fatalf("write step1: %v", err)
	}
	step2 := "#!/usr/bin/env bash\nset -euo pipefail\necho second_step\n"
	if err := os.WriteFile(filepath.Join(stepsDir, "second.sh"), []byte(step2), 0o755); err != nil {
		t.Fatalf("write step2: %v", err)
	}
}

func addContainerRuntimeStub(t *testing.T, workspace string) string {
	t.Helper()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir stub bin: %v", err)
	}
	stub := `#!/usr/bin/env bash
set -euo pipefail
if [[ $# -lt 1 ]]; then
  exit 0
fi
cmd="$1"
shift || true
case "$cmd" in
  run)
    declare -a envs=()
    workdir=""
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --env)
          if [[ $# -lt 2 ]]; then break; fi
          envs+=("$2")
          shift 2
          ;;
        --workdir)
          if [[ $# -lt 2 ]]; then break; fi
          workdir="$2"
          shift 2
          ;;
        --volume|--network|--name)
          shift
          if [[ $# -gt 0 ]]; then
            shift
          fi
          ;;
        --cap-drop=*|--security-opt=*|--rm|--read-only|--tty|--interactive|--cap-add=*)
          shift
          ;;
        --)
          shift
          break
          ;;
        --*)
          shift
          if [[ $# -gt 0 && "$1" != --* ]]; then
            shift
          fi
          ;;
        *)
          # image reference
          shift
          break
          ;;
      esac
    done
    if [[ $# -eq 0 ]]; then
      exit 0
    fi
    script="$1"
    shift || true
    for pair in "${envs[@]}"; do
      key="${pair%%=*}"
      val="${pair#*=}"
      export "$key"="$val"
    done
    if [[ -n "${workdir:-}" ]]; then
      export RUN_DIR="$workdir"
      export FLWD_RUN_DIR="$workdir"
      cd "$workdir"
    fi
    "$script" "$@"
    ;;
  stop|kill|rm|inspect)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	stubPath := filepath.Join(binDir, "podman")
	if err := os.WriteFile(stubPath, []byte(stub), 0o755); err != nil {
		t.Fatalf("write podman stub: %v", err)
	}
	dockerPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(dockerPath, []byte(stub), 0o755); err != nil {
		t.Fatalf("write docker stub: %v", err)
	}
	return binDir
}

func latestRunDir(t *testing.T, workspace string) string {
	t.Helper()
	runsDir := filepath.Join(workspaceDataDir(workspace), "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		t.Fatalf("list runs dir: %v", err)
	}
	var latestPath string
	var latestTime time.Time
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("stat %s: %v", entry.Name(), err)
		}
		if latestPath == "" || info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestPath = filepath.Join(runsDir, entry.Name())
		}
	}
	if latestPath == "" {
		t.Fatalf("no runs recorded under %s", runsDir)
	}
	return latestPath
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func listenAddress(t *testing.T) string {
	t.Helper()
	seed := time.Now().UnixNano()
	r := rand.New(rand.NewSource(seed))
	for i := 0; i < 10; i++ {
		port := 35000 + r.Intn(20000)
		return fmt.Sprintf("127.0.0.1:%d", port)
	}
	return "127.0.0.1:35897"
}

func waitForServe(t *testing.T, cmd *exec.Cmd, addr string, waitCh <-chan error, exited *bool, serveErr *error, stderr *bytes.Buffer) {
	t.Helper()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(10 * time.Second)
	url := fmt.Sprintf("http://%s/healthz", addr)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusNoContent {
				return
			}
		}
		select {
		case err := <-waitCh:
			if exited != nil {
				*exited = true
			}
			if serveErr != nil {
				*serveErr = err
			}
			msg := ""
			if stderr != nil {
				msg = stderr.String()
			}
			if strings.Contains(msg, "operation not permitted") {
				t.Skipf("serve mode requires network listen: %s", msg)
			}
			if err != nil {
				t.Fatalf(":serve exited early: %v\n%s", err, msg)
			}
			t.Fatalf(":serve exited unexpectedly\n%s", msg)
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become ready", addr)
}

func httpGet(t *testing.T, client *http.Client, addr, path string) *http.Response {
	t.Helper()
	url := fmt.Sprintf("http://%s%s", addr, path)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func httpPost(t *testing.T, client *http.Client, addr, path, body string) *http.Response {
	t.Helper()
	url := fmt.Sprintf("http://%s%s", addr, path)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build POST %s: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.HasPrefix(path, "/runs") {
		if err := addIdempotencyHeaders(req, body); err != nil {
			t.Fatalf("add idempotency headers: %v", err)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func addIdempotencyHeaders(req *http.Request, body string) error {
	seq := atomic.AddUint64(&cliIdempotencySeq, 1)
	key := fmt.Sprintf("cli-idem-%012d", seq)
	req.Header.Set("Idempotency-Key", key)

	canonical, err := canonicalizeJSONBody(body)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(canonical)
	req.Header.Set("Idempotency-SHA256", hex.EncodeToString(sum[:]))
	return nil
}

func canonicalizeJSONBody(raw string) ([]byte, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var val any
	if err := dec.Decode(&val); err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	if err := encodeCanonicalJSON(buf, val); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func httpDo(t *testing.T, client *http.Client, req *http.Request) *http.Response {
	t.Helper()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.String(), err)
	}
	return resp
}

func addSpecificIdempotencyHeader(req *http.Request, key, body string) error {
	canonical, err := canonicalizeJSONBody(body)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(canonical)
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("Idempotency-SHA256", hex.EncodeToString(sum[:]))
	return nil
}

func encodeCanonicalJSON(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONString(buf, k)
			buf.WriteByte(':')
			if err := encodeCanonicalJSON(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, elem := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeCanonicalJSON(buf, elem); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case string:
		writeJSONString(buf, t)
	case json.Number:
		buf.WriteString(t.String())
	case float64:
		buf.WriteString(strconv.FormatFloat(t, 'f', -1, 64))
	case int:
		buf.WriteString(strconv.Itoa(t))
	case int64:
		buf.WriteString(strconv.FormatInt(t, 10))
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case nil:
		buf.WriteString("null")
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}

func writeJSONString(buf *bytes.Buffer, s string) {
	b, _ := json.Marshal(s)
	buf.Write(b)
}

func awaitRunCompletion(t *testing.T, client *http.Client, addr, runID string) string {
	return awaitRunCompletionWithAuth(t, client, addr, runID, "")
}

func awaitRunCompletionWithAuth(t *testing.T, client *http.Client, addr, runID, authHeader string) string {
	t.Helper()
	url := fmt.Sprintf("http://%s/runs/%s", addr, runID)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("build run status request: %v", err)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		var payload struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if payload.Status == "completed" || payload.Status == "failed" {
			return payload.Status
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach terminal state", runID)
	return ""
}

type completionCandidateRecord struct {
	Insert  string `json:"insert"`
	Display string `json:"display"`
	Type    string `json:"type"`
}

func parseCompletionCandidates(t *testing.T, out string) []completionCandidateRecord {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(out))
	var results []completionCandidateRecord
	for {
		var cand completionCandidateRecord
		if err := dec.Decode(&cand); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("decode completion candidate: %v", err)
		}
		results = append(results, cand)
	}
	return results
}

func assertCandidate(t *testing.T, cands []completionCandidateRecord, typ, insert string) {
	t.Helper()
	if !hasCandidate(cands, typ, insert) {
		t.Fatalf("expected candidate %s %q, got %#v", typ, insert, cands)
	}
}

func rejectCandidate(t *testing.T, cands []completionCandidateRecord, typ, insert string) {
	t.Helper()
	if hasCandidate(cands, typ, insert) {
		t.Fatalf("did not expect candidate %s %q, got %#v", typ, insert, cands)
	}
}

func hasCandidate(cands []completionCandidateRecord, typ, insert string) bool {
	for _, cand := range cands {
		if cand.Type == typ && cand.Insert == insert {
			return true
		}
	}
	return false
}

func isInvalidCTTY(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Setctty set but Ctty not valid in child")
}

// startPTYCommand launches cmd attached to a pseudo terminal and returns the
// master side for reading/writing.
func startPTYCommand(cmd *exec.Cmd) (*os.File, error) {
	master, slave, err := openPTY()
	if err != nil {
		return nil, err
	}

	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.Stdin = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: int(slave.Fd())}

	if err := cmd.Start(); err != nil {
		_ = master.Close()
		_ = slave.Close()
		return nil, err
	}
	_ = slave.Close()
	return master, nil
}

// openPTY opens a new pseudo terminal pair using Linux ioctl calls.
func openPTY() (*os.File, *os.File, error) {
	const (
		oNoCTTY         = syscall.O_NOCTTY
		ioctlTIOCSPTLCK = 0x40045431
		ioctlTIOCGPTN   = 0x80045430
	)

	masterFD, err := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_CLOEXEC|oNoCTTY, 0)
	if err != nil {
		return nil, nil, err
	}

	unlock := uint32(0)
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFD), ioctlTIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		_ = syscall.Close(masterFD)
		return nil, nil, errno
	}

	var ptyNumber uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFD), ioctlTIOCGPTN, uintptr(unsafe.Pointer(&ptyNumber))); errno != 0 {
		_ = syscall.Close(masterFD)
		return nil, nil, errno
	}

	slaveName := fmt.Sprintf("/dev/pts/%d", ptyNumber)
	slaveFD, err := syscall.Open(slaveName, syscall.O_RDWR|oNoCTTY, 0)
	if err != nil {
		_ = syscall.Close(masterFD)
		return nil, nil, err
	}

	master := os.NewFile(uintptr(masterFD), "pty-master")
	slave := os.NewFile(uintptr(slaveFD), "pty-slave")
	return master, slave, nil
}
