package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

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

type Response struct {
	Ecode  int32       `json:"ecode"`            // 错误代码
	Emsg   string      `json:"emsg"`             // 错误代码
	Result interface{} `json:"result,omitempty"` // 错误代码
}

type ServiceApp struct {
	bExit     bool
	config    Config //配置信息
	dataQuery DataQuery
	allMethod map[string]func(http.ResponseWriter, *http.Request) //所有提供的api方法

}

func (this *ServiceApp) LoadConfig(config_path string) error {
	if err := this.config.Init(config_path); err != nil {
		l4g.Info("service app init, config init failed, error:%s", err.Error())
		return err
	}

	l4g.Info("config:%v", this.config)
	return nil
}

func (this *ServiceApp) Init() error {

	//初始化日志
	l4g.LoadConfiguration(this.config.LogConfigFile)

	this.allMethod = make(map[string]func(http.ResponseWriter, *http.Request))

	if err := this.dataQuery.Init(&this.config); err != nil {
		l4g.Warn("service app init, dataquery init failed, error:%s", err.Error())
		return err
	}

	return nil
}

func (this *ServiceApp) UnInit() error {

	return nil
}

func (this *ServiceApp) Start() error {
	addr := this.config.ListenIp + ":" + fmt.Sprintf("%d", this.config.HttpPort)
	l4g.Info("service start, local addr:%s.", addr)
	go func() {
		this.allMethod["shopex.query.appqueue"] = this.QueryAppQueue
		this.allMethod["shopex.query.useserviceapp"] = this.QueryUseServiceApp
		http.HandleFunc("/query", this.ServeHTTP)

		http.ListenAndServe(addr, nil)

	}()

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

func (this *ServiceApp) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	method := req.FormValue("method")
	if len(method) <= 0 {
		writeResponse(w, RetError, "parameter error", nil)
		return
	}

	if methodfunc, ok := this.allMethod[method]; ok {
		methodfunc(w, req)
	} else {
		writeResponse(w, RetError, "", nil)
	}
}

func (this *ServiceApp) QueryUseServiceApp(w http.ResponseWriter, req *http.Request) {

	req.ParseForm()
	eid := req.FormValue("user_eid")
	serviceid := req.FormValue("serviceid")
	if len(eid) <= 0 || len(serviceid) <= 0 {
		writeResponse(w, RetError, "parameter error", nil)
		return
	}

	if list_app, err := this.dataQuery.QueryUseServiceApp(eid, serviceid); err == nil {
		result := QueryUseServiceAppResult{
			Num:     len(*list_app),
			AppList: *list_app,
		}

		writeResponse(w, RetSuccess, "", result)
		return
	} else {
		writeResponse(w, RetError, err.Error(), nil)
	}
}

func (this *ServiceApp) QueryAppQueue(w http.ResponseWriter, req *http.Request) {

	req.ParseForm()
	eid := req.FormValue("user_eid")
	appid := req.FormValue("appid")
	if len(eid) <= 0 || len(appid) <= 0 {
		writeResponse(w, RetError, "parameter error", nil)
		return
	}

	if list_queue, err := this.dataQuery.QueryAppQueue(eid, appid); err == nil {
		result := QueryAppQueueResult{
			Num:       len(*list_queue),
			QueueList: *list_queue,
		}

		writeResponse(w, RetSuccess, "", result)
		return
	} else {
		writeResponse(w, RetError, err.Error(), nil)
	}
}
