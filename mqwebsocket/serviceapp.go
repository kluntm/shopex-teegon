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

type WriteMsgResult struct {
	Partition int32 `json:"partition"` // 消息数量
	Offset    int64 `json:"offset"`    // 消息数量
}

type Response struct {
	Ecode  int32       `json:"ecode"`            // 错误代码
	Emsg   string      `json:"emsg"`             // 错误代码
	Result interface{} `json:"result,omitempty"` // 错误代码
}

type ServiceApp struct {
	bExit       bool
	config      Config           //配置信息
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
	//初始化日志
	l4g.LoadConfiguration(this.config.LogConfigFile)
	return nil
}

func (this *ServiceApp) Init() error {

	this.apiresource = &api.ApiResource{}
	if err := this.apiresource.Init(this.config.DBDns, 0); err != nil {
		l4g.Error("service app init, ApiResource Init failed, error:%s", err.Error())
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
		http.HandleFunc(this.config.BasePath, this.NewClient)
		http.HandleFunc("/keep_alive", this.ServeKeepAlive)
		if err := http.ListenAndServe(addr, nil); err != nil {
			l4g.Warn("listen addr failed,addr:%s", addr)
			l4g.Close()
			os.Exit(0)
		}

	}()

	return nil
}

func (this *ServiceApp) ServeKeepAlive(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte("ok!"))
}

//检查权限，并且检查该appid是否有使用该队列的权限
//因秘钥签名校验在apiGateway处做了，故此处可认为只要app下有该队列即可使用
func (this *ServiceApp) CheckPermission(appid string, qname string, write bool, read bool) error {
	if app, err := this.apiresource.GetAppInfo(appid); err == nil {
		if queue, ok := app.GetAppQueue(qname); ok {
			if read && queue.Read == 0 { //请求读权限read=true，但是队列未开放读权限则返回错误
				return errors.New(fmt.Sprintf("not read permission, topic:%s.", qname))
			}

			if write && queue.Write == 0 { //请求写权限write=true，但是队列未开放写则返回错误
				return errors.New(fmt.Sprintf("not write permission, topic:%s.", qname))
			}
			//检查 读写权限
			return nil
		} else {
			return errors.New(fmt.Sprintf("user not permission, topic:%s.", qname))
		}
	} else {
		l4g.Warn("service app, check permission failed, app info not found, appid:%s, queue name:%s", appid, qname)
		return errors.New(fmt.Sprintf("user not permission, topic:%s.", qname))
	}
}

//检查是否有写其他应用下队列的权限，该动作只能服务触发，后端服务向应用下面某个队列写数据
//需要校验应用是否申请使用该服务，应用下面是否有该队列
func (this *ServiceApp) CheckOtherQueuePermission(appid string, otherappid string, serviceid string, qname string, write bool, read bool) error {
	if app, err := this.apiresource.GetAppInfo(otherappid); err == nil {
		//检查被写队列所属的应用是否申请了 该服务, 并且为审核通过才能使用
		if _, check := app.CheckGetAppService(serviceid); !check {
			l4g.Warn("service app, check other queue permission, check app service failed, appid:%s, oterappid:%s, serviceid:%s, qname:%s", appid, otherappid, serviceid, qname)
			return errors.New("not permission.")
		}
		//检查是否有队列使用权限
		if queue, ok := app.GetAppQueue(qname); ok {
			if read && queue.Read == 0 { //请求读权限read=true，但是队列未开放读权限则返回错误
				l4g.Warn("service app, check other queue permission, not read permission, appid:%s, oterappid:%s, serviceid:%s, qname:%s", appid, otherappid, serviceid, qname)

				return errors.New("not read permission.")
			}

			if write && queue.Write == 0 { //请求写权限write=true，但是队列未开放写则返回错误
				l4g.Warn("service app, check other queue permission, not write permission, appid:%s, oterappid:%s, serviceid:%s, qname:%s", appid, otherappid, serviceid, qname)

				return errors.New("not write permission.")
			}
			//检查 读写权限
			return nil
		}
	} else {
		l4g.Warn("service app, check permission failed, app info not found, appid:%s, queue name:%s", appid, qname)
		return errors.New("user not permission.")
	}

	return errors.New("user not permission.")
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
