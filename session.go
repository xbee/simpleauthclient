package main

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/golibs/uuid"
	"github.com/liuzhiyi/utils/levelcache"
)

var cacheEng *levelcache.Levelcache

func init() {
	dir, err := os.Getwd()
	if err != nil {
		dir = "./"
	}
	dbPath := path.Join(dir, cachePath)
	cacheEng = levelcache.NewLevecache(defaultExpiration, 2*60*60*time.Second, dbPath)
}

type Session struct {
	sid       string
	container map[string]string
}

func sessionStart(w http.ResponseWriter, req *http.Request) *Session {
	s := new(Session)
	s.container = make(map[string]string)
	if cookie, err := req.Cookie(sessionName); err == nil {
		if ok := cacheEng.Get(cookie.Value, &s.container); ok {
			return s
		}
		fmt.Println(s)
	}
	cookie := new(http.Cookie)
	cookie.Path = "/"
	sid := strings.Replace(uuid.Rand().Hex(), "-", "", -1)
	cookie.Name = sessionName
	cookie.Value = sid
	http.SetCookie(w, cookie)
	s.sid = sid
	cacheEng.Set(s.sid, s.container, 0)
	return s
}

func (s *Session) set(key, val string) {
	s.container[key] = val
	cacheEng.Set(s.sid, s.container, 0)
}
