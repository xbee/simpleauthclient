package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/kardianos/service"
	"github.com/liuzhiyi/daemon/common"
	"github.com/natefinch/npipe"
)

const (
	addr                            = ":3000"
	vesion                          = "1.0"
	defaultExpiration time.Duration = 60 * 60 * time.Second
	cachePath         string        = "/cache"
	sessionName                     = "SESSIONID"
)

var logger service.Logger
var flTls *bool = flag.Bool("-tls", false, "enable tls mode")

// Program structures.
//  Define Start and Stop methods.
type program struct {
	exit chan struct{}
}

func (p *program) Start(s service.Service) error {
	if service.Interactive() {
		logger.Info("Running in terminal.")
	} else {
		logger.Info("Running under service manager.")
	}
	p.exit = make(chan struct{})

	// Start should not block. Do the actual work async.
	go p.run(s)
	return nil
}

func (p *program) run(s service.Service) error {
	logger.Infof("I'm running %v.", service.Platform())
	// createPipeServer()
	httpServer(s)
	return nil
}

func (p *program) Stop(s service.Service) error {
	// Any work in Stop should be quick, usually a few seconds at most.
	logger.Info("I'm Stopping!")
	close(p.exit)
	return nil
}

func httpServer(s service.Service) {
	r := createRouters(s)
	var err error
	if *flTls {
		err = http.ListenAndServeTLS(addr, "cert.pem", "key.pem", r)
	} else {
		err = http.ListenAndServe(addr, r)
	}
	if err != nil {
		logger.Error(err.Error())
		panic(err.Error())
	}
}

type httpHandler func(service.Service, http.ResponseWriter, *http.Request, *Session)

type route struct {
	method string
	path   string
	fn     http.Handler
}

func (r *route) match(path string) bool {
	fmt.Println(path, r.path)
	return r.path == path
}

func (r *route) staticMatch(path string) bool {
	return strings.HasPrefix(path, r.path)
}

type Router struct {
	routes []*route
}

func newRouter() *Router {
	return &Router{}
}

func (r *Router) newRoute() *route {
	route := &route{}
	r.routes = append(r.routes, route)
	return route
}

func (r *Router) HandleFunc(s service.Service, method string, path string, handler httpHandler) {
	route := r.newRoute()
	route.method = method
	route.path = fmt.Sprintf("/v%s%s", vesion, path)
	route.fn = r.makeHttpFnc(s, handler)
}

func (r *Router) makeHttpFnc(s service.Service, handler httpHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		session := sessionStart(w, req)
		handler(s, w, req, session)
	}
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if p := cleanPath(req.URL.Path); p != req.URL.Path {
		sessionStart(w, req)
		url := *req.URL
		url.Path = p
		p = url.String()

		w.Header().Set("Location", p)
		w.WriteHeader(http.StatusMovedPermanently)
		return
	}
	var handler http.Handler
	for _, route := range r.routes {
		if route.method == "static" {
			if route.staticMatch(req.URL.Path) {
				handler = route.fn
				break
			}
		}
		if req.Method == route.method {
			if route.match(req.URL.Path) {
				handler = route.fn
				break
			}
		}
	}
	if handler == nil {
		handler = http.NotFoundHandler()
	}

	handler.ServeHTTP(w, req)
}

func createRouters(s service.Service) *Router {
	r := newRouter()
	m := map[string]map[string]httpHandler{
		"POST": {
			"/token": TokenHandle,
			"/reset": Reset,
		},
		"GET": {
			"/version": Version,
		},
		"static": {
			"/file": Static,
		},
	}

	for method, routers := range m {
		for router, handler := range routers {
			r.HandleFunc(s, method, router, handler)
		}
	}
	return r
}

func Static(s service.Service, w http.ResponseWriter, req *http.Request, session *Session) {
	fmt.Println(http.Dir("./v1.0/file/"))
	staticHandler := http.FileServer(http.Dir("./"))
	staticHandler.ServeHTTP(w, req)
	return

}

func Version(s service.Service, w http.ResponseWriter, req *http.Request, session *Session) {
	data := make(map[string]string)
	data["success"] = "true"
	data["version"] = "1.0.0"
	data["build"] = "13"
	output(w, data)
}

func Reset(s service.Service, w http.ResponseWriter, req *http.Request, session *Session) {
	data, params := decodeData(req)
	if _, logined := session.container["username"]; logined {
		password, ok := params["password"]
		if ok {
			if password == "123456" {
				if err := s.Restart(); err != nil {
					logger.Errorf("restart failed:%s", err.Error())
					data.Code = "202"
					data.Msg = "密码不正确"
				} else {
					logger.Info("restart success")
					data.Code = "200"
					data.Msg = "服务器重启成功"
				}
			}
		} else if data.Code == "" {
			data.Code = "301"
			data.Msg = "参数不正确"
		}
	} else {
		data.Code = "207"
		data.Code = "请登录"
	}
	output(w, data)
}

func decodeData(req *http.Request) (Rsp, map[string]string) {
	var data Rsp
	params := make(map[string]string)
	str, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Info(err.Error())
		data.Code = "100"
		data.Msg = "系统错误"
		return data, params
	}

	if err := json.Unmarshal(str, &params); err != nil {
		logger.Info(err.Error())
		data.Code = "201"
		data.Msg = "数据解析错误"
		return data, params
	}
	return data, params
}

func output(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if ss, err := json.Marshal(data); err != nil {
		logger.Error(err.Error())
		panic(err.Error())
	} else {
		io.WriteString(w, string(ss))
	}
}

func TokenHandle(s service.Service, w http.ResponseWriter, req *http.Request, session *Session) {
	data, params := decodeData(req)
	var username, password string
	var ok bool
	username, ok = params["username"]
	password, ok = params["password"]
	if ok {
		if username == "admin" && password == "123456" {
			data.Code = "200"
			data.Msg = "登录成功"
			session.set("username", username)
		} else {
			data.Code = "202"
			data.Msg = "用户名或密码错误"
		}
	} else if data.Code == "" {
		data.Code = "301"
		data.Msg = "参数不正确"
	}
	output(w, data)
}

func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	np := path.Clean(p)
	// path.Clean removes trailing slash except for root;
	// put the trailing slash back if necessary.
	if p[len(p)-1] == '/' && np != "/" {
		np += "/"
	}
	return np
}

type Rsp struct {
	Code   string
	Msg    string
	Object interface{}
}

// Service setup.
//   Define service config.
//   Create the service.
//   Setup the logger.
//   Handle service controls (optional).
//   Run the service.
func main() {
	//svcFlag := flag.String("service", "", "Control the system service.")
	flag.Parse()

	svcConfig := &service.Config{
		Name:        "daemon",
		DisplayName: "Go Service Test",
		Description: "This is a test Go service.",
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
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

func createPipeServer() {
	ln, err := npipe.Listen(common.PipeAddr)
	if err != nil {
		log.Fatal(err.Error())
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Print(err.Error())
				continue
			}

			// handle connection like any other net.Conn
			go func(conn net.Conn) {
				defer conn.Close()
				r := bufio.NewReader(conn)
				msg, err := r.ReadString('\n')
				if err != nil {
					log.Print(err.Error())
					return
				}
				fmt.Fprint(conn, "heeelo\n")
				fmt.Println(msg)
			}(conn)
		}
	}()
}
