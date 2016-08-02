package lib

import (
	"container/ring"
	// "fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

var Tracing *tracing

type tracing struct {
	index           map[string]*ring.Ring
	data            *ring.Ring
	max             int
	Size            int
	max_body_length int
	lk              sync.Mutex
}

type TracingRecord struct {
	Timestamp        string      `json:"@timestamp"`
	RequestId        string      `json:"request_id"`
	DomainId         string      `json:"site"`
	Processed        bool        `json:"called"`
	RemoteAddr       string      `json:"ip"`
	ApiId            string      `json:"api"`
	Prefix           string      `json:"prefix"`
	ApiMethod        string      `json:"method"`
	Eid              string      `json:"eid"`
	AppId            string      `json:"app_id"`
	Key              string      `json:"app_key"`
	Duration         float32     `json:"used"`
	ResponseCode     int         `json:"code"`
	Host             string      `json:"host"`
	Path             string      `json:"path"`
	Query            string      `json:"query"`
	Backend          string      `json:"backend"`
	UserAgent        string      `json:"user-agent"`
	Error            interface{} `json:"error"`
	Debug            interface{} `json:"debug"`
	time             time.Time
	post_values      *url.Values
	response_headers *http.Header
	response_body    *[]byte
}

func (t *TracingRecord) Body() *[]byte {
	return t.response_body
}

func (t *TracingRecord) Headers() *http.Header {
	return t.response_headers
}

func (t *TracingRecord) Post() *url.Values {
	return t.post_values
}

func (t *TracingRecord) SetResponseBody(b []byte) {
	t.response_body = &b
}

func (t *TracingRecord) SetResponseHeader(h http.Header) {
	t.response_headers = &h
}

func (t *TracingRecord) SetPost(v *url.Values) {
	t.post_values = v
}

func (me *TracingRecord) SetTime(t time.Time) {
	me.time = t
	me.Timestamp = t.Format(time.RFC3339)
}

func (t *TracingRecord) Time() time.Time {
	return t.time
}

func init() {
	Tracing = &tracing{}
	Tracing.max = 10240
	Tracing.max_body_length = 4 * 1024
	Tracing.data = ring.New(1)
	Tracing.index = make(map[string]*ring.Ring)
}

func (me *tracing) Push(t *TracingRecord) {
	r := ring.Ring{
		Value: t,
	}
	me.lk.Lock()
	defer me.lk.Unlock()

	if Tracing.Size >= Tracing.max {
		p := me.data.Prev()
		t := p.Value.(*TracingRecord)
		delete(me.index, t.RequestId)
		p.Prev().Unlink(1)
	} else {
		Tracing.Size++
	}
	me.index[t.RequestId] = &r
	me.data.Link(&r)
}

func (me *tracing) Read(id string) (*TracingRecord, bool) {
	r, ok := me.index[id]
	if ok {
		return r.Value.(*TracingRecord), ok
	}
	return nil, false
}

func (me *tracing) Do(f func(interface{})) {
	me.data.Do(f)
}
