package main

import (
	"flag"
	"fmt"
	"log"

	config "github.com/amitbet/tomcierge/config"
	"github.com/amitbet/tomcierge/logger"
	"github.com/amitbet/volume-go"
	"github.com/kardianos/service"
)

// GetConfig reads the config.json and returns an object
func GetConfig() (*config.Config, error) {
	return config.GetConfig("config.json")
}

func main() {
	isServiceFlag := flag.Bool("service", true, "indicates whether or not we are runnig as a service (defaults to true)")
	installFlag := flag.Bool("install", false, "install the application as a service")
	uninstallFlag := flag.Bool("uninstall", false, "uninstall the service")
	setVolFlag := flag.Int("setvol", -1, " sets the volume and closes the process")

	flag.Parse()

	if *setVolFlag != -1 {
		volume.SetVolume(*setVolFlag)
		return
	}

	srv := &HomeControlServer{}

	svcConfig := &service.Config{
		Name:        "HomeAutomationService",
		DisplayName: "Home Automation Service",
		Description: "A service that controls devices around the house",
	}
	s, err := service.New(srv, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	slogger, err := s.Logger(nil)
	if err != nil {
		log.Fatal(err)
	}
	srv.Logger = slogger
	if err != nil {
		slogger.Errorf("error during service creation: ", err)
	}

	if *installFlag {
		s.Install()
		return
	}

	if *uninstallFlag {
		s.Uninstall()
		return
	}
	if *isServiceFlag {
		err = s.Run()
		if err != nil {
			logger.Error(err)
		}
	} else {
		cfg, err := GetConfig()
		if err != nil {
			fmt.Println("error while loading config: ", err)
		}
		srv.Configuration = cfg
		srv.IsService = false
		srv.Logger = slogger
		srv.BasePath = "./"
		srv.InitServer()
	}
}
