package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-martini/martini"
	"github.com/gogap/env_strings"
	"github.com/martini-contrib/auth"
	"github.com/martini-contrib/cors"

	"github.com/gogap/string_keeper/git"
)

var (
	resDir     string
	keeperConf StringKeeperConfig
)

var (
	gitDirList map[string]bool = make(map[string]bool)

	revisionFileCache map[string][]byte = make(map[string][]byte)

	synclocker sync.Mutex
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
	Revision  string                 `json:"revision,omitempty"`
	File      string                 `json:"file"`
	Envs      map[string]interface{} `json:"envs"`
	RawData   bool                   `json:"raw_data"`
}

func AuthCheck(userName, password string) bool {
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

func BucketAccessAuthCheck(user auth.User, namespace, bucket string) (err error) {

	strUser := string(user)
	correctUserName := strings.Join([]string{namespace, bucket}, "/")

	if strUser == "" {
		err = fmt.Errorf("no auth info found")
		return
	} else if strUser != correctUserName {
		err = fmt.Errorf("did not have matched account of %s", correctUserName)
		return
	}

	return
}

func IPCheck(remoteIP string) (err error) {

	if keeperConf.ACL.IPWhiteList == nil {
		err = fmt.Errorf("the ip of %s is not in white list", remoteIP)
		return
	}

	isWhiteIP := false
	for _, ip := range keeperConf.ACL.IPWhiteList {
		if ip == remoteIP {
			isWhiteIP = true
			break
		}
	}
	if !isWhiteIP {
		err = fmt.Errorf("the ip of %s is not in white list", remoteIP)
		return
	}

	return
}

func GetBucketString(
	res http.ResponseWriter,
	req *http.Request,
	user auth.User,
	params martini.Params) {

	if keeperConf.ACL.IPACLEnabled {
		remoteIP, _, _ := net.SplitHostPort(req.RemoteAddr)
		if e := IPCheck(remoteIP); e != nil {
			respErrorf(res, http.StatusForbidden, "%s", e.Error())
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
		if e := BucketAccessAuthCheck(user, data.Namespace, data.Bucket); e != nil {
			respErrorf(res, http.StatusForbidden, "%s", e.Error())
			return
		}
	}

	bucketRoot := filepath.Join("public", data.Namespace, data.Bucket)
	originalStringFile := filepath.Join(bucketRoot, data.File)
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
			if !filepath.HasPrefix(absPath, resDir) {
				res.WriteHeader(http.StatusBadRequest)
				return
			}
		}
	}

	if fi, e := os.Stat(stringFile); e != nil {
		if os.IsNotExist(e) {
			respErrorf(res, http.StatusNotFound, "file '%s' not exist", originalStringFile)
			return
		} else {
			respErrorf(res, http.StatusInternalServerError, "get file stat of %s error, err: %s", originalStringFile, e.Error())
			return
		}
	} else if fi.IsDir() {
		respErrorf(res, http.StatusExpectationFailed, "the request file of %s is a dir", originalStringFile)
		return
	}

	fileDir := filepath.Dir(stringFile)

	readfileDirect := false
	badRequest := false
	gitFileRoot := ""
	gitFilePath := ""

	var err error

	if data.Revision == "" {
		readfileDirect = true
	} else {

		if fileDir == bucketRoot {
			readfileDirect = true
			badRequest = true
		} else {

			if gitFileRoot, err = getFileGitRoot(bucketRoot, fileDir); err != nil {
				respErrorf(res, http.StatusExpectationFailed, "get file of %s git repo root error, err: %s", originalStringFile, err.Error())
				return
			}

			gitFilePath, _ = filepath.Rel(gitFileRoot, stringFile)

			if isGit, exist := gitDirList[gitFileRoot]; exist && isGit {
				readfileDirect = false
			} else if !exist {
				if fi, e := os.Stat(filepath.Join(gitFileRoot, ".git")); e != nil {
					synclocker.Lock()
					gitDirList[gitFileRoot] = false
					badRequest = true
					synclocker.Unlock()
				} else if fi.IsDir() {
					synclocker.Lock()
					gitDirList[gitFileRoot] = true
					readfileDirect = false
					go gitPuller(gitFileRoot)
					synclocker.Unlock()
				} else {
					badRequest = true
				}
			} else {
				readfileDirect = true
			}
		}
	}

	if badRequest {
		respErrorf(res, http.StatusBadRequest, "read file of %s error, err: the file is not in git dir, could not use revision to pick file", originalStringFile)
		return
	}

	var fileData []byte

	if readfileDirect {
		if fileData, err = ioutil.ReadFile(stringFile); err != nil {
			respErrorf(res, http.StatusExpectationFailed, "read file of %s error, err: %s", originalStringFile, err.Error())
			return
		}
	} else {
		if fileData, err = GetRevisionFile(gitFileRoot, gitFilePath, data.Revision); err != nil {
			respErrorf(res, http.StatusExpectationFailed, "read file of %s error, git root: %s, revision: %s , err: %s", gitFilePath, gitFileRoot, data.Revision, err.Error())
			return
		}
	}

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

func GetRevisionFile(baseDir, filePath, revision string) (data []byte, err error) {
	revisionPath := revision + ":" + filePath

	exist := false
	if data, exist = revisionFileCache[revisionPath]; exist {
		return
	}

	repoGit := git.NewGit(baseDir)

	if data, err = repoGit.CatBlobFile(filePath, revision); err != nil {
		err = fmt.Errorf("%s\n%s", string(data), err.Error())
		return
	}

	revisionFileCache[revisionPath] = data

	return
}

func getFileGitRoot(bucketDir string, fileDir string) (repoGitRoot string, err error) {
	relPath := ""
	if relPath, err = filepath.Rel(bucketDir, fileDir); err != nil {
		return
	}

	dirs := strings.Split(relPath, "/")

	repoGitRoot = filepath.Join(bucketDir, dirs[0])

	return
}

func gitPuller(gitRoot string) {
	repo := git.NewGit(gitRoot)
	for {
		if output, e := repo.Pull(); e != nil {
			log.Printf("[%s]:%s, error: %s\n", gitRoot, string(output), e.Error())
		} else {
			log.Printf("[%s]:%s\n", gitRoot, string(output))
		}
		time.Sleep(time.Second * 30)
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
