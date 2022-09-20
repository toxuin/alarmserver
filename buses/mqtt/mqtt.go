package mqtt

import (
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/toxuin/alarmserver/config"
	"math/rand"
	"strconv"
	"time"
)

type Bus struct {
	Debug  bool
	client MQTT.Client
}

func (mqtt *Bus) Initialize(config config.MqttConfig) {
	fmt.Println("Initializing MQTT bus...")
	mqttOpts := MQTT.NewClientOptions().AddBroker("tcp://" + config.Server + ":" + config.Port)
	mqttOpts.SetUsername(config.Username)
	if config.Password != "" {
		mqttOpts.SetPassword(config.Password)
	}
	mqttOpts.SetAutoReconnect(true)
	mqttOpts.SetMaxReconnectInterval(10 * time.Second)

	mqttOpts.SetClientID("alarmserver-go-" + strconv.Itoa(rand.Intn(100)))
	mqttOpts.SetKeepAlive(2 * time.Second)
	mqttOpts.SetPingTimeout(1 * time.Second)
	mqttOpts.SetWill(config.TopicRoot+"/alarmserver", `{ "status": "down" }`, 0, false)

	mqttOpts.OnConnect = func(client MQTT.Client) {
		fmt.Printf("MQTT: CONNECTED TO %s\n", config.Server)
		mqtt.SendMessage(config.TopicRoot+"/alarmserver", `{ "status": "up" }`)
	}

	mqttOpts.SetConnectionLostHandler(func(client MQTT.Client, err error) {
		fmt.Printf("MQTT ERROR - connection lost: %+v\n", err)
	})
	mqttOpts.SetReconnectingHandler(func(client MQTT.Client, options *MQTT.ClientOptions) {
		fmt.Println("MQTT: ...trying to reconnect...")
	})

	mqttOpts.DefaultPublishHandler = func(client MQTT.Client, msg MQTT.Message) {
		if mqtt.Debug {
			fmt.Printf("  MQTT: TOPIC: %s\n  MQTT: MESSAGE: %s\n", msg.Topic(), msg.Payload())
		}
	}

	mqtt.client = MQTT.NewClient(mqttOpts)
	if token := mqtt.client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
}

func (mqtt *Bus) SendMessage(topic string, payload interface{}) {
	if !mqtt.client.IsConnected() {
		fmt.Println("MQTT: CLIENT NOT CONNECTED")
		return
	}
	if token := mqtt.client.Publish(topic, 0, false, payload); token.Wait() && token.Error() != nil {
		fmt.Printf("MQTT ERROR, %s\n", token.Error())
	}
	if mqtt.Debug {
		fmt.Printf("MQTT: Sent message to %s\n", topic)
	}
}
