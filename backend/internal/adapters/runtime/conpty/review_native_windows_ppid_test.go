//go:build windows

package conpty

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Native Windows reproduction only. Opt in with AO_REVIEW_NATIVE_WINDOWS=1.
// C is a real suspended process with no child, standing in for a supervisor
// before child.Start. The OS, rather than a model, must actually reuse A's PID.
// A skipped test means reuse was not observed and is NOT a successful repro.
func TestReviewWindowsNativeReusedParentPID(t *testing.T) {
	if os.Getenv("AO_REVIEW_NATIVE_WINDOWS") != "1" {
		t.Skip("set AO_REVIEW_NATIVE_WINDOWS=1 to run native PID reuse reproduction")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	oldParent := exec.Command(exe, "--ao-review-windows-helper", "old-parent", dir) //nolint:forbidigo // native process fixture
	oldParent.Stdout, oldParent.Stderr = os.Stdout, os.Stderr
	if err := oldParent.Start(); err != nil {
		t.Fatal(err)
	}
	oldPID := uint32(oldParent.Process.Pid)
	parentWaited := false
	t.Cleanup(func() {
		_ = os.WriteFile(filepath.Join(dir, "release-orphan"), nil, 0o600)
		_ = os.WriteFile(filepath.Join(dir, "release-parent"), nil, 0o600)
		if !parentWaited {
			_ = oldParent.Process.Kill()
			_ = oldParent.Wait()
		}
	})
	if err := reviewWindowsWaitPath(filepath.Join(dir, "orphan-ready"), 10*time.Second); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "orphan-pid"))
	if err != nil {
		t.Fatal(err)
	}
	orphanPID64, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	orphanPID := uint32(orphanPID64)
	orphanHandle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, orphanPID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(filepath.Join(dir, "release-orphan"), nil, 0o600)
		status, waitErr := windows.WaitForSingleObject(orphanHandle, 2000)
		if waitErr != nil || status != windows.WAIT_OBJECT_0 {
			_ = windows.TerminateProcess(orphanHandle, 0)
			_, _ = windows.WaitForSingleObject(orphanHandle, 5000)
		}
		_ = windows.CloseHandle(orphanHandle)
	})
	if err := os.WriteFile(filepath.Join(dir, "release-parent"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// Wait releases Go's process handle. The helper also released its handle to
	// B before exiting, so no handle intentionally keeps A's PID unavailable.
	err = oldParent.Wait()
	parentWaited = true
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("old parent A=%d exited; unrelated child B=%d remains alive", oldPID, orphanPID)

	deadline := time.Now().Add(120 * time.Second)
	var candidate windows.ProcessInformation
	attempts := 0
	for ; attempts < 10000 && time.Now().Before(deadline); attempts++ {
		pi, err := reviewWindowsStartSuspended(exe, dir)
		if err != nil {
			t.Fatal(err)
		}
		if pi.ProcessId == oldPID {
			candidate = pi
			break
		}
		if err := reviewWindowsStopProcess(pi); err != nil {
			t.Fatal(err)
		}
	}
	if candidate.Process == 0 {
		t.Skipf("INCONCLUSIVE: Windows did not reuse parent PID %d after %d candidates within 120s; no native bug reproduced", oldPID, attempts)
	}
	t.Cleanup(func() { _ = reviewWindowsStopProcess(candidate) })
	parentPID, err := reviewWindowsParentPID(orphanPID)
	if err != nil {
		t.Fatal(err)
	}
	orphanCreated, err := reviewWindowsCreatedAt(orphanHandle)
	if err != nil {
		t.Fatal(err)
	}
	candidateCreated, err := reviewWindowsCreatedAt(candidate.Process)
	if err != nil {
		t.Fatal(err)
	}
	if parentPID != candidate.ProcessId || orphanCreated >= candidateCreated {
		t.Fatalf("fixture did not establish stale ancestry: B.parent=%d, C.pid=%d, B.created=%d, C.created=%d", parentPID, candidate.ProcessId, orphanCreated, candidateCreated)
	}
	status, err := windows.WaitForSingleObject(orphanHandle, 0)
	if err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("unrelated child B is no longer running: wait=%d err=%v", status, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	children, err := snapshotManagedChildren(ctx, int(candidate.ProcessId))
	if err != nil {
		t.Fatal(err)
	}
	if !children.supervisorListed {
		t.Fatal("fixture failed: live suspended C was not present in the native process snapshot")
	}
	rt := &Runtime{managedChildren: snapshotManagedChildren}
	workload, workloadErr := rt.supervisedWorkload(ctx, &hostSession{}, int(candidate.ProcessId))
	t.Logf("NATIVE PID REUSE: attempts=%d C.pid=%d B.pid=%d B.parent=%d B.created=%d C.created=%d runningChildren=%d workload=%d workloadErr=%v", attempts+1, candidate.ProcessId, orphanPID, parentPID, orphanCreated, candidateCreated, children.running, workload, workloadErr)
	if children.running != 0 || workloadErr != nil || workload != workloadUnconfirmed {
		t.Fatalf("unrelated older process B was accepted as C's managed child: running=%d workload=%d err=%v; C is suspended and has never created a child", children.running, workload, workloadErr)
	}
}

// The package has no Windows TestMain at PR head 07fc6e1. These helper modes
// create real kernel processes without running any tests in the descendants.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "pty-host" {
		os.Exit(RunHost(os.Args[2:], os.Stdout))
	}
	if len(os.Args) > 3 && os.Args[1] == "--ao-review-windows-helper" {
		if err := reviewWindowsHelper(os.Args[2], os.Args[3]); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func reviewWindowsHelper(mode, dir string) error {
	switch mode {
	case "old-parent":
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		child := exec.Command(exe, "--ao-review-windows-helper", "orphan", dir) //nolint:forbidigo // native process fixture
		if err := child.Start(); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "orphan-pid"), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			_ = child.Process.Kill()
			_ = child.Wait()
			return err
		}
		if err := child.Process.Release(); err != nil {
			return err
		}
		return reviewWindowsWaitPath(filepath.Join(dir, "release-parent"), 180*time.Second)
	case "orphan":
		if err := os.WriteFile(filepath.Join(dir, "orphan-ready"), nil, 0o600); err != nil {
			return err
		}
		return reviewWindowsWaitPath(filepath.Join(dir, "release-orphan"), 180*time.Second)
	case "candidate":
		return errors.New("candidate was unexpectedly resumed; it must remain suspended")
	default:
		return fmt.Errorf("unknown helper mode %q", mode)
	}
}

func reviewWindowsWaitPath(path string, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", path)
}

func reviewWindowsStartSuspended(exe, dir string) (windows.ProcessInformation, error) {
	app, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return windows.ProcessInformation{}, err
	}
	command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{exe, "--ao-review-windows-helper", "candidate", dir}))
	if err != nil {
		return windows.ProcessInformation{}, err
	}
	si := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var pi windows.ProcessInformation
	err = windows.CreateProcess(app, command, nil, nil, false, windows.CREATE_SUSPENDED|windows.CREATE_NO_WINDOW, nil, nil, &si, &pi)
	return pi, err
}

func reviewWindowsStopProcess(pi windows.ProcessInformation) error {
	defer windows.CloseHandle(pi.Thread)
	defer windows.CloseHandle(pi.Process)
	if err := windows.TerminateProcess(pi.Process, 0); err != nil {
		return err
	}
	status, err := windows.WaitForSingleObject(pi.Process, 5000)
	if err != nil {
		return err
	}
	if status != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("candidate %d did not exit: wait=%d", pi.ProcessId, status)
	}
	return nil
}

func reviewWindowsCreatedAt(h windows.Handle) (uint64, error) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return 0, err
	}
	return uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), nil
}

func reviewWindowsParentPID(pid uint32) (uint32, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err := windows.Process32First(snapshot, &entry); ; err = windows.Process32Next(snapshot, &entry) {
		if err != nil {
			return 0, fmt.Errorf("find process %d: %w", pid, err)
		}
		if entry.ProcessID == pid {
			return entry.ParentProcessID, nil
		}
	}
}
