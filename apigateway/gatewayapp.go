package main

import (
	"container/list"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	ds "git.ishopex.cn/teegon/apigateway/api"
	"git.ishopex.cn/teegon/apigateway/lib"
	"git.ishopex.cn/teegon/apigateway/notify"
	"git.ishopex.cn/teegon/apigateway/plugins"
	l4g "github.com/alecthomas/log4go"
	sjson "github.com/bitly/go-simplejson" // for json get
	_redis "gopkg.in/redis.v3"
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

type GatewayApp struct {
	apiresource   *ds.ApiResource //所有api资源，服务、方法、应用、调用关系等等
	apiproxy      *ApiProxy
	webnotify     *notify.WebNotify
	bExit         bool
	config        Config //配置信息
	HTTPSServeMux *http.ServeMux
	udpConn       *net.UDPConn
	redisclient   *_redis.Client //redis链接
}

func (this *GatewayApp) LoadConfig(config_path string) error {
	fmt.Println("test")
	if err := this.config.Init(config_path); err != nil {
		l4g.Info("gateway app init, config init failed, error:%s", err.Error())
		return err
	}

	l4g.Info("config:%v", this.config)
	return nil
}

func (this *GatewayApp) Init() error {

	//初始化日志

	l4g.LoadConfiguration(this.config.LogConfigFile)
	if len(this.config.StatistLog) > 0 {
		l4g.Info("statist log, file path:%s", this.config.StatistLog)
		//初始化，统计信息日志拦
		slog := NewStatistLog()
		slog.Init(this.config.StatistLog)
		l4g.AddFilter("statistlog", l4g.INFO, *slog)
	}

	this.initSign()
	this.apiresource = &ds.ApiResource{}
	this.HTTPSServeMux = http.NewServeMux()
	if err := this.apiresource.Init(this.config.DBDns, this.config.BackEndRequestTimeout); err != nil {
		l4g.Error("gateway app init, ApiResource Init failed, error:%s", err.Error())
		return err
	}

	l4g.Info("gateway app init, ApiResource Init success.")
	this.apiproxy = &ApiProxy{}
	if err := this.apiproxy.Init(this.apiresource, &this.config); err != nil {
		l4g.Error("gateway app init, apiproxy Init failed, error:%s", err.Error())
		return err
	}
	l4g.Info("gateway app init, apiproxy Init success.")

	if err := this.InitWebSiteNotify(); err != nil {
		l4g.Error("gateway app init, InitWebSiteNotify failed, error:%s", err.Error())
		return err
	}

	this.redisclient = _redis.NewClient(&_redis.Options{
		Addr:     this.config.RConfig.Host + ":" + strconv.FormatInt((int64)(this.config.RConfig.Port), 10),
		Password: "",
		DB:       0,
	})

	return nil
}

func (this *GatewayApp) initSign() {
	lib.Init()
	esosSign := &plugins.EcosSign{}
	ppSign := &plugins.PaipaiSign{}
	spErpSign := &plugins.ShopexErpSign{}
	tbSign := &plugins.TaobaoSign{}
	spSign := &plugins.ShopexSign{}
	lib.SignFactory.Register(esosSign.Name(), esosSign)
	lib.SignFactory.Register(ppSign.Name(), ppSign)
	lib.SignFactory.Register(spErpSign.Name(), spErpSign)
	lib.SignFactory.Register(tbSign.Name(), tbSign)
	lib.SignFactory.Register(spSign.Name(), spSign)
}

func (this *GatewayApp) UnInit() error {
	if this.apiresource != nil {
		this.apiresource.UnInit()
	}

	return nil
}

func (this *GatewayApp) ListenAndServeTLS(addr, certFile, keyFile string, handler http.Handler) error {
	server := &http.Server{Addr: addr, Handler: handler}
	return server.ListenAndServeTLS(certFile, keyFile)
}
func (this *GatewayApp) Start() error {
	go func() {
		http.HandleFunc(this.config.BasePath, this.ServeHTTP)
		http.HandleFunc("/keep_alive", this.ServeKeepAlive)
		addr := this.config.ListenIp + ":" + fmt.Sprintf("%d", this.config.HttpPort)
		if err := http.ListenAndServe(addr, nil); err != nil {
			l4g.Error("listen http addr failed, err:%s", err.Error())
			l4g.Close()
			os.Exit(0)
		}
	}()

	go func() {
		this.HTTPSServeMux.HandleFunc(this.config.BasePath, this.ServeHTTPS)

		err := this.ListenAndServeTLS(this.config.ListenIp+":"+fmt.Sprintf("%d", this.config.Https.Port), this.config.Https.CertFile, this.config.Https.KeyFile, this.HTTPSServeMux)
		if err != nil {
			l4g.Error("ListenAndServe: ", err)
			//return err
		}
	}()

	var err error
	if this.udpConn, err = this.StartUdpService(this.config.UdpAddr); err != nil {
		l4g.Warn("app start, start udp service failed, addr:%s, error:%s", this.config.UdpAddr, err.Error())
	}

	return nil
}

func (this *GatewayApp) ServeHTTPS(w http.ResponseWriter, req *http.Request) {
	this.apiproxy.Forward(w, req, true)
}

func (this *GatewayApp) ServeHTTP(w http.ResponseWriter, req *http.Request) {

	this.apiproxy.Forward(w, req, false)

}

func (this *GatewayApp) ServeKeepAlive(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte("ok!"))
}

func (this *GatewayApp) InitWebSiteNotify() error {
	this.webnotify = &notify.WebNotify{}
	if err := this.webnotify.Init(this.config.RConfig.Host, this.config.RConfig.Port); err != nil {
		l4g.Error("gateway app init, webnotify Init failed, error:%s", err.Error())
		this.apiresource.UnInit()
		return err
	}

	//初始化订阅网站通知消息
	nch, err := this.webnotify.Subscribe(this.config.WebNotifyChannel)
	if err != nil {
		l4g.Error("gateway app init, webnotify Init failed, error:%s", err.Error())
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
				case "api_update": //api有更新，修改保存的时候通知 content=>serviceid
					this.apiresource.ReloadAllService(event.Content)
					break
				case "app_add", "app_update", "app_refresh": //app 刷新或者app有更新时，刷新key时通知，  content=>appid
					this.apiresource.ReloadAllApp(event.Content)
					break
				case "app_delete": //app被删除   content=>"{\"appid\":\"2323\", \"appkey\":\"123454\"}"
					if js, err := sjson.NewJson([]byte(event.Content)); err == nil {
						appid := js.Get("appid").MustString("")
						appkey := js.Get("appkey").MustString("")
						if len(appid) <= 0 || len(appkey) <= 0 {
							l4g.Warn("web site notify, recv app delete event, param error, type:%s, from:%s, appid:%s, appkey:%s", event.Type, event.From, appid, appkey)
							return
						}

						l4g.Warn("web site notify, recv app delete event, type:%s, from:%s, appid:%s, appkey:%s", event.Type, event.From, appid, appkey)

						this.apiresource.DeleteApp(appid, appkey)
					} else {
						l4g.Warn("web site notify, recv app delete event, parse content failed, type:%s, from:%s, content:%s, error:%s", event.Type, event.From, event.Content, err.Error())
					}

					break
				case "service_add", "service_update": //服务有更新， 服务信息变更时通知， content=>serviceid
					method_list_old := this.apiresource.GetApiMethod(event.Content)
					this.apiresource.ReloadAllService(event.Content) //加载数据

					method_list_new := this.apiresource.GetApiMethod(event.Content)
					//先删除redis中旧的方法，在添加新的方法
					this.DeleteMethodToRedis(method_list_old)
					this.WriteMethodToRedis(method_list_new)

					break
				case "service_delete":
					method_list := this.apiresource.GetApiMethod(event.Content)
					this.apiresource.DeleteService(event.Content)

					this.DeleteMethodToRedis(method_list)
					break
				}
			} else {
				l4g.Warn("recv website notify, parse message failed, err:%s", jerr.Error())
			}

		}
	}()

	return nil
}

//将方法写入redis，供其他地方查询
func (this *GatewayApp) WriteMethodToRedis(list *list.List) error {
	for e := list.Front(); e != nil; e = e.Next() {
		method := e.Value.(string)
		this.redisclient.Set(method, "", 0) //将方法名写入redis，以备在网站添加方法时查询用
	}

	return nil
}

//删除redis中指定的方法
func (this *GatewayApp) DeleteMethodToRedis(list *list.List) error {
	for e := list.Front(); e != nil; e = e.Next() {
		method := e.Value.(string)
		this.redisclient.Del(method) //将方法名写入redis，以备在网站添加方法时查询用
	}

	return nil
}

func parseCommand(strCommand string) (string, []string) {
	param := make([]string, 0)
	command := strings.TrimRight(strCommand, "\n")
	if index := strings.Index(command, ":"); index != -1 {
		cmd := command[:index]
		return cmd, strings.Split(command[index:], ":")
	}

	return command, param
}

func (this *GatewayApp) StartUdpService(udp_addr string) (*net.UDPConn, error) {

	//addr := fmt.Sprintf(":%d", serverPort)
	l4g.Info("start udp service, addr:%s", udp_addr)

	udpAddr, err := net.ResolveUDPAddr("udp4", udp_addr)
	if err != nil {
		return nil, err
	}

	c, lerr := net.ListenUDP("udp", udpAddr)
	if lerr != nil {
		return nil, err
	}

	go func(conn *net.UDPConn, app *GatewayApp) {
		for {
			if app.bExit {
				l4g.Info("udp service exit.")
				break
			}

			// 读取数据
			data := make([]byte, 4096)
			rlen, remoteAddr, err := conn.ReadFromUDP(data)
			if err != nil {
				l4g.Warn("read from udp failed, err:%s!", err.Error())
				continue
			}

			senddata := []byte("")
			command := string(data[:rlen])
			command = strings.TrimRight(command, "\n")
			command, param := parseCommand(command)

			switch command {
			case "GET_APP_INFO": //打印app信息
				//this.apiresource.GetAppInfo()
				l4g.Info("get app info command")
				if param != nil && len(param) >= 1 {
					if appInfo, err := this.apiresource.GetAppInfo(param[0]); err == nil {
						str := fmt.Sprintf("app info: appid:%s, appname:%s, key:%s, owneruid:%s, sandbox:%t, status:%d", appInfo.AppId, appInfo.AppName, appInfo.Key, appInfo.OwnerUid, appInfo.SandBox, appInfo.Status)
						senddata = []byte(str)
						l4g.Info("get app info success, appinfo:%s", str)
					} else {
						senddata = []byte(err.Error())
						l4g.Info("get app info failed, error:%s", err.Error())
					}

				} else {
					senddata = []byte("parameter error")
					l4g.Info("get app info failed, parameter error")
				}
			case "GET_APP_SERVICE_INFO": //打印app下服务的信息
				l4g.Info("get app service info command")
				if param != nil && len(param) >= 2 {
					if appInfo, err := this.apiresource.GetAppInfo(param[0]); err == nil {
						if appservice := appInfo.GetAppService(param[1]); appservice != nil {
							str := fmt.Sprintf("app service info: id:%s, appid:%s, serviceid:%s, status:%d", appservice.Id, appservice.AppId, appservice.Serviceid, appservice.Status)
							senddata = []byte(str)
							l4g.Info("get app service info success, appinfo:%s", str)
						} else {
							senddata = []byte("app service not found")
							l4g.Info("get app service info failed, app service not found")
						}
					} else {
						senddata = []byte(err.Error())
						l4g.Info("get app service info failed, error:%s", err.Error())
					}

				} else {
					senddata = []byte("parameter error")
					l4g.Info("get app service info failed, parameter error")
				}
			default:
				l4g.Info("recv unknown udp command, recv:%s", string(data))
				senddata = []byte(fmt.Sprintf("recv unknown udp command, command:%s", string(data)))
			}
			// 发送数据
			_, err = conn.WriteToUDP(senddata, remoteAddr)
			if err != nil {
				l4g.Info("send data failed, to addr:%s, err:%s!", remoteAddr.String(), err.Error())
			}
		}
	}(c, this)

	return c, nil
}
