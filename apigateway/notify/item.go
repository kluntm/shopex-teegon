package notify

type NotifyData struct {
	Channel string //通道
	Message string //数据
}

/**
Gateway跟外部的交互通知
type:
	api_update //api有更新，修改保存的时候通知 content=>serviceid
	app_refresh //app 刷新或者app有更新时，刷新key时通知，  content=>appid
	service_update //服务有更新， 服务信息变更时通知， content=>serviceid
*/
type NotifyEvent struct {
	Type    string `json:"type,omitempty"`    //配置项的名字
	From    string `json:"from,omitempty"`    //配置项的名字
	Content string `json:"content,omitempty"` //配置项的名字
}
