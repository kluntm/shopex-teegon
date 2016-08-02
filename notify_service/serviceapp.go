package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"git.ishopex.cn/teegon/apigateway/lib"
	"git.ishopex.cn/teegon/apigateway/notify"
	l4g "github.com/alecthomas/log4go"
	_redis "gopkg.in/redis.v3"
)

type RetResponse struct {
	ECode int32  `json:"ecode"`          //
	EMsg  string `json:"emsg,omitempty"` //
}

type ServiceApp struct {
	httpnotify *HttpNotify
	webnotify  *notify.WebNotify
	config     Config //配置信息
}

func (this *ServiceApp) LoadConfig(config_path string) error {
	if err := this.config.Init(config_path); err != nil {
		l4g.Info("NotifyServiceApp init, config init failed, error:%s", err.Error())
		return err
	}

	l4g.Info("config:%v", this.config)
	return nil
}

func (this *ServiceApp) Init() error {
	fmt.Println("service app")

	//初始化日志
	l4g.LoadConfiguration(this.config.LogConfigFile)
	lib.Init()

	if err := this.Start(); err != nil {
		fmt.Printf("NotifyServiceApp init, http_server start failed, error:%s\n", err.Error())
		return err
	}

	if err := this.InitHttpNotify(); err != nil {
		l4g.Error("NotifyServiceApp init, InitHttpNotify failed, error:%s", err.Error())
		return err
	}

	l4g.Info("service app, init success.")
	return nil
}

func (this *ServiceApp) UnInit() error {
	err := this.httpnotify.Cli.Close()
	if err != nil {
		return err
	}
	err = this.webnotify.UnInit()

	return err
}

func (this *ServiceApp) InitHttpNotify() error {
	this.httpnotify = &HttpNotify{
		IP:   this.config.RConfig.Host,
		Port: this.config.RConfig.Port,
	}

	this.httpnotify.Cli = _redis.NewClient(&_redis.Options{
		Addr:     this.httpnotify.IP + ":" + strconv.FormatInt((int64)(this.httpnotify.Port), 10),
		Password: "",
		DB:       0,
	})

	this.httpnotify.Init()
	this.webnotify = &notify.WebNotify{}
	this.webnotify.Init(this.config.RConfig.Host, this.config.RConfig.Port)

	nch, err := this.webnotify.Subscribe(this.config.WebNotifyChannel)
	if err != nil {
		l4g.Error("service app init, httpnotify Init failed, error:%s", err.Error())
		return err
	}

	this.httpnotify.Monitor(nch)
	return nil

}

func (this *ServiceApp) Start() error {
	go func() {
		http.HandleFunc(this.config.BasePath, this.ServeHTTP)
		err := http.ListenAndServe(this.config.ListenIp+":"+fmt.Sprintf("%d", this.config.HttpPort), nil)
		if err != nil {
			l4g.Error("listen failed is:%s", err)
		}
	}()

	l4g.Info("service app, start success.")
	return nil
}

func (this *ServiceApp) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	l4g.Info(" server .... ")
	req.ParseForm()

	var msg RetResponse
	var temp HttpNotifyEvent
	var temp_notifydata notify.NotifyData

	data := req.FormValue("data")
	if data == "" {
		msg = RetResponse{-1, "invaild data"}
	} else {
		err := json.Unmarshal([]byte(data), &temp)
		if err != nil {
			l4g.Error("http request data is invaild:%s", err)
			msg = RetResponse{-1, err.Error()}
		} else {
			temp_notifydata.Message = data
			temp_notifydata.Channel = this.config.WebNotifyChannel

			go this.httpnotify.process_data(temp_notifydata)
			msg = RetResponse{0, ""}
		}
	}

	ret, _ := json.Marshal(msg)
	w.Write(ret)
}
