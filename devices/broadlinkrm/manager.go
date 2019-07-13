package broadlinkrm

// import (
// 	"errors"
// 	"fmt"
// 	"github.com/amitbet/KidControl/config"
// 	"github.com/amitbet/KidControl/util"
// 	"time"
// )

// type Manager struct {
// 	deviceInfoList DeviceInfoList
// }

// func (h *Manager) PostRemoteCommand(dev *config.Device, category, cmdName string) error {
// 	cmd, ok := dev.Commands[cmdName]
// 	if !ok {
// 		msg := fmt.Sprintf("remote %q has no command %q", remote.Name, cmdName)
// 		return errors.New(msg)
// 	}

// 	// Defaults to first device unless told otherwise
// 	dev := h.deviceInfoList[0].GetBroadlinkDevice()
// 	if devName != "" {

// 		devInfo, found := h.deviceInfoList.Find(ByName(devName))
// 		if devInfo == nil || !found {
// 			return errors.New("device " + devName + " found")
// 		}
// 		dev = devInfo.GetBroadlinkDevice()
// 	}
// 	if err := dev.SendIRRemoteCode(cmd, 1); err != nil {
// 		return fmt.Errorf("IR code send failure: %s", err)
// 	}
// 	return nil
// }

// func (h *Manager) LoadConfiguration(devicesFile, remotesFile string) error {

// 	h.deviceInfoList = DeviceInfoList{}
// 	if err := util.LoadFromFile(&h.deviceInfoList, devicesFile); err != nil {
// 		return errors.New("Failed to load devices from file: " + devicesFile)
// 	}
// 	if len(h.deviceInfoList) == 0 {
// 		return errors.New("No device listed in file: " + devicesFile + " Aborting.")
// 	}
// 	if err := h.deviceInfoList.InitializeDevices(10 * time.Second); err != nil {
// 		return errors.New("Failed to initialize a Broadlink device")
// 	}

// 	h.remoteList = RemoteList{}
// 	if err := util.LoadFromFile(&h.remoteList, remotesFile); err != nil {
// 		errors.New("Failed to load remotes from file: " + remotesFile)
// 	}
// 	if len(h.remoteList) == 0 {
// 		errors.New("No remote listed in file: " + remotesFile + " Aborting.")
// 	}

// 	return nil
// }
