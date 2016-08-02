package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"git.ishopex.cn/teegon/apigateway/lib"
	"git.ishopex.cn/teegon/apigateway/notify"
	l4g "github.com/alecthomas/log4go"
	uuid "github.com/satori/go.uuid"
	_redis "gopkg.in/redis.v3"
)

const (
	Hashkey = "HttpNotify"
)

type HttpNotify struct {
	Cli             *_redis.Client
	IP              string
	Port            int
	Pass            string
	Db              int64
	lk              sync.Mutex
	cur_request_num int32 //当前在请求的数量
	bExit           bool
}

type RequestParam struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

//json 格式
// {"type":"POST","req_num":30,"req_url":"http://127.0.0.1:8989/","sign_method":"prism", "secret":"qwerty","param":[{"key":"post1","value":"ash"},{"key":"post2","value":"24"}]}
type HttpNotifyEvent struct {
	Method     string         `json:"type,omitempty"`        //请求方法
	Count      int            `json:"req_num,omitempty"`     //请求次数
	Url        string         `json:"req_url,omitempty"`     //请求地址
	Param      []RequestParam `json:"param,omitempty"`       //请求参数
	SignMethod string         `json:"sign_method,omitempty"` //签名方式 未传递则默认以prism方式签名
	Secret     string         `json:"secret,omitempty"`      //签名的秘钥
}

func (this *HttpNotify) Init() {
	this.bExit = false
	this.MainWork()
}

func (this *HttpNotify) UnInit() {
	this.bExit = true
}

func (this *HttpNotify) SetHashValue(key string, value string) error {
	this.lk.Lock()
	defer this.lk.Unlock()
	return this.Cli.HMSet(Hashkey, key, value).Err()

}

func (this *HttpNotify) GetHashKeys() ([]string, error) {
	return this.Cli.HKeys(Hashkey).Result()
}

func (this *HttpNotify) GetHashValues(key string) (string, error) {
	return this.Cli.HGet(Hashkey, key).Result()
}

func (this *HttpNotify) DelHashKey(key string) error {
	return this.Cli.HDel(Hashkey, key).Err()
}

//监视channel
func (this *HttpNotify) Monitor(StartChan <-chan notify.NotifyData) {
	for {
		ndata := <-StartChan
		go this.process_data(ndata)
	}
}

//分析请求
func (this *HttpNotify) process_data(notifydata notify.NotifyData) {
	key := uuid.NewV1().String()

	err := this.SetHashValue(key, notifydata.Message)
	if err != nil {
		l4g.Error("set %s value fail:%s ", key, err)
		return
	}
	this.handle(key)
	return

}

func (this *HttpNotify) MainWork() {
	go this.PrintLog()
	keys, err := this.GetHashKeys()
	if err != nil {
		l4g.Error("get hash keys error:%s", err)
		return
	}

	if len(keys) == 0 {
		l4g.Info("no data in redis")
		return
	}

	sort.Strings(keys)
	for _, v := range keys {
		go this.handle(v) //每个请求开辟一个协程处理
	}
}

func (this *HttpNotify) PrintLog() {
	count := 0
	for {
		if !this.bExit {
			if count%10 == 0 {
				l4g.Info("print log, cur req:%d", this.cur_request_num)
			}
			count = count + 1
			time.Sleep(1 * time.Second)
		}
	}
}

//处理redis上的数据
func (this *HttpNotify) handle(key string) {
	atomic.AddInt32(&this.cur_request_num, 1)
	defer atomic.AddInt32(&this.cur_request_num, -1)

	var tmp HttpNotifyEvent
	data, err := this.GetHashValues(key)
	if err != nil {
		l4g.Info("Get Value failed, key:%s", key)
		return
	}

	l4g.Info("process request, key:%s, data:%s", key, data)
	err = json.Unmarshal([]byte(data), &tmp)
	if err != nil {
		l4g.Error("json Unmarshal failed, key:%s, value:%s, error:%s", key, data, err)
		err := this.DelHashKey(key)
		if err != nil {
			l4g.Error("deleted key failed, key:%s, error:%s", key, err)
		}
		return
	}

	this.requesethandle(key, tmp)
}

//请求（请求错误返回0，请求成功或循环结束返回200）
func (this *HttpNotify) requesethandle(key string, data HttpNotifyEvent) int {
	l4g.Info("process request, key:%s, url:%s, request way:%s, num:%d", key, data.Url, data.Method, data.Count)
	if data.Count <= 0 {
		for i := 0; i < 100; i++ {
			begin := time.Now()
			StatusCode, err := this.Request(data)
			use_time := float32(time.Now().Sub(begin).Seconds() * 1000)
			if StatusCode == 200 {
				l4g.Info("request success, url:%s,request total num:%d,cur num:%d,status:200, time:%.0f ms", data.Url, data.Count, i+1, use_time)
				err := this.DelHashKey(key)
				if err != nil {
					l4g.Error("del key error:%s ", err.Error())
				}
				break
			}

			strErr := ""
			if err != nil {
				strErr = err.Error()
			}
			l4g.Info("reqeust failed, url:%s,request total num:%d,cur num:%d,status:%d, time:%.0f ms, error:%s", data.Url, data.Count, i+1, StatusCode, use_time, strErr)
			time.Sleep(5 * time.Second)
		}
		return 0
	} else {
		//var ret error
		for i := 0; i < data.Count; i++ {
			begin := time.Now()
			StatusCode, err := this.Request(data)
			use_time := float32(time.Now().Sub(begin).Seconds() * 1000)
			if StatusCode == 200 {
				l4g.Info("request success, url:%s,request total num:%d,cur num:%d,status:200, time:%.0f ms ", data.Url, data.Count, i+1, use_time)
				break
			}
			strErr := ""
			if err != nil {
				strErr = err.Error()
			}
			l4g.Info("reqeust failed, url:%s,request total num:%d,cur num:%d,status:%d, time:%.0f ms, error:%s", data.Url, data.Count, i+1, StatusCode, use_time, strErr)
			time.Sleep(5 * time.Second)
		}
		err := this.DelHashKey(key)
		if err != nil {
			l4g.Error("del key error:%s ", err.Error())
			return 0
		}
	}
	return 0
}

func (this *HttpNotify) Request(data HttpNotifyEvent) (int, error) {
	client := http.Client{}

	var req *http.Request
	var query url.Values

	if data.Method == "POST" || data.Method == "PUT" {
		url_param := url.Values{}
		for _, v := range data.Param {
			url_param.Set(v.Key, v.Value)
		}
		url_param.Set("time", fmt.Sprintf("%d", time.Now().Unix()))

		url_param_reader := bytes.NewReader([]byte(url_param.Encode()))

		req, _ = http.NewRequest(
			data.Method,
			data.Url,
			url_param_reader,
		)
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		query = req.URL.Query()
	} else {
		temp_url, err := url.Parse(data.Url)
		if err != nil {
			return 0, err
		}

		query = temp_url.Query()
		for _, v := range data.Param {
			query.Set(v.Key, v.Value)
		}

		query.Set("time", fmt.Sprintf("%d", time.Now().Unix()))

		temp_url.RawQuery = query.Encode()
		req, _ = http.NewRequest(
			data.Method,
			temp_url.String(),
			nil,
		)
	}

	//签名秘钥有值则做签名
	if len(data.Secret) > 0 {
		value := lib.SignFactory.Sign("prism", req, &map[string]string{"secret": data.Secret})

		query.Set("sign", value)
		req.URL.RawQuery = query.Encode()
	}

	//req.Form.Set("sign", value)

	res, err := client.Do(req)
	if err != nil {
		//l4g.Error("request error is %s", err)
		return 0, err
	}

	//l4g.Info("request code:%d", res.StatusCode)

	return res.StatusCode, err
}
