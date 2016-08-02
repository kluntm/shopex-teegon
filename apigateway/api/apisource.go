package api

import (
	"container/list"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"git.ishopex.cn/teegon/apigateway/lib"
	l4g "github.com/alecthomas/log4go"
	_ "github.com/go-sql-driver/mysql"
)

type ApiUserInfo struct {
	Key    string
	Secret string
}

//api服务
type ApiService struct {
	ServiceId   string `json:"fd_serviceid"`   //服务的id
	ServiceName string `json:"fd_name"`        //服务的名字
	OwnerUid    string `json:"fd_uid"`         //服务属主
	Status      int    `json:"fd_status"`      //服务状态
	Description string `json:"fd_description"` //服务描述
	Config      string `json:"fd_config"`      //服务接口配置 使用swagger格式  http://swagger.io/
	Visible     int    `json:"fd_visible"`     //是否全平台可见（0-不可见 1-可见  默认为0）
	apiClient   *ApiClient
}

//应用跟服务的对应关系
type ApiApp struct {
	AppId            string                    `json:"fd_id"`      //appid
	OwnerUid         string                    `json:"fd_uid"`     // 应用创建者的uid
	AppName          string                    `json:"fd_name"`    // 应用名字
	Status           int                       `json:"fd_status"`  //应用状态 1-可用， 2-被禁用
	Key              string                    `json:"fd_key"`     //应用的key，
	Secret           string                    `json:"fd_secret"`  //应用的secret
	SandBox          int                       `json:"fd_sandbox"` //是否为沙箱测试app 1-为沙箱测试app 系统初始化时自动创建， 0-正常用户app
	app_service      map[string]*ApiAppService //app下面申请的所有服务
	appservice_mutex sync.RWMutex              //app_service 的读写锁
	app_queue        map[string]*ApiAppQueue   // app下面队列信息
	appqueue_mutex   sync.RWMutex              //app_queue 的读写锁
}

// 应用申请使用的服务列表
type ApiAppService struct {
	Id             int               `json:"fd_id"`              //t_app_service主键
	AppId          string            `json:"fd_app_id"`          // 所属的应用id
	Serviceid      string            `json:"fd_serviceid"`       // 被使用的服务的id
	Status         int               `json:"fd_status"`          // 被使用服务的申请状态，0-未申请使用，1-申请使用 2-审核通过已可用
	TrafficControl string            `json:"fd_traffic_control"` //流量控制字段
	Description    string            `json:"fd_description"`     // 备注
	TrafficInfo    ApiTrafficControl //流量控制信息
}

// app应用下的队列信息
type ApiAppQueue struct {
	Id          int32  `json:"fd_id"`          //t_app_queue主键
	AppId       string `json:"fd_app_id"`      // 所属的应用id
	QueueName   string `json:"fd_queue_name"`  // 队列的名字
	Read        int32  `json:"fd_read"`        //是否可读
	Write       int32  `json:"fd_write"`       //是否可写
	Description string `json:"fd_description"` // 描述
}

type ApiServiceMethod struct {
	Name string // 方法名
}

type ApiResource struct {
	db *sql.DB
	//app_service      map[string]map[string]*ApiAppService //保存所有的key下面对应的appservice,用于调用时候验证是否申请了该服务， key=>value map => key->serviceid=> value->ApiAppService
	all_app        map[string]*ApiApp     //所有的应用信息  appkey=> ApiApp 用于通过key查找secret
	all_service    map[string]*ApiService //所有的后端服务信息  serviceid => ApiService
	all_mothed     map[string]*ApiClient  //所有服务下面对应的api方法(故系统中所有的方法名唯一),每个服务的api方法统一放入自己的ApiClient  method=>ApiClient
	app_mutex      sync.RWMutex           //all_app 的读写锁
	service_mutex  sync.RWMutex           //all_service 的读写锁
	mothed_mutex   sync.RWMutex           //all_mothed 的读写锁
	requestTimeout uint32                 //请求后端服务的超时时间
}

//连接并初始化数据库"user:password@/dbname?charset=utf8"

func (this *ApiResource) Init(dns string, reqTimeout uint32) error {
	//1、初始化mysql
	db, err := sql.Open("mysql", dns)
	if err != nil {
		l4g.Warn("open db failed, dns:%s, error:%s", dns, err.Error())
		return err
	}
	//2、设置最大连接和空闲连接
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	if err = db.Ping(); err != nil {
		l4g.Error("db ping failed, dns:%s, error:%s", dns, err.Error())
		return err
	}

	this.requestTimeout = reqTimeout
	this.db = db
	//3、初始化数据结构
	//this.app_service = make(map[string]map[string]*ApiAppService)
	this.all_mothed = make(map[string]*ApiClient)
	this.all_service = make(map[string]*ApiService)
	this.all_app = make(map[string]*ApiApp)

	//4、将所有的api服务从db load入内存，存入all_service，将服务下的所有方法解析保存入all_mothed
	this.ReloadAllService("")
	//5、将所有app应用，load入all_app， 将app申请使用的api服务存入app_service
	this.ReloadAllApp("")
	return nil
}

func (this *ApiResource) UnInit() error {
	return nil
}

func (this *ApiResource) GetApiService(serviceid string) (*ApiService, error) {
	this.service_mutex.RLock()
	defer this.service_mutex.RUnlock()
	if apiservice, ok := this.all_service[serviceid]; ok {
		return apiservice, nil
	}

	return nil, errors.New("service not found")
}

/**
根据方法名获取后端api方法信息
*/
func (this *ApiResource) GetServiceMethod(method string) (*ApiClient, error) {
	this.mothed_mutex.RLock()
	defer this.mothed_mutex.RUnlock()
	if method, ok := this.all_mothed[method]; ok {
		return method, nil
	}

	return nil, lib.Api_Not_Exists //errors.New("method not found, method:" + method)
}

/**
从数据库加载所有服务信息，并保存如map
parameter:
serviceid: 服务的id，不传该参数则加载所有，传递则加载指定服务
*/
func (this *ApiResource) ReloadAllService(serviceid string) error {
	l4g.Info("reload all service begin, serviceid:%s", serviceid)
	//1、从数据库读取所有的服务
	service, err := this.queryAllService(serviceid)
	if err != nil {
		l4g.Info("reload all service failed, serviceid:%s, error:%s", serviceid, err.Error())
		return err
	}

	for e := service.Front(); e != nil; e = e.Next() {
		apiser := e.Value.(*ApiService)
		//2、将服务放入all_service中保存
		this.service_mutex.Lock()
		this.all_service[apiser.ServiceId] = apiser
		this.service_mutex.Unlock()

		//3、解析config字段配置文件，将方法信息解析保存
		client, perr := this.parseApiMethod(apiser.ServiceId, apiser.Config)
		if perr != nil {
			l4g.Warn("reload app service, parse api method failed, serviceid:%s, servicename:%s, error:%s", apiser.ServiceId, apiser.ServiceName, perr.Error())
			//continue
		}

		apiser.apiClient = client //将apiclient对象保存

		//4、将每个服务下的所有方法保存入all_mothed
		this.mothed_mutex.Lock()
		for method, _ := range client.GetAllMethod() {
			//检查方法是否重复，全平台方法名必须唯一
			if oclient, ok := this.all_mothed[method]; ok {
				//方法名全局唯一,检查查找到的方法是否属于当前加载的服务，是则更新方法
				if oclient.ServiceId() != client.ServiceId() {
					l4g.Warn("reload app service, save api method failed, the method repetition, serviceid:%s, servicename:%s, method:%s", apiser.ServiceId, apiser.ServiceName, method)
					continue
				}
			}

			this.all_mothed[method] = client
		}
		this.mothed_mutex.Unlock()
	}

	l4g.Info("reload all service end, serviceid:%s", serviceid)

	return nil
}

/**
从数据库加载所有服务信息，
parameter:
serviceid: 服务的id，不传该参数则加载所有，传递则加载指定服务
*/
func (this *ApiResource) queryAllService(serviceid string) (*list.List, error) {
	l4g.Info("query all service, begin, serviceid:" + serviceid)
	var rows *sql.Rows
	sql := ""
	var err error
	if len(serviceid) <= 0 {
		sql = "select fd_name, fd_serviceid, fd_status, fd_uid, fd_visible, fd_config from t_service"
		//rows, err = this.db.Query("select fd_name, fd_serviceid, fd_status, fd_uid, fd_visible, fd_config from t_service")
	} else {
		sql = fmt.Sprintf("select fd_name, fd_serviceid, fd_status, fd_uid, fd_visible, fd_config from t_service where fd_serviceid=\"%s\"", serviceid)
		//rows, err = this.db.Query("select fd_name, fd_serviceid, fd_status, fd_uid, fd_visible, fd_config from t_service where fd_serviceid=\"?\"", serviceid)
	}

	rows, err = this.db.Query(sql)

	if err != nil {
		l4g.Warn("query all service, query failed, sql:%s, error:", sql, err.Error())
		return nil, err
	}

	defer rows.Close()

	l4g.Info("query all service, query success, sql:%s", sql)
	//	var status, visible int
	//	var name, service_id, ownerid, config string
	all_service := list.New()
	for rows.Next() {
		service := new(ApiService)
		err := rows.Scan(&service.ServiceName, &service.ServiceId, &service.Status, &service.OwnerUid, &service.Visible, &service.Config)
		if err != nil {
			l4g.Warn("query all service, scan result failed, error:" + err.Error())
			return nil, err
		}

		all_service.PushBack(service)
		l4g.Debug(fmt.Sprintf("query all service, read line, name:%s, serviceid:%s, status:%d, owneruid:%s, visible:%d", service.ServiceName, service.ServiceId, service.Status, service.OwnerUid, service.Visible))
	}

	l4g.Info(fmt.Sprintf("query all service, end, serviceid:%s, service num:%d", serviceid, all_service.Len()))
	return all_service, nil
}

/**
查询key下面的所有申请过的服务列表
*/
func (this *ApiResource) queryAppService(appid string) (*list.List, error) {
	sql := fmt.Sprintf("select fd_id, fd_app_id, fd_serviceid, fd_status,fd_traffic_control from t_app_service where fd_app_id='%s'", appid)
	l4g.Info("query all app service, begin, appid:" + appid + ", sql:" + sql)
	rows, err := this.db.Query(sql)
	if err != nil {
		l4g.Warn("query all app service failed, appid:" + appid + ", error:" + err.Error())
		return nil, err
	}

	defer rows.Close()
	all_apps := list.New()
	for rows.Next() {
		appserv := new(ApiAppService)
		err := rows.Scan(&appserv.Id, &appserv.AppId, &appserv.Serviceid, &appserv.Status, &appserv.TrafficControl)
		if err != nil {
			l4g.Warn("query all app service, scan result failed, appid:" + appid + ", error:" + err.Error())
			return nil, err
		}

		all_apps.PushBack(appserv)

		l4g.Debug("query all app service, read line, id:%d, appid:%s, serviceid:%s, status:%d", appserv.Id, appserv.AppId, appserv.Serviceid, appserv.Status)
	}

	l4g.Info(fmt.Sprintf("query all app service, end, appid:%s, result num:%d", appid, all_apps.Len()))

	return all_apps, nil
}

/**
查找数据库中所有的app信息
parameter：
appid：未传递则查询所有app， 传入则查具体app
*/
func (this *ApiResource) queryApp(appid string) (*list.List, error) {
	l4g.Info("query all app, begin, appid:%s", appid)
	var rows *sql.Rows
	sql := ""
	var err error
	if len(appid) <= 0 {
		sql = "select fd_id, fd_uid, fd_name, fd_status, fd_key, fd_secret from t_app"
	} else {
		sql = fmt.Sprintf("select fd_id, fd_uid, fd_name, fd_status, fd_key, fd_secret from t_app where fd_id=\"%s\"", appid)
	}

	rows, err = this.db.Query(sql)

	if err != nil {
		l4g.Warn("query all app, query failed, sql:%s, error:", sql, err.Error())
		return nil, err
	}

	defer rows.Close()

	l4g.Info("query all app, query success, sql:%s", sql)

	all_app := list.New()
	for rows.Next() {
		apiapp := new(ApiApp)
		err := rows.Scan(&apiapp.AppId, &apiapp.OwnerUid, &apiapp.AppName, &apiapp.Status, &apiapp.Key, &apiapp.Secret)
		if err != nil {
			l4g.Warn("query all app, scan result failed, error:" + err.Error())
			return nil, err
		}

		all_app.PushBack(apiapp)
		l4g.Debug("query all app, read line, appid:%s, owneruid:%s, appname:%s, status:%d, key:%s, secret:%s", apiapp.AppId, apiapp.OwnerUid, apiapp.AppName, apiapp.Status, apiapp.Key, apiapp.Secret)
	}

	l4g.Info("query all app, end, appid:%s, result num:%d", appid, all_app.Len())
	return all_app, nil
}

/**
加载app下面申请的服务到内存
*/
func (this *ApiResource) reloadAppService(app *ApiApp) error {
	l4g.Info("reload app service begin")
	//2、未找到则查找数据库
	all_apps, err := this.queryAppService(app.AppId)
	if err != nil {
		l4g.Warn("reload app service failed, query DB error, error:%s", err.Error())
		return err
	}
	//app_service    map[string]*ApiAppService //保存app下面所有已申请的appservice,用于调用时候验证是否申请了该服务， map ==> key->serviceid=> value->ApiAppService

	app.appservice_mutex.Lock()
	for e := all_apps.Front(); e != nil; e = e.Next() {
		appserv := e.Value.(*ApiAppService)
		if err := appserv.TrafficInfo.Init(appserv.TrafficControl); err != nil {
			l4g.Warn("reload app service, init traffic info failed, error:%s", err.Error())
		}
		//将APPservice保存入map
		app.app_service[appserv.Serviceid] = appserv
	}
	app.appservice_mutex.Unlock()

	l4g.Info("reload app service end")
	return nil
}

/**
查找数据库中app下的所有的队列信息
parameter：
appid：应用id
*/
func (this *ApiResource) queryAppQueue(appid string) (*list.List, error) {
	l4g.Info("query app queue, begin, appid:%s", appid)
	var rows *sql.Rows
	sql := fmt.Sprintf("select fd_id, fd_app_id, fd_queue_name, fd_read, fd_write from t_app_queue where fd_app_id=\"%s\"", appid)

	var err error
	rows, err = this.db.Query(sql)

	if err != nil {
		l4g.Warn("query app queue, query failed, sql:%s, error:", sql, err.Error())
		return nil, err
	}

	defer rows.Close()

	l4g.Info("query app queue, query success, sql:%s", sql)

	queue_list := list.New()
	for rows.Next() {
		appqueue := new(ApiAppQueue)
		err := rows.Scan(&appqueue.Id, &appqueue.AppId, &appqueue.QueueName, &appqueue.Read, &appqueue.Write)
		if err != nil {
			l4g.Warn("query app queue, scan result failed, error:" + err.Error())
			return nil, err
		}

		queue_list.PushBack(appqueue)
		l4g.Debug("query app queue, read line, id:%d, appid:%s, queuename:%s, read:%d, write:%d", appqueue.Id, appqueue.AppId, appqueue.QueueName, appqueue.Read, appqueue.Write)
	}

	l4g.Info("query app queue, end, appid:%s, result num:%d", appid, queue_list.Len())
	return queue_list, nil
}

//reloadAppQueue 加载应用下面的所有队列
func (this *ApiResource) ReloadAppQueue(app *ApiApp) error {
	l4g.Info("reload app queue begin, appid:%s", app.AppId)
	//1、从数据库load所有或者一个  app信息
	queue_list, err := this.queryAppQueue(app.AppId)
	if err != nil {
		l4g.Info("reload app queue failed, query DB error, appid:%s, error:%s", app.AppId, err.Error())
		return err
	}

	app.appqueue_mutex.Lock()
	// 先删除所有队列
	for k, _ := range app.app_queue {
		delete(app.app_queue, k)
	}
	for e := queue_list.Front(); e != nil; e = e.Next() {
		queue := e.Value.(*ApiAppQueue)
		app.app_queue[queue.QueueName] = queue
	}

	app.appqueue_mutex.Unlock()
	l4g.Info("reload app queue end, appid:%s", app.AppId)
	return nil
}

func (this *ApiResource) ReloadAllApp(appid string) error {

	l4g.Info("reload all app begin, appid:%s", appid)
	//1、从数据库load所有或者一个  app信息
	all_apps, err := this.queryApp(appid)
	if err != nil {
		l4g.Info("reload all app failed, query DB error, appid:%s, error:%s", appid, err.Error())
		return err
	}

	for e := all_apps.Front(); e != nil; e = e.Next() {
		app := e.Value.(*ApiApp)
		app.app_service = make(map[string]*ApiAppService)
		app.app_queue = make(map[string]*ApiAppQueue)
		//2、更新app下面申请使用的service
		this.reloadAppService(app)
		//加载服务下的队列
		this.ReloadAppQueue(app)

		//3、查找map中是否存在该appkey，找到则替换值，未找到则插入
		this.app_mutex.Lock()
		this.all_app[app.Key] = app
		this.app_mutex.Unlock()
	}
	l4g.Info("reload all app end, appid:%s", appid)
	return nil
}

func (this *ApiResource) GetAppInfo(appkey string) (*ApiApp, error) {
	this.app_mutex.RLock()
	defer this.app_mutex.RUnlock()
	if app, ok := this.all_app[appkey]; ok {
		return app, nil
	}

	return nil, lib.ClientId_Error
}

/***
根据key，获取应用下面的所有服务
*/
func (this *ApiResource) GetAllAppService(key string) (map[string]*ApiAppService, error) {
	/*
		//1、根据key查找下面的 map => ApiAppService
		v, ok := this.app_service[key]
		if ok {
			return v, nil
		}
		//2、未找到则查找数据库
		all_apps, err := this.queryAppService(key)
		if err != nil {
			return nil, err
		}
		//	err = rows.Err()
		//	if err != nil {
		//		log.Fatal(err)
		//	}

		if !ok {
			v = make(map[string]*ApiAppService)
			this.app_service[key] = v

		}

		for e := all_apps.Front(); e != nil; e = e.Next() {
			//3、查找map中是否存在该serviceid，找到则替换值，未找到则插入
			if serv, ok_ := v[e.value.Serviceid]; ok_ {
				serv = e.value
			} else {
				v[e.value.Serviceid] = e.Value
			}
		}
	*/
	return nil, nil
}

func (this *ApiResource) DeleteService(serviceid string) error {
	l4g.Info("apiresource delete service,serviceid:%s", serviceid)
	this.service_mutex.Lock()
	defer this.service_mutex.Unlock()

	if service, ok := this.all_service[serviceid]; ok {
		//删除服务下面的所有方法
		this.mothed_mutex.Lock()
		for method, _ := range service.apiClient.GetAllMethod() {
			delete(this.all_mothed, method)
		}
		this.mothed_mutex.Unlock()

		//删除服务
		delete(this.all_service, serviceid)
	}

	l4g.Info("apiresource delete service, delete succeed,serviceid:%s", serviceid)

	return nil
}

//DeleteApp 删除app信息，删除app下面关联的服务
func (this *ApiResource) DeleteApp(appid string, appkey string) error {
	l4g.Info("apiresource delete app, appid:%s, appkey:%s", appid, appkey)

	this.app_mutex.RLock()
	defer this.app_mutex.RUnlock()
	if _, ok := this.all_app[appkey]; ok {
		delete(this.all_app, appkey)
		l4g.Info("apiresource delete app, delete success, appid:%s, appkey:%s", appid, appkey)
		return nil
	}

	l4g.Info("apiresource delete app, delete failed, app not exist, appid:%s, appkey:%s", appid, appkey)

	return nil
}

func (this *ApiResource) parseApiMethod(serviceid, config string) (*ApiClient, error) {
	apiClient := &ApiClient{}
	err := apiClient.Init(serviceid, config, this.requestTimeout)
	return apiClient, err
}

//GetApiMethod 若指定serviceid，则获取指定服务下的方法，否则获取所有api方法，
func (this *ApiResource) GetApiMethod(serviceid string) *list.List {
	allmethod := list.New()
	this.service_mutex.RLock()
	defer this.service_mutex.RUnlock()

	if len(serviceid) > 0 {
		if service, ok_ := this.all_service[serviceid]; ok_ {
			//判断该服务对于应用的状态，0-未申请使用，1-申请使用 2-审核通过已可用
			for method, _ := range service.apiClient.GetAllMethod() {
				allmethod.PushBack(method)
			}
		}

	} else {
		//获取所有服务的方法
		for _, service := range this.all_service {
			for method, _ := range service.apiClient.GetAllMethod() {
				allmethod.PushBack(method)
			}
		}
	}

	return allmethod
}

//获取app下面指定的service
func (this *ApiApp) GetAppService(serviceid string) *ApiAppService {
	this.appservice_mutex.RLock()
	defer this.appservice_mutex.RUnlock()

	if appservice, ok_ := this.app_service[serviceid]; ok_ {
		return appservice
	}

	return nil
}

//检查app下是否已申请使用该服务, 并且返回该服务对象
func (this *ApiApp) CheckGetAppService(serviceid string) (*ApiAppService, bool) {
	this.appservice_mutex.RLock()
	defer this.appservice_mutex.RUnlock()
	//1、根据serviceid查看该app 是否有使用权限
	if appservice, ok_ := this.app_service[serviceid]; ok_ {
		//判断该服务对于应用的状态，0-未申请使用，1-申请使用 2-审核通过已可用
		if appservice.Status == 2 {
			return appservice, true
		}

		return appservice, false
	}

	return nil, false
}

func (this *ApiApp) GetAppQueue(qname string) (*ApiAppQueue, bool) {
	this.appqueue_mutex.RLock()
	defer this.appqueue_mutex.RUnlock()
	queue, ok := this.app_queue[qname]
	return queue, ok
}
