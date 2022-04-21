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
	mqtt "github.com/eclipse/paho.mqtt.golang"
	gmux "github.com/gorilla/mux"
	"github.com/grandcat/zeroconf"
	"github.com/itchyny/volume-go"
	"github.com/kardianos/service"
	ssdp "github.com/koron/go-ssdp"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
)

// HomeControlServer is the listener for the REST Api and the device controller
type HomeControlServer struct {
	IsService        bool
	QuitChan         chan bool
	SearcherQuitChan chan bool
	Logger           service.Logger
	Configuration    *config.Config
	BasePath         string
	taskTimer        *time.Ticker
	localVol         int
	MqttClient       mqtt.Client
	StartTime        time.Time
	SystemInfo       *SystemInfo
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
func (s *HomeControlServer) hibernate(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Hibernating ... ")
	util.MachineSleep()
}

func (s *HomeControlServer) playAlertSound(w http.ResponseWriter, r *http.Request) {
	//vars := gmux.Vars(r)
	//sndFile := vars["file"]

	fmt.Printf("Playing Alert sounds ... ")
	util.PlayLocalAlert(s.IsService, s.Logger, s.Configuration.Alert)
	// for _, sndFile := range s.Configuration.Alert {
	// 	util.SndPlaySoundW(path.Join("sound", sndFile), util.SND_SYNC|util.SND_SYSTEM|util.SND_RING)
	// }
	//fmt.Println("done")
	jObj := map[string]interface{}{
		"play": "done",
	}
	s.httpRespond(w, jObj)
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

func (s *HomeControlServer) ssdpSearcher(quit chan bool) {

	aliveTick := time.Tick(5 * time.Second)

	// run Searcher periodically.
	for {
		select {
		case <-aliveTick:
			svcList := s.ssdpSearch("urn:schemas-upnp-org:service:tomcierge:1", 5, "")
			for _, svc := range svcList {
				svcName := svc.USN[3:]
				svcUrl := svc.Location
				s.Configuration.VolumeServiceList[strings.ToLower(svcName)] = svcUrl
			}
		case <-quit:
			s.Logger.Info("Closing ssdp searcher")
			return
		}
	}
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

// startChronJobs runs the polling loop so volume level data remains up to date
func (s *HomeControlServer) startChronJobs() {
	s.taskTimer = time.NewTicker(10 * time.Second)
	go func() {
		for {
			<-s.taskTimer.C
			newVol, err := volume.GetVolume()
			if err != nil {
				s.Logger.Errorf("error while getting volume: %v", err)
				continue
			}
			//s.Logger.Info("got volume: ", s.localVol)
			s.localVol = newVol

			s.SendMqttStatistics()
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
	s.StartTime = time.Now()

	s.InitMqtt()
	s.startChronJobs()
	if s.Configuration.CanControlDevices {
		go s.initDevices()
	}
	var sigTerm = make(chan os.Signal)
	s.QuitChan = make(chan bool)
	s.SearcherQuitChan = make(chan bool)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	go func() {
		sig := <-sigTerm
		fmt.Printf("caught sig: %+v\n", sig)
		fmt.Println("Waiting for a second to finish processing")
		s.QuitChan <- true
		s.SearcherQuitChan <- true
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()

	//go zconfRegister(quit)
	//go zconfDiscover(cfg.VolumeServiceList)

	go s.ssdpAdvertise(s.QuitChan)
	go s.ssdpSearcher(s.SearcherQuitChan)

	mux := gmux.NewRouter() //.StrictSlash(true)

	mux.HandleFunc("/set-volume", s.setVolumeOnMachine).Queries("machine", "{machine}")
	//mux.HandleFunc("/set-volume", setVolume)
	mux.HandleFunc("/get-volume", s.getVolumeOnMachine).Queries("machine", "{machine}")
	mux.HandleFunc("/get-volume", s.getVolume)
	mux.HandleFunc("/configuration", s.sendConfig)
	mux.HandleFunc("/alert", s.playAlertSound)
	mux.HandleFunc("/hibernate", s.hibernate)

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

// var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
func (s *HomeControlServer) MqttHandler(client mqtt.Client, msg mqtt.Message) {
	fmt.Printf("Received message: %s from topic: %s\n", msg.Payload(), msg.Topic())
	cmndPrefix := "cmnd/" + s.Configuration.MachineName + "/"
	switch msg.Topic() {
	case cmndPrefix + "hibernate":
		util.MachineSleep()
	case cmndPrefix + "sleep":
		util.MachineSleep()
	case cmndPrefix + "shutdown":
		util.MachineSleep()
	case cmndPrefix + "setvol":
		volStr := string(msg.Payload())
		vol, err := strconv.ParseInt(volStr, 10, 64)
		if err != nil {
			s.Logger.Error(err)
		}
		volume.SetVolume(int(vol))
	}
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	fmt.Println("Connected")
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	fmt.Printf("Connect lost: %v", err)
}

func (s *HomeControlServer) MqttPub(topic, message string) {
	token := s.MqttClient.Publish(topic, 0, false, message)
	token.Wait()
}

func (s *HomeControlServer) MqttSub(topic string) {
	//topic := "topic/test"
	token := s.MqttClient.Subscribe(topic, 1, nil)
	token.Wait()
	fmt.Printf("Subscribed to topic %s: ", topic)
}

func (s *HomeControlServer) MqttSubToCommands(commands []string) {
	cmndPrefix := "cmnd/" + s.Configuration.MachineName + "/"
	for _, cmd := range commands {
		s.MqttSub(cmndPrefix + cmd)
	}
}

func (s *HomeControlServer) InitMqtt() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%s", s.Configuration.MqttBrokerAddress, s.Configuration.MqttBrokerPort))

	opts.SetClientID(s.Configuration.MachineName + "_mqtt_client")
	opts.SetUsername(s.Configuration.MqttUser)
	opts.SetPassword(s.Configuration.MqttPassword)
	opts.SetDefaultPublishHandler(s.MqttHandler)

	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler
	s.MqttClient = mqtt.NewClient(opts)
	if token := s.MqttClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	s.MqttSubToCommands([]string{"hibernate", "setvol", "shutdown", "sleep"})

}

// Generated by https://quicktype.io

type MachineStats struct {
	Time      string `json:"Time"`
	Uptime    string `json:"Uptime"`
	UptimeSEC int64  `json:"UptimeSec"`
	Heap      int64  `json:"Heap"`
	// SleepMode string `json:"SleepMode"`
	// Sleep     int64  `json:"Sleep"`
	LoadAvg   int64  `json:"LoadAvg"`
	MqttCount int64  `json:"MqttCount"`
	Power     string `json:"POWER"`
	// Wifi               Wifi       `json:"Wifi"`
	Volume             string     `json:"volume"`
	MemoryUsagePrecent float64    `json:"memoryUsagePrecent"`
	MemoryTotal        uint64     `json:"memoryTotal"`
	MemoryUsed         uint64     `json:"memoryUsed"`
	System             SystemInfo `json:"system"`
}
type SystemInfo struct {
	CpuFamily    string `json:"cpuFamily"`
	CpuModel     string `json:"cpuModel"`
	CpuModelName string `json:"cpuModelName"`
	CpuVendorID  string `json:"cpuVendorID"`
	CpuCores     int32  `json:"cpuCores"`
	Hostname     string `json:"hostname"`
	Uptime       string `json:"uptime"`
	UptimeSec    uint64 `json:"uptimeSec"`
	Os           string `json:"os"`
}

type Timespan time.Duration

func (t Timespan) TasmotaFormat() string {
	z := time.Unix(0, 0).UTC()
	hrsMinSec := z.Add(time.Duration(t)).Format("15:04:05")
	days := int64(t) / int64(time.Second) / 60 / 60 / 24
	return fmt.Sprint(days) + "T" + hrsMinSec
}

func (s *HomeControlServer) SendMqttStatistics() {

	//cpuAvg, err := util.CpuLoad()
	cpuTimes, err := cpu.Times(false)
	if err != nil {
		s.Logger.Error(err)
	}
	cpuTime := cpuTimes[0]
	cpuLoad := cpuTime.Idle / cpuTime.Total()

	if s.SystemInfo == nil {

		hostInfo, err := host.Info()
		if err != nil {
			s.Logger.Error(err)
		}
		cpuInfos, err := cpu.Info()
		if err != nil {
			s.Logger.Error(err)
		}
		cpuInfo := cpuInfos[0]
		utime := Timespan(hostInfo.Uptime * uint64(time.Second))
		s.SystemInfo = &SystemInfo{
			CpuFamily:    cpuInfo.Family,
			CpuModel:     cpuInfo.Model,
			CpuModelName: cpuInfo.ModelName,
			CpuVendorID:  cpuInfo.VendorID,
			CpuCores:     cpuInfo.Cores,
			Hostname:     hostInfo.Hostname,
			Uptime:       utime.TasmotaFormat(),
			UptimeSec:    hostInfo.Uptime,
			Os:           hostInfo.OS,
		}
	}

	hostInfo1, err := host.Info()
	if err != nil {
		s.Logger.Error(err)
	}

	s.SystemInfo.UptimeSec = hostInfo1.Uptime
	s.SystemInfo.Uptime = Timespan(hostInfo1.Uptime * uint64(time.Second)).TasmotaFormat()

	v, err := mem.VirtualMemory()
	if err != nil {
		s.Logger.Error(err)
	}

	UptimeDuration := (time.Since(s.StartTime))
	durTime := Timespan(UptimeDuration)
	mStats := MachineStats{
		Volume:             fmt.Sprint(s.localVol),
		Uptime:             durTime.TasmotaFormat(),
		UptimeSEC:          int64(durTime) / int64(time.Second),
		LoadAvg:            int64(cpuLoad),
		System:             *s.SystemInfo,
		MemoryUsagePrecent: v.UsedPercent,
		MemoryTotal:        v.Total,
		MemoryUsed:         v.Used,
	}

	b, err := json.Marshal(mStats)
	if err != nil {
		s.Logger.Error(err)
	}

	s.MqttPub("tele/"+s.Configuration.MachineName+"/", string(b))
}

// Start initializes and runs the service
func (s *HomeControlServer) Start(svc service.Service) error {

	slogger, err := svc.Logger(nil)
	if err != nil {
		return fmt.Errorf("problem while creating logger: %w", err)
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
	//os.Exit(0)
	return nil
}
