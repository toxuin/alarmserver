package ftp

import (
	"bytes"
	"errors"
	"fmt"
	"goftp.io/server/v2"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Driver struct {
	Debug           bool
	RootPath        string
	AllowFileUpload bool
	EventChannel    chan<- Event
	rootFInfo       os.FileInfo
}

type EventMaker interface {
	MakeEvent(eventStr string) Event
}

func (driver *Driver) createEvent(eventStr string) Event {
	if driver.Debug {
		fmt.Printf("FTP: PARSING STRING TO EVENT %s\n", eventStr)
	}

	return Event{
		Type:    "ftpUpload",
		Message: eventStr,
	}
}

func (driver *Driver) realPath(path string) string {
	paths := strings.Split(path, "/")
	return filepath.Join(append([]string{driver.RootPath}, paths...)...)
}

func (driver *Driver) Stat(context *server.Context, path string) (os.FileInfo, error) {
	if driver.Debug {
		fmt.Printf("FTP: DRIVER: Stat(%s)\n", path)
	}

	if !driver.AllowFileUpload {
		return driver.rootFInfo, nil
	}

	basePath := driver.realPath(path)
	absolutePath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, err
	}
	return os.Lstat(absolutePath)
}

func (driver *Driver) ListDir(context *server.Context, path string, callback func(os.FileInfo) error) error {
	if driver.Debug {
		fmt.Printf("FTP: DRIVER: ListDir(%s)\n", path)
	}
	if !driver.AllowFileUpload {
		// THIS WILL RESULT IN AN INFINITELY DEEP TREE CONTAINING ROOT DIR CONTAINING ITSELF
		return callback(driver.rootFInfo)
	}

	basePath := driver.realPath(path)
	return filepath.Walk(basePath, func(localPath string, fInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rPath, _ := filepath.Rel(basePath, localPath)
		if rPath == fInfo.Name() {
			err = callback(fInfo)
			if err != nil {
				return err
			}
			if fInfo.IsDir() {
				return filepath.SkipDir
			}
		}
		return nil
	})
}

func (driver *Driver) DeleteDir(context *server.Context, path string) error {
	if driver.Debug {
		fmt.Printf("FTP: DRIVER: DeleteDir(%s)\n", path)
	}
	if !driver.AllowFileUpload {
		return nil
	}

	realPath := driver.realPath(path)
	fInfo, err := os.Lstat(realPath)
	if err != nil {
		return err
	}
	if !fInfo.IsDir() {
		return errors.New("not a directory")
	}
	return os.RemoveAll(realPath)
}

func (driver *Driver) DeleteFile(context *server.Context, path string) error {
	if driver.Debug {
		fmt.Printf("FTP: DRIVER: DeleteFile(%s)\n", path)
	}
	if !driver.AllowFileUpload {
		return nil
	}

	realPath := driver.realPath(path)
	fInfo, err := os.Lstat(realPath)
	if err != nil {
		return err
	}
	if !fInfo.Mode().IsRegular() {
		return errors.New("not a file")
	}
	return os.Remove(realPath)
}

func (driver *Driver) Rename(context *server.Context, fromPath string, toPath string) error {
	if driver.Debug {
		fmt.Printf("FTP: DRIVER: Rename(fromPath: %s, toPath: %s)\n", fromPath, toPath)
	}
	if !driver.AllowFileUpload {
		return nil
	}

	return os.Rename(driver.realPath(fromPath), driver.realPath(toPath))
}

func (driver *Driver) MakeDir(context *server.Context, path string) error {
	if driver.Debug {
		fmt.Printf("FTP: DRIVER: MakeDir(%s)\n", path)
	}
	if !driver.AllowFileUpload {
		return nil
	}

	return os.MkdirAll(driver.realPath(path), os.ModePerm)
}

func (driver *Driver) GetFile(context *server.Context, path string, offset int64) (int64, io.ReadCloser, error) {
	if driver.Debug {
		fmt.Printf("FTP: DRIVER: GetFile(path: %s, offset: %v)\n", path, offset)
	}
	if !driver.AllowFileUpload {
		// TODO
	}

	realPath := driver.realPath(path)
	fInfo, err := os.Open(realPath)
	if err != nil {
		return -1, nil, err
	}

	defer func() {
		// CLOSE FILE
		if err != nil && fInfo != nil {
			_ = fInfo.Close()
		}
	}()

	info, err := fInfo.Stat()
	if err != nil {
		return -1, nil, err
	}

	_, err = fInfo.Seek(offset, io.SeekStart)
	if err != nil {
		return -1, nil, err
	}

	return info.Size() - offset, fInfo, nil
}

func (driver *Driver) PutFile(context *server.Context, destPath string, data io.Reader, filepos int64) (int64, error) {
	if driver.Debug {
		fmt.Printf("FTP: DRIVER: PutFile(destPath: %s, filepos: %v)\n", destPath, filepos)
	}

	go func() {
		var event Event = driver.createEvent(destPath)
		if event.CameraName == "" {
			event.CameraName = context.Sess.LoginUser()
		}
		// DISPATCH EVENT
		driver.EventChannel <- event
	}()

	if !driver.AllowFileUpload { // JUST RETURN SUCCESSFUL UPLOAD
		buf := &bytes.Buffer{}
		bytesRead, err := io.Copy(buf, data)
		if err != nil {
			return -1, err
		}
		return bytesRead, nil
	}

	realPath := driver.realPath(destPath)

	var isExist bool
	f, err := os.Lstat(realPath)
	if err == nil {
		isExist = true
		if f.IsDir() {
			return -1, errors.New("name conflict")
		}
	} else {
		if os.IsNotExist(err) {
			isExist = false
		} else {
			return -1, errors.New(fmt.Sprintln("put file error: ", err))
		}
	}

	if filepos > -1 && !isExist {
		filepos = -1
	}

	if filepos == -1 {
		if isExist {
			err = os.Remove(realPath)
			if err != nil {
				return -1, err
			}
		}
		f, err := os.Create(realPath)
		if err != nil {
			return -1, err
		}
		defer f.Close()
		bytesRead, err := io.Copy(f, data)
		if err != nil {
			return -1, err
		}
		return bytesRead, nil
	}

	of, err := os.OpenFile(realPath, os.O_APPEND|os.O_RDWR, 0660)
	if err != nil {
		return -1, err
	}
	defer of.Close()

	info, err := of.Stat()
	if err != nil {
		return -1, err
	}
	if filepos > info.Size() {
		return -1, fmt.Errorf("offset %d is beyond file size %d", filepos, info.Size())
	}

	_, err = of.Seek(filepos, io.SeekEnd)
	if err != nil {
		return -1, err
	}

	bytesRead, err := io.Copy(of, data)
	if err != nil {
		return -1, err
	}

	return bytesRead, nil
}

func NewDriver(debug bool, rootPath string, allowFileUpload bool, eventChannel chan<- Event) (server.Driver, error) {
	var err error
	rootPath, err = filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}

	// MAKE SURE FTP ROOT DIR EXISTS
	_ = os.MkdirAll(rootPath, os.ModePerm)

	// POPULATE ROOT FILE INFO
	absolutePath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}
	fInfo, err := os.Lstat(absolutePath)
	if err != nil {
		return nil, err
	}

	return &Driver{
		Debug:           debug,
		RootPath:        rootPath,
		AllowFileUpload: allowFileUpload,
		EventChannel:    eventChannel,
		rootFInfo:       fInfo,
	}, nil
}
