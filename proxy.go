package main

import (
	"errors"
	"io"
	"log"
	"net"
	"os"
	"time"
	//"io/ioutil"
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"net/url"
	"runtime"
	"strings"
)

var ErrNoRedirect = errors.New("No Redirect!")
var DwonPath = "./down/"
var cacheDwon = make(map[string]bool)

func myDial(netw, addr string) (net.Conn, error) {
	deadline := time.Now().Add(25 * time.Second)
	c, err := net.DialTimeout(netw, addr, time.Second*20)
	if err != nil {
		return nil, err
	}
	c.SetDeadline(deadline)
	return c, nil
}

func noRedirect(req *http.Request, via []*http.Request) error {
	return ErrNoRedirect
}

func defaultHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("============================")
	log.Println(r)
	if r.URL.Scheme == "" {
		r.URL.Scheme = "http"
	}
	if r.Host == "" {
		log.Println("No Host")
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
			log.Println(err)
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
	req.Header.Add("X-Log", "Go Proxy Send")
	//	cert, err := tls.LoadX509KeyPair("./ssl-cert-snakeoil.pem",
	//		"./ssl-cert-snakeoil.key")
	//  if err != nil {
	//	  log.Println("Cannot load certificate: [%s]", err)
	//  }
	tr := &http.Transport{
		Dial: myDial,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			//			Certificates:       []tls.Certificate{cert},
		},
	}
	client := &http.Client{
		Transport:     tr,
		CheckRedirect: noRedirect,
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
				log.Println(err)
			} else {
				defer fWiter.Close()
				cacheDwon[urlB64] = true
			}
		}
	}
	mWriter := io.MultiWriter(w, fWiter)
	resp.Header.Add("X-Log", "Go Proxy Recived")
	log.Println(resp)
	log.Println(">>>>", resp.StatusCode)
	for k, v := range resp.Header {
		log.Println(v)
		for _, vv := range v {
			log.Println(vv)
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	writed, err := io.Copy(mWriter, resp.Body)
	if err != nil {
		log.Println(err, resp.ContentLength, writed)
	}
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

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	runtime.GOMAXPROCS(runtime.NumCPU()*2 - 1)
	http.HandleFunc("/", defaultHandler)
	err := http.ListenAndServe(":7777", nil)
	if err != nil {
		log.Println(err)
	}
}
