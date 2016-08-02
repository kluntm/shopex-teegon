package lib

import (
	"bytes"
	"crypto/md5"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	//"git.ishopex.cn/base/metronome"
	"github.com/russross/blackfriday"
	//"gopkg.in/mgo.v2/bson"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"time"
)

var (
	PrismApiPrefix    string
	PrismApiPrefixLen int
	RunMode           *string
)

type GlobalParam struct {
	Name      string `json:"name"`
	ParamType string `json:"param_type"`
	ValueType string `json:"value_type"`
	Format    string `json:"format"`
}

const (
	MQTopicOAuth = "@system.oauth.login"
	// MQTopicApi_Action_New    = "@system.api.new"
	// MQTopicApi_Action_Update = "@system.api.update"
	// MQTopicApi_Action_Delete = "@system.api.delete"
	MQTopicAlert = "@system.log.apierror"
	// MQTopicOperator_Log      = "@system.operator"
	// MQTopicMember_Log        = "@system.member"
)

var Upstream = &http.Transport{
	MaxIdleConnsPerHost: 50,
	//      ResponseHeaderTimeout: 10000,
	DisableKeepAlives: false,
}

const SINGLE_DOMAIN_ID = "default"

func init() {
	PrismApiPrefix = "platform/"
	PrismApiPrefixLen = len(PrismApiPrefix)
}

type ApiResult struct {
	Error  interface{} `json:"error"`
	Result interface{} `json:"result"`
}

func (a ApiResult) Write(rw *http.ResponseWriter) {
	header := (*rw).Header()
	header.Set("Content-Type", "application/json")

	http_code := 200

	if a.Error != nil {
		if reflect.TypeOf(a.Error).String() != "*lib.Error" {
			a.Error = NewError(a.Error)
		}
		http_code = a.Error.(*Error).HttpCode
	}

	(*rw).WriteHeader(http_code)

	jsonbytes, _ := json.MarshalIndent(a, "", "\t")
	(*rw).Write(jsonbytes)
	(*rw).Write([]byte("\n"))
}

var global_cnt int32

func Guid(l int, check func(string) bool, step ...int) string {
	h := md5.New()

	if len(step) < 1 {
		step = append(step, 0)
	}

	atomic.AddInt32(&global_cnt, 1)
	binary.Write(h, binary.LittleEndian, global_cnt)
	binary.Write(h, binary.LittleEndian, step)
	binary.Write(h, binary.LittleEndian, time.Now().UnixNano())

	hname, _ := os.Hostname()
	binary.Write(h, binary.LittleEndian, []byte(hname))

	if global_cnt == 2147483646 {
		atomic.StoreInt32(&global_cnt, 0)
	}

	str := base32.StdEncoding.EncodeToString(h.Sum(nil))
	str = strings.TrimRight(str, "=")

	n := len(str)
	if n < l {
		str = str + Guid(l-n, func(_ string) bool { return true }, 5)
	} else {
		str = str[0:l]
	}
	str = strings.ToLower(str)

	if check(str) {
		return str
	} else {
		return Guid(l, check, step[0]+1)
	}
}

type ClosingBuffer struct {
	*bytes.Buffer
}

func (cb *ClosingBuffer) Close() (err error) {
	return
}

// var RRD *rrd.RRDServer

func init() {
	// RRD = rrd.NewRRDServer()
}

func RendMarkdown(s string, opt int) string {
	extensions := 0
	extensions |= blackfriday.EXTENSION_NO_INTRA_EMPHASIS
	extensions |= blackfriday.EXTENSION_TABLES
	extensions |= blackfriday.EXTENSION_FENCED_CODE
	extensions |= blackfriday.EXTENSION_AUTOLINK
	extensions |= blackfriday.EXTENSION_STRIKETHROUGH
	extensions |= blackfriday.EXTENSION_SPACE_HEADERS

	htmlFlags := 0
	htmlFlags |= blackfriday.HTML_USE_XHTML
	htmlFlags |= opt
	renderer := blackfriday.HtmlRenderer(htmlFlags, "title", "css")

	input := []byte(s)
	return string(blackfriday.Markdown(input, renderer, extensions))
}

func RemoteIp(req *http.Request) string {
	forwarded := req.Header.Get("X-Forwarded-For")
	if len(forwarded) > 0 {
		return strings.Split(forwarded, ",")[0]
	}
	return strings.Split(req.RemoteAddr, ":")[0]
}
