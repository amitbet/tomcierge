package broadlinkrm

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/amitbet/tomcierge/config"
	"github.com/amitbet/tomcierge/logger"
	"github.com/mixcode/broadlink"
)

// BroadlinkDevice holds the information to access a Broadlink device on the network.
type BroadlinkDevice struct {
	Name       string `json:"name"`
	UDPAddress string `json:"udpAddress"`
	MACAddress string `json:"macAddress"`
	Type       uint16 `json:"type"`
	TypeName   string `json:"typeName,omitempty"`
	NumRetries int    `json:"numRetries,omitempty"`
	device     *broadlink.Device
}

// BroadlinkDeviceList represents a list of Broadlink device info.
// type BroadlinkDeviceList []*BroadlinkDevice

// NewBroadlinkDevice creates a structure holding the information from a Broadlink device, as well as a user provided name.
func NewBroadlinkDevice(name string, device broadlink.Device) *BroadlinkDevice {
	model, _ := device.DeviceName()

	return &BroadlinkDevice{
		Name:       name,
		UDPAddress: device.UDPAddr.String(),
		MACAddress: net.HardwareAddr(device.MACAddr).String(),
		Type:       device.Type,
		TypeName:   model,
		NumRetries: 1,
	}
}

func (d *BroadlinkDevice) RunCommand(cmd config.DeviceCommand) error {
	if d.NumRetries < 1 {
		d.NumRetries = 1
	}

	cmdBytes, err := cmd.GetBytesToSend()
	if err != nil {
		return fmt.Errorf("IR code GetBytesToSend failure: %s", err)
	}
	counter := 1
	for {
		err = d.device.SendIRRemoteCode(cmdBytes, 1)
		if err == nil {
			return nil
		} else if counter != d.NumRetries {
			logger.Warnf("Retrying: IR code send failure (%s)", err)
		} else {
			break
		}
		counter++
	}

	if err != nil {
		logger.Errorf("Quitting: IR code send failure: %s", err)
		return fmt.Errorf("IR code send failure: %s", err)
	}

	return nil
}

// createDevice prepares a new, uninitliased broadlink.Device from the device information
func (d *BroadlinkDevice) createDevice() error {
	mac, err := net.ParseMAC(d.MACAddress)
	if err != nil {
		return fmt.Errorf("failed to parse MAC address, %s", err)
	}
	// Parse UDP address
	udpAddr, err := net.ResolveUDPAddr("udp", d.UDPAddress)
	if err != nil {
		return fmt.Errorf("failed to parse UDP address, %s", err)
	}
	d.device = &broadlink.Device{
		Type:    d.Type,
		MACAddr: mac,
		UDPAddr: *udpAddr,
	}
	return nil
}

// GetBroadlinkDevice returns the associated Broadlink device.
// The returned device may not be initialized or even created. Make sure to call InitializeDevice before calling that function.
func (d *BroadlinkDevice) GetBroadlinkDevice() *broadlink.Device {
	return d.device
}

// InitializeDevice initialize the device by creating a broadlink.Device and authenticating with it.
// Device communication timeout is provided as a parameter.
func (d *BroadlinkDevice) Initialize(props map[string]string, timeout time.Duration) error {
	d.Name = props["name"]
	d.UDPAddress = props["udpAddress"]
	d.MACAddress = props["macAddress"]
	d.TypeName = props["typeName"]

	intType, _ := strconv.Atoi(props["type"])
	d.Type = uint16(intType)
	intNumRetries, _ := strconv.Atoi(props["numRetries"])
	d.NumRetries = intNumRetries

	if d.device == nil {
		if err := d.createDevice(); err != nil {
			return nil
		}
	}

	// Already auth'd
	if d.device.ID != 0 {
		return nil
	}

	if d.NumRetries < 1 {
		d.NumRetries = 1
	}

	hostname, _ := os.Hostname() // Your local machine's name.
	fakeID := make([]byte, 15)   // Must be 15 bytes long.

	d.device.Timeout = timeout

	counter := 1
	var err error
	for {
		err = d.device.Auth(fakeID, hostname)
		if err == nil {
			return nil
		} else if counter != d.NumRetries {
			logger.Warnf("Retrying: failed to authenticate with device %s  (%s)", d.Name, err)
		} else {
			break
		}
		counter++
	}

	if err != nil {
		logger.Errorf("Quitting: failed to authenticate with device %s (%v)", d.Name, err)
		return fmt.Errorf("failed to authenticate with device %s, addr %s, %s", d.Name, d.UDPAddress, err)
	}

	return nil
}

// func (dl *BroadlinkDeviceList) AddDevice(name string, device broadlink.Device) error {
// 	dev := NewBroadlinkDevice(name, device)

// 	for idx, d := range *dl {
// 		// Same MAC address -> update existing record
// 		if d.MACAddress == dev.MACAddress {
// 			(*dl)[idx] = dev
// 			return nil
// 		}
// 		if d.Name == dev.Name {
// 			return fmt.Errorf("device %s already exists but MAC address does not match (existing=%s, new=%s)", name, d.MACAddress, dev.MACAddress)
// 		}
// 	}
// 	*dl = append(*dl, dev)
// 	return nil
// }

// type BroadlinkDevicePredicate func(*BroadlinkDevice) bool

// func ByName(name string) BroadlinkDevicePredicate {
// 	return func(d *BroadlinkDevice) bool {
// 		return d.Name == name
// 	}
// }

// func (dl BroadlinkDeviceList) Find(predicate BroadlinkDevicePredicate) (*BroadlinkDevice, bool) {
// 	for _, dev := range dl {
// 		if predicate(dev) {
// 			return dev, true
// 		}
// 	}
// 	return nil, false
// }

// func (dl BroadlinkDeviceList) InitializeDevices(timeout time.Duration) error {
// 	for _, d := range dl {
// 		err := d.InitializeDevice(timeout, Properties)
// 		if err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }
