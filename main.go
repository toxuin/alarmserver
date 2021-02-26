package main

import (
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/spf13/viper"
	"github.com/toxuin/alarmserver/lib/hikvision"
	"github.com/toxuin/alarmserver/lib/hisilicon"
	"math/rand"
	"time"
)

type Config struct {
	Debug     bool            `json:"debug"`
	Mqtt      MqttConfig      `json:"mqtt"`
	Hisilicon HisiliconConfig `json:"hisilicon"`
	Hikvision HikvisionConfig `json:"hikvision"`
}

type MqttConfig struct {
	Server    string `json:"server"`
	Port      string `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	TopicRoot string `json:"topicRoot"`
}

type HisiliconConfig struct {
	Enabled bool   `json:"enabled"`
	Port    string `json:"port"`
}

type HikvisionConfig struct {
	Enabled bool                  `json:"enabled"`
	Cams    []hikvision.HikCamera `json:"cams"`
}

var mqttClient MQTT.Client
var config Config

func init() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	viper.SetDefault("debug", false)
	viper.SetDefault("mqtt.port", 1883)
	viper.SetDefault("mqtt.topicRoot", "camera-alerts")
	viper.SetDefault("mqtt.server", "mqtt.example.com")
	viper.SetDefault("mqtt.username", "anonymous")
	viper.SetDefault("mqtt.password", "")
	viper.SetDefault("hisilicon.enabled", true)
	viper.SetDefault("hisilicon.port", 15002)
	viper.SetDefault("hikvision.enabled", false)

	_ = viper.BindEnv("debug", "DEBUG")
	_ = viper.BindEnv("mqtt.port", "MQTT_PORT")
	_ = viper.BindEnv("mqtt.topicRoot", "MQTT_TOPIC_ROOT")
	_ = viper.BindEnv("mqtt.server", "MQTT_SERVER")
	_ = viper.BindEnv("mqtt.username", "MQTT_USERNAME")
	_ = viper.BindEnv("mqtt.password", "MQTT_PASSWORD")
	_ = viper.BindEnv("hisilicon.enabled", "HISILICON_ENABLED")
	_ = viper.BindEnv("hisilicon.port", "HISILICON_PORT", "TCP_PORT")
	_ = viper.BindEnv("hikvision.enabled", "HIKVISION_ENABLED")
	_ = viper.BindEnv("hikvision.cams", "HIKVISION_ENABLED")

	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			err := viper.SafeWriteConfig()
			if err != nil {
				panic(fmt.Errorf("error saving default config file: %s \n", err))
			}
		} else {
			panic(fmt.Errorf("error reading config file: %s \n", err))
		}
	}
}

func sendMqttMessage(topic string, payload interface{}) {
	if !mqttClient.IsConnected() {
		fmt.Println("MQTT CLIENT NOT CONNECTED")
		return
	}
	if token := mqttClient.Publish(topic, 0, false, payload); token.Wait() && token.Error() != nil {
		fmt.Printf("MQTT ERROR, %s\n", token.Error())
	}
}

func main() {
	// LOAD CONFIG
	config = Config{
		Debug:     viper.GetBool("debug"),
		Mqtt:      MqttConfig{},
		Hisilicon: HisiliconConfig{},
		Hikvision: HikvisionConfig{
			Enabled: viper.GetBool("hikvision.enabled"),
		},
	}

	err := viper.Sub("mqtt").Unmarshal(&config.Mqtt)
	if err != nil {
		panic(fmt.Errorf("unable to decode mqtt config, %v", err))
	}
	err = viper.Sub("hisilicon").Unmarshal(&config.Hisilicon)
	if err != nil {
		panic(fmt.Errorf("unable to decode hisilicon config, %v", err))
	}

	if !config.Hisilicon.Enabled && !config.Hikvision.Enabled {
		panic("Both Hisilicon and Hikvision modules are disabled. Nothing to do!")
	}

	hikvisionCamsConfig := viper.Sub("hikvision.cams")
	if hikvisionCamsConfig != nil {
		camConfigs := viper.GetStringMapString("hikvision.cams")

		for camName := range camConfigs {
			camConfig := viper.Sub("hikvision.cams." + camName)
			// CONSTRUCT CAMERA URL
			url := ""
			if camConfig.GetBool("https") {
				url += "https://"
			} else {
				url += "http://"
			}
			url += camConfig.GetString("address") + "/ISAPI/"

			camera := hikvision.HikCamera{
				Name:     camName,
				Url:      url,
				Username: camConfig.GetString("username"),
				Password: camConfig.GetString("password"),
			}
			if config.Debug {
				fmt.Printf("Added Hikvision camera:\n"+
					"  name: %s \n"+
					"  url: %s \n"+
					"  username: %s \n"+
					"  password set: %t\n",
					camera.Name,
					camera.Url,
					camera.Username,
					camera.Password != "")
			}

			config.Hikvision.Cams = append(config.Hikvision.Cams, camera)
		}
	}

	fmt.Println("STARTING...")
	if config.Debug {
		fmt.Printf("CONFIG:\n"+
			"  Hisilicon module enabled: %t \n"+
			"  Hikvision module enabled: %t \n"+
			"  mqtt.port: %s \n"+
			"  mqtt.topicRoot: %s \n"+
			"  mqtt.server: %s \n"+
			"  mqtt.username: %s \n"+
			"  mqtt.password set: %t \n",
			config.Hisilicon.Enabled,
			config.Hikvision.Enabled,
			config.Mqtt.Port,
			config.Mqtt.TopicRoot,
			config.Mqtt.Server,
			config.Mqtt.Username,
			config.Mqtt.Password != "",
		)
	}

	// START MQTT BUS
	mqttOpts := MQTT.NewClientOptions().AddBroker("tcp://" + config.Mqtt.Server + ":" + config.Mqtt.Port)
	mqttOpts.SetUsername(config.Mqtt.Username)
	if config.Mqtt.Password != "" {
		mqttOpts.SetPassword(config.Mqtt.Password)
	}
	mqttOpts.SetAutoReconnect(true)
	mqttOpts.SetClientID("alarmserver-go" + string(rune(rand.Intn(100))))
	mqttOpts.SetKeepAlive(2 * time.Second)
	mqttOpts.SetPingTimeout(1 * time.Second)
	mqttOpts.SetWill(config.Mqtt.TopicRoot+"/alarmserver", `{ "status": "down" }`, 0, false)

	mqttOpts.OnConnect = func(client MQTT.Client) {
		fmt.Printf("MQTT: CONNECTED TO %s\n", config.Mqtt.Server)
	}

	mqttOpts.DefaultPublishHandler = func(client MQTT.Client, msg MQTT.Message) {
		if config.Debug {
			fmt.Printf("MQTT TOPIC: %s\n", msg.Topic())
			fmt.Printf("MQTT MESSAGE: %s\n", msg.Payload())
		}
	}

	mqttClient = MQTT.NewClient(mqttOpts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	sendMqttMessage(config.Mqtt.TopicRoot+"/alarmserver", `{ "status": "up" }`)

	if config.Hisilicon.Enabled {
		// START HISILICON ALARM SERVER
		hisiliconServer := hisilicon.Server{
			Debug: config.Debug,
			Port:  config.Hisilicon.Port,
			MessageHandler: func(topic string, data string) {
				sendMqttMessage(config.Mqtt.TopicRoot+"/"+topic, data)
			},
		}
		hisiliconServer.Start()
	}

	if config.Hikvision.Enabled {
		// START HIKVISION SERVER
		hikvisionServer := hikvision.Server{
			Debug:   config.Debug,
			Cameras: &config.Hikvision.Cams,
		}
		hikvisionServer.Start()
	}
}
