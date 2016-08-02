package notify

/*
{
	"type":"api_update"
	"from":"website"
	"content":""
}

type:
	api_update //api有更新，修改保存的时候通知 content=>serviceid
	app_refresh //app 刷新或者app有更新时，刷新key时通知，  content=>appid
	app_delete //app被删除  content=>"{\"appid\":\"2323\", \"appkey\":\"123454\"}"
	service_add //新增服务
	service_update //服务有更新， 服务信息变更时通知， content=>serviceid
	service_delete //服务被删除

*/

import (
	"strconv"
	"time"

	"runtime/debug"

	l4g "github.com/alecthomas/log4go"
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

type WebNotify struct {
	connector *_redis.Client
	bExit     bool
}

func (this *WebNotify) Init(host string, port int) error {
	this.connector = _redis.NewClient(&_redis.Options{
		Addr:     host + ":" + strconv.FormatInt((int64)(port), 10),
		Password: "",
		DB:       0,
	})
	l4g.Info(nil, "Connected to Redis")

	return nil
}

func (this *WebNotify) UnInit() error {
	return nil
}

func (this *WebNotify) Subscribe(channel string) (<-chan NotifyData, error) {
	nch := make(chan NotifyData, 128)
	go func() {
		defer ExitRecovery()
	_RESUBSCRIBE:
		pubsub, err := this.connector.Subscribe(channel)
		if err != nil {
			l4g.Warn("webnotify subscribe failed, channel:%s", channel)
			time.Sleep(2 * time.Second) //订阅失败，则延迟两秒在订阅
			err = pubsub.Ping("")
			goto _RESUBSCRIBE
		}
		defer pubsub.Close()
		for {
			if this.bExit {
				break
			}
			//等待10秒钟，等待消息达到，若无消息，则发送ping
			msgi, err := pubsub.ReceiveTimeout(10 * time.Second)
			if err != nil {
				//l4g.Error(nil, "PubSub error:", err.Error())
				err = pubsub.Ping("")
				if err != nil {
					l4g.Error(nil, "PubSub failure:", err.Error())
					if !this.bExit {
						goto _RESUBSCRIBE //若ping失败则重新订阅
					}
				}
				continue
			}
			switch msg := msgi.(type) {
			case *_redis.Message:
				l4g.Info(nil, "Received", msg.Payload, "on channel", msg.Channel)
				ndata := NotifyData{
					Channel: msg.Channel,
					Message: msg.Payload,
				}
				nch <- ndata
			default:
				l4g.Debug(nil, "Got control message", msg)
			}
		}
	}()

	return nch, nil
}
