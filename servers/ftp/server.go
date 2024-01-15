package ftp

import (
	"fmt"
	"goftp.io/server/v2"
	"sync"
)

type Server struct {
	Debug        bool
	WaitGroup    *sync.WaitGroup
	Port         int
	AllowFiles   bool
	RootPath     string
	Password     string
	PublicIP     string
	PassivePorts string

	MessageHandler func(cameraName string, eventType string, extra string)
}

type Event struct {
	CameraName string `json:"camera"`
	Type       string `json:"type"`
	Message    string `json:"message"`
}

func (serv *Server) Start() {
	if serv.MessageHandler == nil {
		fmt.Println("FTP: Message handler is not set for FTP server - that's probably not what you want")
		serv.MessageHandler = func(cameraName string, eventType string, extra string) {
			fmt.Printf("FTP: Lost alarm: %s - %s: %s\n", cameraName, eventType, extra)
		}
	}
	// DEFAULT FTP PASSWORD
	if serv.Password == "" {
		serv.Password = "root"
	}

	go func() {
		defer serv.WaitGroup.Done()
		serv.WaitGroup.Add(1)

		eventChannel := make(chan Event, 5)

		// START MESSAGE PROCESSOR
		go func(channel <-chan Event) {
			for {
				event := <-channel
				go serv.MessageHandler(event.CameraName, event.Type, event.Message)
			}
		}(eventChannel)

		driver, err := NewDriver(serv.Debug, serv.RootPath, serv.AllowFiles, eventChannel)
		if err != nil {
			fmt.Println("FTP: Cannot init driver")
		}

		opt := &server.Options{
			Name:           "alarmserver-go",
			WelcomeMessage: "HI",
			Driver:         driver,
			Port:           serv.Port,
			Perm:           server.NewSimplePerm("root", "root"),
			Auth:           &DumbAuth{Debug: serv.Debug, Password: serv.Password},
			PublicIP:       serv.PublicIP,
			PassivePorts:   serv.PassivePorts,
		}

		if !serv.Debug {
			opt.Logger = &server.DiscardLogger{}
		}

		ftpServer, err := server.NewServer(opt)
		if err != nil {
			fmt.Println("FTP: Cannot start FTP server", err)
			return
		}
		err = ftpServer.ListenAndServe()
		if err != nil {
			fmt.Println(fmt.Sprintf("FTP: Cannot listen on port %v", serv.Port), err)
			return
		}
		defer ftpServer.Shutdown()
		fmt.Printf("FTP: Listening on port %v", serv.Port)
	}()
}
