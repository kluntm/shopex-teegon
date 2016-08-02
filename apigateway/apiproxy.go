package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"git.ishopex.cn/teegon/apigateway/api"
	"git.ishopex.cn/teegon/apigateway/lib"
	l4g "github.com/alecthomas/log4go"
)

//api代理，实现转发逻辑处理
type ApiProxy struct {
	apiresouce *api.ApiResource //api资源，所有的api信息，包括app=>service对应关系等
	config     *Config          //配置信息
	callStat   *api.ApiCallStat //调用统计
	curConnNum int32            //当前请求连接数
}

func (this *ApiProxy) Init(apiresource *api.ApiResource, config *Config) error {
	this.apiresouce = apiresource

	this.config = config
	this.callStat = &api.ApiCallStat{}
	this.callStat.Init()
	return nil
}

func ApiErrorHandle(req *http.Request, err interface{}, rw *http.ResponseWriter) ([]byte, int) {
	errdata, ok := err.(lib.Error)

	if !ok {
		errdata = *lib.NewError(err)
	}

	if rw != nil {
		msg := &lib.ApiResult{
			Result: "error",
			Error:  &errdata,
		}
		msg.Write(rw)
	}
	err_bin, _ := errdata.MarshalJSON()
	return err_bin, errdata.HttpCode
}

func (this *ApiProxy) Forward(w http.ResponseWriter, req *http.Request, https bool) (res_err error) {

	if this.config.MaxConnection <= this.curConnNum { //超过最大限制数量，则返回错误

		return nil
	}

	defer ExitRecovery()

	request_id := req.Header.Get("X-Request-Id")
	if request_id == "" {
		request_id = lib.Guid(16, func(_ string) bool { return true })
		req.Header.Set("X-Request-Id", request_id)
	}

	remote_ip := lib.RemoteIp(req)
	req.Header.Set("X-Caller-Ip", remote_ip)
	req.Header.Set("X-Proxy-UA", req.Header.Get("User-Agent"))
	h := w.Header()
	h.Set("X-Request-Id", request_id)
	var req_sign, app_key, method string
	req_sign = req.FormValue("sign")
	app_key = req.FormValue("app_key")
	debug := req.FormValue("debug")

	method = req.FormValue("method")
	//	t.ResponseCode = 200
	//	t.RequestId = request_id
	//	t.Host = req.Host
	//	t.Path = req.URL.Path
	//	t.Query = req.URL.RawQuery
	//	t.UserAgent = req.Header.Get("User-Agent")
	//	t.RemoteAddr = remote_ip

	t := &lib.TracingRecord{
		ResponseCode: 200,
		RequestId:    request_id,
		Host:         req.Host,
		Path:         req.URL.Path,
		Query:        req.URL.RawQuery,
		UserAgent:    req.Header.Get("User-Agent"),
		RemoteAddr:   remote_ip,
		ApiMethod:    method,
	}
	t.SetTime(time.Now())
	atomic.AddInt32(&this.curConnNum, 1)
	l4g.Info("apiproxy forward, begin request, app_key:%s, method:%s, request_id:%s, remote_ip:%s, host:%s, cur_conn_num:%d", app_key, method, request_id, remote_ip, req.Host, this.curConnNum)

	defer func() {
		atomic.AddInt32(&this.curConnNum, -1)
		if res_err != nil {
			body, code := ApiErrorHandle(req, res_err, &w)
			t.SetResponseBody(body)
			t.ResponseCode = code
		}

		t.Duration = float32(time.Now().Sub(t.Time()).Seconds() * 1000)
		t.SetResponseHeader(w.Header())

		//发生错误则打印错误信息
		if res_err != nil {
			t.Processed = false
			l4g.Info("apiproxy forward, end request, request error, app_key:%s, method:%s, request_id:%s, remote_ip:%s, host:%s, time:%.0f ms, cur_conn_num:%d, error:%s", app_key, method, request_id, remote_ip, req.Host, t.Duration, this.curConnNum, res_err.Error())
		} else {
			t.Processed = true
			l4g.Info("apiproxy forward, end request, request success, app_key:%s, method:%s, request_id:%s, remote_ip:%s, host:%s, time:%.0f ms, cur_conn_num:%d", app_key, method, request_id, remote_ip, req.Host, t.Duration, this.curConnNum)
		}

		b, er := json.Marshal(t)
		if er == nil {
			//日志输出到统计文件
			l4g.Log(l4g.INFO, "statist_log", string(b))
			//if config.SandBox {
			if debug == "true" {
				//将调用结果写入redis 共前端查看
				//l4g.Info("debugger:%s", string(b))
				if _, err := gApp.redisclient.Set(request_id, string(b), time.Duration(this.config.TracingRecordTTL)*time.Second).Result(); err != nil {
					l4g.Warn("apiproxy forward, end request, save tracing record failed, app_key:%s, method:%s, request_id:%s, error:%s", app_key, method, request_id, err.Error())
				}
			}
		} else {
			l4g.Warn("apiproxy forward, marshal call info failed, app_key:%s, method:%s, request_id:%s, remote_ip:%s, host:%s, time:%.0f ms", app_key, method, request_id, remote_ip, req.Host, t.Duration)
		}

		//}
	}()

	if len(app_key) <= 0 {
		return lib.ClientId_Error
	}

	//l4g.Info("forward method:%s, app_key:%s", method, app_key)
	//this.apiresouce.GetAllAppService(app_key)

	//根据appkey 查找secret
	appinfo, aerr := this.apiresouce.GetAppInfo(app_key)
	if aerr != nil {
		l4g.Warn("forward failed, access denied, appkey error, key:%s, method:%s", app_key, method)
		//w.Write([]byte("access denied, appkey error"))
		res_err = aerr
		return
	}
	t.Eid = appinfo.OwnerUid
	t.AppId = appinfo.AppId
	t.Key = appinfo.Key

	//非https则校验签名， https则校验key和secret
	if !https {
		//校验签名
		query := req.URL.Query()

		post_form := &req.PostForm
		if req.MultipartForm != nil {
			post_form = (*url.Values)(&req.MultipartForm.Value)
		}
		//sign := lib.Sign(req, appinfo.Secret)
		if res_err = api.ValidateApiRequest(appinfo.Key, appinfo.Secret, req, time.Now().Unix(), t); res_err != nil {
			l4g.Warn("forward failed, sign error, key:%s, method:%s, req_sign:%s, url param:%v, post param:%v", app_key, method, req_sign, query, post_form)
			//w.Write([]byte(res_err.Error()))
			return res_err
		}
	} else {
		client_secret := req.FormValue("client_secret")
		if appinfo.Key != app_key || appinfo.Secret != client_secret {
			res_err = lib.Secret_Error
			return res_err
		}
	}

	//根据方法名获取该方法对于的，apiclient对象
	client, err := this.apiresouce.GetServiceMethod(method)
	if err != nil {
		l4g.Warn("forward failed, method not found, key:%s, method:%s", app_key, method)
		//w.Write([]byte(err.Error()))
		res_err = err
		return res_err
	}

	var appservice *api.ApiAppService
	var bret bool
	//检查传递的appkey 是否有调用指定服务的权限
	if appservice, bret = appinfo.CheckGetAppService(client.ServiceId()); !bret {
		l4g.Warn("forward failed, access denied, app can not call the api, key:%s, serviceid:%s, method:%s", app_key, client.ServiceId(), method)
		//w.Write([]byte("app key no calls permissions"))
		res_err = lib.ApiCall_Permis_Error
		return res_err
	}

	//检查调用次数限制
	// 先获取api流控信息,若无控制信息 则表示不限制调用
	if limit := appservice.TrafficInfo.GetApiCallLimit(method); limit != nil {
		// 检查调用是否超过限制
		if err := this.callStat.CheckUpdateCallInfo(app_key, method, limit.Time, limit.Num); err != nil {
			res_err = err
			return res_err
		}
	}

	//this.callStat.SaveCallInfo(app_key, method)
	res_err = client.Proxy(method, request_id, appinfo.OwnerUid, w, req, t, appinfo.SandBox == 1)
	return
}

func get_value(req *http.Request, key string) (string, error) {
	val := req.FormValue(key)

	if val == "" {
		return "", lib.Errors.Get("0040005", key)
	}

	return val, nil
}
