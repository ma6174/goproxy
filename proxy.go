package main

import (
    "fmt"
    //    "io"
    "io/ioutil"
    "net/http"
    "net/url"
)

//"&{GET http://www.qiniu.com/favicon.ico HTTP/1.1 1 1 map[Proxy-Connection:[keep-alive] Proxy-Authorization:[SpdyProxy ps="1394902812-109215405-2169405763-1591551543", sid="b0d98b3204cf59c18363063fe74560b6"] User-Agent:[Mozilla/5.0 (iPad; CPU OS 7_1 like Mac OS X) AppleWebKit/537.51.1 (KHTML, like Gecko) CriOS/33.0.1750.15 Mobile/11D167 Safari/9537.53] Accept-Encoding:[gzip,deflate,sdch] Accept-Language:[zh-CN,zh;q=0.8,en;q=0.6]] 0x104847e0 0 [] false www.qiniu.com map[] map[] <nil> map[] 192.168.10.12:53788 http://www.qiniu.com/favicon.ico <nil>}"

func defaultHandler(w http.ResponseWriter, r *http.Request) {
    fmt.Println("============================")
    fmt.Println(r)
    if r.URL.Scheme == "" {
        r.URL.Scheme = "http"
    }
    if r.Host == "" {
        fmt.Println("No Host")
        return
    }
    if r.URL.RawQuery != "" {
        r.URL.RawQuery = "?" + r.URL.RawQuery
    }
    rawurl := r.URL.Scheme + "://" + r.Host + r.URL.Path + r.URL.RawQuery
    fmt.Println(rawurl)
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

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()
    fmt.Println(resp)
    data, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        fmt.Println(err)
    }
    fmt.Println(len(string(data)))
    for k, v := range resp.Header {
        for _, vv := range v {
            w.Header().Add(k, vv)
        }
    }
    fmt.Println(">>>>", resp.StatusCode)
    w.WriteHeader(resp.StatusCode)
    w.Write(data)
}

func main() {
    http.HandleFunc("/", defaultHandler)
    err := http.ListenAndServe(":7777", nil)
    if err != nil {
        fmt.Println(err)
    }
}

