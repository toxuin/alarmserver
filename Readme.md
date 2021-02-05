# IP Camera Alarm Server

Many off-brand IP Cameras come with "Alarm Server" feature that has no details about it - no protocol, no specific company name, nothing.

My IP cameras have ONVIF support and that includes motion alarms, but turns out it does not mean the Human Detection alarm is exposed through ONVIF. As well as all the other alarms like "SD card dead" or "failed admin login". 

This project is supposed to fix that.

It can accept alarms from IP cameras and re-transmit it to MQTT. This allows you to integrate all your cheap Chinese cameras with your Home Assistant, Node-red or what have you. 

When alarm server is coming online, it will also send a status message to `/camera-alerts` topic with its status.

It does also support logs from the camera - as long as they're streamed through the same "alarm-server" protocol

### Supported cameras

If your camera needs Internet Explorer to access its Web Interface and would not work in any other browser - you've come to the right place. Known affected are cameras made by HiSilicon and sold by hundreds of different names.

If your camera works with this alarm server - create an issue with some details about it and a picture and we'll post it here. 

### Configuration

All the configuration can be done through environmental variables.

`TCP_PORT`: Port on which the app will listen for incoming alarms. Default: 15002

`MQTT_TOPIC_ROOT`: MQTT topic that all the messages will originate in. Example topic: `/camera-alerts/23476289374/HumanDetect` Default: `camera-alerts`
 
`MQTT_SERVER`: Your MQTT broker's address. Default: mqtt.example.com **<- SET THIS!**

`MQTT_PORT`: Your MQTT broker's port. Default: 1883

`MQTT_USERNAME`: Username you use to authenticate with your broker. Default: `anonymous`

`MQTT_PASSWORD`: Password for authentication with your broker. Default: empty

`DEBUG`: If set to "true", app will generate some debug messages. Default: false

### Docker

There is a pre-built image `toxuin/alarmserver`.

Usage: `docker run -d -e MQTT_SERVER=mqtt.yourbroker.com -e MQTT_USERNAME=admin -e MQTT_PASSWORD=assword13 -p 15002:15002 toxuin/alarmserver`

### License

MIT License
