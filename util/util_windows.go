package util

import (
	"fmt"
	"github.com/gonutz/w32/v2"
	"github.com/itchyny/volume-go"
	"github.com/kardianos/service"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func CpuLoad() (int64, error) {
	var idle, kernel, user w32.FILETIME

	w32.GetSystemTimes(&idle, &kernel, &user)
	idleFirst := idle.DwLowDateTime | (idle.DwHighDateTime << 32)
	kernelFirst := kernel.DwLowDateTime | (kernel.DwHighDateTime << 32)
	userFirst := user.DwLowDateTime | (user.DwHighDateTime << 32)

	time.Sleep(time.Second)

	w32.GetSystemTimes(&idle, &kernel, &user)
	idleSecond := idle.DwLowDateTime | (idle.DwHighDateTime << 32)
	kernelSecond := kernel.DwLowDateTime | (kernel.DwHighDateTime << 32)
	userSecond := user.DwLowDateTime | (user.DwHighDateTime << 32)

	totalIdle := float64(idleSecond - idleFirst)
	totalKernel := float64(kernelSecond - kernelFirst)
	totalUser := float64(userSecond - userFirst)
	totalSys := float64(totalKernel + totalUser)

	cpuload := (totalSys - totalIdle) * 100 / totalSys
	// fmt.Printf("Idle: %f%%\nKernel: %f%%\nUser: %f%%\n", (totalIdle / totalSys) * 100, (totalKernel / totalSys) * 100, (totalUser / totalSys) * 100)
	// fmt.Printf("\nTotal: %f%%\n", cpuload)
	return int64(cpuload), nil
}

var (
	modwtsapi32 *windows.LazyDLL = windows.NewLazySystemDLL("wtsapi32.dll")
	modkernel32 *windows.LazyDLL = windows.NewLazySystemDLL("kernel32.dll")
	modadvapi32 *windows.LazyDLL = windows.NewLazySystemDLL("advapi32.dll")
	moduserenv  *windows.LazyDLL = windows.NewLazySystemDLL("userenv.dll")

	procWTSEnumerateSessionsW        *windows.LazyProc = modwtsapi32.NewProc("WTSEnumerateSessionsW")
	procWTSGetActiveConsoleSessionId *windows.LazyProc = modkernel32.NewProc("WTSGetActiveConsoleSessionId")
	procWTSQueryUserToken            *windows.LazyProc = modwtsapi32.NewProc("WTSQueryUserToken")
	procDuplicateTokenEx             *windows.LazyProc = modadvapi32.NewProc("DuplicateTokenEx")
	procCreateEnvironmentBlock       *windows.LazyProc = moduserenv.NewProc("CreateEnvironmentBlock")
	procCreateProcessAsUser          *windows.LazyProc = modadvapi32.NewProc("CreateProcessAsUserW")
)

const (
	WTS_CURRENT_SERVER_HANDLE uintptr = 0
)

type WTS_CONNECTSTATE_CLASS int

const (
	WTSActive WTS_CONNECTSTATE_CLASS = iota
	WTSConnected
	WTSConnectQuery
	WTSShadow
	WTSDisconnected
	WTSIdle
	WTSListen
	WTSReset
	WTSDown
	WTSInit
)

type SECURITY_IMPERSONATION_LEVEL int

const (
	SecurityAnonymous SECURITY_IMPERSONATION_LEVEL = iota
	SecurityIdentification
	SecurityImpersonation
	SecurityDelegation
)

type TOKEN_TYPE int

const (
	TokenPrimary TOKEN_TYPE = iota + 1
	TokenImpersonazion
)

type SW int

const (
	SW_HIDE            SW = 0
	SW_SHOWNORMAL         = 1
	SW_NORMAL             = 1
	SW_SHOWMINIMIZED      = 2
	SW_SHOWMAXIMIZED      = 3
	SW_MAXIMIZE           = 3
	SW_SHOWNOACTIVATE     = 4
	SW_SHOW               = 5
	SW_MINIMIZE           = 6
	SW_SHOWMINNOACTIVE    = 7
	SW_SHOWNA             = 8
	SW_RESTORE            = 9
	SW_SHOWDEFAULT        = 10
	SW_MAX                = 1
)

type WTS_SESSION_INFO struct {
	SessionID      windows.Handle
	WinStationName *uint16
	State          WTS_CONNECTSTATE_CLASS
}

const (
	CREATE_UNICODE_ENVIRONMENT uint32 = 0x00000400
	CREATE_NO_WINDOW                  = 0x08000000
	CREATE_NEW_CONSOLE                = 0x00000010
)

// getFirstActiveSessionId will attempt to resolve
// the session ID of the user currently active on
// the system.
func getFirstActiveSessionId() (windows.Handle, error) {
	sessionList, err := enumerateSessions()
	if err != nil {
		return 0xFFFFFFFF, fmt.Errorf("get current user session token: %s", err)
	}

	for i := range sessionList {
		if sessionList[i].State == WTSActive {
			return sessionList[i].SessionID, nil
		}
	}

	consoleSessionId, _, consoleSessionErr := procWTSGetActiveConsoleSessionId.Call()

	if consoleSessionId == 0xFFFFFFFF {
		return 0xFFFFFFFF, fmt.Errorf("get current user session token: call native WTSGetActiveConsoleSessionId: %s", consoleSessionErr)
	} else {
		return windows.Handle(consoleSessionId), nil
	}
}

// getConsoleSessionId returns the sessionID for the windows console session
func getConsoleSessionId() (uint, error) {

	consoleSessionId, _, consoleSessionErr := procWTSGetActiveConsoleSessionId.Call()

	if consoleSessionId == 0xFFFFFFFF {
		return 0xFFFFFFFF, fmt.Errorf("get current user session token: call native WTSGetActiveConsoleSessionId: %s", consoleSessionErr)
	} else {
		return uint(consoleSessionId), nil
	}
}

// enumerateSessions uses the WTSEnumerateSession
// native function for Windows and parses the result
// to a Golang friendly version
func enumerateSessions() ([]*WTS_SESSION_INFO, error) {
	var (
		sessionInformation windows.Handle      = windows.Handle(0)
		sessionCount       int                 = 0
		sessionList        []*WTS_SESSION_INFO = make([]*WTS_SESSION_INFO, 0)
	)

	if returnCode, _, err := procWTSEnumerateSessionsW.Call(WTS_CURRENT_SERVER_HANDLE, 0, 1, uintptr(unsafe.Pointer(&sessionInformation)), uintptr(unsafe.Pointer(&sessionCount))); returnCode == 0 {
		return nil, fmt.Errorf("call native WTSEnumerateSessionsW: %s", err)
	}

	structSize := unsafe.Sizeof(WTS_SESSION_INFO{})
	current := uintptr(sessionInformation)
	for i := 0; i < sessionCount; i++ {
		sessionList = append(sessionList, (*WTS_SESSION_INFO)(unsafe.Pointer(current)))
		current += structSize
	}

	return sessionList, nil
}

// duplicateUserTokenFromSessionID will attempt
// to duplicate the user token for the user logged
// into the provided session ID
func duplicateUserTokenFromSessionID(sessionId windows.Handle) (windows.Token, error) {
	var (
		impersonationToken windows.Handle = 0
		userToken          windows.Token  = 0
	)

	if returnCode, _, err := procWTSQueryUserToken.Call(uintptr(sessionId), uintptr(unsafe.Pointer(&impersonationToken))); returnCode == 0 {
		return 0xFFFFFFFF, fmt.Errorf("call native WTSQueryUserToken: %s", err)
	}

	if returnCode, _, err := procDuplicateTokenEx.Call(uintptr(impersonationToken), 0, 0, uintptr(SecurityImpersonation), uintptr(TokenPrimary), uintptr(unsafe.Pointer(&userToken))); returnCode == 0 {
		return 0xFFFFFFFF, fmt.Errorf("call native DuplicateTokenEx: %s", err)
	}

	if err := windows.CloseHandle(impersonationToken); err != nil {
		return 0xFFFFFFFF, fmt.Errorf("close windows handle used for token duplication: %s", err)
	}

	return userToken, nil
}

func StartProcessAsCurrentUser(appPath, cmdLine, workDir string) error {
	var (
		sessionId windows.Handle
		userToken windows.Token
		envInfo   windows.Handle

		startupInfo windows.StartupInfo
		processInfo windows.ProcessInformation

		commandLine uintptr = 0
		workingDir  uintptr = 0

		err error
	)

	if sessionId, err = getFirstActiveSessionId(); err != nil {
		return err
	}

	if userToken, err = duplicateUserTokenFromSessionID(sessionId); err != nil {
		return fmt.Errorf("get duplicate user token for current user session: %s", err)
	}

	if returnCode, _, err := procCreateEnvironmentBlock.Call(uintptr(unsafe.Pointer(&envInfo)), uintptr(userToken), 0); returnCode == 0 {
		return fmt.Errorf("create environment details for process: %s", err)
	}

	creationFlags := uint32(CREATE_UNICODE_ENVIRONMENT) | uint32(CREATE_NO_WINDOW)
	startupInfo.ShowWindow = uint16(SW_HIDE)
	startupInfo.Desktop = windows.StringToUTF16Ptr("winsta0\\default")

	if len(cmdLine) > 0 {
		commandLine = uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(cmdLine)))
	}
	if len(workDir) > 0 {
		workingDir = uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(workDir)))
	}

	if returnCode, _, err := procCreateProcessAsUser.Call(
		uintptr(userToken), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(appPath))), commandLine, 0, 0, 0,
		uintptr(creationFlags), uintptr(envInfo), workingDir, uintptr(unsafe.Pointer(&startupInfo)), uintptr(unsafe.Pointer(&processInfo)),
	); returnCode == 0 {
		return fmt.Errorf("create process as user: %s", err)
	}

	return nil
}

// SetLocalVolume sets volume on the active console session
func SetLocalVolume(vol int, isService bool, logger service.Logger) error {
	logger.Info("Running as service: ", isService)
	if isService {
		exePath, err := os.Executable()
		if err != nil {
			return err
		}
		exeDir, _ := PathSplit(exePath)

		cmd := exePath + " -setvol " + strconv.Itoa(vol)
		logger.Infof("SetVol by running on another session run-command: %s", cmd)
		err = StartProcessAsCurrentUser(exePath, cmd, exeDir)
		//cmd := exec.Command(exePath, "-setvol", strconv.Itoa(vol))
		//err = cmd.Run()
		if err != nil {
			logger.Errorf("SetVol error: %v", err)
			return err
		}
	} else {
		volume.SetVolume(vol)
	}
	return nil
}

// PlayLocalAlert plays a sound on the active console session
func PlayLocalAlert(isService bool, logger service.Logger, sndFileArray []string) error {
	logger.Info("Running as service: ", isService)
	if isService {
		exePath, err := os.Executable()
		if err != nil {
			return err
		}
		exeDir, _ := PathSplit(exePath)

		cmd := exePath + " -alert "
		logger.Infof("SetVol by running on another session run-command: %s", cmd)
		err = StartProcessAsCurrentUser(exePath, cmd, exeDir)
		//cmd := exec.Command(exePath, "-setvol", strconv.Itoa(vol))
		//err = cmd.Run()
		if err != nil {
			logger.Errorf("SetVol error: %v", err)
			return err
		}
	} else {
		for _, sndFile := range sndFileArray {
			SndPlaySoundW(path.Join("sound", sndFile), SND_SYNC|SND_SYSTEM|SND_RING)
		}
	}
	return nil
}

func Execute(command string) (bool, string, error) {
	// splitting head => g++ parts => rest of the command
	parts := strings.Fields(command)
	head := parts[0]
	parts = parts[1:len(parts)]

	out, err := exec.Command(head, parts...).Output()
	if err != nil {
		return false, "", err
	}
	return true, string(out), nil
}

func sleepCommandLineImplementation(logger service.Logger) {
	cmd := "C:\\Windows\\System32\\rundll32.exe powrprof.dll,SetSuspendState 0,1,0"

	logger.Infof("Sleep implementation [windows], sleep command is [", cmd, "]")
	_, _, err := Execute(cmd)
	if err != nil {
		logger.Infof("Can't execute command [" + cmd + "] : " + err.Error())
	} else {
		logger.Infof("Command correctly executed")
	}
}
func MachineSleep(logger service.Logger) {
	sleepDLLImplementation(false, logger)
}
func MachineHibernate(logger service.Logger) {
	sleepDLLImplementation(true, logger)
}

func sleepDLLImplementation(hibernate bool, logger service.Logger) {
	logger.Infof("sleepDLLImplementation, hibernate = %t", hibernate)
	var mod = syscall.NewLazyDLL("Powrprof.dll")
	var proc = mod.NewProc("SetSuspendState")
	var hiber uintptr = 0
	if hibernate {
		hiber = 1
	}
	// DLL API : public static extern bool SetSuspendState(bool hiberate, bool forceCritical, bool disableWakeEvent);
	// ex. : uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("Done Title"))),
	ret, _, _ := proc.Call(0,
		hiber,      // hibernate
		uintptr(1), // forceCritical
		uintptr(0)) // disableWakeEvent

	logger.Infof("Command executed, result code [" + fmt.Sprint(ret) + "]")
}

func PlaySoundFile(pathToFile string, sndFile string) {
	SndPlaySoundW(path.Join(pathToFile, sndFile), SND_SYNC|SND_SYSTEM|SND_RING)
}
