//go:build windows

package jobutil

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCreateJobObject(t *testing.T) {
	h := CreateJobObject()
	if h == 0 {
		t.Log("CreateJobObject returned 0 (expected on some configurations)")
	}
}

func TestGetAllPids(t *testing.T) {
	pids := GetAllPids()
	if len(pids) == 0 {
		t.Fatal("GetAllPids returned empty list")
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CollectNewPids(tt.before, tt.after)
			if len(got) != len(tt.want) {
				t.Fatalf("CollectNewPids() = %v, want %v", got, tt.want)
			}
			if len(tt.want) > 0 {
				slices.Sort(got)
				slices.Sort(tt.want)
				for i := range got {
					if got[i] != tt.want[i] {
						t.Fatalf("CollectNewPids() = %v, want %v", got, tt.want)
					}
				}
			}
		})
	}
}

func TestKillProcessesEmpty(t *testing.T) {
	KillProcesses(nil)
	KillProcesses([]uint32{})
}

func TestFindProcessesByNameSelf(t *testing.T) {
	exe := strings.ToUpper(filepath.Base(os.Args[0]))
	pids := FindProcessesByName(exe)
	if len(pids) == 0 {
		t.Fatalf("FindProcessesByName(%q) не нашёл собственный тестовый процесс", exe)
	}
	pid := uint32(os.Getpid())
	found := slices.Contains(pids, pid)
	if !found {
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
	AssignPidsToJob(h, []uint32{}, []uint32{})
}
