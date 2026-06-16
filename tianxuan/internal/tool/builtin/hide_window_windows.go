//go:build windows

package builtin

import (
	"os/exec"
	"syscall"
	"unsafe"
)

// hideBashWindow 防止 bash 子进程在 Windows 上弹出 cmd 黑框
func hideBashWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}

// Windows Job Object 常量
const (
	jobObjectInfoClassExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose              = 0x2000
	processAllAccess                          = 0x1FFFFF // PROCESS_ALL_ACCESS
)

// JOBOBJECT_BASIC_LIMIT_INFORMATION 对应 Windows
// JOBOBJECT_BASIC_LIMIT_INFORMATION 结构体。
type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinWorkingSetSize       uintptr
	MaxWorkingSetSize       uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	ChildProcessCount       uint32
	Reserved                [2]uintptr
}

// JOBOBJECT_EXTENDED_LIMIT_INFORMATION 对应 Windows
// JOBOBJECT_EXTENDED_LIMIT_INFORMATION 结构体。
type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                [6]uintptr // IO_COUNTERS — 占位，不直接使用
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

var (
	modKernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW              = modKernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject       = modKernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject      = modKernel32.NewProc("AssignProcessToJobObject")
	procCloseHandle                   = modKernel32.NewProc("CloseHandle")
)

// createJobObject 创建或打开一个 Windows Job Object。
func createJobObject(name *uint16) (syscall.Handle, error) {
	r, _, err := procCreateJobObjectW.Call(0, uintptr(unsafe.Pointer(name)))
	if r == 0 {
		return 0, err
	}
	return syscall.Handle(r), nil
}

// setInformationJobObject 设置 Job Object 的信息。
func setInformationJobObject(job syscall.Handle, infoClass uint32, info unsafe.Pointer, infoLen uint32) error {
	r, _, err := procSetInformationJobObject.Call(uintptr(job), uintptr(infoClass), uintptr(info), uintptr(infoLen))
	if r == 0 {
		return err
	}
	return nil
}

// assignProcessToJobObject 将进程分配给 Job Object。
func assignProcessToJobObject(job syscall.Handle, process syscall.Handle) error {
	r, _, err := procAssignProcessToJobObject.Call(uintptr(job), uintptr(process))
	if r == 0 {
		return err
	}
	return nil
}

// closeHandle 关闭一个内核对象句柄。
func closeHandle(handle syscall.Handle) error {
	r, _, err := procCloseHandle.Call(uintptr(handle))
	if r == 0 {
		return err
	}
	return nil
}

// assignToJobObject 在 cmd.Start() 后立即将进程加入 Job Object，
// Job Object 设置 JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE。
// 当 job handle 关闭时（CloseHandle 触发），Windows 内核递归终止该 job
// 内的所有进程，包括孙进程。返回 job handle 供调用者推迟关闭。
func assignToJobObject(cmd *exec.Cmd) (syscall.Handle, error) {
	if cmd == nil || cmd.Process == nil {
		return 0, syscall.EINVAL
	}

	job, err := createJobObject(nil) // 无名 Job Object
	if err != nil {
		return 0, err
	}

	// 设置 JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	info := jobObjectExtendedLimitInformation{
		BasicLimitInformation: jobObjectBasicLimitInformation{
			LimitFlags: jobObjectLimitKillOnJobClose,
		},
	}
	if err := setInformationJobObject(job, jobObjectInfoClassExtendedLimitInformation, unsafe.Pointer(&info), uint32(unsafe.Sizeof(info))); err != nil {
		closeHandle(job)
		return 0, err
	}

	procHandle, err := getProcessHandle(cmd.Process.Pid)
	if err != nil {
		closeHandle(job)
		return 0, err
	}
	defer closeHandle(procHandle)

	if err := assignProcessToJobObject(job, procHandle); err != nil {
		closeHandle(job)
		return 0, err
	}

	return job, nil
}

// getProcessHandle returns a handle to the process identified by pid
// with PROCESS_ALL_ACCESS so it can be assigned to a Job Object.
// The caller must call syscall.CloseHandle on the returned handle.
func getProcessHandle(pid int) (syscall.Handle, error) {
	return syscall.OpenProcess(processAllAccess, false, uint32(pid))
}
