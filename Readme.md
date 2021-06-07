# IP Camera Alarm Server

![docker pulls](https://img.shields.io/docker/pulls/toxuin/alarmserver)

Universal Alarm Server for all your IP cameras in one place!

Integrates well with Home Assistant, Node-Red, etc. Runs great on Raspberry-Pi!

Supported Cameras 📸:
  - Hikvision
  - Hisilicon
  - Anything that can upload alarms to FTP

Supported Delivery 📬:
  - MQTT
  - Webhooks

## Configuration

Create file `config.yaml` in the folder where the application binary lives.

Also see [sample config](docs/config.yaml).

When alarm server is coming online, it will also send a status message to `/camera-alerts` topic with its status.

#### HiSilicon

This includes most of no-brand Chinese cameras that have "Alarm Server" feature.

If your camera needs Internet Explorer to access its Web Interface ([here's a picture!](docs/hisilicon.jpg)) and would not work in any other browser - that's the server you need. These cameras are all made by HiSilicon and sold under hundreds of different names.

Logs from the camera can also work as alarm - set that in your camera under "send logs" setting.

```yaml
hisilicon:
  enabled: true  # if false, will not listen for alarms from hisilicon cams  
  port: 15002    # has to be the same in cameras' settings
```
 
#### Hikvision

Alarm Server uses HTTP streaming to connect to each camera individually and subscribe to all the events.

Some lower-end cameras, especially doorbells and intercoms, have broken HTTP streaming implementation that can't open more that 1 connection and "close" the http response, but leave TCP connection open (without sending keep-alive header!). For those, Alarm Server has an alternative streaming implementation. To use it, set `rawTcp: true` in camera's config file.

Hikvision cameras can also be used with FTP server no problem. 

```yaml
hikvision:
  enabled: true              # if not enabled, it won't connect to any hikvision cams
  cams:
    myCam:                   # name of your camera
      address: 192.168.1.69  # ip address or domain name
      https: false           # if your camera supports ONLY https - set to true
      username: admin        # username that you use to log in to camera's web panel 
      password: admin1234    # password that you use to log in to camera's web panel
      rawTcp: false          # some cams have broken streaming. Set to true if normal HTTP streaming doesn't work 
```

#### FTP

Alarm Server will accept any username as FTP login username and use it as camera's name. As long as the password matches, it will allow the connection.

```yaml
ftp:
  enabled: true      # if not enabled, it won't accept connections
  port: 21           # has to match settings in the cameras
  password: "root"   # FTP password that will be accepted
  allowFiles: true   # if false, no files will be stored (but transfers will still happen)
  rootPath: "./ftp"  # folder where to save cameras' uploads
```

_Q_: Isn't FTP, like, slow??

_A_: No. Alarm processing part happens before the actual file upload even begins, and on your typical wireless home network that is less than 0.2 seconds. It's plenty fast.

_Q_: Why this if there is ONVIF?

_A_: Some IP cameras have ONVIF, and sometimes that even includes motion alarms, but not always does that mean that the Human Detection alarm is exposed through ONVIF, as well as all the other alarms like "SD card dead" or "failed admin login". This and, well, not all the cameras support ONVIF.

## Tested cameras:

- 3xLogic VX-2M-2D-RIA (Hikvision server)

- Uniden U-Bell DB1 (also known as Hikvision DS-KB6003, LaView LV-PDB1630-U and RCA HSDB2A)

- Misecu 5MP Wifi AI Cam (Hisilicon server)

If your camera works with Alarm Server - create an issue with some details about it and a picture, and we'll post it here. 

## Docker

There is a pre-built image `toxuin/alarmserver`. It is a multi-architecture image and will work both on Intel/AMD machines, and your Raspberry PI too.

Usage: `docker run -d -v $PWD/config.yml:/config.yml -v $PWD/ftp:/ftp -p 21:21 -p 15002:15002 toxuin/alarmserver`

Explanation:

  - `-d` makes it run in the background, so you don't have to stare at its logs for it to keep running

  - `-v $PWD/config.yml:/config.yml` passes through your config from your machine into the container. Make sure the file exists.

  - `-v $PWD/ftp:/ftp` passes through a folder `ftp` from where you're running this command into the container. Not needed if you don't need FTP.

  - `-p 21:21` allows your machine to pass through port 21 that is used for FTP server. Not needed if you're not using FTP server.

  - `-p 15002:15002` same as above, but for port 15002 that's used by HiSilicon alarms server. Not needed if you don't need HiSilicon server.

## Feedback

This project was created for author's personal use and while it will probably work for you too, author has no idea if you use it the same way as author. To fit everyone's usage scenario better, it would be beneficial if you could describe how YOU use the Alarm Server.

If you just started to use Alarm Server in your network (or use it for long time!), if you like it (or hate it!) - feel free to create an issue and just share how it works for you and what cameras you have. I would be curious to know if it fits your use case well (or not at all!).

If you have any feature suggestions - create an issue, and we'll probably figure something out.

## License

MIT License
