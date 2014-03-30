package task

import (
	. "github.com/stretchr/testify/assert"
	"strconv"
	"testing"
)

func TestTask(t *testing.T) {
	task, err := New()
	Equal(t, err, nil)
	Equal(t, task.Flush(), nil)
	for i := 0; i < 20; i++ {
		Equal(t, task.Add("http://a.com/"+strconv.Itoa(i)), nil)
	}
	for i := 0; i < 10; i++ {
		url, id := task.Get()
		Equal(t, url, "http://a.com/"+strconv.Itoa(i))
		Equal(t, id, strconv.Itoa(i+1))
	}
	url, id := task.Get()
	Equal(t, url, "")
	Equal(t, id, "-1")
}
