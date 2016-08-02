package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.ishopex.cn/teegon/apigateway/api"
	"git.ishopex.cn/teegon/apigateway/lib"
	msg "git.ishopex.cn/teegon/mqservice/message"
	l4g "github.com/alecthomas/log4go"
	"github.com/gorilla/websocket"
	gwebsocket "golang.org/x/net/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
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
	backConn   *gwebsocket.Conn
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
	}()

	conn.lastTime = time.Now()
	conn.ws.SetReadLimit(maxMessageSize)
	conn.ws.SetReadDeadline(time.Now().Add(firstMsgWait)) //连接后3秒没消息则断开连接

	for {
		_, message, err := conn.ws.ReadMessage()
		if err != nil {
			l4g.Warn("read msg client:%s, error:%s", conn.remoteAddr, err.Error())
			break
		}

		conn.ws.SetReadDeadline(time.Now().Add(pongWait))
		strMessage := string(message)
		reqMap, _ := url.ParseQuery(strMessage)
		appkey := reqMap.Get("app_key")
		req_sign := reqMap.Get("sign")
		topic := reqMap.Get("topic")
		reqdata := reqMap.Get("body")
		cmd, cerr := conn.ParseCommand(reqdata)
		if cerr != nil {
			l4g.Warn("recv client request, %s, client:%s, msg:%s", cerr.Error(), conn.remoteAddr, strMessage)
			conn.ErrorResponse(RetError, nil, cerr.Error())
			return
		}
		l4g.Info("recv client command, client:%s, cmd:%s ", conn.remoteAddr, cmd.Cmd)

		//先校验签名, 此处签名校验的secret需要使用app的secret
		l4g.Info("api resource :%v", conn.app.apiresource)
		if appinfo, err := conn.app.apiresource.GetAppInfo(appkey); err != nil {
			l4g.Warn("recv client request, not found appinfo, client:%s, appid:%s, cmd:%s, topic:%s", conn.remoteAddr, appkey, reqdata, topic)
			conn.ErrorResponse(RetError, cmd, "app_key error.")
			return
		} else {
			req := &http.Request{URL: &url.URL{}}
			req.URL.RawQuery = strMessage
			conn.lastTime = time.Now()
			t := lib.TracingRecord{}
			if res_err := api.ValidateApiRequest(appinfo.Key, appinfo.Secret, req, time.Now().Unix(), &t); res_err != nil {
				l4g.Warn("wsconn check request param, sign error, command:%s, req_sign:%s, param:%s", cmd.Cmd, req_sign, strMessage)
				conn.ErrorResponse(RetError, cmd, res_err.Error())
				return
			}

		}

		//未创建后端连接，则创建
		if conn.backConn == nil {
			if err := conn.CreateBackConn(); err != nil {
				conn.ErrorResponse(RetError, cmd, err.Error())
				return
			}

			go conn.processChanData()
		}

		switch cmd.Cmd {
		case msg.SubMessageCmd: //订阅队列消息
			//			subcmd, ok := cmd.Body.(msg.SubMessage)
			//			if !ok {
			//				l4g.Info("submessage error, body to submessage failed, client:%s, cmd:%s, cmd:%v", conn.remoteAddr, cmd.Cmd, cmd)
			//				conn.ErrorResponse(RetError, "parameter error")
			//				continue
			//			}

			//			if err := conn.CheckQueuePermission(appkey, subcmd.Topic); err != nil {
			//				l4g.Warn("wsconn check permission, permission error, command:%s, group:%s, topic:%s, req_sign:%s", cmd.Cmd, subcmd.Group, subcmd.Topic, req_sign)
			//				conn.ErrorResponse(RetError, "permission error")
			//				return //权限错误直接断掉连接
			//			}

			if subcmd, err := conn.OnSubMessage(appkey, cmd); err != nil {
				l4g.Info("submessage failed, %s, command:%s, app_key:%s, client:%s, cmd:%v", err.Error(), cmd.Cmd, appkey, conn.remoteAddr, cmd)

				conn.ErrorResponse(RetError, cmd, err.Error())
				return
			} else {
				l4g.Info("submessage success, command:%s, app_key:%s, group:%s, topic:%s, req_sign:%s", cmd.Cmd, appkey, subcmd.Group, subcmd.Topic, req_sign)
			}
		case msg.WriteMessageCmd: //队列写数据
			if err := conn.OnWriteMessage(appkey, cmd); err != nil {
				conn.ErrorResponse(RetError, cmd, err.Error())
				continue
			}
		case msg.HeartBeatCmd:
			l4g.Info("recv heartbeat, client:%s", conn.remoteAddr)
		case msg.UnSubMessageCmd:
			l4g.Info("unsubmessage success, client:%s", conn.remoteAddr)
		case msg.MessageAckCmd:
			l4g.Info("message ack success, client:%s", conn.remoteAddr)
		}

		conn.backConn.Write(message)
	}
}
func (conn *WsConn) CheckQueuePermission(app_key string, topic string) error {
	if err := conn.app.CheckPermission(app_key, topic, true, true); err != nil {
		//conn.ErrorResponse(RetError, err.Error())
		return err
	}

	return nil
}

func (conn *WsConn) OnWriteMessage(app_key string, cmd *msg.Command) error {
	writedata := &msg.WriteMessage{}
	jdata, _ := json.Marshal(cmd.Body)

	if err := json.Unmarshal(jdata, writedata); err == nil {
		l4g.Info("recv writemessage command, client:%s, app_key:%s, topic:%s", conn.remoteAddr, app_key, writedata.Topic)

		if cerr := conn.CheckQueuePermission(app_key, writedata.Topic); cerr != nil {
			l4g.Info("recv writemessage command, %s, client:%s, app_key:%s, topic:%s", cerr.Error(), conn.remoteAddr, app_key, writedata.Topic)
			return cerr
		}
	} else {
		l4g.Info("recv writemessage command, unmarshal body failed, client:%s, app_key:%s", conn.remoteAddr, app_key)

		return err
	}

	return nil
}

func (conn *WsConn) OnSubMessage(app_key string, cmd *msg.Command) (*msg.SubMessage, error) {
	//	mapSub, ok := cmd.Body.(map[string]interface{})
	//	l4g.Warn("type:======%v", reflect.ValueOf(cmd.Body))
	//	if !ok {
	//		return nil, errors.New("body to submessage failed")
	//	}
	subcmd := &msg.SubMessage{}
	jdata, _ := json.Marshal(cmd.Body)

	l4g.Warn("map to json:======%s", string(jdata))

	if err := json.Unmarshal(jdata, subcmd); err == nil {
		return subcmd, conn.CheckQueuePermission(app_key, subcmd.Topic)
	} else {
		return nil, err
	}

	//	if err := json.Unmarshal([]byte(strSub), subcmd); err == nil {
	//		return subcmd, conn.CheckQueuePermission(app_key, subcmd.Topic)
	//	} else {
	//		return nil, err
	//	}
}

func (conn *WsConn) CreateBackConn() error {
	l4g.Info("connect back service, url:%s", conn.app.config.BackWebsocketAddr)
	if ws, err := gwebsocket.Dial(conn.app.config.BackWebsocketAddr, "", conn.app.config.BackWebsocketOrigin); err != nil {
		l4g.Warn("create back conn failed, error:%s", err.Error())
		return err
	} else {
		conn.backConn = ws
	}

	return nil
}

func (conn *WsConn) processChanData() {
	l4g.Info("run processChanData")
	defer l4g.Info("run processChanData end")
	message := make([]byte, 4096)
	for {
		rlen, err := conn.backConn.Read(message)
		if err != nil {
			l4g.Warn("back conn read msg error, client:%s, error:%s", conn.remoteAddr, err.Error())
			conn.ws.Close()
			break
		}

		l4g.Info("recv message:%s", string(message))

		conn.ws.WriteMessage(websocket.TextMessage, message[:rlen])
	}
}
