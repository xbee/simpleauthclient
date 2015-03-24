package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"time"

	"github.com/kardianos/osext"
	"github.com/kardianos/service"
	"github.com/liuzhiyi/daemon/common"
)

const (
	version string = "1.0"
	proto   string = "tcp"
	addr    string = "127.0.0.1:3000"
	helper         = "helper.exe"
)

var (
	logger service.Logger
	s      service.Service
)

type DaemonCli struct {
	proto     string
	addr      string
	scheme    string
	in        io.ReadCloser
	out       io.Writer
	err       io.Writer
	transport *http.Transport
}

func NewDaemonCli() *DaemonCli {
	scheme := "http"
	tr := &http.Transport{
	//TLSClientConfig: tlsConfig,
	}
	return &DaemonCli{
		proto:     proto,
		addr:      addr,
		scheme:    scheme,
		in:        os.Stdin,
		out:       os.Stdout,
		err:       os.Stderr,
		transport: tr,
	}
}

func (c *DaemonCli) getMethod(args ...string) (func(...string) error, bool) {
	camelArgs := make([]string, len(args))
	for i, s := range args {
		if len(s) == 0 {
			return nil, false
		}
		camelArgs[i] = strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
	}
	methodName := "Cmd" + strings.Join(camelArgs, "")
	method := reflect.ValueOf(c).MethodByName(methodName)
	if !method.IsValid() {
		return nil, false
	}
	return method.Interface().(func(...string) error), true
}

func (c *DaemonCli) Cmd(args ...string) error {
	if len(args) > 1 {
		method, exists := c.getMethod(args[:2]...)
		if exists {
			return method(args[2:]...)
		}
	}
	if len(args) > 0 {
		method, exists := c.getMethod(args[0])
		if !exists {
			fmt.Fprintf(c.err, "'%s' is not a command. See '--help'.\n", args[0])
			os.Exit(1)
		}
		return method(args[1:]...)
	}
	return c.CmdHelp()
}

func (c *DaemonCli) CmdHelp(args ...string) error {
	if len(args) > 1 {
		method, exists := c.getMethod(args[:2]...)
		if exists {
			method("--help")
			return nil
		}
	}
	if len(args) > 0 {
		method, exists := c.getMethod(args[0])
		if !exists {
			fmt.Fprintf(c.err, "'%s' is not a command. See '--help'.\n", args[0])
			os.Exit(1)
		} else {
			method("--help")
			return nil
		}
	}

	flag.Usage()

	return nil
}

func main() {
	flag.Parse()
	if len(*flHost) == 0 {
		*flHost = addr
	}
	svcConfig := &service.Config{
		Name:        "tokentest",
		DisplayName: "Api Service",
		Description: "This is a Api Go service.",
	}
	prg := &program{}
	var err error
	s, err = service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err.Error())
	}
	errs := make(chan error, 5)
	logger, err = s.Logger(errs)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			err := <-errs
			if err != nil {
				log.Print(err)
			}
		}
	}()

	cli := NewDaemonCli()
	if *flTls {
		cli.scheme = "https"
	}
	if len(flag.Args()) > 0 {
		if err := cli.Cmd(flag.Args()...); err != nil {
			fmt.Fprint(cli.err, err.Error())
		}
		return
	}
	err = s.Run()
	if err != nil {
		logger.Error(err)
	}
}

type program struct {
	exit chan struct{}
}

func (p *program) Stop(s service.Service) error {
	// Any work in Stop should be quick, usually a few seconds at most.
	logger.Info("I'm Stopping!")
	close(p.exit)
	return nil
}

func (p *program) Start(s service.Service) error {
	p.exit = make(chan struct{})
	p.run(s)
	return nil
}

func (p *program) run(s service.Service) error {
	checkHelper()
	common.Timer(60*time.Second, checkHelper)
	return nil
}

func checkHelper() {
	if !common.IsRunning(helper) {
		startHelper()
	}
}

func startHelper() error {
	cmd := exec.Command(helper, "start")
	cmd.Dir, _ = osext.ExecutableFolder()
	return cmd.Start()
}

func (c *DaemonCli) CmdInstall(args ...string) error {
	return s.Install()
}

func (c *DaemonCli) CmdUninstall(args ...string) error {
	return s.Uninstall()
}

func (c *DaemonCli) CmdStart(args ...string) error {
	if err := s.Start(); err != nil {
		s.Install()
		return s.Start()
	} else {
		return nil
	}

}

func (c *DaemonCli) CmdStop(args ...string) error {

	if err := s.Stop(); err != nil {
		s.Install()
		return s.Stop()
	} else {
		return nil
	}
}
