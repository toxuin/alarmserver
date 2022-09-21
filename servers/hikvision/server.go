package hikvision

import (
	"encoding/xml"
	"fmt"
	"github.com/icholy/digest"
	"net/http"
	"strings"
	"sync"
	"time"
)

type HttpAuthMethod int

const (
	Basic HttpAuthMethod = iota
	Digest
)

type HikCamera struct {
	Name        string `json:"name"`
	Url         string `json:"url"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	EventReader HikEventReader
	BrokenHttp  bool
	AuthMethod  HttpAuthMethod
}

type HikEvent struct {
	Type    string
	Message string
	Camera  *HikCamera
}

type Server struct {
	Debug          bool
	WaitGroup      *sync.WaitGroup
	Cameras        *[]HikCamera
	MessageHandler func(cameraName string, eventType string, extra string)
}

type XmlEvent struct {
	XMLName     xml.Name  `xml:"EventNotificationAlert"`
	IpAddress   string    `xml:"ipAddress"`
	Port        int       `xml:"portNo"`
	ChannelId   int       `xml:"channelID"`
	Time        time.Time `xml:"dateTime"`
	Id          int       `xml:"activePostCount"`
	Type        string    `xml:"eventType"`
	State       string    `xml:"eventState"`
	Description string    `xml:"eventDescription"`
	Active      bool
	Camera      *HikCamera
}

type HikEventReader interface {
	ReadEvents(camera *HikCamera, channel chan<- HikEvent, callback func())
}

func (server *Server) addCamera(waitGroup *sync.WaitGroup, camera *HikCamera, eventChannel chan<- HikEvent) {
	waitGroup.Add(1)
	if !camera.BrokenHttp {
		camera.EventReader = &HttpEventReader{Debug: server.Debug}
	} else {
		camera.EventReader = &TcpEventReader{Debug: server.Debug}
	}
	if server.Debug {
		fmt.Printf("HIK: Adding camera %s: %s\n", camera.Name, camera.Url)
	}

	// PROBE AUTH
	client := &http.Client{}
	request, err := http.NewRequest("GET", camera.Url+"System/status", nil)
	if err != nil {
		fmt.Printf("HIK: Error probing auth method for camera %s\n", camera.Name)
		fmt.Println(err)
		return
	}
	request.SetBasicAuth(camera.Username, camera.Password)
	response, err := client.Do(request)
	if err != nil {
		fmt.Printf("HIK: Error probing HTTP Auth method for camera %s\n", camera.Name)
		fmt.Println(err)
		return
	}
	if response.StatusCode == 401 {
		if response.Header.Get("WWW-Authenticate") == "" {
			// BAD PASSWORD
			fmt.Printf("HIK: UNKNOWN AUTH METHOD FOR CAMERA %s! SKIPPING...", camera.Name)
			return
		}
		authMethod := strings.Split(response.Header.Get("WWW-Authenticate"), " ")[0]
		if authMethod == "Basic" {
			// BAD PASSWORD
			fmt.Printf("HIK: BAD PASSWORD FOR CAMERA %s! SKIPPING...", camera.Name)
			return
		}

		// TRY ANOTHER TIME WITH DIGEST TRANSPORT
		client.Transport = &digest.Transport{
			Username: camera.Username,
			Password: camera.Password,
		}
		response, err := client.Do(request)
		if err != nil || response.StatusCode == 401 {
			if err != nil {
				fmt.Println(err)
			}
			// BAD PASSWORD
			fmt.Printf("HIK: BAD PASSWORD FOR CAMERA %s! SKIPPING...", camera.Name)
			return
		}

		camera.AuthMethod = Digest
		if server.Debug {
			fmt.Println("HIK: USING DIGEST AUTH")
			if camera.BrokenHttp {
				fmt.Println("HIK: WARNING: rawTCP CONFIG VALUE Digest AUTH COMBO IS NOT SUPPORTED!")
				fmt.Println("    PLEASE OPEN A GITHUB ISSUE AT https://github.com/toxuin/alarmserver/issues")
				fmt.Println("    AND INCLUDE YOUR CAMERA MODEL. THANK YOU!")
			}
		}
	} else {
		camera.AuthMethod = Basic
		if server.Debug {
			fmt.Println("HIK: USING BASIC AUTH")
		}
	}

	go func() {
		defer waitGroup.Done()
		done := false
		callback := func() {
			done = true
		}

		for {
			if done {
				break
			}
			camera.EventReader.ReadEvents(camera, eventChannel, callback)
		}
		fmt.Printf("HIK: Closed connection to camera %s\n", camera.Name)
	}()
}

func (server *Server) Start() {
	if server.Cameras == nil || len(*server.Cameras) == 0 {
		fmt.Println("HIK: Error: no cameras defined")
		return
	}

	if server.MessageHandler == nil {
		fmt.Println("HIK: Message handler is not set for Hikvision cams - that's probably not what you want")
		server.MessageHandler = func(cameraName string, eventType string, extra string) {
			fmt.Printf("HIK: Lost alarm: %s - %s: %s\n", cameraName, eventType, extra)
		}
	}

	cameraWaitGroup := sync.WaitGroup{}
	eventChannel := make(chan HikEvent, 5)

	// START ALL CAMERA LISTENERS
	for _, camera := range *server.Cameras {
		server.addCamera(&cameraWaitGroup, &camera, eventChannel)
	}

	// START MESSAGE PROCESSOR
	go func(camWaitGroup *sync.WaitGroup, channel <-chan HikEvent) {
		// WAIT GROUP FOR INDIVIDUAL CAMERAS
		defer camWaitGroup.Done()

		// EXTERNAL WAIT GROUP FOR PROCESSES
		defer server.WaitGroup.Done()
		server.WaitGroup.Add(1)
		for {
			event := <-channel
			go server.MessageHandler(event.Camera.Name, event.Type, event.Message)
		}
	}(&cameraWaitGroup, eventChannel)

	cameraWaitGroup.Wait()
}
