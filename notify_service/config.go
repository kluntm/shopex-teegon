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
	RConfig          RedisConfig `json:"Redis,omitempty"`            //redis 配置信息
	WebNotifyChannel string      `json:"WebNotifyChannel,omitempty"` //web 通知的通道 redis Subscribe
	LogConfigFile    string      `json:"LogConfigFile,omitempty"`    //l4g配置文件
	ListenIp         string      `json:"ListenIp,omitempty"`         //需要绑定的ip
	HttpPort         int         `json:"HttpPort,omitempty"`         //http端口
	BasePath         string      `json:"BasePath,omitempty"`         //url基础路径
	Daemon           bool        `json:"Daemon,omitempty"`           //
}

func (this *Config) Init(configpath string) error {
	this.RConfig.Host = "127.0.0.1"
	this.RConfig.Port = 6379
	this.WebNotifyChannel = "http_notify"
	this.HttpPort = 8001
	this.ListenIp = "0.0.0.0"
	this.LogConfigFile = "./etc/log4go.xml"
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

	return nil
}
