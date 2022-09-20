package hikvision

import (
	"encoding/xml"
	"fmt"
	"github.com/icholy/digest"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
)

type HttpEventReader struct {
	Debug  bool
	client *http.Client
}

func (eventReader *HttpEventReader) ReadEvents(camera *HikCamera, channel chan<- HikEvent, callback func()) {
	if eventReader.client == nil {
		eventReader.client = &http.Client{}
		if camera.AuthMethod == Digest {
			eventReader.client.Transport = &digest.Transport{
				Username: camera.Username,
				Password: camera.Password,
			}
		}
	}

	request, err := http.NewRequest("GET", camera.Url+"Event/notification/alertStream", nil)
	if err != nil {
		fmt.Printf("HIK: Error: Could not connect to camera %s\n", camera.Name)
		fmt.Println("HIK: Error", err)
		callback()
		return
	}
	if camera.AuthMethod == Basic {
		request.SetBasicAuth(camera.Username, camera.Password)
	}

	response, err := eventReader.client.Do(request)
	if err != nil {
		fmt.Printf("HIK: Error opening HTTP connection to camera %s\n", camera.Name)
		fmt.Println(err)
		return
	}

	if response.StatusCode != 200 {
		fmt.Printf("HIK: BAD STATUS %d", response.StatusCode)
	}
	defer response.Body.Close()

	// FIGURE OUT MULTIPART BOUNDARY
	mediaType, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaType != "multipart/mixed" || params["boundary"] == "" {
		fmt.Println("HIK: ERROR: Camera " + camera.Name + " does not seem to support event streaming")
		fmt.Println("            Is it a doorbell? Try adding rawTcp to its config!")
		callback()
		return
	}
	multipartBoundary := params["boundary"]

	xmlEvent := XmlEvent{}

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

		err = xml.Unmarshal(body, &xmlEvent)
		if err != nil {
			fmt.Println(err)
			continue
		}

		// FILL IN THE CAMERA INTO FRESHLY-UNMARSHALLED EVENT
		xmlEvent.Camera = camera

		if eventReader.Debug {
			log.Printf("%s event: %s (%s - %d)", xmlEvent.Camera.Name, xmlEvent.Type, xmlEvent.State, xmlEvent.Id)
		}

		switch xmlEvent.State {
		case "active":
			if !xmlEvent.Active {
				if eventReader.Debug {
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
}
