package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	config "github.com/amitbet/tomcierge/config"
	"github.com/amitbet/tomcierge/devices/broadlinkrm"
	"github.com/amitbet/tomcierge/util"
	"github.com/grandcat/zeroconf"
	"github.com/kardianos/service"
	ssdp "github.com/koron/go-ssdp"

	"github.com/amitbet/volume-go"
	gmux "github.com/gorilla/mux"
)

// HomeControlServer is the listener for the REST Api and the device controller
type HomeControlServer struct {
	IsService     bool
	QuitChan      chan bool
	Logger        service.Logger
	Configuration *config.Config
	BasePath      string
	volPollTimer  *time.Ticker
	localVol      int
}

// httpRespond is a helper function for returning http responses to REST calls
func (s *HomeControlServer) httpRespond(wr http.ResponseWriter, message map[string]interface{}) {
	jsonStr, err := json.Marshal(message)
	if err != nil {
		s.Logger.Errorf("SendMessage failed: %+v", err)
	}

	wr.Write([]byte(jsonStr))
}

// prepareMachineUrl returns the correct URL for a given machine
func (s *HomeControlServer) prepareMachineUrl(machine string) string {
	machineUrl := s.Configuration.VolumeServiceList[machine]
	if !strings.HasSuffix(machineUrl, "/") {
		machineUrl += "/"
	}
	return machineUrl
}

// setVolumeOnMachine sets the volume on the target machine, by calling the parallel home-control API on that machine
func (s *HomeControlServer) setVolumeOnMachine(wr http.ResponseWriter, req *http.Request) {

	body, err := ioutil.ReadAll(req.Body)
	s.Logger.Info("setVolumeOnMachine got: ", string(body))
	if err != nil {
		s.Logger.Error("error in reading req body: ", err)
	}
	bodyBuff := bytes.NewBuffer(body)

	vars := gmux.Vars(req)
	machine := vars["machine"]
	murl := s.prepareMachineUrl(machine)
	if machine == "localhost" || machine == "127.0.0.1" {
		parsed := map[string]interface{}{}
		err := json.Unmarshal(body, &parsed)
		volStr := parsed["volume"].(string)
		vol, err := strconv.Atoi(volStr)

		err = util.SetLocalVolume(vol, s.IsService, s.Logger)
		if err != nil {
			s.Logger.Errorf("set volume failed: %+v", err)
		}
		s.Logger.Infof("set volume success val=%d", vol)

		jObj1 := map[string]interface{}{
			"volume": vol,
		}
		s.httpRespond(wr, jObj1)
		s.Logger.Infof("sending volume back: %d\n", vol)
		return
	}
	res, err := http.Post(murl+"set-volume?machine=localhost", "application/json", bodyBuff)
	if err != nil {
		s.Logger.Error("setVolumeOnMachine Error setting volume from remote machine: ", err)
		return
	}
	if res.StatusCode != 200 {
		s.Logger.Error("setVolumeOnMachine Error setting volume from remote machine: ", res.StatusCode, res.Status)
		return
	}

}

// NameSorter sorts by name.
type NameSorter []map[string]interface{}

func (a NameSorter) Len() int           { return len(a) }
func (a NameSorter) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a NameSorter) Less(i, j int) bool { return a[i]["name"].(string) < a[j]["name"].(string) }

// sendConfig sends the machine configuration
func (s *HomeControlServer) sendConfig(wr http.ResponseWriter, req *http.Request) {
	//cfg := GetConfig()
	clientConfig := map[string]interface{}{
		"machines": []map[string]interface{}{},
	}
	machines := clientConfig["machines"].([]map[string]interface{})

	machineMap := map[string]string{}
	urls := []string{}
	if len(s.Configuration.VolumeServiceList) == 0 {
		_, err := wr.Write([]byte("{\"machines\":[]}"))
		if err != nil {
			s.Logger.Error("sendConfig Error in writing config to response: ", err)
			return
		}
		return
	}
	for k := range s.Configuration.VolumeServiceList {

		murl := s.prepareMachineUrl(k) + "/get-volume"
		machineMap[murl] = k
		urls = append(urls, murl)
	}

	results := util.AsyncHttpGets(urls)
	for _, result := range results {
		parsed := map[string]interface{}{}
		if result.Error != nil {
			continue
		}
		//logger.Debug("sendConfig async get volumes got: ", string(result.Body))
		err := json.Unmarshal(result.Body, &parsed)
		if err != nil {
			s.Logger.Error("sendConfig Error in unmarshaling results: ", err)
			continue
		}
		if parsed["volume"] == nil {
			continue
		}
		vol := parsed["volume"].(float64)
		//	vol, err := strconv.Atoi(volStr)

		machines = append(machines, map[string]interface{}{
			"name":   machineMap[result.Url],
			"volume": vol,
		})
		sort.Sort(NameSorter(machines))
	}

	clientConfig["machines"] = machines
	jStr, err := json.Marshal(clientConfig)
	if err != nil {
		s.Logger.Error("sendConfig Error in marshaling: ", err)
		return
	}
	_, err = wr.Write(jStr)
	if err != nil {
		s.Logger.Error("sendConfig Error in writing config to response: ", err)
		return
	}
}

func (s *HomeControlServer) getMachineVol(machine string) int {
	if machine == "" {
		machine = "localhost"
	}
	timeout := time.Duration(2 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}

	murl := s.prepareMachineUrl(machine)

	res, err := client.Get(murl + "get-volume")
	if err != nil {
		s.Logger.Error("getVolumeOnMachine Error getting volume from remote machine: ", err)
		return -1
	}
	if res.StatusCode != 200 {
		s.Logger.Error("getVolumeOnMachine Error getting volume from remote machine: ", res.StatusCode, res.Status)
		return -1
	}

	jsonStr, err := ioutil.ReadAll(res.Body)
	if err != nil {
		s.Logger.Error("getVolumeOnMachine Error reading body: ", err)
		return -1
	}
	jObj := map[string]int{}
	json.Unmarshal(jsonStr, &jObj)

	return jObj["volume"]
}

func (s *HomeControlServer) handleDeviceCommand(wr http.ResponseWriter, req *http.Request) {
	vars := gmux.Vars(req)
	devices := s.Configuration.Devices
	device := devices[0]
	remoteName := vars["remote"]
	commandName := vars["command"]

	cmd := device.GetCommandByNameAndCategory(commandName, remoteName)
	err := device.RunCommand(cmd)
	if err != nil {
		s.Logger.Error("handleDeviceCommand, Error: ", err)
		wr.WriteHeader(500)
	} else {
		wr.WriteHeader(200)
	}

}

func (s *HomeControlServer) getVolumeOnMachine(wr http.ResponseWriter, req *http.Request) {

	vars := gmux.Vars(req)
	machine := vars["machine"]
	vol := s.getMachineVol(machine)
	jObj := map[string]interface{}{
		"volume": vol,
	}
	s.httpRespond(wr, jObj)
}

func (s *HomeControlServer) getVolume(wr http.ResponseWriter, req *http.Request) {
	// vol, err := volume.GetVolume()
	// if err != nil {
	// 	logger.Errorf("get volume failed: %+v", err)
	// }
	jObj := map[string]interface{}{
		"volume": s.localVol,
	}
	s.httpRespond(wr, jObj)
	s.Logger.Infof("sending volume: %d\n", s.localVol)
}

// ssdpAdvertise broadcasts the service name in the network using the ssdp protocol
func (s *HomeControlServer) ssdpAdvertise(quit chan bool) {
	myIp := s.getHostIp().String()
	hname, err := os.Hostname()
	if err != nil {
		s.Logger.Error("Error getting hostname: ", err)
	}

	ad, err := ssdp.Advertise(
		"urn:schemas-upnp-org:service:tomcierge:1", // send as "ST"
		"id:"+hname,             // send as "USN"
		"http://"+myIp+":7777/", // send as "LOCATION"
		"ssdp for tomcierge",    // send as "SERVER"
		3600)                    // send as "maxAge" in "CACHE-CONTROL"
	if err != nil {
		s.Logger.Error("Error advertising ssdp: ", err)
	}

	aliveTick := time.Tick(5 * time.Second)

	// run Advertiser infinitely.
	for {
		select {
		case <-aliveTick:
			ad.Alive()
		case <-quit:
			s.Logger.Info("Closing ssdp service")
			// send/multicast "byebye" message.
			ad.Bye()
			// teminate Advertiser.
			ad.Close()
			return
		}
	}
}

// ssdpSearch searches for other home-automation services of our type by using ssdp
func (s *HomeControlServer) ssdpSearch(searchType string, waitTime int, listenAddress string) []ssdp.Service {

	list, err := ssdp.Search(searchType, waitTime, listenAddress)
	if err != nil {
		s.Logger.Error("Error while searching ssdp: ", err)
	}
	for i, srv := range list {
		//fmt.Printf("%d: %#v\n", i, srv)
		fmt.Printf("%d: %s %s\n", i, srv.Type, srv.Location)
	}
	return list
}

func (s *HomeControlServer) getHostIp() net.IP {
	host, _ := os.Hostname()
	addrs, _ := net.LookupIP(host)

	for _, addr := range addrs {
		if ipv4 := addr.To4(); ipv4 != nil && ipv4[0] == 192 {
			return ipv4
			//fmt.Println("IPv4: ", ipv4)
		}
	}
	return net.IP{}
}

// zconfRegister registeres the service as a zeroconf object
func (s *HomeControlServer) zconfRegister(quit chan bool) {
	myIp := s.getHostIp().String()
	hname, err := os.Hostname()
	meta := []string{
		"version=0.1.0",
		"ip=" + myIp,
	}
	if hname == "" {
		hname = myIp
	}

	zservice, err := zeroconf.Register(
		hname,              // service instance name
		"vol-control._tcp", // service type and protocol
		"local.",           // service domain
		7777,               // service port
		meta,               // service metadata
		nil,                // register on all network interfaces
	)

	if err != nil {
		log.Fatal(err)
	}
	select {
	case <-quit:
		s.Logger.Info("stopping zeroconf publishing server")
		return
	}
	defer zservice.Shutdown()
	s.Logger.Info("stopping zeroconf publishing server")
}

// zconfDiscover runs discovery over the network using the zero-conf protocol
func (s *HomeControlServer) zconfDiscover(serviceMap map[string]string) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Fatal(err)
	}

	// Channel to receive discovered service entries
	entries := make(chan *zeroconf.ServiceEntry)

	go func(results <-chan *zeroconf.ServiceEntry) {
		for entry := range results {
			s.Logger.Infof("Found service:, %s desc: %v address: %v:%s", entry.Instance, entry.Text, entry.AddrIPv4, strconv.Itoa(entry.Port))
			svcInstance := strings.ToLower(entry.Instance)
			var ip string
			for _, prop := range entry.Text {
				if strings.HasPrefix(prop, "ip=") {
					ip = prop[3:]
				}
			}
			if ip == "" {
				ip = entry.AddrIPv4[0].String()
			}
			svcUrl := "http://" + ip + ":" + strconv.Itoa(entry.Port) + "/"
			s.Logger.Infof("instance: %s svcUrl: %s", svcInstance, svcUrl)
			serviceMap[svcInstance] = svcUrl
		}
	}(entries)

	ctx := context.Background()

	err = resolver.Browse(ctx, "vol-control._tcp", "local.", entries)
	if err != nil {
		log.Fatalln("Failed to browse:", err.Error())
	}

	<-ctx.Done()
}

// startPollingVolume runs the polling loop so volume level data remains up to date
func (s *HomeControlServer) startPollingVolume() {
	s.volPollTimer = time.NewTicker(10 * time.Second)
	go func() {
		for {
			<-s.volPollTimer.C
			newVol, err := volume.GetVolume()
			if err != nil {
				s.Logger.Errorf("error while getting volume: %v", err)
				continue
			}
			//s.Logger.Info("got volume: ", s.localVol)
			s.localVol = newVol
		}
	}()
}

// initDevices initializes all registered devices
func (s *HomeControlServer) initDevices() error {
	//	cfg := GetConfig()
	// go over devices and create hardwaer devices for them
	for _, d := range s.Configuration.Devices {
		d.SetDevice(&broadlinkrm.BroadlinkDevice{})
		err := d.Initialize(d.Properties, 10*time.Second)
		if err != nil {
			s.Logger.Error("Error while Initializing Devices: ", err)
		}
		return err
	}
	return nil
}

// InitServer initializes the devices and runs the REST server
func (s *HomeControlServer) InitServer() {
	fmt.Println("starting!")
	s.startPollingVolume()
	if s.Configuration.CanControlDevices {
		go s.initDevices()
	}
	var sigTerm = make(chan os.Signal)
	s.QuitChan = make(chan bool)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	go func() {
		sig := <-sigTerm
		fmt.Printf("caught sig: %+v\n", sig)
		fmt.Println("Waiting for a second to finish processing")
		s.QuitChan <- true
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()

	//go zconfRegister(quit)
	//go zconfDiscover(cfg.VolumeServiceList)

	go s.ssdpAdvertise(s.QuitChan)
	svcList := s.ssdpSearch("urn:schemas-upnp-org:service:tomcierge:1", 5, "")
	for _, svc := range svcList {
		svcName := svc.USN[3:]
		svcUrl := svc.Location
		s.Configuration.VolumeServiceList[strings.ToLower(svcName)] = svcUrl
	}

	mux := gmux.NewRouter() //.StrictSlash(true)

	mux.HandleFunc("/set-volume", s.setVolumeOnMachine).Queries("machine", "{machine}")
	//mux.HandleFunc("/set-volume", setVolume)
	mux.HandleFunc("/get-volume", s.getVolumeOnMachine).Queries("machine", "{machine}")
	mux.HandleFunc("/get-volume", s.getVolume)
	mux.HandleFunc("/configuration", s.sendConfig)

	if s.Configuration.CanControlDevices {
		mux.HandleFunc("/commands/{remote}/{command}", s.handleDeviceCommand)
	}

	mux.PathPrefix("/").Handler(http.FileServer(http.Dir(s.BasePath + "/public")))
	s.Logger.Info("Listening on address: ", s.Configuration.ListeningAddress)
	err := http.ListenAndServe(s.Configuration.ListeningAddress, mux)
	if err != nil {
		s.Logger.Errorf("listening error: ", err)
	}
}

// Start initializes and runs the service
func (s *HomeControlServer) Start(svc service.Service) error {

	slogger, err := svc.Logger(nil)
	if err != nil {
		return fmt.Errorf("problem while creating logger: ", err)
	}
	slogger.Info("Starting home automation server!")
	exePath, err := os.Executable()
	if err != nil {
		slogger.Error("error getting exe path:", err)
	}
	exeDir, _ := util.PathSplit(exePath)
	s.BasePath = exeDir
	// slogger.Info("exe path: ", exePath)
	// slogger.Info("exe file: ", exeFile)
	// slogger.Info("exe dir: ", exeDir)
	cfgFile := path.Join(s.BasePath, "config.json")

	cfg, err := config.GetConfig(cfgFile)
	if err != nil {
		slogger.Errorf("error loading config file:%s, err: %v", cfgFile, err)
	}
	s.Configuration = cfg

	if err != nil {
		slogger.Error("log creation err: ", err)
	}
	slogger.Infof("read config: %v", cfg)

	s.Logger = slogger
	s.Configuration = cfg
	s.IsService = true
	go s.InitServer()
	return nil
}

// Stop halts the service operation
func (s *HomeControlServer) Stop(_ service.Service) error {
	s.QuitChan <- true
	time.Sleep(1 * time.Second)
	os.Exit(0)
	return nil
}
