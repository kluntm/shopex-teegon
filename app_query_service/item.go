package main

type AppInfo struct {
	AppId       string `json:"app_id"`      //appid
	AppName     string `json:"app_name"`    // 应用名字
	Status      int    `json:"status"`      //应用状态 1-可用， 2-被禁用
	Key         string `json:"key"`         //应用的key，
	Description string `json:"description"` //应用的描述
}

type AppQueue struct {
	QueueId     int32  `json:"queue_id"`    //队列id
	QueueName   string `json:"queue_name"`  // 队列名字
	Read        int    `json:"read"`        //是否可读
	Write       int    `json:"write"`       //是否可写，
	Description string `json:"description"` //应用的描述
}

type QueryUseServiceAppResult struct {
	Num     int       `json:"num"`     //结果数量
	AppList []AppInfo `json:"applist"` // 应用信息
}

type QueryAppQueueResult struct {
	Num       int        `json:"num"`       //结果数量
	QueueList []AppQueue `json:"queuelist"` // 队列信息
}
