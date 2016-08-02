package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	//"git.ishopex.cn/teegon/apigateway/api"
	//"git.ishopex.cn/teegon/apigateway/lib"
	msg "git.ishopex.cn/teegon/mqservice/message"
	l4g "github.com/alecthomas/log4go"
	"github.com/gorilla/websocket"

	//"os"
	//"path/filepath"
	"strings"
	"time"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	//连接上后3秒不发消息则断开连接
	firstMsgWait = 3 * time.Second

	// Maximum message size allowed from peer.
	maxMessageSize = 1024

	CommandSucceed = 0
	CommandError   = -1
)

type WsConn struct {
	ws         *websocket.Conn
	app        *ServiceApp
	remoteAddr string
	bStop      bool
	lastTime   time.Time //最后接收数据的时间
	group      string
	topic      string
	appkey     string
}

func spliteReq(req string) map[string]string {
	reqArray := strings.Split(req, "&")

	reqmap := make(map[string]string)
	for _, key := range reqArray {
		keyArray := strings.Split(key, "=")
		if len(keyArray) >= 2 {
			reqmap[keyArray[0]] = keyArray[1]
		}
	}

	return reqmap
}

func (conn *WsConn) SendMsg(messageType int, msg []byte) error {
	//conn.ws.SetWriteDeadline(time.Now().Add(5)) //5秒超时

	err := conn.ws.WriteMessage(messageType, msg)
	if err != nil {
		l4g.Warn("send msg failed, toaddr:%s, error:%s", conn.remoteAddr, err.Error())
	}

	return err
}

func (conn *WsConn) ErrorResponse(code int32, cmd *msg.Command, emsg string) {
	strCmd := ""
	if cmd != nil {
		strCmd = cmd.Cmd
	}

	resp := msg.Response{
		Ecode:   code,
		Command: strCmd,
		Emsg:    emsg,
	}
	jdata, _ := json.Marshal(resp)

	unresp := msg.Response{}
	err := json.Unmarshal(jdata, &unresp)
	l4g.Info("Unmarshal:%v, error:%v", unresp, err)
	conn.SendMsg(websocket.TextMessage, jdata)
	return
}

//成功的响应
func (conn *WsConn) WriteResponse(code int32, cmd *msg.Command, result interface{}) {
	strCmd := ""
	if cmd != nil {
		strCmd = cmd.Cmd
	}

	resp := msg.Response{
		Ecode:   code,
		Command: strCmd,
		Result:  result,
	}
	jdata, _ := json.Marshal(resp)

	conn.SendMsg(websocket.TextMessage, jdata)
	return
}

func (conn *WsConn) ParseCommand(data string) (*msg.Command, error) {
	if len(data) <= 0 {
		return nil, errors.New("parse command failed, parameter error")
	}

	//	base64.NewDecoder()
	if jcmd, err := base64.URLEncoding.DecodeString(data); err == nil {
		cmd := &msg.Command{}
		if err := json.Unmarshal(jcmd, cmd); err != nil {
			return nil, err
		}

		return cmd, nil
	} else {
		return nil, err
	}
}
func (conn *WsConn) process() {
	conn.bStop = false
	conn.remoteAddr = conn.ws.RemoteAddr().String()

	defer func() {
		conn.bStop = true
		conn.ws.Close()
		conn.OnUnSubMessage()
	}()

	conn.lastTime = time.Now()
	conn.ws.SetReadLimit(maxMessageSize)
	conn.ws.SetReadDeadline(time.Now().Add(firstMsgWait)) //连接后3秒没消息则断开连接

	for {
		_, message, err := conn.ws.ReadMessage()
		if err != nil {
			l4g.Warn("read msg client:%s, group:%s, topic:%s, error:%s", conn.remoteAddr, conn.group, conn.topic, err.Error())
			break
		}

		conn.ws.SetReadDeadline(time.Now().Add(pongWait))
		strMessage := string(message)
		reqMap, _ := url.ParseQuery(strMessage)
		app_key := reqMap.Get("app_key")
		//req_sign := reqMap.Get("sign")
		//topic := reqMap.Get("topic")
		reqCmd := reqMap.Get("body")
		cmd, cerr := conn.ParseCommand(reqCmd)
		if cerr != nil {
			l4g.Warn("recv client request, %s, client:%s, msg:%s", err.Error(), conn.remoteAddr, strMessage)
			conn.ErrorResponse(RetError, nil, err.Error())
			return
		}
		l4g.Info("recv client command, client:%s, cmd:%s ", conn.remoteAddr, cmd.Cmd)

		//先校验签名, 此处签名校验的secret需要使用app的secret
		//因为websocket的调用时用户直接调用mqservice， 其他的http调用均通过apiGateway转发
		/*		if appinfo, err := conn.app.apiresource.GetAppInfo(appid); err != nil {
					l4g.Warn("recv client request, not found appinfo, client:%s, appid:%s, cmd:%s, topic:%s", conn.remoteAddr, appid, reqCmd, topic)
					conn.ErrorResponse(RetError, "app_key error.")
					return
				} else {
					req := &http.Request{URL: &url.URL{}}
					req.URL.RawQuery = strMessage
					conn.lastTime = time.Now()
					t := lib.TracingRecord{}
					if res_err := api.ValidateApiRequest(appinfo.Key, appinfo.Secret, req, time.Now().Unix(), &t); res_err != nil {
						l4g.Warn("wsconn check request param, sign error, command:%s, req_sign:%s, param:%s", reqCmd, req_sign, strMessage)
						conn.ErrorResponse(RetError, res_err.Error())
						return
					}

					if err := conn.app.CheckPermission(appid, topic, false, true); err != nil {
						conn.ErrorResponse(RetError, err.Error())
						return
					}
				}
		*/
		switch cmd.Cmd {
		case msg.SubMessageCmd: //订阅队列消息
			if err := conn.OnSubMessage(app_key, cmd); err != nil {
				conn.ErrorResponse(RetError, cmd, err.Error())
			}
		case msg.HeartBeatCmd:
			l4g.Info("recv heartbeat, client:%s, group:%s, topic:%s", conn.remoteAddr, conn.group, conn.topic)
		case msg.UnSubMessageCmd:
			if err := conn.OnUnSubMessage(); err != nil {
				conn.ErrorResponse(RetError, cmd, err.Error())
			}
		case msg.MessageAckCmd:
			if err := conn.OnMessageAck(app_key, cmd); err != nil {
				conn.ErrorResponse(RetError, cmd, err.Error())
			}
		case msg.WriteMessageCmd: //队列写数据
			if p, offset, err := conn.OnWriteMessage(app_key, cmd); err != nil {
				conn.ErrorResponse(RetError, cmd, err.Error())
				continue
			} else {

				conn.WriteResponse(RetSuccess, cmd, WriteMsgResult{
					Partition: p,
					Offset:    offset,
				})
			}
		}
	}
}

func (conn *WsConn) OnWriteMessage(app_key string, cmd *msg.Command) (partition int32, offset int64, err error) {
	writedata := &msg.WriteMessage{}
	jdata, _ := json.Marshal(cmd.Body)

	if err := json.Unmarshal(jdata, writedata); err == nil {
		l4g.Info("recv writemessage command, client:%s, app_key:%s, topic:%s, key:%s", conn.remoteAddr, app_key, writedata.Topic, writedata.Key)

		p, offset, werr := conn.app.kafka.Write(writedata.Topic, writedata.Data, writedata.Key)
		if werr != nil {
			l4g.Info("recv writemessage command, write failed, client:%s, app_key:%s, topic:%s, key:%s, error:%s", conn.remoteAddr, app_key, writedata.Topic, writedata.Key, werr.Error())
			return 0, 0, werr
		}

		l4g.Info("recv writemessage command, write success, client:%s, app_key:%s, topic:%s, key:%s, partition:%d, offset:%d", conn.remoteAddr, app_key, writedata.Topic, writedata.Key, p, offset)
		return p, offset, werr
	} else {
		l4g.Info("recv writemessage command, unmarshal body failed, client:%s, app_key:%s", conn.remoteAddr, app_key)

		return 0, 0, err
	}
}

func (conn *WsConn) OnMessageAck(app_key string, cmd *msg.Command) error {
	msgAck := &msg.MsgAck{}
	jdata, _ := json.Marshal(cmd.Body)

	err := json.Unmarshal(jdata, msgAck)
	if err != nil {
		l4g.Info("recv message ack, unmarshal body failed, client:%s, app_key:%s", conn.remoteAddr, app_key)

		return err
	}

	l4g.Info("recv message ack, client:%s, app_key:%s, group:%s, topic:%s, partition:%d, msgid:%d", conn.remoteAddr, app_key, msgAck.Group, msgAck.Topic, msgAck.Partition, msgAck.Msgid)

	return conn.app.kafka.SetOffset(msgAck.Group, getQueueName(msgAck.Topic, app_key), msgAck.Partition, msgAck.Msgid)
}

func (conn *WsConn) OnSubMessage(app_key string, cmd *msg.Command) error {
	subcmd := &msg.SubMessage{}
	jdata, _ := json.Marshal(cmd.Body)

	err := json.Unmarshal(jdata, subcmd)
	if err != nil {
		l4g.Info("recv submessage, unmarshal body failed, client:%s, app_key:%s", conn.remoteAddr, app_key)
		return err
	}
	l4g.Info("recv submessage, client:%s, app_key:%s, group:%s, topic:%s", conn.remoteAddr, app_key, subcmd.Group, subcmd.Topic)

	if len(conn.topic) > 0 {
		l4g.Info("recv submessage, has subscribed other queue, client:%s, app_key:%s, group:%s, topic:%s", conn.remoteAddr, app_key, subcmd.Group, subcmd.Topic)

		return errors.New(fmt.Sprintf("has subscribed to %s.", conn.group))
	}

	conn.group = subcmd.Group
	conn.topic = subcmd.Topic
	conn.appkey = app_key
	if len(conn.topic) <= 0 {
		l4g.Info("recv submessage, topic is empty, client:%s, app_key:%s, group:%s, topic:%s", conn.remoteAddr, app_key, subcmd.Group, subcmd.Topic)

		return errors.New("topic is empty.")
	}

	if len(conn.appkey) <= 0 {
		l4g.Info("recv submessage, appkey is empty, client:%s, app_key:%s, group:%s, topic:%s", conn.remoteAddr, app_key, subcmd.Group, subcmd.Topic)

		return errors.New("appkey is empty.")
	}

	ch, err := conn.app.kafka.Subscribe(conn.group, getQueueName(conn.topic, conn.appkey))
	if err != nil {
		l4g.Info("recv submessage, subscrib failed, client:%s, app_key:%s, group:%s, topic:%s, error:%s", conn.remoteAddr, app_key, subcmd.Group, subcmd.Topic, err.Error())

		return err
	}

	go conn.processChanData(ch)
	return nil
}

func (conn *WsConn) OnUnSubMessage() error {
	if len(conn.topic) <= 0 { //topic有值则取消订阅
		return nil
	}

	l4g.Info("OnUnSubMessage success, group:%s, topic:%s", conn.group, conn.topic)
	if err := conn.app.kafka.UnSubscribe(conn.group, getQueueName(conn.topic, conn.appkey)); err != nil {
		l4g.Warn("unsub message, failed")
		return err
	}

	conn.topic = ""
	return nil
}

func (conn *WsConn) processChanData(ch <-chan *msg.MsgData) {
	l4g.Info("run processChanData, group:%s, topic:%s", conn.group, conn.topic)
	defer l4g.Info("run processChanData end, group:%s, topic:%s", conn.group, conn.topic)
	for {
		select {
		case read_msg := <-ch:
			l4g.Debug("read stream:", read_msg)
			if read_msg == nil {
				return
			}
			//l4g.Debug("read stream:%s", read_msg)
			msgtype := websocket.TextMessage

			resp := Response{
				Ecode:   0,
				Command: msg.MessageNotifyCmd,
				Result: ReadMsgResult{
					Num:      1,
					Messages: []msg.MsgData{*read_msg},
				},
			}
			/*
				resp := ReadMsgResult{
					Num:      1,
					Messages: []msg.MsgData{*read_msg},
				}
			*/
			jdata, _ := json.Marshal(resp)

			err := conn.ws.WriteMessage(msgtype, jdata)
			if err != nil {
				l4g.Warn("send stream failed, group:%s, topic:%s, error:%s", conn.group, conn.topic, err.Error())
				conn.ws.Close()
				return
			}
		}
	}
}
