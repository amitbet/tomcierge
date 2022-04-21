//go:build !windows
// +build !windows

package util

import (
	"github.com/itchyny/volume-go"
	"github.com/kardianos/service"
	"github.com/shirou/gopsutil/v3/load"
)

// SetLocalVolume sets volume on the active console session
func SetLocalVolume(vol int, isService bool, logger service.Logger) error {
	// logger.Info("Running as service: ", isService)
	// if isService {
	// 	exePath, err := os.Executable()
	// 	if err != nil {
	// 		return err
	// 	}
	// 	logger.Infof("SetVol by running on another session")
	// 	cmd := exec.Command(exePath, "-setvol", strconv.Itoa(vol))
	// 	err = cmd.Run()
	// 	if err != nil {
	// 		logger.Errorf("SetVol error: %v", err)
	// 		return err
	// 	}
	// } else {
	volume.SetVolume(vol)
	// }
	return nil
}

func MachineSleep() {
	//no implementation for linux yet
}

func PlayLocalAlert(isService bool, logger service.Logger, sndFileArray []string) error {
	//no implementation for linux yet
	return nil
}

func PlaySoundFile(path, sndFile string) {
	//no implementation for linux yet
}

func CpuLoad() (int64, error) {
	info, err := load.Avg()
	if err != nil {
		return 0, err
	}

	return int64(info.Load1), nil
}
