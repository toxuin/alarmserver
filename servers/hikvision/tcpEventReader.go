package hikvision

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
)

type TcpEventReader struct {
	Debug bool
}

func (eventReader *TcpEventReader) ReadEvents(camera *HikCamera, channel chan<- HikEvent, callback func()) {
	// PARSE THE ADDRESS OUTTA CAMERA URL
	cameraUrl, err := url.Parse(camera.Url)
	if err != nil {
		fmt.Printf("HIK-TCP: Error parsing address of camera %s: %s\n", camera.Name, camera.Url)
		callback()
		return
	}

	if cameraUrl.Scheme == "https:" {
		fmt.Printf("HIK-TCP: Cannot read events for camera %s: HTTPS support is not implemented\n", camera.Name)
		callback()
		return
	}

	for {
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
			fmt.Println("HIK-TCP: Error sending auth request")
			fmt.Println(err)
			break
		}

		// READ AND PARSE HTTP STATUS
		httpStatusLine, err := textConn.ReadLine()
		if err != nil {
			fmt.Println("HIK-TCP: Could not get status header")
			fmt.Println(err)
			return
		}
		if !strings.Contains(httpStatusLine, "HTTP/1.1") {
			fmt.Printf("HIK-TCP: Bad response from camera %s: %s", camera.Name, httpStatusLine)
			return
		}
		statusParts := strings.SplitN(strings.Split(httpStatusLine, "HTTP/1.1 ")[1], " ", 2)
		statusCode := statusParts[0]
		statusMessage := statusParts[1]

		// READ HTTP HEADERS
		var headers = make(map[string]string)
		if eventReader.Debug {
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

			if eventReader.Debug {
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
				fmt.Println("HIK-TCP: Error reading error message, dammit")
				return
			}
			errorBody := make([]byte, contentLen)
			_, _ = io.ReadFull(textConn.R, errorBody)
			fmt.Printf("HIK-TCP: HTTP Error authenticating with camera %s: %s - %s\n", camera.Name, statusCode, statusMessage)
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
					fmt.Println("HIK-TCP: Error unmarshalling xml event!")
					continue
				}
				if eventReader.Debug {
					log.Printf("%s event: %s (%s - %d), %s", xmlEvent.Camera.Name, xmlEvent.Type, xmlEvent.State, xmlEvent.Id, xmlEvent.Description)
				}

				switch xmlEvent.State {
				case "active":
					if !xmlEvent.Active {
						if eventReader.Debug {
							fmt.Println("HIK-TCP: SENDING CAMERA EVENT!")
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
}
