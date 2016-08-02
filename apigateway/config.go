package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
)

type RedisConfig struct {
	Host string `json:"Host,omitempty"`
	Port int    `json:"Port,omitempty"`
}

type HttpsConfig struct {
	Port     int    `json:"Port,omitempty"`
	CertFile string `json:"CertFile,omitempty"`
	KeyFile  string `json:"KeyFile,omitempty"`
}

type Config struct {
	RConfig               RedisConfig `json:"Redis,omitempty"`                 //redis 配置信息
	DBDns                 string      `json:"DBDns,omitempty"`                 //db连接信息
	WebNotifyChannel      string      `json:"WebNotifyChannel,omitempty"`      //web 通知的通道 redis Subscribe
	TracingRecordTTL      uint32      `json:"TracingRecordTTL,omitempty"`      // 调试信息存留时间 秒为单位
	SandBox               bool        `json:"SandBox,omitempty"`               //是否沙箱环境，沙箱环境会写日志等
	BackEndRequestTimeout uint32      `json:"BackEndRequestTimeout,omitempty"` //请求后端服务的超时时间 默认10秒
	LogConfigFile         string      `json:"LogConfigFile,omitempty"`         //l4g配置文件
	ListenIp              string      `json:"ListenIp,omitempty"`              //需要绑定的ip
	HttpPort              int         `json:"HttpPort,omitempty"`              //http端口
	Https                 HttpsConfig `json:"Https,omitempty"`                 //https端口
	BasePath              string      `json:"BasePath,omitempty"`              //url基础路径
	MaxConnection         int32       `json:"MaxConnection,omitempty"`         //最大并发连接数
	Daemon                bool        `json:"Daemon,omitempty"`                //
	UdpAddr               string      `json:"UdpCommandAddr,omitempty"`        //接收udp命令的地址
	StatistLog            string      `json:"StatistLog,omitempty"`            //统计信息日志文件
}

func (this *Config) Init(configpath string) error {
	this.RConfig.Host = "127.0.0.1"
	this.RConfig.Port = 6379
	this.DBDns = "root:root@tcp(localhost:3306)/shopex?charset=utf8"
	this.WebNotifyChannel = "website_notify"
	this.TracingRecordTTL = 20 //20秒
	this.SandBox = true
	this.BackEndRequestTimeout = 10
	this.HttpPort = 8001
	this.ListenIp = "0.0.0.0"
	this.LogConfigFile = "./etc/log4go.xml"
	this.MaxConnection = 50000 //默认最大并发数5w
	if len(configpath) <= 0 {
		return errors.New("config file is empty.")
	}
	//读取配置文件内容
	configbuf, oerr := ioutil.ReadFile(configpath)
	if oerr != nil {
		fmt.Println("config init, read config failed, error:%s.", oerr.Error())
		return oerr
	}

	//解析文件内容
	if err := json.Unmarshal(configbuf, this); err != nil {
		fmt.Println("config init, parse config failed, error:%s.", err.Error())
		return err
	}

	return this.checkConfig()
}

func (this *Config) checkConfig() error {
	if len(this.DBDns) <= 0 {
		return errors.New("config error, DBDns not exist.")
	}
	return nil
}
