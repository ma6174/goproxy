package main

import (
	"bufio"
	"encoding/base64"
	"errors"
	"html/template"
	"io"
	"io/ioutil"
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
	cacheDwon     = make(map[string]bool)
	ADList        = make(map[string]bool)
	Client        *http.Client
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
				delete(cacheDwon, urlB64)
				os.Remove(DwonPath + urlB64)
				os.Remove(DwonPath + urlB64 + ".info")
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
				delete(cacheDwon, urlB64)
				os.Remove(DwonPath + urlB64)
				os.Remove(DwonPath + urlB64 + ".info")
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
			delete(cacheDwon, urlB64)
			os.Remove(DwonPath + urlB64)
			os.Remove(DwonPath + urlB64 + ".info")
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
		fInfo, err := os.Create(DwonPath + urlB64 + ".info")
		if err != nil {
			log.Println("create cache encoding file failed:", err)
		}
		contentEncoding := resp.Header.Get("Content-Encoding")
		if contentEncoding != "" {
			fInfo.WriteString("Content-Encoding:" + contentEncoding + "\n")
		}
		contentType := resp.Header.Get("Content-Type")
		if contentType != "" {
			fInfo.WriteString("Content-Type:" + contentType + "\n")
		}
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
		cacheDwon[urlB64] = true
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
		fReader, err := os.Open(DwonPath + urlB64)
		if err != nil {
			log.Println("open cache file failed", err)
		} else {
			defer fReader.Close()
			finfo, err := os.Open(DwonPath + urlB64 + ".info")
			if err == nil {
				info, err := ioutil.ReadAll(finfo)
				if err != nil {
					log.Println("read data from cache file failed:", err)
				} else {
					sp := strings.Split(string(info), "\n")
					for _, line := range sp[:len(sp)-1] {
						spl := strings.Split(line, ":")
						w.Header().Set(spl[0], spl[1])
					}
				}
				finfo.Close()
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
