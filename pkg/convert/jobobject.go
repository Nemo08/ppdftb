//go:build windows

package convert

import (
	"unsafe"

	"golang.org/x/sys/windows"
	"log/slog"
)

// createJobObject создаёт Job Object с флагом JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE.
// При краше процесса Windows автоматически убьёт все процессы, привязанные к Job.
func createJobObject() windows.Handle {
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
		windows.CloseHandle(h)
		return 0
	}
	return h
}

// getAllPids возвращает PID всех процессов в системе.
func getAllPids() []uint32 {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)

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

// assignPidsToJob привязывает новые PID (из after, отсутствующие в before) к Job Object.
// Если привязка не удалась (AlreadyAssigned, AccessDenied), это не фатально.
func assignPidsToJob(job windows.Handle, before, after []uint32) {
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
		windows.AssignProcessToJobObject(job, hProcess)
		windows.CloseHandle(hProcess)
	}
}

// collectNewPids возвращает PID из after, которых не было в before.
func collectNewPids(before, after []uint32) []uint32 {
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

// killProcesses убивает процессы по списку PID.
func killProcesses(pids []uint32) {
	for _, pid := range pids {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
		if err != nil {
			continue
		}
		windows.TerminateProcess(h, 1)
		windows.CloseHandle(h)
	}
}
