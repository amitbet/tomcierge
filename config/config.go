package config

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/amitbet/tomcierge/util"
)

type Config struct {
	ListeningAddress  string    `"json:listeningAddress"`
	CanControlDevices bool      `"json:canControlDevices"`
	Devices           []*Device `"json:devices"`

	VolumeServiceList map[string]string `"json:volumeServiceList"`
}
type IHardwareDevice interface {
	Initialize(props map[string]string, timeout time.Duration) error
	RunCommand(cmd DeviceCommand) error
}

type DeviceCommand interface {
	GetBytesToSend() ([]byte, error)
}

type Device struct {
	Properties     map[string]string  `"json:properties"`
	Commands       map[string]Command `json:"commands"`
	internalDevice IHardwareDevice
}

func (d *Device) SetDevice(device IHardwareDevice) {
	d.internalDevice = device
}

func (d *Device) Initialize(props map[string]string, timeout time.Duration) error {

	return d.internalDevice.Initialize(props, timeout)
}

func (d *Device) RunCommand(cmd DeviceCommand) error {
	err := d.internalDevice.RunCommand(cmd)
	return err
}

type Command struct {
	Name       string `json:"name"`       // the command name (also id)
	Category   string `json:"category"`   // the remote it belongs to (category / remote control)
	CommandHex string `json:"commandHex"` // the command bytes in hex
}

func (cmd *Command) GetBytesToSend() ([]byte, error) {
	return hex.DecodeString(cmd.CommandHex)
}

// GetCommandByNameAndCategory returns the first command to match the name & category, category or name can also be nil but not both
func (dev *Device) GetCommandByNameAndCategory(cmdName, cmdCategory string) *Command {
	if cmdName == "" && cmdCategory == "" {
		return nil
	}

	for _, cmd := range dev.Commands {
		if (cmd.Name == cmdName || cmdName == "") &&
			(cmd.Category == cmdCategory || cmdCategory == "") {
			return &cmd
		}
	}
	return nil
}

var dConfig *Config

func loadConfiguration(configFile string) error {
	dConfig = &Config{}
	if err := util.LoadFromFile(&dConfig, configFile); err != nil {
		return fmt.Errorf("Failed to load devices from file: %s err: %s", configFile, err)
	}
	return nil
}

func GetConfig(configFile string) *Config {
	if dConfig != nil {
		return dConfig
	}

	loadConfiguration(configFile)
	if dConfig == nil {
		dConfig = &Config{
			ListeningAddress:  ":7777",
			CanControlDevices: false,
			Devices:           []*Device{},
			VolumeServiceList: map[string]string{},
		}
	}
	if dConfig.VolumeServiceList == nil {
		dConfig.VolumeServiceList = map[string]string{}
	}
	if dConfig.Devices == nil {
		dConfig.Devices = []*Device{}
	}
	return dConfig
}
