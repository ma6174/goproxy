package main

import (
	"bufio"
	"encoding/base64"
	"errors"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ma6174/aria2rpc"
)

var (
	ErrNoRedirect = errors.New("No Redirect!")
	DwonPath      = "./down/"
	StaticPath    = "./static/"
	cacheFN       = "cache_log"
	cacheDwon     = make(map[string]interface{})
	ADList        = make(map[string]bool)
	Client        *http.Client
	CacheLog      *os.File
)

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
		Close:         true,
	}
	via := req.Header.Get("Via")
	if via == "" {
		req.Header.Set("Via", "1.1 rpiup")
	} else {
		req.Header.Set("Via", via+", 1.1 rpiup")
	}
	return req
}

func buildHTTPClient() {
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
	}
	Client = &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return ErrNoRedirect
		},
	}
}

func addLog(urlB64 string, resp *http.Response) {
	url, _ := base64.StdEncoding.DecodeString(urlB64)
	info := []string{
		time.Now().Format("2006-01-02 15:04:05"),
		strconv.FormatInt(resp.ContentLength, 10),
		resp.Header.Get("Content-Type"),
		resp.Header.Get("Cache-Control"),
		resp.Header.Get("Expires"),
		string(url)}
	CacheLog.WriteString(strings.Join(info, "\t") + "\n")
}

func removeCache(urlB64 string) {
	delete(cacheDwon, urlB64)
	delete(cacheDwon, urlB64+".type")
	delete(cacheDwon, urlB64+".encoding")
	delete(cacheDwon, urlB64+".date")
	delete(cacheDwon, urlB64+".etag")
	os.Remove(DwonPath + urlB64)
}

func doCache(urlB64 string, resp *http.Response, w http.ResponseWriter, r *http.Request) {
	var fWiter *os.File
	var mWriter io.Writer
	var err error
	var isCache bool
	var maxAge int
	cacheControl := resp.Header.Get("Cache-Control")
	noCaches := []string{
		"private",
		"no-cache",
		"no-store",
	}
	start := strings.Index(cacheControl, "max-age=")
	if start != -1 {
		var d []byte
		for i := start + len("max-age="); i < len(cacheControl); i++ {
			if cacheControl[i] >= '0' && cacheControl[i] <= '9' {
				d = append(d, cacheControl[i])
			} else {
				break
			}
		}
		maxAge, _ = strconv.Atoi(string(d))
		if maxAge > 0 {
			log.Println("****max-age", maxAge, cacheControl)
			time.AfterFunc(time.Duration(maxAge)*time.Second, func() {
				removeCache(urlB64)
			})
			isCache = true
		}
	}
	expires := resp.Header.Get("Expires")
	if expires != "" {
		expTime, err := time.Parse(time.RFC1123, expires)
		if err != nil {
			log.Println("Cannot convert time:", expires)
			expTime = time.Now().Add(time.Hour * 24)
		}
		lastTime := expTime.Sub(time.Now())
		if lastTime.Seconds() > 0 && lastTime.Seconds() < float64(maxAge)-5 {
			log.Println("****expires", lastTime.Seconds(), expires)
			isCache = true
			time.AfterFunc(lastTime, func() {
				removeCache(urlB64)
			})
		}
	}
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/html") {
		delete(cacheDwon, urlB64)
		isCache = false
	}
	if !isCache && strings.Contains(ct, "image/") {
		isCache = true
		time.AfterFunc(time.Hour*24, func() {
			removeCache(urlB64)
		})
	}
	if resp.StatusCode%100 > 2 {
		delete(cacheDwon, urlB64)
		isCache = false
	}
	for _, v := range noCaches {
		if strings.Contains(cacheControl, v) {
			delete(cacheDwon, urlB64)
			isCache = false
			break
		}
	}
	if r.Method != "GET" {
		delete(cacheDwon, urlB64)
		isCache = false
	}
	if isCache {
		fWiter, err = os.Create(DwonPath + urlB64)
		if err != nil {
			log.Println("create cache file failed:", err)
			isCache = false
		} else {
			defer fWiter.Close()
		}
	}
	if fWiter != nil {
		mWriter = io.MultiWriter(w, fWiter)
	} else {
		mWriter = w
	}
	log.Println("resp.StatusCode:", resp.StatusCode)
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	writed, err := io.Copy(mWriter, resp.Body)
	if err != nil {
		log.Println("io.Copy failed:", err, resp.ContentLength, writed)
		isCache = false
	}
	if isCache {
		addLog(urlB64, resp)
		cacheDwon[urlB64] = true
		cacheDwon[urlB64+".encoding"] = resp.Header.Get("Content-Encoding")
		cacheDwon[urlB64+".type"] = resp.Header.Get("Content-Type")
		cacheDwon[urlB64+".etag"] = resp.Header.Get("Etag")
		cacheDwon[urlB64+".date"] = resp.Header.Get("Date")
		if cacheDwon[urlB64+".date"] == "" {
			cacheDwon[urlB64+".data"] = time.Now().Format(time.RFC1123)
		}
	}
	return
}

func defaultHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("============================")
	reqHeader, _ := httputil.DumpRequest(r, false)
	log.Println("\n", string(reqHeader))
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
		matchEtag := r.Header.Get("If-None-Match")
		if matchEtag != "" && matchEtag == cacheDwon[urlB64+".etag"] {
			log.Println("304 etag same", matchEtag)
			w.WriteHeader(304)
			return
		}
		modfiSince := r.Header.Get("If-Modified-Since")
		if modfiSince != "" {
			date := cacheDwon[urlB64+".date"].(string)
			tdate, err1 := time.Parse(time.RFC1123, date)
			tmod, err2 := time.Parse(time.RFC1123, modfiSince)
			if err1 == nil && err2 == nil {
				if tmod.Sub(tdate).Nanoseconds() < 0 {
					log.Println("304 Not modify since", modfiSince, "#cache", date)
					w.WriteHeader(304)
					return
				}
			}
		}
		fReader, err := os.Open(DwonPath + urlB64)
		if err != nil {
			log.Println("open cache file failed", err)
		} else {
			defer fReader.Close()
			if cacheDwon[urlB64+".type"] != "" {
				w.Header().Set("Content-Type", cacheDwon[urlB64+".type"].(string))
			}
			if cacheDwon[urlB64+".encoding"] != "" {
				w.Header().Set("Content-Encoding", cacheDwon[urlB64+".encoding"].(string))
			}
			if cacheDwon[urlB64+".etag"] != "" {
				w.Header().Set("Etag", cacheDwon[urlB64+".etag"].(string))
			}
			w.Header().Set("Via", "rpi cache")
			io.Copy(w, fReader)
			return
		}
	}
	req := buildRequest(rawurl, r)
	if req == nil {
		log.Println("build request failed: wrong url: ", rawurl)
		return
	}
	resp, err := Client.Do(req)
	if err != nil {
		if !strings.Contains(err.Error(), "No Redirect!") {
			return
		}
		err = nil
	}
	defer resp.Body.Close()
	log.Println("...............")
	respHeader, _ := httputil.DumpResponse(resp, false)
	log.Println("\n", string(respHeader))
	via := w.Header().Get("Via")
	if via == "" {
		w.Header().Set("Via", "1.1 rpidown")
	} else {
		w.Header().Set("Via", via+", 1.1 rpidown")
	}

	doCache(urlB64, resp, w, r)

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
	log.Println("build http client")
	buildHTTPClient()
	log.Println("open log file")
	var err error
	CacheLog, err = os.OpenFile(cacheFN, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0660)
	if err != nil {
		log.Println("open log file failed:", err)
		panic(err)
	}
	log.Println("init finish")
}

func main() {
	mux1 := http.NewServeMux()
	mux1.HandleFunc("/", defaultHandler)
	mux1.Handle("/dl/", http.StripPrefix("/dl/", http.FileServer(http.Dir(StaticPath+"/aria2/"))))
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
