package main

import (
	"fmt"
	"github.com/toxuin/alarmserver/buses/mqtt"
	"github.com/toxuin/alarmserver/buses/webhooks"
	conf "github.com/toxuin/alarmserver/config"
	"github.com/toxuin/alarmserver/servers/ftp"
	"github.com/toxuin/alarmserver/servers/hikvision"
	"github.com/toxuin/alarmserver/servers/hisilicon"
)

var config *conf.Config

func init() {
	config.SetDefaults()
}

func main() {
	config = config.Load()
	fmt.Println("STARTING...")
	if config.Debug {
		config.Printout()
	}

	// INIT BUSES
	mqttBus := mqtt.Bus{Debug: config.Debug}
	if config.Mqtt.Enabled {
		mqttBus.Initialize(config.Mqtt)
		if config.Debug {
			fmt.Println("MQTT BUS INITIALIZED")
		}
	}

	webhookBus := webhooks.Bus{Debug: config.Debug}
	if !config.Webhooks.Enabled {
		webhookBus.Initialize(config.Webhooks)
		if config.Debug {
			fmt.Println("WEBHOOK BUS INITIALIZED")
		}
	}

	messageHandler := func(topic string, data string) {
		if config.Mqtt.Enabled {
			mqttBus.SendMessage(config.Mqtt.TopicRoot+"/"+topic, data)
		}
		if config.Webhooks.Enabled {
			webhookBus.SendMessage(topic, data)
		}
	}

	if config.Hisilicon.Enabled {
		// START HISILICON ALARM SERVER
		hisiliconServer := hisilicon.Server{
			Debug:          config.Debug,
			Port:           config.Hisilicon.Port,
			MessageHandler: messageHandler,
		}
		hisiliconServer.Start()
		if config.Debug {
			fmt.Println("STARTED HISILICON SERVER")
		}
	}

	if config.Hikvision.Enabled {
		// START HIKVISION SERVER
		hikvisionServer := hikvision.Server{
			Debug:          config.Debug,
			Cameras:        &config.Hikvision.Cams,
			MessageHandler: messageHandler,
		}
		hikvisionServer.Start()
		if config.Debug {
			fmt.Println("STARTED HIKVISION SERVER")
		}
	}

	if config.Ftp.Enabled {
		// START FTP SERVER
		ftpServer := ftp.Server{
			Debug:          config.Debug,
			Port:           config.Ftp.Port,
			AllowFiles:     config.Ftp.AllowFiles,
			RootPath:       config.Ftp.RootPath,
			MessageHandler: messageHandler,
		}
		ftpServer.Start()
		if config.Debug {
			fmt.Println("STARTED FTP SERVER")
		}
	}
}
