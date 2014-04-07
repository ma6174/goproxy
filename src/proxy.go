package main

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/ma6174/aria2rpc"
)

var ErrNoRedirect = errors.New("No Redirect!")
var DwonPath = "./down/"
var StaticPath = "./static/"
var cacheDwon = make(map[string]bool)
var ADList = make(map[string]bool)

func buildADList(filename string) {
	f, err := os.Open(filename)
	if err != nil {
		log.Println("open ad list file failed!")
		return
	}
	buf := bufio.NewReader(f)
	for {
		line, err := buf.ReadString('\n')
		if err != nil || err == io.EOF {
			break
		}
		line = strings.TrimSpace(line)
		ADList[line] = true
	}
	return
}

func isAdHost(host string) bool {
	sp := strings.Split(host, ".")
	for i := 0; i < len(sp); i++ {
		host = strings.Join(sp[i:], ".")
		log.Println(host)
		if ADList[host] == true {
			return true
		}
	}
	return false
}

func genURL(r *http.Request) (url string) {
	if r.URL.Scheme == "" {
		r.URL.Scheme = "http"
	}
	if r.Host == "" {
		log.Println("No Host")
		return
	}
	var rawQuery, fragment, userInfo string
	if r.URL.RawQuery != "" {
		rawQuery = "?" + r.URL.RawQuery
	}
	if r.URL.Fragment != "" {
		fragment = "#" + r.URL.Fragment
	}
	if r.URL.User != nil {
		userInfo = r.URL.User.String()
	}
	url = r.URL.Scheme + userInfo + "://" + r.Host + r.URL.Path + rawQuery + fragment
	return
}

func buildRequest(rawurl string, r *http.Request) *http.Request {
	rurl, err := url.Parse(rawurl)
	if err != nil {
		return nil
	}
	req := &http.Request{
		Method:        r.Method,
		URL:           rurl,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        r.Header,
		Body:          r.Body,
		Host:          r.Host,
		ContentLength: r.ContentLength,
	}
	via := req.Header.Get("Via")
	req.Header.Set("Via", via+", 1.1 rpi")
	return req
}

func buildHTTPClient() *http.Client {
	tr := &http.Transport{
		Dial: func(netw, addr string) (net.Conn, error) {
			deadline := time.Now().Add(24 * time.Hour)
			c, err := net.DialTimeout(netw, addr, time.Second*60*5)
			if err != nil {
				return nil, err
			}
			c.SetDeadline(deadline)
			return c, nil
		},

		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return ErrNoRedirect
		},
	}
	return client
}

func defaultHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("============================")
	if r.Host == "127.0.0.1" || r.Host == "localhost" {
		log.Println("No Local Loop!")
		w.Write([]byte("No Local Loop!"))
		return
	}
	rawurl := genURL(r)
	if rawurl == "" {
		http.Redirect(w, r, "http://browsehappy.com/", 303)
		return
	}
	log.Println(rawurl)
	if isAdHost(r.Host) {
		log.Println("Find AD: ", rawurl)
		return
	}
	urlB64 := base64.StdEncoding.EncodeToString([]byte(rawurl))
	if _, ok := cacheDwon[urlB64]; ok {
		fReader, err := os.Open(DwonPath + urlB64)
		if err != nil {
			log.Println("63", err)
		} else {
			defer fReader.Close()
			w.Header().Add("X-Cache", "From Proxy")
			io.Copy(w, fReader)
			return
		}
	}
	req := buildRequest(rawurl, r)
	if req == nil {
		log.Println("build request failed: wrong url: ", rawurl)
		return
	}
	client := buildHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		if !strings.Contains(err.Error(), "No Redirect!") {
			return
		}
		err = nil
	}
	defer resp.Body.Close()
	var fWiter *os.File
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		if strings.HasPrefix(contentType, "image/") {
			fWiter, err = os.Create(DwonPath + urlB64)
			if err != nil {
				log.Println("119", err)
			} else {
				defer fWiter.Close()
				cacheDwon[urlB64] = true
			}
		}
	}
	var mWriter io.Writer
	if fWiter != nil {
		mWriter = io.MultiWriter(w, fWiter)
	} else {
		mWriter = w
	}
	log.Println("129>>>>", resp.StatusCode)
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	writed, err := io.Copy(mWriter, resp.Body)
	if err != nil {
		log.Println("io.Copy failed:", err, resp.ContentLength, writed)
	}
	return
}

func downloadHandle(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		t, err := template.ParseFiles("./tmpl/download.html")
		if err != nil {
			log.Println(err)
			return
		}
		t.Execute(w, nil)
		return
	}
	r.ParseForm()
	uri := r.Form.Get("url")
	aria2rpc.AddUri(uri, nil)
}

func startAria2c() {
	cmd := "aria2c --enable-rpc --rpc-listen-all=true --rpc-allow-origin-all -c -D"
	sp := strings.Split(cmd, " ")
	process := exec.Command(sp[0], sp[1:]...)
	process.Dir = "./dl"
	process.Run()
	log.Println("aria2c version", aria2rpc.RpcVersion, "serve at:", aria2rpc.RpcUrl)
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	runtime.GOMAXPROCS(runtime.NumCPU()*2 - 1)
	log.Println("init...")
	log.Println("clean cache: ", DwonPath)
	os.RemoveAll(DwonPath)
	log.Println("create cache dir")
	os.Mkdir(DwonPath, 0755)
	log.Println("build ad list")
	buildADList("./ad_hosts.list")
	log.Println("start aria2c")
	startAria2c()
	log.Println("init finish")
}

func main() {
	mux1 := http.NewServeMux()
	mux1.HandleFunc("/", defaultHandler)
	mux1.HandleFunc("/dl/", http.StripPrefix("/dl/", http.FileServer(http.Dir(StaticPath+"/aria2/"))))
	mux1.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(StaticPath))))
	s := &http.Server{
		Addr:           ":7080",
		Handler:        mux1,
		ReadTimeout:    0,
		WriteTimeout:   0,
		MaxHeaderBytes: 1 << 20,
	}
	log.Println("start proxy server")
	log.Fatal(s.ListenAndServe())
}
