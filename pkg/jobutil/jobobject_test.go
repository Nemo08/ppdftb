//go:build windows

package jobutil

import (
	"sort"
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
				sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
				sort.Slice(tt.want, func(i, j int) bool { return tt.want[i] < tt.want[j] })
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
