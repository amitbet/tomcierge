// +build !windows

package util

import (
	"github.com/amitbet/volume-go"
	"github.com/kardianos/service"
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
