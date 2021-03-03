package ftp

import (
	"fmt"
	"goftp.io/server/v2"
)

type Server struct {
	Debug          bool
	Port           int
	AllowFiles     bool
	RootPath       string
	Password       string
	MessageHandler func(topic string, data string)
}

type Event struct {
	CameraName string `json:"camera"`
	Type       string `json:"type"`
	Message    string `json:"message"`
}

func (serv Server) Start() {
	if serv.MessageHandler == nil {
		fmt.Println("FTP: Message handler is not set for FTP server - that's probably not what you want")
		serv.MessageHandler = func(topic string, data string) {
			fmt.Printf("FTP: Lost alarm: %s: %s\n", topic, data)
		}
	}
	// DEFAULT FTP PASSWORD
	if serv.Password == "" {
		serv.Password = "root"
	}

	eventChannel := make(chan Event, 5)

	// START MESSAGE PROCESSOR
	go func(channel <-chan Event) {
		for {
			event := <-channel
			go serv.MessageHandler(event.CameraName+"/"+event.Type, event.Message)
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
}
