package ftp

import (
	"fmt"
	"goftp.io/server/v2"
)

type DumbAuth struct {
	Debug    bool
	Password string
}

func (d *DumbAuth) CheckPasswd(ctx *server.Context, username string, password string) (bool, error) {
	if d.Debug {
		fmt.Printf("FTP: %s is connecting with username %s and password %s\n", ctx.Sess.RemoteAddr().String(), username, password)
	}
	return password == d.Password, nil
}
