package dahua

import (
	"fmt"
	"github.com/icholy/digest"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type DhCamera struct {
	Debug    bool
	Name     string   `json:"name"`
	Url      string   `json:"url"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	Channel  string   `json:"channel"`
	Events   []string `json:"events"`
	client   *http.Client
}

type Server struct {
	Debug          bool
	WaitGroup      *sync.WaitGroup
	Cameras        *[]DhCamera
	MessageHandler func(cameraName string, eventType string, extra string)
}

type DhEvent struct {
	Camera  *DhCamera
	Type    string
	Message string
}

type Event struct {
	Code   string
	Action string
	Index  int
	Data   string
	active bool
}

func (camera *DhCamera) readEvents(channel chan<- DhEvent, callback func()) {
	eventUrlSuffix := "/cgi-bin/eventManager.cgi?action=attach&heartbeat=10"
	if camera.Channel != "" {
		eventUrlSuffix += "&channel=" + camera.Channel
	}
	if camera.Events != nil && len(camera.Events) > 0 {
		eventUrlSuffix += "&codes=[" + strings.Join(camera.Events, ",") + "]"
	} else {
		eventUrlSuffix += "&codes=[All]"
	}
	request, err := http.NewRequest("GET", camera.Url+eventUrlSuffix, nil)
	if err != nil {
		fmt.Printf("DAHUA: Error: Could not connect to camera %s\n", camera.Name)
		fmt.Println("DAHUA: Error", err)
		callback()
		return
	}
	if camera.client.Transport == nil { // BASIC AUTH
		request.SetBasicAuth(camera.Username, camera.Password)
	}

	response, err := camera.client.Do(request)
	if err != nil {
		fmt.Printf("DAHUA: Error opening HTTP connection to camera %s\n", camera.Name)
		fmt.Println(err)
		callback()
		return
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		fmt.Printf("DAHUA: Warning: Status Code was not 200, but %v\n", response.StatusCode)
		if camera.Debug { // DUMP BODY
			body, err := io.ReadAll(response.Body)
			if err != nil {
				log.Fatalln(err)
			}
			fmt.Println(string(body))
		}
	}

	// FIGURE OUT MULTIPART BOUNDARY
	mediaType, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if camera.Debug {
		fmt.Printf("DAHUA: Media type is %s\n", mediaType)
	}

	if params["boundary"] == "" {
		fmt.Println("DAHUA: ERROR: Camera " + camera.Name + " does not seem to support event streaming")
		callback()
		return
	}
	multipartBoundary := params["boundary"]

	event := Event{}

	// READ PART BY PART
	multipartReader := multipart.NewReader(response.Body, multipartBoundary)
	for {
		part, err := multipartReader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println(err)
			continue
		}
		contentLength, _ := strconv.Atoi(part.Header.Get("Content-Length"))
		body := make([]byte, contentLength)
		_, err = part.Read(body)
		if err != nil {
			fmt.Println(err)
			continue
		}

		if camera.Debug {
			fmt.Printf("DAHUA: Read event body: %s\n", body)
		}

		// EXAMPLE: "Code=VideoMotion; action=Start; index=0\r\n\r\n"
		line := strings.Trim(string(body), " \n\r")
		if line == "Heartbeat" {
			continue
		}
		items := strings.Split(line, ";")
		keyValues := make(map[string]string, len(items))
		for _, item := range items {
			parts := strings.Split(item, "=")
			if len(parts) > 0 {
				keyValues[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
		// EXAMPLE: { Code: VideoMotion, action: Start, index: 0 }
		index := 0
		index, _ = strconv.Atoi(keyValues["index"])
		event.Code = keyValues["Code"]
		event.Action = keyValues["action"]
		event.Index = index
		event.Data = keyValues["data"]

		switch event.Action {
		case "Start":
			if !event.active {
				if camera.Debug {
					fmt.Println("DAHUA: SENDING CAMERA EVENT!")
				}
				dahuaEvent := DhEvent{
					Camera:  camera,
					Type:    event.Code,
					Message: event.Data,
				}
				if dahuaEvent.Message == "" {
					dahuaEvent.Message = event.Action
				}
				channel <- dahuaEvent
			}
			event.active = true
		case "Stop":
			event.active = false
		}
	}
}

func (server *Server) addCamera(waitGroup *sync.WaitGroup, cam *DhCamera, channel chan<- DhEvent) {
	waitGroup.Add(1)

	if server.Debug {
		fmt.Printf("DAHUA: Adding camera %s: %s\n", cam.Name, cam.Url)
	}

	if cam.client == nil {
		cam.client = &http.Client{}
	}

	// PROBE AUTH
	request, err := http.NewRequest("GET", cam.Url+"/cgi-bin/configManager.cgi?action=getConfig&name=General", nil)
	if err != nil {
		fmt.Printf("DAHUA: Error probing auth method for camera %s\n", cam.Name)
		fmt.Println(err)
		return
	}
	request.SetBasicAuth(cam.Username, cam.Password)
	response, err := cam.client.Do(request)
	if err != nil {
		fmt.Printf("DAHUA: Error probing HTTP Auth method for camera %s\n", cam.Name)
		fmt.Println(err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode == 401 {
		if response.Header.Get("WWW-Authenticate") == "" {
			// BAD PASSWORD
			fmt.Printf("DAHUA: UNKNOWN AUTH METHOD FOR CAMERA %s! SKIPPING...", cam.Name)
			return
		}
		authMethod := strings.Split(response.Header.Get("WWW-Authenticate"), " ")[0]
		if authMethod == "Basic" {
			// BAD PASSWORD
			fmt.Printf("DAHUA: BAD PASSWORD FOR CAMERA %s! SKIPPING...", cam.Name)
			return
		}

		// TRY ANOTHER TIME WITH DIGEST TRANSPORT
		cam.client.Transport = &digest.Transport{
			Username: cam.Username,
			Password: cam.Password,
		}
		response, err := cam.client.Do(request)
		if err != nil || response.StatusCode == 401 {
			if err != nil {
				fmt.Println(err)
			}
			// BAD PASSWORD
			fmt.Printf("DAHUA: BAD PASSWORD FOR CAMERA %s! SKIPPING...", cam.Name)
			return
		}

		if server.Debug {
			fmt.Println("DAHUA: USING DIGEST AUTH")
		}
	} else if server.Debug {
		fmt.Println("DAHUA: USING BASIC AUTH")
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
			cam.readEvents(channel, callback)
		}
		fmt.Printf("DAHUA: Closed connection to camera %s\n", cam.Name)
	}()
}

func (server *Server) Start() {
	if server.Cameras == nil || len(*server.Cameras) == 0 {
		fmt.Println("DAHUA: Error: no cameras defined")
		return
	}

	if server.MessageHandler == nil {
		fmt.Println("DAHUA: Message handler is not set for Dahua cams - that's probably not what you want")
		server.MessageHandler = func(cameraName string, eventType string, extra string) {
			fmt.Printf("DAHUA: Lost alarm: %s - %s: %s\n", cameraName, eventType, extra)
		}
	}

	waitGroup := sync.WaitGroup{}
	eventChannel := make(chan DhEvent, 5)

	for _, camera := range *server.Cameras {
		server.addCamera(&waitGroup, &camera, eventChannel)
	}

	// START MESSAGE PROCESSOR
	go func(waitGroup *sync.WaitGroup, channel <-chan DhEvent) {
		// WAIT GROUP FOR INDIVIDUAL CAMERAS
		defer waitGroup.Done()

		// EXTERNAL WAIT GROUP FOR PROCESSES
		defer server.WaitGroup.Done()
		server.WaitGroup.Add(1)

		for {
			event := <-channel
			go server.MessageHandler(event.Camera.Name, event.Type, event.Message)
		}
	}(&waitGroup, eventChannel)

	waitGroup.Wait()
}
