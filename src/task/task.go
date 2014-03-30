package task

import (
	"github.com/fzzy/radix/redis"
	"strconv"
	"time"
)

var MaxProcess = 10

type Task struct {
	Client *redis.Client
	Count  int
}

func New() (t *Task, err error) {
	t = &Task{}
	t.Client, err = redis.DialTimeout("tcp", "127.0.0.1:6379", time.Duration(10)*time.Second)
	return
}

func (t *Task) Add(url string) error {
	id := t.Client.Cmd("incr", "id").String()
	err := t.Client.Cmd("rpush", "all", id).Err
	if err != nil {
		return err
	}
	return t.Client.Cmd("set", "task:"+id+":url", url).Err
}

func (t *Task) Get() (url string, id string) {
	if t.Count >= MaxProcess {
		return "", "-1"
	}
	r := t.Client.Cmd("lpop", "all")
	if r.Err != nil {
		return "", "-1"
	}
	id = r.String()
	t.Client.Cmd("rpush", "doing", id)
	t.Count += 1
	url = t.Client.Cmd("get", "task:"+id+":url").String()
	return
}

func (t *Task) Done(id string) error {
	return t.Client.Cmd("rpush", "done", id).Err
}

func (t *Task) Flush() error {
	return t.Client.Cmd("flushall").Err
}
func (t *Task) GetAll() (urls []string) {
	len, _ := t.Client.Cmd("llen", "all").Int()
	for i := 0; i < len; i++ {
		id := t.Client.Cmd("lindex", "all", strconv.Itoa(i)).String()
		urls = append(urls, t.Client.Cmd("get", "task:"+id+":url").String())
	}
	return
}
func (t *Task) GetDoing() (urls []string) {
	len, _ := t.Client.Cmd("llen", "doing").Int()
	for i := 0; i < len; i++ {
		id := t.Client.Cmd("lindex", "doing", strconv.Itoa(i)).String()
		urls = append(urls, t.Client.Cmd("get", "task:"+id+":url").String())
	}
	return
}
func (t *Task) GetDone() (urls []string) {
	len, _ := t.Client.Cmd("llen", "done").Int()
	for i := 0; i < len; i++ {
		id := t.Client.Cmd("lindex", "done", strconv.Itoa(i)).String()
		urls = append(urls, t.Client.Cmd("get", "task:"+id+":url").String())
	}
	return
}
