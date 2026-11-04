package sessionmanager

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/runtime/conpty"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// review6869WindowsHelperAgent is a real terminal child without a dependency
// on /bin/sh. TestMain dispatches its private argv before normal tests.
func review6869WindowsHelperAgent() int {
	if err := os.WriteFile(os.Getenv("AO_REVIEW6869_READY"), []byte("ready"), 0o600); err != nil {
		return 2
	}
	f, err := os.OpenFile(os.Getenv("AO_REVIEW6869_INPUT"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 3
	}
	defer f.Close()
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		_, _ = fmt.Fprintln(f, line)
		if line == "__exit__" {
			return 0
		}
	}
	return 0
}

func review6869WaitForFile(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		raw, _ := os.ReadFile(path)
		if strings.Contains(string(raw), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("waiting for %q in %s; got %q", want, path, raw)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestReview6869WindowsNativeLegacySwitchRecovery(t *testing.T) {
	if runtime.GOOS != "windows" && os.Getenv("AO_REVIEW6869_ALLOW_OTHER_OS") != "1" {
		t.Skip("requires native Windows")
	}
	mode := os.Getenv("AO_REVIEW6869_EXPECT")
	if mode == "" {
		t.Skip("set AO_REVIEW6869_EXPECT=head or base-runtime")
	}
	if mode != "head" && mode != "base-runtime" {
		t.Fatal("invalid expected mode")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	supervisor := os.Getenv("AO_REVIEW6869_AO_BIN")
	if supervisor == "" {
		supervisor = exe
	}
	for _, state := range []domain.AgentSwitchState{domain.AgentSwitchStoppingSource, domain.AgentSwitchStartingTarget} {
		t.Run(string(state), func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			runPath := filepath.Join(dir, "running.json")
			ready, input := filepath.Join(dir, "agent.ready"), filepath.Join(dir, "agent.input")
			rt := conpty.New(conpty.Options{RunFilePath: runPath})
			generation := "source-generation"
			if state == domain.AgentSwitchStartingTarget {
				generation = "target-generation"
			}
			handle, err := rt.Create(ctx, ports.RuntimeConfig{SessionID: "proj-1", WorkspacePath: dir, Argv: []string{supervisor, "agent-process", "supervise", "--session", "proj-1", "--launch", generation, "--", exe, "review6869-windows-agent"}, Env: map[string]string{"AO_RUNTIME_LAUNCH_ID": generation, "AO_REVIEW6869_READY": ready, "AO_REVIEW6869_INPUT": input, "AO_RUN_FILE": runPath, "AO_DATA_DIR": filepath.Join(dir, "data"), "AO_TELEMETRY_REMOTE": "off"}})
			if err != nil {
				t.Fatalf("real native Create: %v", err)
			}
			t.Cleanup(func() {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := rt.Destroy(cleanupCtx, handle); err != nil {
					t.Errorf("real native cleanup: %v", err)
				}
			})
			review6869WaitForFile(t, ready, "ready")
			ref := ports.FencedRuntimeRef{Handle: handle, SessionID: "proj-1", Generation: generation}
			if probe := rt.ProbeFencedRuntime(ctx, ref); probe.Liveness != ports.FencedAlive {
				t.Fatalf("new live launch: %+v", probe)
			}
			if err := rt.SendMessage(ctx, handle, "native-agent-receipt"); err != nil {
				t.Fatal(err)
			}
			review6869WaitForFile(t, input, "native-agent-receipt")
			registry := filepath.Join(dir, "windows-pty-hosts.json")
			raw, err := os.ReadFile(registry)
			if err != nil {
				t.Fatal(err)
			}
			var entries []map[string]any
			if err := json.Unmarshal(raw, &entries); err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				delete(entry, "supervised")
			}
			raw, err = json.Marshal(entries)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(registry, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			adopted := conpty.New(conpty.Options{RunFilePath: runPath})
			probe := adopted.ProbeFencedRuntime(ctx, ref)
			t.Logf("OS=%s expected=%s real supervisor=%s native handle=%s adopted probe=%+v; prior child receipt verified", runtime.GOOS, mode, supervisor, handle.ID, probe)
			if mode == "head" && runtime.GOOS == "windows" && (probe.Liveness != ports.FencedUnknown || string(probe.Reason) != "supervision_unknown") {
				t.Fatalf("expected permanently unavailable Windows identity: %+v", probe)
			}
			if mode == "base-runtime" && probe.Liveness != ports.FencedAlive {
				t.Fatalf("expected base live evidence: %+v", probe)
			}
			manager, store, _ := newSwitchTestManager(t, adopted)
			now := time.Now().UTC()
			native := domain.AgentNativeSession{ID: "native-target", AOSessionID: "proj-1", Harness: domain.HarnessCodex, NativeSessionID: "codex-target", LastGenerationID: "target-generation", CreatedAt: now, LastUsedAt: now}
			store.native[native.ID] = native
			nativeRef := native.ID
			sw := domain.AgentSwitch{ID: "legacy-native-switch", SessionID: "proj-1", IdempotencyKey: "legacy-native", RequestFingerprint: domain.ComputeAgentSwitchRequestFingerprint("proj-1", domain.HarnessCodex, ""), FromHarness: domain.HarnessClaudeCode, TargetHarness: domain.HarnessCodex, State: state, AgentHandoffStatus: domain.AgentHandoffUnavailable, SourceGenerationID: "source-generation", TargetGenerationID: "target-generation", TargetRuntimeHandleID: handle.ID, TargetNativeSessionRef: &nativeRef, TargetStartMode: domain.AgentSwitchTargetStartFresh, RequestedAt: now, UpdatedAt: now}
			store.switches[sw.ID] = sw
			rec := store.sessions["proj-1"]
			rec.Activity = domain.Activity{State: domain.ActivityIdle, LastActivityAt: now}
			if state == domain.AgentSwitchStartingTarget {
				rec.Activity.State = domain.ActivityExited
			}
			store.sessions[rec.ID] = rec
			for attempt := 1; attempt <= 3; attempt++ {
				resolved, recoverErr := manager.reconcileRetainedAgentSwitchOnce(ctx, store, rec.ID)
				t.Logf("recovery attempt %d resolved=%v err=%v gate=%v", attempt, resolved, recoverErr, manager.SessionMutationInProgress(rec.ID))
				wantGated := mode == "head" && runtime.GOOS == "windows"
				if wantGated && (resolved || recoverErr == nil || !manager.SessionMutationInProgress(rec.ID)) {
					t.Fatal("Windows legacy unknown host unexpectedly recovered")
				}
				if !wantGated && (!resolved || recoverErr != nil || manager.SessionMutationInProgress(rec.ID)) {
					t.Fatal("confirmed live base/Unix host failed recovery")
				}
			}
			if mode == "head" && runtime.GOOS == "windows" {
				_, resumeErr := manager.ResumeAgentWithMode(ctx, rec.ID)
				_, exitErr := manager.ExitAgent(ctx, rec.ID)
				_, killErr := manager.Kill(ctx, rec.ID)
				if !errors.Is(resumeErr, ErrSwitchInProgress) || !errors.Is(exitErr, ErrAgentExitInProgress) || !errors.Is(killErr, ErrSwitchInProgress) {
					t.Fatalf("relaunch escape: Resume=%v Exit=%v Kill=%v", resumeErr, exitErr, killErr)
				}
				t.Logf("blocked escape APIs Resume=%v Exit=%v Kill=%v", resumeErr, exitErr, killErr)
			}
			if alive, err := adopted.IsChildAlive(ctx, handle); err != nil || !alive {
				t.Fatalf("healthy supervisor stopped: %v,%v", alive, err)
			}
			if err := adopted.SendMessage(ctx, handle, "still-healthy-after-recovery"); err != nil {
				t.Fatal(err)
			}
			review6869WaitForFile(t, input, "still-healthy-after-recovery")
			t.Logf("real native child remained healthy and read independent receipt after recovery; final gate=%v", manager.SessionMutationInProgress(rec.ID))
		})
	}
}
