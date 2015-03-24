// +build windows

package main

/*
#include <stdio.h>
#include <stdlib.h>
#include <Windows.h>

HANDLE sema;

void singleProc() {

	sema = CreateSemaphoreA(NULL, 1, 1, (LPCSTR)"flagSingle");
	if (GetLastError() == 183) {
		printf("进程已经运行");
		exit(0);
	}
}
*/
import "C"

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	//"syscall"
	"time"

	"github.com/kardianos/osext"
	"github.com/kardianos/service"
	"github.com/liuzhiyi/daemon/common"
	"github.com/natefinch/npipe"
)

const (
	currentVersion = "1.0"
	master         = "tokentest.exe"
)

var (
	logger      service.Logger
	pConn       *npipe.PipeConn
	versionData map[string]string
)

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

	// Start should not block. Do the actual work async.
	go p.run(s)
	return nil
}

func (p *program) run(s service.Service) error {
	logger.Info("启动中....")
	//检测更新
	checkUpdate()
	//监测主程
	checkMaster()
	//设置定时器函数
	setTimer()
	return nil
}

func main() {
	defer C.CloseHandle(C.sema)
	C.singleProc()
	// // hd, err := common.CreateSema(0, 1, syscall.StringToUTF16Ptr("streamSemaphore"))
	// // if err != nil {
	// // 	fmt.Println(err.Error)
	// // }
	// defer syscall.Close(hd)
	flag.Parse()
	svcConfig := &service.Config{
		Name:        "helper",
		DisplayName: "Update Service",
		Description: "This is a helper Go service.",
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
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

	args := flag.Args()
	if len(args) == 1 {
		err := service.Control(s, args[0])
		if err != nil {
			if args[0] == service.ControlAction[0] {
				fmt.Println(args[0])
				s.Install()
				s.Start()
			} else if args[0] == service.ControlAction[1] {
				s.Install()
				s.Stop()
			} else {
				log.Printf("Valid actions: %q\n", service.ControlAction)
				log.Fatal(err)
			}
		}
		return
	}

	err = s.Run()
	if err != nil {
		logger.Error(err)
	}
}

func setTimer() {
	common.Timer(60*60*time.Second, checkUpdate)
	common.Timer(60*time.Second, checkMaster)
}

func checkUpdate() {
	if compareVersion(getLatestVersion(),
		getLocalVersion()) > 0 {
		updateVersion()
		logger.Info("版本更新完成")
	}
}

func checkMaster() {
	if !common.IsRunning(master) {
		err := startMaster()
		if err != nil {
			logger.Info(err.Error())
		}
	}
}

func heartBeat() bool {
	msg := common.SendCmd(pConn, common.CmdVersion)
	if msg != "" {
		return true
	} else {
		pConn.Close()
		createPipeConn()
	}
	return false
}

//命名管道实现心跳，定时检测版本更新。
func createPipeConn() {
	var err error
	tries := 5
	for tries > 0 {
		pConn, err = npipe.DialTimeout(common.PipeAddr, 5*time.Second)
		if err != nil {
			log.Print(err.Error())
			err = startMaster()
			if err != nil {
				log.Fatal(err.Error())
			}
			tries = 1
		} else {
			break
		}
		tries--
	}
	if pConn == nil {
		log.Fatal("the program has an fatal error")
	}
}

func startMaster() error {
	cmd := exec.Command(master, "start")
	binPath, _ := osext.ExecutableFolder()
	logger.Info(binPath)
	cmd.Dir = binPath
	return cmd.Start()
}

func stopMaster() error {
	cmd := exec.Command(master, "stop")
	cmd.Dir, _ = osext.ExecutableFolder()
	return cmd.Start()
}

func unzip(name string) error {
	return exec.Command(`C:\Program Files\WinRAR\winrar`, fmt.Sprintf("./%s", name), "/x", " ./").Start()
}

func updateVersion() {
	stopMaster()
	exec.Command(master, "uninstall").Start()

	binPath, _ := osext.ExecutableFolder()
	//rename master
	oldMaster := fmt.Sprintf("%s.old", master)
	os.Rename(path.Join(binPath, master), oldMaster)

	//download
	version := versionData["version"]
	build := versionData["build"]
	url := fmt.Sprintf("http://127.0.0.1:3000/v%s/file/%s/%s", version, build, master)
	downloadFromUrl(url)

	//unzip
	//unzip(master)

	//run server
	startMaster()
}

func compareVersion(v1, v2 string) int {
	return bytes.Compare([]byte(v1), []byte(v2))
}

func getLocalVersion() string {
	return "12"
}

func processResult(rsp *http.Response) (rel map[string]string) {
	rel = make(map[string]string)
	str, err := ioutil.ReadAll(rsp.Body)
	err = json.Unmarshal(str, &rel)
	if err != nil {
		fmt.Println(err.Error())
	}
	return
}

func getLatestVersion() string {
	rsp, err := http.Get("http://127.0.0.1:3000/v1.0/version")
	if err == nil {
		data := processResult(rsp)
		if build, ok := data["build"]; ok {
			fmt.Println(build)
			versionData = data
			return build
		}
		rsp.Body.Close()
	}
	return ""
}

func downloadFromUrl(url string) {
	tokens := strings.Split(url, "/")
	fileName := tokens[len(tokens)-1]
	fmt.Println("Downloading", url, "to", fileName)

	// TODO: check file existence first with io.IsExist
	output, err := os.Create(fileName)
	if err != nil {
		fmt.Println("Error while creating", fileName, "-", err)
		return
	}
	defer output.Close()

	response, err := http.Get(url)
	if err != nil {
		fmt.Println("Error while downloading", url, "-", err)
		return
	}
	defer response.Body.Close()

	n, err := io.Copy(output, response.Body)
	if err != nil {
		fmt.Println("Error while downloading", url, "-", err)
		return
	}

	fmt.Println(n, "bytes downloaded.")
}
