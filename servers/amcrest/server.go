package amcrest

import (
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type AmcCamera struct {
	Debug    bool
	Name     string `json:"name"`
	Url      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	client   *http.Client
}

type Server struct {
	Debug          bool
	WaitGroup      *sync.WaitGroup
	Cameras        *[]AmcCamera
	MessageHandler func(topic string, data string)
}

type AmcEvent struct {
	Camera  *AmcCamera
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

func (camera *AmcCamera) readEvents(channel chan<- AmcEvent, callback func()) {
	request, err := http.NewRequest("GET", camera.Url+"/cgi-bin/eventManager.cgi?action=attach&codes=All", nil)
	if err != nil {
		fmt.Printf("AMC: Error: Could not connect to camera %s\n", camera.Name)
		fmt.Println("AMC: Error", err)
		callback()
		return
	}
	request.SetBasicAuth(camera.Username, camera.Password)

	response, err := camera.client.Do(request)
	if err != nil {
		fmt.Printf("AMC: Error opening HTTP connection to camera %s\n", camera.Name)
		fmt.Println(err)
		return
	}

	if response.StatusCode != 200 {
		fmt.Printf("AMC: Warning: Status Code was not 200, but %v\n", response.StatusCode)
	}

	// FIGURE OUT MULTIPART BOUNDARY
	mediaType, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if camera.Debug {
		fmt.Printf("AMC: Media type is %s\n", mediaType)
	}

	if params["boundary"] == "" {
		fmt.Println("AMC: ERROR: Camera " + camera.Name + " does not seem to support event streaming")
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

		if camera.Debug {
			fmt.Printf("AMC: Read event body: %s\n", body)
		}

		// EXAMPLE: "Code=VideoMotion; action=Start; index=0\r\n\r\n"
		line := strings.Trim(string(body), " \n\r")
		items := strings.Split(line, "; ")
		keyValues := make(map[string]string, len(items))
		for _, item := range items {
			parts := strings.Split(item, "=")
			if len(parts) > 0 {
				keyValues[parts[0]] = parts[1]
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
					fmt.Println("AMC: SENDING CAMERA EVENT!")
				}
				amcEvent := AmcEvent{
					Camera:  camera,
					Type:    event.Code,
					Message: event.Data,
				}
				if amcEvent.Message == "" {
					amcEvent.Message = event.Action
				}
				channel <- amcEvent
			}
			event.active = true
		case "Stop":
			event.active = false
		}
	}
}

func (server *Server) addCamera(waitGroup *sync.WaitGroup, cam *AmcCamera, channel chan<- AmcEvent) {
	waitGroup.Add(1)

	if server.Debug {
		fmt.Printf("AMC: Adding camera %s: %s\n", cam.Name, cam.Url)
	}

	if cam.client == nil {
		cam.client = &http.Client{}
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
			go cam.readEvents(channel, callback)
		}
		fmt.Printf("AMC: Closed connection to camera %s\n", cam.Name)
	}()
}

func (server *Server) Start() {
	if server.Cameras == nil || len(*server.Cameras) == 0 {
		fmt.Println("AMC: Error: no cameras defined")
		return
	}

	if server.MessageHandler == nil {
		fmt.Println("AMC: Message handler is not set for Amcrest cams - that's probably not what you want")
		server.MessageHandler = func(topic string, data string) {
			fmt.Printf("AMC: Lost alarm: %s: %s\n", topic, data)
		}
	}

	waitGroup := sync.WaitGroup{}
	eventChannel := make(chan AmcEvent, 5)

	for _, camera := range *server.Cameras {
		server.addCamera(&waitGroup, &camera, eventChannel)
	}

	// START MESSAGE PROCESSOR
	go func(waitGroup *sync.WaitGroup, channel <-chan AmcEvent) {
		// WAIT GROUP FOR INDIVIDUAL CAMERAS
		defer waitGroup.Done()

		// EXTERNAL WAIT GROUP FOR PROCESSES
		defer server.WaitGroup.Done()
		server.WaitGroup.Add(1)

		for {
			event := <-channel
			go server.MessageHandler(event.Camera.Name+"/"+event.Type, event.Message)
		}
	}(&waitGroup, eventChannel)

	waitGroup.Wait()
}
