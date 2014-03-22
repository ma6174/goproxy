package main

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	//	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

var ErrNoRedirect = errors.New("No Redirect!")
var DwonPath = "./down/"
var cacheDwon = make(map[string]bool)

func defaultHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("============================")
	dump, err := httputil.DumpRequest(r, true)
	if err != nil {
		log.Println("42", err)
		return
	}
	log.Println(string(dump))
	log.Println("------------------")
	if r.URL.Scheme == "" {
		r.URL.Scheme = "http"
	}
	if r.Host == "" {
		log.Println("No Host")
		return
	}
	if r.Header.Get("X-Proxy") == "rpi" {
		log.Println("No Local Loop!")
		w.WriteHeader(400)
		return
	}
	if r.URL.RawQuery != "" {
		r.URL.RawQuery = "?" + r.URL.RawQuery
	}
	rawurl := r.URL.Scheme + "://" + r.Host + r.URL.Path + r.URL.RawQuery
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
	log.Println(rawurl)
	rurl, err := url.Parse(rawurl)
	if err != nil {
		panic(err)
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
	req.Header.Add("X-Proxy", "rpi")
	//	cert, err := tls.LoadX509KeyPair("./ssl-cert-snakeoil.pem",
	//		"./ssl-cert-snakeoil.key")
	//  if err != nil {
	//	  log.Println("Cannot load certificate: [%s]", err)
	//  }
	tr := &http.Transport{
		Dial: func(netw, addr string) (net.Conn, error) {
			deadline := time.Now().Add(25 * time.Second)
			c, err := net.DialTimeout(netw, addr, time.Second*20)
			if err != nil {
				return nil, err
			}
			c.SetDeadline(deadline)
			return c, nil
		},

		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			//Certificates:       []tls.Certificate{cert},
		},
	}
	client := &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return ErrNoRedirect
		},
	}
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
	log.Println("------>>>", contentType, rurl)
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
	log.Println("128", resp)
	log.Println("129>>>>", resp.StatusCode)
	log.Println("130>>>>", resp.Header)
	for k, v := range resp.Header {
		for _, vv := range v {
			log.Println("133", k, vv)
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	writed, err := io.Copy(mWriter, resp.Body)
	if err != nil {
		log.Println("140", err, resp.ContentLength, writed)
	}
	log.Println("142---", writed)
	return
	/*
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Println(err)
			return
		}
		log.Println(len(string(data)))
		w.Write(data)
	*/
}

func httpsHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("============================")
	log.Println(r)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	runtime.GOMAXPROCS(runtime.NumCPU()*2 - 1)

	mux1 := http.NewServeMux()
	mux1.HandleFunc("/", defaultHandler)
	s := &http.Server{
		Addr:           ":7080",
		Handler:        mux1,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	//	go s.ListenAndServe()
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/", httpsHandler)
	s2 := &http.Server{
		Addr:           ":7443",
		Handler:        mux2,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	log.Println(s)
	log.Println(s2)
	log.Fatal(s2.ListenAndServeTLS("public.pem", "private.pem"))
}
