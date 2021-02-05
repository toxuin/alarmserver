package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/spf13/viper"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

func init()  {
	viper.SetDefault("TcpPort", "15002")
	viper.SetDefault("MqttPort", "1883")
	viper.SetDefault("MqttTopicRoot", "camera-alerts")
	viper.SetDefault("MqttServer", "mqtt.example.com")
	viper.SetDefault("MqttUsername", "anonymous")
	viper.SetDefault("MqttPassword", "")
	viper.SetDefault("Debug", false)

	_ = viper.BindEnv("TcpPort", "TCP_PORT")
	_ = viper.BindEnv("MqttPort", "MQTT_PORT")
	_ = viper.BindEnv("MqttTopicRoot", "MQTT_TOPIC_ROOT")
	_ = viper.BindEnv("MqttServer", "MQTT_SERVER")
	_ = viper.BindEnv("MqttUsername", "MQTT_USERNAME")
	_ = viper.BindEnv("MqttPassword", "MQTT_PASSWORD")
	_ = viper.BindEnv("Debug", "DEBUG")

	if viper.GetBool("Debug") {
		fmt.Printf(`STARTING WITH CONFIG:
		TcpPort: %s,
		MqttPort: %s,
		MqttTopicRoot: %s,
		MqttServer: %s,
		MqttUsername: %s,
		MqttPassword set: %t,
		Debug: %t`+"\n",
			viper.GetString("TcpPort"),
			viper.GetString("MqttPort"),
			viper.GetString("MqttTopicRoot"),
			viper.GetString("MqttServer"),
			viper.GetString("MqttUsername"),
			viper.GetString("MqttPassword") != "",
			viper.GetBool("Debug"),
		)
	}
}

var mqttClient MQTT.Client

var TcpPort string
var MqttPort string
var MqttServer string
var MqttUsername string
var MqttPassword string
var MqttTopicRoot string
var Debug bool

func sendMqttMessage(topic string, payload interface{}) {
	if !mqttClient.IsConnected() {
		fmt.Println("MQTT CLIENT NOT CONNECTED")
		return
	}
	if token := mqttClient.Publish(topic, 0, false, payload); token.Wait() && token.Error() != nil {
		fmt.Printf("MQTT ERROR, %s\n", token.Error())
	}
}

func hexIpToCIDR(hexAddr string) string { // 0x1704A8C0 -> 192.168.4.23
	hexAddrStr := fmt.Sprintf("%v", hexAddr)[2:]
	ipAddrHexParts := strings.Split(hexAddrStr, "")

	var decParts []string
	lastPart := ""
	for ind, part := range ipAddrHexParts {
		if ind%2 == 0 {
			lastPart = part
		} else {
			decParts = append(decParts, lastPart+part)
		}
	}
	var strParts []string
	for _, part := range decParts {
		dec, _ := strconv.ParseInt(part, 16, 64)
		// PREPEND RESULT TO SLICE
		strParts = append(strParts, "")
		copy(strParts[1:], strParts)
		strParts[0] = strconv.Itoa(int(dec))
	}
	ipAddr := fmt.Sprintf("%s", strings.Join(strParts[:], "."))
	return ipAddr
}

func handleTcpConnection(conn net.Conn) {
	defer conn.Close()

	if Debug {
		fmt.Printf("DEVICE CONNECTED: %s\n", conn.RemoteAddr().String())
	}
	var buf bytes.Buffer

	_, err := io.Copy(&buf, conn)
	if err != nil {
		fmt.Printf("TCP READ ERROR: %s\n", err)
		return
	}
	bufString := buf.String()
	resultString := bufString[strings.IndexByte(bufString, '{'):]
	if Debug {
		fmt.Printf("DEVICE ALERT: %s\n", resultString)
	}


	var dataMap map[string]interface{}

	if err := json.Unmarshal([]byte(resultString), &dataMap); err != nil {
		fmt.Printf("JSON PARSE ERROR: %s\n", err)
		return
	}
	if dataMap["Address"] != nil {
		hexAddrStr := fmt.Sprintf("%v", dataMap["Address"])
		dataMap["ipAddr"] = hexIpToCIDR(hexAddrStr)
	}

	jsonBytes, err := json.Marshal(dataMap)
	if err != nil {
		fmt.Printf("JSON STRINGIFY ERROR: %s\n", err)
		return
	}

	if dataMap["SerialID"] == nil {
		fmt.Println("UNKNOWN DEVICE SERIAL ID")
		fmt.Println(dataMap)
		return
	}

	serialId := fmt.Sprintf("%v", dataMap["SerialID"])
	event := fmt.Sprintf("%v", dataMap["Event"])

	sendMqttMessage(MqttTopicRoot+ "/" + serialId + "/" + event, string(jsonBytes))
}

func main() {
	// LOAD CONFIG
	TcpPort = viper.GetString("TcpPort")
	MqttPort = viper.GetString("MqttPort")
	MqttServer = viper.GetString("MqttServer")
	MqttUsername = viper.GetString("MqttUsername")
	MqttPassword = viper.GetString("MqttPassword")
	MqttTopicRoot = viper.GetString("MqttTopicRoot")
	Debug = viper.GetBool("Debug")

	fmt.Println("STARTING...")
	// START MQTT BUS
	mqttOpts := MQTT.NewClientOptions().AddBroker("tcp://" + MqttServer + ":" + MqttPort)
	mqttOpts.SetUsername(MqttUsername)
	if MqttPassword != "" {
		mqttOpts.SetPassword(MqttPassword)
	}
	mqttOpts.SetAutoReconnect(true)
	mqttOpts.SetClientID("alarmserver-go")
	mqttOpts.SetKeepAlive(2 * time.Second)
	mqttOpts.SetPingTimeout(1 * time.Second)
	mqttOpts.SetWill(MqttTopicRoot, `{ "status": "down" }`, 0, false)

	mqttOpts.OnConnect = func(client MQTT.Client) {
		fmt.Printf("MQTT: CONNECTED TO %s\n", MqttServer)
	}

	mqttOpts.DefaultPublishHandler = func(client MQTT.Client, msg MQTT.Message) {
		if Debug {
			fmt.Printf("MQTT TOPIC: %s\n", msg.Topic())
			fmt.Printf("MQTT MESSAGE: %s\n", msg.Payload())
		}
	}

	mqttClient = MQTT.NewClient(mqttOpts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	sendMqttMessage(MqttTopicRoot, `{ "status": "up" }`)

	// START TCP SERVER
	tcpListener, err := net.Listen("tcp4", ":" + TcpPort)
	if err != nil {
		panic(err)
	}
	defer tcpListener.Close()

	for {
		conn, err := tcpListener.Accept()
		if err != nil {
			panic(err)
		}
		go handleTcpConnection(conn)
	}

}
