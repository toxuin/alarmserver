package hikvision

import (
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"sync"
	"time"
)

type HikCamera struct {
	Name     string `json:"name"`
	Url      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type HikEvent struct {
	Type    string
	Message string
	Camera  *HikCamera
}

type Server struct {
	Debug   bool
	Cameras *[]HikCamera
	client  *http.Client
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

var (
	eventTagAlertStart = []byte("<EventNotificationAlert")
	eventTagAlertEnd   = []byte("</EventNotificationAlert>")
)

func (event *HikEvent) isReady() bool {
	return len(event.Type) != 0
}

func (server Server) readEventsForCamera(waitGroup *sync.WaitGroup, camera *HikCamera, channel chan<- HikEvent) {
	defer waitGroup.Done()
	done := false

	for {
		if done {
			break
		}

		request, err := http.NewRequest("GET", camera.Url+"/Event/notification/alertStream", nil)
		if err != nil {
			fmt.Printf("HIK: Error: Could not connect to camera %s\n", camera.Name)
			fmt.Println("HIK: Error", err)
			continue
		}
		request.SetBasicAuth(camera.Username, camera.Password)

		response, err := server.client.Do(request)
		if err != nil {
			fmt.Println("DO ERROR", camera, err)
			continue
		}

		// FIGURE OUT MULTIPART BOUNDARY
		mediaType, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if mediaType != "multipart/mixed" || params["boundary"] == "" {
			fmt.Println("HIK: ERROR: Camera " + camera.Name + " does not seem to support event streaming")
			done = true
			return
		}
		multipartBoundary := params["boundary"]

		xmlEvent := XmlEvent{}

		// READ PART BY PART
		multipartReader := multipart.NewReader(response.Body, multipartBoundary)
		for {
			part, err := multipartReader.NextPart()
			if err == io.EOF {
				return
			}
			if err != nil {
				fmt.Println(err)
				continue
			}
			body, err := ioutil.ReadAll(part)
			if err != nil {
				fmt.Println(err)
				continue
			}

			err = xml.Unmarshal(body, &xmlEvent)
			if err != nil {
				fmt.Println(err)
				continue
			}

			// FILL IN THE CAMERA INTO FRESHLY-UNMARSHALLED EVENT
			xmlEvent.Camera = camera

			if server.Debug {
				log.Printf("%s event: %s (%s - %d)", xmlEvent.Camera.Name, xmlEvent.Type, xmlEvent.State, xmlEvent.Id)
			}

			switch xmlEvent.State {
			case "active":
				if !xmlEvent.Active {
					if server.Debug {
						fmt.Println("SENDING CAMERA EVENT!")
					}
					event := HikEvent{Camera: camera}
					event.Type = xmlEvent.Type
					event.Message = xmlEvent.Description
					channel <- event
				}
				xmlEvent.Active = true
			case "inactive":
				xmlEvent.Active = false
			}
		}

		fmt.Printf("HIK: DONE READING STREAM FOR CAMERA %s\n", camera.Name)
	}
	fmt.Printf("HIK: Closed connection to camera %s\n", camera.Name)
}

func (server Server) Start() {
	if server.Cameras == nil || len(*server.Cameras) == 0 {
		fmt.Println("HIK: Error: no cameras defined")
		return
	}

	server.client = &http.Client{}

	waitGroup := sync.WaitGroup{}
	eventChannel := make(chan HikEvent)

	for _, camera := range *server.Cameras {
		waitGroup.Add(1)
		go server.readEventsForCamera(&waitGroup, &camera, eventChannel)
		if server.Debug {
			fmt.Printf("Adding camera %s: %s\n", camera.Name, camera.Url)
		}
	}

	waitGroup.Wait()
}
