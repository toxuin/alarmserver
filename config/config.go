package config

import (
	"fmt"
	"github.com/spf13/viper"
	"github.com/toxuin/alarmserver/servers/amcrest"
	"github.com/toxuin/alarmserver/servers/hikvision"
)

type Config struct {
	Debug     bool            `json:"debug"`
	Mqtt      MqttConfig      `json:"mqtt"`
	Webhooks  WebhooksConfig  `json:"webhooks"`
	Hisilicon HisiliconConfig `json:"hisilicon"`
	Hikvision HikvisionConfig `json:"hikvision"`
	Amcrest   AmcrestConfig   `json:"amcrest"`
	Ftp       FtpConfig       `json:"ftp"`
}

type MqttConfig struct {
	Enabled   bool   `json:"enabled"`
	Server    string `json:"server"`
	Port      string `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	TopicRoot string `json:"topicRoot"`
}

type WebhooksConfig struct {
	Enabled bool     `json:"enabled"`
	Urls    []string `json:"urls"`
}

type HisiliconConfig struct {
	Enabled bool   `json:"enabled"`
	Port    string `json:"port"`
}

type HikvisionConfig struct {
	Enabled bool                  `json:"enabled"`
	Cams    []hikvision.HikCamera `json:"cams"`
}

type AmcrestConfig struct {
	Enabled bool                `json:"enabled"`
	Cams    []amcrest.AmcCamera `json:"cams"`
}

type FtpConfig struct {
	Enabled    bool   `json:"enabled"`
	Port       int    `json:"port"`
	AllowFiles bool   `json:"allowFiles"`
	Password   string `json:"password"`
	RootPath   string `json:"rootPath"`
}

func (c *Config) SetDefaults() {
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
	viper.SetDefault("amcrest.enabled", false)
	viper.SetDefault("ftp.enabled", false)
	viper.SetDefault("ftp.port", 21)
	viper.SetDefault("ftp.allowFiles", true)
	viper.SetDefault("ftp.password", "root")
	viper.SetDefault("ftp.rootPath", "./ftp")

	_ = viper.BindEnv("debug", "DEBUG")
	_ = viper.BindEnv("mqtt.port", "MQTT_PORT")
	_ = viper.BindEnv("mqtt.topicRoot", "MQTT_TOPIC_ROOT")
	_ = viper.BindEnv("mqtt.server", "MQTT_SERVER")
	_ = viper.BindEnv("mqtt.username", "MQTT_USERNAME")
	_ = viper.BindEnv("mqtt.password", "MQTT_PASSWORD")
	_ = viper.BindEnv("hisilicon.enabled", "HISILICON_ENABLED")
	_ = viper.BindEnv("hisilicon.port", "HISILICON_PORT", "TCP_PORT")
	_ = viper.BindEnv("hikvision.enabled", "HIKVISION_ENABLED")
	_ = viper.BindEnv("hikvision.cams", "HIKVISION_CAMS")
	_ = viper.BindEnv("amcrest.enabled", "AMCREST_ENABLED")
	_ = viper.BindEnv("amcrest.cams", "AMCREST_CAMS")
	_ = viper.BindEnv("ftp.enabled", "FTP_ENABLED")
	_ = viper.BindEnv("ftp.port", "FTP_PORT")
	_ = viper.BindEnv("ftp.allowFiles", "FTP_ALLOW_FILES")
	_ = viper.BindEnv("ftp.password", "FTP_PASSWORD")
	_ = viper.BindEnv("ftp.rootPath", "FTP_ROOT_PATH")

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

func (c *Config) Load() *Config {
	myConfig := Config{
		Debug:     viper.GetBool("debug"),
		Mqtt:      MqttConfig{},
		Webhooks:  WebhooksConfig{},
		Hisilicon: HisiliconConfig{},
		Hikvision: HikvisionConfig{},
		Amcrest:   AmcrestConfig{},
	}

	if viper.IsSet("mqtt") {
		err := viper.Sub("mqtt").Unmarshal(&myConfig.Mqtt)
		if err != nil {
			panic(fmt.Errorf("unable to decode mqtt config, %v", err))
		}
	}
	if viper.IsSet("webhooks") {
		err := viper.Sub("webhooks").Unmarshal(&myConfig.Webhooks)
		if err != nil {
			panic(fmt.Errorf("unable to decode webhooks config, %v", err))
		}
	}
	if viper.IsSet("hisilicon") {
		err := viper.Sub("hisilicon").Unmarshal(&myConfig.Hisilicon)
		if err != nil {
			panic(fmt.Errorf("unable to decode hisilicon config, %v", err))
		}
	}
	if viper.IsSet("amcrest") {
		err := viper.Sub("amcrest").Unmarshal(&myConfig.Amcrest)
		if err != nil {
			panic(fmt.Errorf("unable to decode amcrest config, %v", err))
		}
	}
	if viper.IsSet("ftp") {
		err := viper.Sub("ftp").Unmarshal(&myConfig.Ftp)
		if err != nil {
			panic(fmt.Errorf("unable to decode FTP config, %v", err))
		}
	}

	if !myConfig.Mqtt.Enabled && !myConfig.Webhooks.Enabled {
		panic("Both MQTT and Webhook buses are disabled. Nothing to do!")
	}

	if !myConfig.Hisilicon.Enabled && !myConfig.Hikvision.Enabled && !myConfig.Amcrest.Enabled && !myConfig.Ftp.Enabled {
		panic("No Servers are enabled. Nothing to do!")
	}

	if viper.IsSet("hikvision.cams") {
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
				if camConfig.GetBool("rawTcp") {
					camera.BrokenHttp = true
				}
				if myConfig.Debug {
					fmt.Printf("Added Hikvision camera:\n"+
						"  name: %s \n"+
						"  url: %s \n"+
						"  username: %s \n"+
						"  password set: %t\n"+
						"  rawRcp: %t\n",
						camera.Name,
						camera.Url,
						camera.Username,
						camera.Password != "",
						camera.BrokenHttp,
					)
				}

				myConfig.Hikvision.Cams = append(myConfig.Hikvision.Cams, camera)
			}
		}
	}

	if viper.IsSet("amcrest.cams") {
		amcrestCamsConfig := viper.Sub("amcrest.cams")
		if amcrestCamsConfig != nil {
			camConfigs := viper.GetStringMapString("amcrest.cams")
			for camName := range camConfigs {
				camConfig := viper.Sub("amcrest.cams." + camName)
				// CONSTRUCT CAMERA URL
				url := ""
				if camConfig.GetBool("https") {
					url += "https://"
				} else {
					url += "http://"
				}
				url += camConfig.GetString("address")

				camera := amcrest.AmcCamera{
					Debug:    myConfig.Debug,
					Name:     camName,
					Url:      url,
					Username: camConfig.GetString("username"),
					Password: camConfig.GetString("password"),
				}
				if myConfig.Debug {
					fmt.Printf("Added Amcrest camera:\n"+
						"  name: %s \n"+
						"  url: %s \n"+
						"  username: %s \n"+
						"  password set: %t\n",
						camera.Name,
						camera.Url,
						camera.Username,
						camera.Password != "",
					)
				}

				myConfig.Amcrest.Cams = append(myConfig.Amcrest.Cams, camera)
			}
		}
	}

	return &myConfig
}

func (c *Config) Printout() {
	fmt.Printf("CONFIG:\n"+
		"  Hisilicon server enabled: %t\n"+
		"    port: %s\n"+
		"  Hikvision server enabled: %t\n"+
		"    cams: %v\n"+
		"  Amcrest server enabled: %t\n"+
		"    cams: %v\n"+
		"  FTP server enabled: %t\n"+
		"    allow files: %t\n"+
		"    root path: %s\n"+
		"    port: %v\n"+
		"  Webhooks bus enabled: %t\n"+
		"    URLs: %v\n"+
		"  MQTT bus enabled: %t\n"+
		"    port: %s\n"+
		"    root topic: %s\n"+
		"    server address: %s\n"+
		"    username: %s\n"+
		"    password set: %t\n",
		c.Hisilicon.Enabled,
		c.Hisilicon.Port,
		c.Hikvision.Enabled,
		len(c.Hikvision.Cams),
		c.Amcrest.Enabled,
		len(c.Amcrest.Cams),
		c.Ftp.Enabled,
		c.Ftp.AllowFiles,
		c.Ftp.RootPath,
		c.Ftp.Port,
		c.Webhooks.Enabled,
		len(c.Webhooks.Urls),
		c.Mqtt.Enabled,
		c.Mqtt.Port,
		c.Mqtt.TopicRoot,
		c.Mqtt.Server,
		c.Mqtt.Username,
		c.Mqtt.Password != "",
	)
}
