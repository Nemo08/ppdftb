//go:build windows

package jobutil

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestCreateJobObject(t *testing.T) {
	h := CreateJobObject()
	if h == 0 {
		t.Log("CreateJobObject returned 0 (expected on some configurations)")
		return
	}
	if err := windows.CloseHandle(h); err != nil {
		t.Fatalf("CloseHandle: %v", err)
	}
}

func TestGetAllPids(t *testing.T) {
	pids := GetAllPids()
	if len(pids) == 0 {
		t.Fatal("GetAllPids returned empty list")
	}
	self := uint32(os.Getpid())
	if !slices.Contains(pids, self) {
		t.Fatalf("GetAllPids() не содержит собственный PID %d", self)
	}
}

func TestCollectNewPids(t *testing.T) {
	tests := []struct {
		name   string
		before []uint32
		after  []uint32
		want   []uint32
	}{
		{"no new pids", []uint32{1, 2, 3}, []uint32{1, 2, 3}, nil},
		{"one new pid", []uint32{1, 2}, []uint32{1, 2, 3}, []uint32{3}},
		{"multiple new", []uint32{1}, []uint32{1, 2, 3}, []uint32{2, 3}},
		{"all new", []uint32{}, []uint32{1, 2}, []uint32{1, 2}},
		{"empty after", []uint32{1, 2}, []uint32{}, nil},
		{"both empty", []uint32{}, []uint32{}, nil},
		{"nil before", nil, []uint32{10, 20}, []uint32{10, 20}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CollectNewPids(tt.before, tt.after)
			if len(got) != len(tt.want) {
				t.Fatalf("CollectNewPids() = %v, want %v", got, tt.want)
			}
			if len(tt.want) == 0 {
				return
			}
			slices.Sort(got)
			want := append([]uint32(nil), tt.want...)
			slices.Sort(want)
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("CollectNewPids() = %v, want %v", got, want)
				}
			}
		})
	}
}

func TestKillProcessesEmpty(t *testing.T) {
	KillProcesses(nil)
	KillProcesses([]uint32{})
	KillProcesses([]uint32{0}) // pid 0 пропускается
}

// TestKillProcessesDeadPid — несуществующий PID не должен паниковать
// и не должен висеть дольше нескольких ретраев (processAlreadyGone).
func TestKillProcessesDeadPid(t *testing.T) {
	start := time.Now()
	KillProcesses([]uint32{0xFFFFFFFE}) // заведомо мёртвый/невалидный
	if time.Since(start) > 3*time.Second {
		t.Fatal("KillProcesses для мёртвого PID занял слишком много времени")
	}
}

func TestFindProcessesByNameSelf(t *testing.T) {
	exe := strings.ToUpper(filepath.Base(os.Args[0]))
	pids := FindProcessesByName(exe)
	if len(pids) == 0 {
		t.Fatalf("FindProcessesByName(%q) не нашёл собственный тестовый процесс", exe)
	}
	pid := uint32(os.Getpid())
	if !slices.Contains(pids, pid) {
		t.Fatalf("FindProcessesByName(%q) = %v, не содержит собственный PID %d", exe, pids, pid)
	}
}

func TestFindProcessesByNameNotFound(t *testing.T) {
	pids := FindProcessesByName("несуществующий-процесс-ppdftb.exe")
	if len(pids) != 0 {
		t.Fatalf("FindProcessesByName() для несуществующего имени = %v, ожидался пустой список", pids)
	}
}

func TestAssignPidsToJobNoop(t *testing.T) {
	AssignPidsToJob(0, []uint32{1, 2}, []uint32{1, 2, 3})
}

func TestAssignPidsToJobSamePids(t *testing.T) {
	h := CreateJobObject()
	if h == 0 {
		t.Skip("Job Object not available")
	}
	defer windows.CloseHandle(h)
	// Нечего добавлять — before и after совпадают / пусты.
	AssignPidsToJob(h, []uint32{}, []uint32{})
	AssignPidsToJob(h, []uint32{1, 2}, []uint32{1, 2})
}

// TestAssignPidsToJobNilBefore — путь olepool: before=nil, after=[pid].
func TestAssignPidsToJobNilBefore(t *testing.T) {
	h := CreateJobObject()
	if h == 0 {
		t.Skip("Job Object not available")
	}
	defer windows.CloseHandle(h)
	self := uint32(os.Getpid())
	// Привязка собственного процесса к job допустима; ошибки OpenProcess
	// для чужих/защищённых PID логируются, тест не должен паниковать.
	AssignPidsToJob(h, nil, []uint32{self})
}

func TestProcessAlreadyGone(t *testing.T) {
	if !processAlreadyGone(windows.ERROR_INVALID_PARAMETER) {
		t.Error("ERROR_INVALID_PARAMETER должен считаться processAlreadyGone")
	}
	if !processAlreadyGone(windows.ERROR_ACCESS_DENIED) {
		t.Error("ERROR_ACCESS_DENIED должен считаться processAlreadyGone")
	}
	if !processAlreadyGone(windows.ERROR_INVALID_HANDLE) {
		t.Error("ERROR_INVALID_HANDLE должен считаться processAlreadyGone")
	}
	if processAlreadyGone(nil) {
		t.Error("nil не processAlreadyGone")
	}
	if processAlreadyGone(windows.ERROR_BUSY) {
		t.Error("ERROR_BUSY не должен считаться processAlreadyGone")
	}
}
