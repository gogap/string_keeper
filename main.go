package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-martini/martini"
	"github.com/gogap/env_strings"
	"github.com/martini-contrib/auth"
	"github.com/martini-contrib/cors"
)

var (
	resDir     string
	keeperConf StringKeeperConfig
)

func main() {

	if conf, e := LoadConfig("conf/string_keeper.conf"); e != nil {
		if os.IsNotExist(e) {
			fmt.Println("'conf/string_keeper.conf' not exist, it will use default config.")
			keeperConf = DefaultConfig()
		} else {
			fmt.Printf("load config file 'conf/string_keeper.conf' failed, err: %s\n", e.Error())
			os.Exit(1)
		}
	} else {
		keeperConf = conf
	}

	m := martini.Classic()

	m.Post("/", GetBucketString)

	m.Get("/ping", func() string {
		return "pong"
	})

	if cwd, e := os.Getwd(); e != nil {
		fmt.Printf("get current dir failed, err: %s", e.Error())
		os.Exit(1)
	} else if !filepath.IsAbs(cwd) {
		if absPath, e := filepath.Abs(cwd); e != nil {
			fmt.Printf("get current dir abs path failed, err: %s", e.Error())
			os.Exit(1)
			return
		} else {
			resDir = filepath.Join(absPath, "public")
		}
	} else {
		resDir = filepath.Join(cwd, "public")
	}

	m.Use(cors.Allow(&cors.Options{
		AllowOrigins:     keeperConf.HTTP.CORS.AllowOrigins,
		AllowMethods:     keeperConf.HTTP.CORS.AllowMethods,
		AllowHeaders:     keeperConf.HTTP.CORS.AllowHeaders,
		ExposeHeaders:    keeperConf.HTTP.CORS.ExposeHeaders,
		AllowCredentials: keeperConf.HTTP.CORS.AllowCerdentials,
	}))

	if keeperConf.ACL.AuthEnabled {
		m.Use(auth.BasicFunc(AuthCheck))
	} else {
		m.Map(auth.User(""))
	}

	m.RunOnAddr(keeperConf.HTTP.Address)
}

type PostData struct {
	Namespace string                 `json:"namespace"`
	Bucket    string                 `json:"bucket"`
	File      string                 `json:"file"`
	Envs      map[string]interface{} `json:"envs"`
	RawData   bool                   `json:"raw_data"`
}

func AuthCheck(userName, password string) bool {
	fmt.Printf("format")
	if keeperConf.ACL.AuthEnabled {
		if keeperConf.ACL.BasicAuth != nil {
			if pwd, exist := keeperConf.ACL.BasicAuth[userName]; exist && pwd == password {
				return true
			}
		}
		return false
	}
	return true
}

func GetBucketString(
	res http.ResponseWriter,
	req *http.Request,
	user auth.User,
	params martini.Params) {

	if keeperConf.ACL.IPACLEnabled {
		clientIP, _, _ := net.SplitHostPort(req.RemoteAddr)

		if keeperConf.ACL.IPWhiteList == nil {
			respErrorf(res, http.StatusForbidden, "the ip of %s is not in white list", clientIP)
			return
		}

		isWhiteIP := false
		for _, ip := range keeperConf.ACL.IPWhiteList {
			if ip == clientIP {
				isWhiteIP = true
				break
			}
		}
		if !isWhiteIP {
			respErrorf(res, http.StatusForbidden, "the ip of %s is not in white list", clientIP)
			return
		}
	}

	postBody, e := ioutil.ReadAll(req.Body)

	if e != nil {
		res.WriteHeader(http.StatusBadRequest)
		return
	}

	data := PostData{}

	if postBody != nil && string(postBody) != "" {
		if e := json.Unmarshal(postBody, &data); e != nil {
			respErrorf(res, http.StatusInternalServerError, "unmarshal post body to struct failed, err: %s", e.Error())
			return
		}
	}

	data.Namespace = strings.TrimSpace(data.Namespace)
	data.Bucket = strings.TrimSpace(data.Bucket)
	data.File = strings.TrimSpace(data.File)

	if data.Namespace == "" ||
		data.Bucket == "" ||
		data.File == "" {
		respErrorf(res, http.StatusBadRequest, "namespace/bucket/file could not empty.")
		return
	}

	if keeperConf.ACL.AuthEnabled {
		strUser := string(user)
		correctUserName := strings.Join([]string{data.Namespace, data.Bucket}, "/")

		if strUser == "" {
			respErrorf(res, http.StatusForbidden, "no auth info found")
			return
		} else if strUser != correctUserName {
			respErrorf(res, http.StatusForbidden, "did not have matched account of %s", correctUserName)
			return
		}
	}

	originalStringFile := filepath.Join("public", data.Namespace, data.Bucket, data.File)
	stringFile := originalStringFile

	if strings.Contains(stringFile, "..") || strings.Contains(stringFile, "./") {
		respErrorf(res, http.StatusForbidden, "request data contains denied string")
		return
	}

	if !filepath.IsAbs(stringFile) {
		if absPath, e := filepath.Abs(stringFile); e != nil {
			respErrorf(res, http.StatusInternalServerError, "get file of %s's abs path failed, err: %s", stringFile, e.Error())
			return
		} else {
			stringFile = absPath
		}
	}

	if !filepath.HasPrefix(stringFile, resDir) {
		res.WriteHeader(http.StatusBadRequest)
		return
	}

	if fi, e := os.Stat(stringFile); e != nil {
		if os.IsNotExist(e) {
			respErrorf(res, http.StatusInternalServerError, "file '%s' not exist", originalStringFile)
			return
		} else {
			respErrorf(res, http.StatusInternalServerError, "get file stat of %s error, err: %s", originalStringFile, e.Error())
			return
		}
	} else if fi.IsDir() {
		respErrorf(res, http.StatusInternalServerError, "the request file of %s is a dir", originalStringFile)
		return
	}

	if fileData, e := ioutil.ReadFile(stringFile); e != nil {
		respErrorf(res, http.StatusInternalServerError, "read file of %s error, err: %s", originalStringFile, e.Error())
		return
	} else {
		if !data.RawData {
			if retStr, e := env_strings.ExecuteWith(string(fileData), data.Envs); e != nil {
				respErrorf(res, http.StatusInternalServerError, "build file of %s error, err: %s", originalStringFile, e.Error())
				return
			} else {
				respString(res, retStr)
				return
			}
		} else {
			respBytes(res, fileData)
			return
		}
	}
}

func respErrorf(res http.ResponseWriter, code int, format string, v ...interface{}) (int, error) {
	res.WriteHeader(code)
	return respString(res, fmt.Sprintf(format, v...))
}

func respString(res http.ResponseWriter, str string) (int, error) {
	return res.Write([]byte(str))
}

func respBytes(res http.ResponseWriter, bytes []byte) (int, error) {
	return res.Write(bytes)
}
