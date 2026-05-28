//go:build windows

package jobutil

import (
	"unsafe"

	"golang.org/x/sys/windows"
	"log/slog"
)

// CreateJobObject создаёт Job Object с флагом JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE.
// При краше процесса Windows автоматически убьёт все процессы, привязанные к Job.
func CreateJobObject() windows.Handle {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		slog.Warn("Job Object не создан", slog.String("err", err.Error()))
		return 0
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info))); err != nil {
		slog.Warn("Job Object не настроен", slog.String("err", err.Error()))
		_ = windows.CloseHandle(h)
		return 0
	}
	return h
}

// GetAllPids возвращает PID всех процессов в системе.
func GetAllPids() []uint32 {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer func() { _ = windows.CloseHandle(h) }()

	var pids []uint32
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))

	err = windows.Process32First(h, &pe)
	if err != nil {
		return nil
	}
	for {
		pids = append(pids, pe.ProcessID)
		err = windows.Process32Next(h, &pe)
		if err != nil {
			break
		}
	}
	return pids
}

// AssignPidsToJob привязывает новые PID (из after, отсутствующие в before) к Job Object.
// Если привязка не удалась (AlreadyAssigned, AccessDenied), это не фатально.
func AssignPidsToJob(job windows.Handle, before, after []uint32) {
	if job == 0 {
		return
	}
	isBefore := make(map[uint32]bool, len(before))
	for _, pid := range before {
		isBefore[pid] = true
	}
	for _, pid := range after {
		if isBefore[pid] {
			continue
		}
		hProcess, err := windows.OpenProcess(
			windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
			false, pid,
		)
		if err != nil {
			continue
		}
		_ = windows.AssignProcessToJobObject(job, hProcess)
		_ = windows.CloseHandle(hProcess)
	}
}

// CollectNewPids возвращает PID из after, которых не было в before.
func CollectNewPids(before, after []uint32) []uint32 {
	isBefore := make(map[uint32]bool, len(before))
	for _, pid := range before {
		isBefore[pid] = true
	}
	var newPids []uint32
	for _, pid := range after {
		if !isBefore[pid] {
			newPids = append(newPids, pid)
		}
	}
	return newPids
}

// KillProcesses убивает процессы по списку PID.
func KillProcesses(pids []uint32) {
	for _, pid := range pids {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
		if err != nil {
			continue
		}
		_ = windows.TerminateProcess(h, 1)
		_ = windows.CloseHandle(h)
	}
}
