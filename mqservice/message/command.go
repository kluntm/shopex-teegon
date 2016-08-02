package message

const (
	SubMessageCmd    = "submessage"    //订阅队列消息
	UnSubMessageCmd  = "unsubmessage"  //取消订阅
	MessageAckCmd    = "messageack"    //消息确认
	HeartBeatCmd     = "heartbeat"     //心跳
	MessageNotifyCmd = "messagenotify" //订阅成功后的数据通知
	WriteMessageCmd  = "writemessage"  //往队列写消息
)

//消息结构
/*
type Message struct {
	Group     string `json:"group"`
	Topic     string `json:"topic"`
	Data      string `json:"data"`
	Msgid     int64  `json:"msg_id"`
	Partition int    `json:"partition"`
}
*/
//消息数据
type MsgData struct {
	Group     string `json:"group,omitempty"`
	Topic     string `json:"topic,omitempty"` // 消息队列
	Data      string `json:"data,omitempty"`  // 消息数据
	Offset    int64  `json:"message_id"`      //偏移量
	Partition int32  `json:"partition"`       //分区
}

type SubMessage struct {
	Group string `json:"group"`
	Topic string `json:"topic"`
}

//消息确认结构
type MsgAck struct {
	Group     string `json:"group"`
	Topic     string `json:"topic"`
	Msgid     int64  `json:"msg_id"`
	Partition int32  `json:"partition"`
}

//写数据的结构
type WriteMessage struct {
	Topic string `json:"topic"`
	Data  string `json:"data"`
	Key   string `json:"key"`
}

//命令结构
type Command struct {
	Cmd  string      `json:"command"`
	Body interface{} `json:"body"`
}

//请求响应的返回
type Response struct {
	Ecode   int32       `json:"ecode"`             // 错误代码
	Emsg    string      `json:"emsg,omitempty"`    // 错误代码
	Command string      `json:"command,omitempty"` // 命令 主要用于websocket时
	Result  interface{} `json:"result,omitempty"`  // 错误代码
}
