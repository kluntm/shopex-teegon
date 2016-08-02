package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"git.ishopex.cn/teegon/apigateway/api"
	"git.ishopex.cn/teegon/apigateway/lib"
	"git.ishopex.cn/teegon/apigateway/notify"
	msg "git.ishopex.cn/teegon/mqservice/message"
	"git.ishopex.cn/teegon/mqservice/mq"
	l4g "github.com/alecthomas/log4go"
)

func ExitRecovery() {
	time.Sleep(time.Millisecond * 100)
	if err := recover(); err != nil {
		l4g.Error("runtime error:", err) //这里的err其实就是panic传入的内容，55
		l4g.Error("stack", string(debug.Stack()))
		//l4g.Close() //打印出日志
		//os.Exit(-1) //出现错误主动退出，重启
	}
}

const (
	RetSuccess = 0  // 成功
	RetError   = -1 //失败
)

type ReadMsgResult struct {
	Num      int           `json:"num"`  // 消息数量
	Messages []msg.MsgData `json:"data"` // 消息数量
}

type WriteMsgResult struct {
	Partition int32 `json:"partition"` // 消息数量
	Offset    int64 `json:"offset"`    // 消息数量
}

type Response struct {
	Ecode   int32       `json:"ecode"`             // 错误代码
	Emsg    string      `json:"emsg,omitempty"`    // 错误代码
	Command string      `json:"command,omitempty"` // 命令 主要用于websocket时
	Result  interface{} `json:"result,omitempty"`  // 错误代码
}

type ServiceApp struct {
	bExit       bool
	config      Config //配置信息
	kafka       mq.Kafka
	apiresource *api.ApiResource //所有api资源，服务、方法、应用、调用关系等等
	webnotify   *notify.WebNotify
	allMethod   map[string]func(http.ResponseWriter, *http.Request) //所有提供的api方法
}

func (this *ServiceApp) LoadConfig(config_path string) error {
	if err := this.config.Init(config_path); err != nil {
		l4g.Info("gateway app init, config init failed, error:%s", err.Error())
		return err
	}

	l4g.Info("config:%v", this.config)
	return nil
}

func (this *ServiceApp) Init() error {

	//初始化日志
	l4g.LoadConfiguration(this.config.LogConfigFile)

	this.allMethod = make(map[string]func(http.ResponseWriter, *http.Request))
	this.apiresource = &api.ApiResource{}
	if err := this.apiresource.Init(this.config.DBDns, 0); err != nil {
		l4g.Error("service app init, ApiResource Init failed, error:%s", err.Error())
		return err
	}

	if err := this.kafka.Init(this.config.KafkaConfig, this.config.Redis.Addr, this.config.ZkList); err != nil {
		l4g.Warn("service app init, kafka init failed, error:%s", err.Error())
		return err
	}

	lib.Init()
	return nil
}

func (this *ServiceApp) UnInit() error {

	return nil
}

func (this *ServiceApp) Start() error {
	addr := this.config.ListenIp + ":" + fmt.Sprintf("%d", this.config.HttpPort)
	l4g.Info("mqservice start, local addr:%s.", addr)
	go func() {
		l4g.Info("==============")
		this.allMethod["shopex.queue.create"] = this.QueueCreate
		this.allMethod["shopex.queue.drop"] = this.QueueDrop
		this.allMethod["shopex.queue.write"] = this.QueueWrite
		this.allMethod["shopex.queue.write.otherqueue"] = this.QueueWriteOtherQueue
		this.allMethod["shopex.queue.read"] = this.QueueRead
		this.allMethod["shopex.queue.purge"] = this.QueuePurge
		this.allMethod["shopex.queue.status"] = this.QueueStatus
		this.allMethod["shopex.queue.msgack"] = this.QueueMsgAck
		http.HandleFunc("/message", this.ServeHTTP)
		http.HandleFunc("/message/websocket/", this.NewClient)
		if err := http.ListenAndServe(addr, nil); err != nil {
			l4g.Error("listen addr failed, addr:%s, error:%s", addr, err.Error())
			l4g.Close()
			os.Exit(0)
		}

	}()

	return nil
}

//检查权限，并且检查该appid是否有使用该队列的权限
//因秘钥签名校验在apiGateway处做了，故此处可认为只要app下有该队列即可使用
func (this *ServiceApp) CheckPermission(appkey string, qname string, write bool, read bool) error {
	if app, err := this.apiresource.GetAppInfo(appkey); err == nil {
		if queue, ok := app.GetAppQueue(qname); ok {
			if read && queue.Read == 0 { //请求读权限read=true，但是队列未开放读权限则返回错误
				return errors.New("not read permission.")
			}

			if write && queue.Write == 0 { //请求写权限write=true，但是队列未开放写则返回错误
				return errors.New("not write permission.")
			}
			//检查 读写权限
			return nil
		} else {
			return errors.New("user not permission.")
		}
	} else {
		l4g.Warn("service app, check permission failed, app info not found, appid:%s, queue name:%s", appkey, qname)
		return errors.New("user not permission.")
	}
}

//检查是否有写其他应用下队列的权限，该动作只能服务触发，后端服务向应用下面某个队列写数据
//需要校验应用是否申请使用该服务，应用下面是否有该队列
func (this *ServiceApp) CheckOtherQueuePermission(appkey string, otherappkey string, serviceid string, qname string, write bool, read bool) error {
	if app, err := this.apiresource.GetAppInfo(otherappkey); err == nil {
		//检查被写队列所属的应用是否申请了 该服务, 并且为审核通过才能使用
		if _, check := app.CheckGetAppService(serviceid); !check {
			l4g.Warn("service app, check other queue permission, check app service failed, appid:%s, oterappid:%s, serviceid:%s, qname:%s", appkey, otherappkey, serviceid, qname)
			return errors.New("not permission.")
		}
		//检查是否有队列使用权限
		if queue, ok := app.GetAppQueue(qname); ok {
			if read && queue.Read == 0 { //请求读权限read=true，但是队列未开放读权限则返回错误
				l4g.Warn("service app, check other queue permission, not read permission, appid:%s, oterappid:%s, serviceid:%s, qname:%s", appkey, otherappkey, serviceid, qname)

				return errors.New("not read permission.")
			}

			if write && queue.Write == 0 { //请求写权限write=true，但是队列未开放写则返回错误
				l4g.Warn("service app, check other queue permission, not write permission, appid:%s, oterappid:%s, serviceid:%s, qname:%s", appkey, otherappkey, serviceid, qname)

				return errors.New("not write permission.")
			}
			//检查 读写权限
			return nil
		}
	} else {
		l4g.Warn("service app, check permission failed, app info not found, appid:%s, queue name:%s", appkey, qname)
		return errors.New("user not permission.")
	}

	return errors.New("user not permission.")
}

func (this *ServiceApp) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if err := this.Request(w, req); err != nil {
		writeResponse(w, RetError, err.Error(), nil)
	}
}

func (this *ServiceApp) Request(w http.ResponseWriter, req *http.Request) error {
	request_id := req.Header.Get("X-Request-Id")
	if request_id == "" {
		request_id = lib.Guid(16, func(_ string) bool { return true })
		req.Header.Set("X-Request-Id", request_id)
	}

	remote_ip := lib.RemoteIp(req)
	req.Header.Set("X-Caller-Ip", remote_ip)
	req.Header.Set("X-Proxy-UA", req.Header.Get("User-Agent"))
	h := w.Header()
	h.Set("X-Request-Id", request_id)
	var app_key, method string
	//req_sign := req.FormValue("sign")
	app_key = req.FormValue("app_key")
	method = req.FormValue("method")
	l4g.Info("apiproxy forward, begin request, app_key:%s, method:%s, request_id:%s, remote_ip:%s, host:%s", app_key, method, request_id, remote_ip, req.Host)

	if len(app_key) <= 0 {
		return lib.ClientId_Error
	}
	/*
		t := &lib.TracingRecord{
			ResponseCode: 200,
			RequestId:    request_id,
			Host:         req.Host,
			Path:         req.URL.Path,
			Query:        req.URL.RawQuery,
			UserAgent:    req.Header.Get("User-Agent"),
			RemoteAddr:   remote_ip,
			ApiMethod:    method,
		}
	*/
	//校验签名
	//query := req.URL.Query()
	/*
		post_form := &req.PostForm
		if req.MultipartForm != nil {
			post_form = (*url.Values)(&req.MultipartForm.Value)
		}
	*/
	//sign := lib.Sign(req, appinfo.Secret)
	/*
		if res_err := api.ValidateApiRequest("", this.config.Secret, req, time.Now().Unix(), t); res_err != nil {
			l4g.Warn("forward failed, sign error, key:%s, method:%s, req_sign:%s, url param:%v, post param:%v", app_key, method, req_sign, query, post_form)
			//w.Write([]byte(res_err.Error()))
			return res_err
		}
	*/

	//	var bret bool
	//	//检查传递的appkey 是否有调用指定服务的权限
	//	if _, bret = appinfo.CheckGetAppService(client.ServiceId()); !bret {
	//		l4g.Warn("forward failed, access denied, app can not call the api, key:%s, serviceid:%s, method:%s", app_key, client.ServiceId(), method)
	//		//w.Write([]byte("app key no calls permissions"))
	//		return lib.ApiCall_Permis_Error
	//	}

	if reqmethod, ok := this.allMethod[method]; !ok {
		return lib.Api_Not_Exists
	} else {
		l4g.Warn("reqmethod：%s， %v", method, reqmethod)
		reqmethod(w, req)
	}

	return nil
}

func writeResponse(w http.ResponseWriter, ecode int32, emsg string, result interface{}) error {

	resp := Response{
		Ecode:  ecode,
		Emsg:   emsg,
		Result: result,
	}
	jdata, _ := json.Marshal(resp)
	w.Write(jdata)
	return nil
}

func getReqValue(req *http.Request, name string) string {
	return req.FormValue(name)
}

func getQueueName(topic string, extend string) string {
	return extend + "_" + topic
}

func (this *ServiceApp) QueueCreate(w http.ResponseWriter, req *http.Request) {

	req.ParseForm()
	qname := req.FormValue("qname")
	appid := req.FormValue("app_key")
	if err := this.CheckPermission(appid, qname, true, false); err != nil {
		writeResponse(w, RetError, err.Error(), nil)
		return
	}
	l4g.Info("create queue, app_key:%s, topic:%s", appid, qname)

	if len(appid) > 0 && len(qname) > 0 {
		if err := this.kafka.Create(getQueueName(qname, appid)); err != nil {
			writeResponse(w, RetError, err.Error(), nil)
			return
		}

		writeResponse(w, RetSuccess, "", nil)
		return
	}

	writeResponse(w, RetError, "parameter error", nil)

}

func (this *ServiceApp) QueueMsgAck(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	group := getReqValue(req, "group")
	topic := getReqValue(req, "topic")
	appkey := getReqValue(req, "app_key")
	msgid, _ := strconv.ParseInt(getReqValue(req, "msgid"), 10, 64)
	partition, _ := strconv.Atoi(getReqValue(req, "partition"))
	if err := this.CheckPermission(appkey, topic, false, true); err != nil {
		writeResponse(w, RetError, err.Error(), nil)
		return
	}

	l4g.Info("message ack, app_key:%s, group:%s, topic:%s, msg_id;%s", appkey, group, topic, msgid)

	if err := this.kafka.SetOffset(group, getQueueName(topic, appkey), int32(partition), msgid); err != nil {
		writeResponse(w, RetError, err.Error(), nil)
	} else {
		writeResponse(w, RetSuccess, "", nil)
	}
}

func (this *ServiceApp) QueueDrop(w http.ResponseWriter, req *http.Request) {
	writeResponse(w, RetSuccess, "", nil)
}

func (this *ServiceApp) QueueWrite(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	topic := req.FormValue("topic")
	data := req.FormValue("data")
	key := req.FormValue("key")
	appkey := req.FormValue("app_key")
	if err := this.CheckPermission(appkey, topic, true, false); err != nil {
		writeResponse(w, RetError, err.Error(), nil)
		return
	}
	l4g.Info("write queue, app_key:%s, topic:%s, key;%s", appkey, topic, key)

	p, offset, err := this.kafka.Write(getQueueName(topic, appkey), data, key)
	if err != nil {
		writeResponse(w, RetError, "", nil)
		return
	}

	writeResponse(w, RetSuccess, "", WriteMsgResult{
		Partition: p,
		Offset:    offset,
	})
}

func (this *ServiceApp) QueueWriteOtherQueue(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	topic := req.FormValue("topic")
	data := req.FormValue("data")
	key := req.FormValue("key")
	app_key := req.FormValue("app_key")
	other_appkey := req.FormValue("other_app_key")
	serviceid := req.FormValue("service_id")
	if err := this.CheckOtherQueuePermission(app_key, other_appkey, serviceid, topic, true, false); err != nil {
		writeResponse(w, RetError, err.Error(), nil)
		return
	}

	l4g.Info("write other queue, app_key:%s, other_appkey:%s, serviceid:%s, topic:%s, key;%s", app_key, other_appkey, serviceid, topic, key)

	p, offset, err := this.kafka.Write(getQueueName(topic, other_appkey), data, key)
	if err != nil {
		writeResponse(w, RetError, "", nil)
		return
	}

	writeResponse(w, RetSuccess, "", WriteMsgResult{
		Partition: p,
		Offset:    offset,
	})
}

func (this *ServiceApp) QueueRead(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	appkey := req.FormValue("app_key")
	group := req.FormValue("group")
	topic := req.FormValue("topic")
	if err := this.CheckPermission(appkey, topic, false, true); err != nil {
		writeResponse(w, RetError, err.Error(), nil)
		return
	}

	drop, _ := strconv.ParseBool(req.FormValue("drop"))
	num, _ := strconv.Atoi(req.FormValue("num"))

	l4g.Info("mqservice read, topic:%s, drop:%t, num:%d", topic, drop, num)
	if data, err := this.kafka.Read(group, getQueueName(topic, appkey), drop, num); err != nil {
		l4g.Info("mqservice read error, topic:%s, drop:%t, num:%d", topic, drop, num)
		writeResponse(w, RetError, err.Error(), nil)
		return
	} else {
		l4g.Info("read success, read num:%d, %v", len(*data), data)

		writeResponse(w, RetSuccess, "", ReadMsgResult{
			Num:      len(*data),
			Messages: *data,
		})
	}
}

func (this *ServiceApp) QueuePurge(w http.ResponseWriter, req *http.Request) {

}

func (this *ServiceApp) QueueStatus(w http.ResponseWriter, req *http.Request) {

}

func (app *ServiceApp) NewClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		l4g.Warn("upgrade failed,error:%s", err.Error())
		return
	}

	l4g.Info("new client, remoteaddr:%s", ws.RemoteAddr().String())
	conn := &WsConn{
		ws:  ws,
		app: app,
	}

	//开个go程处理数据
	go conn.process()
}

func (this *ServiceApp) InitWebSiteNotify() error {
	this.webnotify = &notify.WebNotify{}
	raddr := strings.Split(this.config.Redis.Addr, ":")
	port, _ := strconv.Atoi(raddr[1])
	if err := this.webnotify.Init(raddr[0], port); err != nil {
		l4g.Error("service app init, webnotify Init failed, redis addr:%s, error:%s", this.config.Redis.Addr, err.Error())
		this.apiresource.UnInit()
		return err
	}

	//初始化订阅网站通知消息
	nch, err := this.webnotify.Subscribe(this.config.WebNotifyChannel)
	if err != nil {
		l4g.Error("service app init, webnotify Init failed, error:%s", err.Error())
		return err
	}

	go func() {
		defer ExitRecovery()
		for {
			data := <-nch
			if this.bExit {
				break
			}

			l4g.Info("web site notify, channel:%s, message:%s", data.Channel, data.Message)
			//解析通知结构

			event := notify.NotifyEvent{}
			if jerr := json.Unmarshal([]byte(data.Message), &event); jerr == nil {
				l4g.Info("recv website notify, type:%s, from:%s, content:%s", event.Type, event.From, event.Content)
				switch event.Type {
				case "app_queue_update": //app 刷新或者app有更新时，刷新key时通知，  content=>appid
					if app, err := this.apiresource.GetAppInfo(event.Content); err == nil {
						if err := this.apiresource.ReloadAppQueue(app); err != nil {
							l4g.Warn("website notify, realod app queue failed, appid:%s, error:%s", event.Content, err.Error())
						} else {
							l4g.Info("website notify, realod app queue success, appid:%s", event.Content)
						}
					} else {
						l4g.Warn("website notify, realod app queue, get app info failed, appid:%s, error:%s", event.Content, err.Error())
					}
					break
				}
			} else {
				l4g.Warn("recv website notify, parse message failed, err:%s", jerr.Error())
			}

		}
	}()

	return nil
}
