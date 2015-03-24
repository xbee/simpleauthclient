package common

import (
	"bufio"
	"fmt"
	"github.com/natefinch/npipe"
)

const (
	master         = "../daemon.exe"
	currentVersion = "1.0"

	CmdVersion = "show version"
	CmdStop    = "stop"
	CmdStart   = "start"
	Success    = "success"
	Failure    = "failure"
	PipeAddr   = `\\.\pipe\daemon`
)

func SendCmd(conn *npipe.PipeConn, cmd string) string {
	fmt.Fprint(conn, fmt.Sprintf("", cmd))
	msg, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return ""
	}
	return msg
}
