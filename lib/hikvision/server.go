package hikvision

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HikCamera struct {
	Name       string `json:"name"`
	Url        string `json:"url"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	BrokenHttp bool
}

type HikEvent struct {
	Type    string
	Message string
	Camera  *HikCamera
}

type Server struct {
	Debug          bool
	Cameras        *[]HikCamera
	MessageHandler func(topic string, data string)
	client         *http.Client
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
			fmt.Printf("HIK: Error opening HTTP connection to camera %s\n", camera.Name)
			fmt.Println(err)
			continue
		}

		// FIGURE OUT MULTIPART BOUNDARY
		mediaType, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if mediaType != "multipart/mixed" || params["boundary"] == "" {
			fmt.Println("HIK: ERROR: Camera " + camera.Name + " does not seem to support event streaming")
			fmt.Println("            Is it a doorbell? Try adding rawTcp to its config!")
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
						fmt.Println("HIK: SENDING CAMERA EVENT!")
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

		fmt.Printf("HIK: Done reading stream for camera %s\n", camera.Name)
	}
	fmt.Printf("HIK: Closed connection to camera %s\n", camera.Name)
}

func (server Server) readTcpEventsForCamera(waitGroup *sync.WaitGroup, camera *HikCamera, channel chan<- HikEvent) {
	defer waitGroup.Done()
	done := false

	// PARSE THE ADDRESS OUTTA CAMERA URL
	cameraUrl, err := url.Parse(camera.Url)
	if err != nil {
		fmt.Printf("HIK: Error parsing address of camera %s: %s\n", camera.Name, camera.Url)
		return
	}

	if cameraUrl.Scheme == "https:" {
		fmt.Printf("HIK: Cannot read events for camera %s: HTTPS support is not implemented\n", camera.Name)
		return
	}

	for {
		if done {
			break
		}

		var address, host string
		if strings.Contains(cameraUrl.Host, ":") {
			address = cameraUrl.Host
			host = strings.Split(cameraUrl.Host, ":")[1]
		} else {
			address = cameraUrl.Host + ":80"
			host = cameraUrl.Host
		}

		// BASE64-ENCODED VALUE FOR BASIC HTTP AUTH HEADER
		basicAuth := base64.StdEncoding.EncodeToString([]byte(camera.Username + ":" + camera.Password))

		textConn, err := textproto.Dial("tcp", address)
		if err != nil {
			fmt.Printf("HIK: Error opening TCP connection to camera %s\n", camera.Name)
			break
		}

		// SEND INITIAL REQUEST
		err = textConn.PrintfLine("GET /ISAPI/Event/notification/alertStream HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Authorization: Basic %s\r\n\r\n\r\n",
			host,
			basicAuth,
		)
		if err != nil {
			fmt.Println("HIK: TCP: Error sending auth request")
			fmt.Println(err)
			break
		}

		// READ AND PARSE HTTP STATUS
		httpStatusLine, err := textConn.ReadLine()
		if err != nil {
			fmt.Println("HIK: TCP: Could not get status header")
			fmt.Println(err)
			return
		}
		if !strings.Contains(httpStatusLine, "HTTP/1.1") {
			fmt.Printf("HIK: TCP: Bad response from camera %s: %s", camera.Name, httpStatusLine)
			return
		}
		statusParts := strings.SplitN(strings.Split(httpStatusLine, "HTTP/1.1 ")[1], " ", 2)
		statusCode := statusParts[0]
		statusMessage := statusParts[1]

		// READ HTTP HEADERS
		var headers = make(map[string]string)
		if server.Debug {
			fmt.Println("HEADERS:")
		}
		for {
			headerLine, err := textConn.ReadLine()
			if err == io.EOF {
				// CONNECTION CLOSED
				return
			}
			if strings.Trim(headerLine, " ") == "" {
				// END OF HEADERS
				break
			}

			if server.Debug {
				fmt.Println("  " + headerLine)
			}

			headerKey := strings.SplitN(headerLine, ": ", 2)[0]
			headerValue := strings.SplitN(headerLine, ": ", 2)[1]
			headers[headerKey] = headerValue
		}

		// PRINT ERROR
		if statusCode != "200" {
			contentLen, err := strconv.Atoi(headers["Content-Length"])
			if err != nil {
				fmt.Println("HIK: TCP: Error reading error message, dammit")
				return
			}
			errorBody := make([]byte, contentLen)
			_, _ = io.ReadFull(textConn.R, errorBody)
			fmt.Printf("HIK: TCP: HTTP Error authenticating with camera %s: %s - %s\n", camera.Name, statusCode, statusMessage)
			fmt.Println(string(errorBody))
			return
		}

		// READ ACTUAL EVENTS
		var eventString string
		xmlEvent := XmlEvent{}
		for {
			line, err := textConn.ReadLine()
			if err == io.EOF { // CONNECTION CLOSED
				return
			}
			if err != nil {
				fmt.Println("ERROR READING FROM CONNECTION")
				fmt.Println(err)
				break
			}

			if strings.Trim(line, " ") == "" {
				// FOUND END OF ONE EVENT IN STREAM
				if strings.Contains(eventString, ">HTTP/1.1 ") {
					// PART OF THE LAST PACKET IS STUCK TO THE NEXT PACKET
					eventString = strings.SplitN(eventString, "HTTP/1.1", 2)[0]
				}

				err = xml.Unmarshal([]byte(eventString), &xmlEvent)
				xmlEvent.Camera = camera
				if err != nil {
					fmt.Println("HIK: TCP: Error unmarshalling xml event!")
					continue
				}
				if server.Debug {
					log.Printf("%s event: %s (%s - %d), %s", xmlEvent.Camera.Name, xmlEvent.Type, xmlEvent.State, xmlEvent.Id, xmlEvent.Description)
				}

				switch xmlEvent.State {
				case "active":
					if !xmlEvent.Active {
						if server.Debug {
							fmt.Println("HIK: SENDING CAMERA EVENT!")
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

				eventString = ""
			} else {
				eventString += line
			}
		}
	}
	fmt.Printf("HIK: TCP: Disconnected from camera %s", camera.Name)
}

func (server Server) addCamera(waitGroup *sync.WaitGroup, camera *HikCamera, eventChannel chan<- HikEvent) {
	waitGroup.Add(1)
	if !camera.BrokenHttp {
		go server.readEventsForCamera(waitGroup, camera, eventChannel)
	} else {
		if server.Debug {
			fmt.Printf("HIK: Adding camera %s using raw TCP\n", camera.Name)
		}
		go server.readTcpEventsForCamera(waitGroup, camera, eventChannel)
	}
	if server.Debug {
		fmt.Printf("HIK: Adding camera %s: %s\n", camera.Name, camera.Url)
	}
}

func (server Server) Start() {
	if server.Cameras == nil || len(*server.Cameras) == 0 {
		fmt.Println("HIK: Error: no cameras defined")
		return
	}

	if server.MessageHandler == nil {
		fmt.Println("HIK: Message handler is not set for Hikvision cams - that's probably not what you want")
		server.MessageHandler = func(topic string, data string) {
			fmt.Printf("HIK: Lost alarm: %s: %s\n", topic, data)
		}
	}

	server.client = &http.Client{}

	waitGroup := sync.WaitGroup{}
	eventChannel := make(chan HikEvent, 5)

	// START ALL CAMERA LISTENERS
	for _, camera := range *server.Cameras {
		waitGroup.Add(1)
		if !camera.BrokenHttp {
			go server.readEventsForCamera(&waitGroup, &camera, eventChannel)
		} else {
			go server.readTcpEventsForCamera(&waitGroup, &camera, eventChannel)
		}

		if server.Debug {
			fmt.Printf("HIK: Adding camera %s: %s\n", camera.Name, camera.Url)
		}
	}

	// START MESSAGE PROCESSOR
	go func(waitGroup *sync.WaitGroup, channel <-chan HikEvent) {
		defer waitGroup.Done()
		event := <-channel
		server.MessageHandler(event.Camera.Name+"/"+event.Type, event.Message)
	}(&waitGroup, eventChannel)
	waitGroup.Add(1)

	waitGroup.Wait()
}
