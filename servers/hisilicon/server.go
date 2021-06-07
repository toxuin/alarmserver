package hisilicon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
)

// Converts 0x1704A8C0 to 192.168.4.23
func hexIpToCIDR(hexAddr string) string {
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

type Server struct {
	Debug          bool
	WaitGroup      *sync.WaitGroup
	Port           string
	MessageHandler func(topic string, data string)
}

func (server *Server) handleTcpConnection(conn net.Conn) {
	defer conn.Close()

	if server.Debug {
		fmt.Printf("HISI: DEVICE CONNECTED: %s\n", conn.RemoteAddr().String())
	}
	var buf bytes.Buffer

	_, err := io.Copy(&buf, conn)
	if err != nil {
		fmt.Printf("HISI: TCP READ ERROR: %s\n", err)
		return
	}
	bufString := buf.String()
	resultString := bufString[strings.IndexByte(bufString, '{'):]
	if server.Debug {
		fmt.Printf("HISI: DEVICE ALERT: %s\n", resultString)
	}

	var dataMap map[string]interface{}

	if err := json.Unmarshal([]byte(resultString), &dataMap); err != nil {
		fmt.Printf("HISI: JSON PARSE ERROR: %s\n", err)
		return
	}
	if dataMap["Address"] != nil {
		hexAddrStr := fmt.Sprintf("%v", dataMap["Address"])
		dataMap["ipAddr"] = hexIpToCIDR(hexAddrStr)
	}

	jsonBytes, err := json.Marshal(dataMap)
	if err != nil {
		fmt.Printf("HISI: JSON STRINGIFY ERROR: %s\n", err)
		return
	}

	if dataMap["SerialID"] == nil {
		fmt.Println("HISI: UNKNOWN DEVICE SERIAL ID")
		fmt.Println(dataMap)
		return
	}

	serialId := fmt.Sprintf("%v", dataMap["SerialID"])
	event := fmt.Sprintf("%v", dataMap["Event"])

	server.MessageHandler(serialId+"/"+event, string(jsonBytes))
}

func (server *Server) Start() {
	if server.Port == "" {
		server.Port = "15002" // DEFAULT PORT
	}
	if server.MessageHandler == nil {
		fmt.Println("HISI: Message handler is not set for HiSilicon cams - that's probably not what you want")
		server.MessageHandler = func(topic string, data string) {
			fmt.Printf("HISI: Lost alarm: %s: %s\n", topic, data)
		}
	}

	go func() {
		defer server.WaitGroup.Done()
		server.WaitGroup.Add(1)

		// START TCP SERVER
		tcpListener, err := net.Listen("tcp4", ":"+server.Port)
		if err != nil {
			panic(err)
		}
		defer tcpListener.Close()

		for {
			conn, err := tcpListener.Accept()
			if err != nil {
				panic(err)
			}
			go server.handleTcpConnection(conn)
		}
	}()
}
