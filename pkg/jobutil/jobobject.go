//go:build windows

package jobutil

import (
	"strings"
	"time"
	"unsafe"

	"log/slog"

	"golang.org/x/sys/windows"
)

// winRetryAttempts и winRetryDelay задают повторение win32-вызовов над
// только что запущенными/завершающимися процессами: OpenProcess,
// AssignProcessToJobObject и TerminateProcess могут transiently сбоить
// (например, ERROR_ACCESS_DENIED, пока процесс ещё создаётся), поэтому
// попытка повторяется, а не трактуется как окончательный отказ.
const (
	winRetryAttempts = 5
	winRetryDelay    = 100 * time.Millisecond
)

// winRetry выполняет fn до winRetryAttempts попыток с паузой winRetryDelay.
func winRetry(fn func() error) error {
	var err error
	for range winRetryAttempts {
		if err = fn(); err == nil {
			return nil
		}
		time.Sleep(winRetryDelay)
	}
	return err
}

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
		if err := windows.CloseHandle(h); err != nil {
			slog.Warn("не удалось закрыть Job Object после ошибки настройки", slog.String("err", err.Error()))
		}
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
	defer func() {
		if err := windows.CloseHandle(h); err != nil {
			slog.Debug("не удалось закрыть snapshot-хендл", slog.String("err", err.Error()))
		}
	}()

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
		// OpenProcess и привязку повторяем: свежезапущенный процесс может
		// быть ещё недоступен (transient ERROR_ACCESS_DENIED/INVALID_PARAMETER).
		if err := winRetry(func() error {
			hProcess, err := windows.OpenProcess(
				windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
				false, pid,
			)
			if err != nil {
				return err
			}
			defer func() {
				if err := windows.CloseHandle(hProcess); err != nil {
					slog.Debug("не удалось закрыть хендл процесса", slog.Uint64("pid", uint64(pid)), slog.String("err", err.Error()))
				}
			}()
			return windows.AssignProcessToJobObject(job, hProcess)
		}); err != nil {
			slog.Warn("не удалось привязать процесс к Job Object",
				slog.Uint64("pid", uint64(pid)), slog.String("err", err.Error()))
		}
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

// FindProcessesByName возвращает PID процессов, чьё имя exe (без учёта регистра)
// совпадает с одним из names (например, "WINWORD.EXE", "ACAD.EXE").
// Используется для очистки зависших COM-серверов Word/AutoCAD, оставшихся
// в системе после аварийного завершения процесса-хозяина (когда Job Object
// не был создан или не успел прибить дочерние процессы при закрытии хендла).
func FindProcessesByName(names ...string) []uint32 {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer func() {
		if err := windows.CloseHandle(h); err != nil {
			slog.Debug("не удалось закрыть snapshot-хендл", slog.String("err", err.Error()))
		}
	}()

	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[strings.ToLower(n)] = true
	}

	var pids []uint32
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))

	err = windows.Process32First(h, &pe)
	if err != nil {
		return nil
	}
	for {
		exe := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if want[exe] {
			pids = append(pids, pe.ProcessID)
		}
		if err = windows.Process32Next(h, &pe); err != nil {
			break
		}
	}
	return pids
}

// KillProcesses убивает процессы по списку PID.
func KillProcesses(pids []uint32) {
	for _, pid := range pids {
		// Открытие и завершение повторяем: процесс может быть в стадии
		// создания/завершения, когда вызовы transiently сбоят.
		if err := winRetry(func() error {
			h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
			if err != nil {
				return err
			}
			defer func() {
				if err := windows.CloseHandle(h); err != nil {
					slog.Debug("не удалось закрыть хендл процесса", slog.Uint64("pid", uint64(pid)), slog.String("err", err.Error()))
				}
			}()
			return windows.TerminateProcess(h, 1)
		}); err != nil {
			slog.Warn("не удалось завершить процесс", slog.Uint64("pid", uint64(pid)), slog.String("err", err.Error()))
		}
	}
}
