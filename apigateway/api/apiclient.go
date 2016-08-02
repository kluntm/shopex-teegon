package api

import (
	"encoding/json"
	"fmt"
	//"io/ioutil"
	"net/http"
	//"net/http/httputil"
	"bytes"
	"io"
	"mime/multipart"
	"net/url"

	l4g "github.com/alecthomas/log4go"

	"git.ishopex.cn/teegon/apigateway/lib"
	//"github.com/astaxie/beego"
	"time"

	sjson "github.com/bitly/go-simplejson" // for json get
)

/**
公共参数配置，此处主要用于配置向后端的签名和签名时间戳
*/
type ApiGlobalParam struct {
	Name        string `json:"name,omitempty"`         //公共参数名字
	RequestType string `json:"request_type,omitempty"` //公共参数传递到后端的方式，post/get/REQUEST(根据前端请求一致的方式传递前端用get，则用get)/none为不传递
	ValueType   string `json:"value_type,omitempty"`   //该参数的编码方式，此处主要用于签名，值为"sign" 则表示签名
	Format      string `json:"format,omitempty"`       //公共参数传递到后端的方式，post/get/REQUEST(根据前端请求一致的方式传递前端用get，则用get)
	Description string `json:"description,omitempty"`  //配置描述
	Value       string `json:"value,omitempty"`        //默认值
}

/**
公共参数配置部分，会在向后端发送请求时，传递到后端
*/
type ApiGlobalConfig struct {
	Name         string `json:"name,omitempty"`          //配置项的名字
	Description  string `json:"description,omitempty"`   //配置描述
	DefaultValue string `json:"default_value,omitempty"` //默认值
}

//到后端的请求方式，参数传递方式
type BackRequestWay struct {
	RequestMode       string `json:"request_mode,omitempty"`        // 参数传递方式(若为：param，则表示以参数方式传递，为：path 则以reset方式)
	RequestMethodName string `json:"request_method_name,omitempty"` // 传递请求方法的键名
}

/**
接口定于中的公共参数和配置部分
*/
type ApiGlobalInfo struct {
	Param      []ApiGlobalParam  `json:"params,omitempty"`           //公共参数部分，对后端的签名配置等
	Config     []ApiGlobalConfig `json:"config,omitempty"`           //公共配置部分，如后端签名所需的 属性值等
	RequestWay BackRequestWay    `json:"back_request_way,omitempty"` //到后端的参数的，请求传递方式
}

/**
接口信息定义，swagger格式
*/
type ApiDesign struct {
	BackEndUrl string           `json:"backend_url,omitempty"`
	SandBoxUrl string           `json:"sandbox_url,omitempty"`
	Schemes    []string         `json:"Schemes,omitempty"`
	Produces   []string         `json:"produces,omitempty"`
	Paths      *json.RawMessage `json:"paths,omitempty"`
	GlobalInfo ApiGlobalInfo    `json:"global,omitempty"` //公共参数（如：对后端的签名，对后端需要传递的特殊参数等）
}

type ApiParam struct {
	Name string `json:"name,omitempty"`
	//Desc     string `json:"description,omitempty"`
	Required bool   `json:"required,omitempty"`
	Type     string `json:"type,omitempty"`
	Format   string `json:"format,omitempty"`
}

type ApiMethod struct {
	MethodName string     //方法名
	RequestWay string     //请求方式 get/post
	Param      []ApiParam `json:"parameters,omitempty"`
	TimeOut    uint32     `json:"timeout,omitempty"` //后端超时时间
}

var proxy_transport = &http.Transport{
	DisableKeepAlives:   false,
	DisableCompression:  true,
	MaxIdleConnsPerHost: 65536,
	//ResponseHeaderTimeout: time.Second * 10, //默认请求超时时间
}

type ApiClient struct {
	apiMethod      map[string]*ApiMethod // key mothedname => ApiMethod
	apiDesign      ApiDesign
	serviceid      string
	backUrl        *url.URL //后端地址
	sandboxUrl     *url.URL //沙箱地址
	RequestNum     int64    //总调用次数
	RequestFailed  int64    //调用失败次数
	RequestSuccess int64    //调用成功次数
	RequestTimeout uint32   //请求后端服务的超时时间
	curReqEntid    string   // 当前请求的用户的entid
	curBackUrl     *url.URL
}

func (this *ApiClient) Init(serviceid, config string, BackEndRequestTimeout uint32) error {
	l4g.Info("api client init, serviceid:%s", serviceid)
	this.apiMethod = make(map[string]*ApiMethod)
	this.serviceid = serviceid
	this.RequestTimeout = BackEndRequestTimeout
	proxy_transport.ResponseHeaderTimeout = time.Second * time.Duration(this.RequestTimeout)

	err := json.Unmarshal([]byte(config), &this.apiDesign)

	if err != nil {
		fmt.Println("error:", err)
		return err
	}

	//解析paths
	if path, err := this.apiDesign.Paths.MarshalJSON(); err == nil {
		jspath, jerr := sjson.NewJson(path)
		if jerr != nil {
			fmt.Println(jerr)
			return jerr
		}
		//遍历所有方法
		if mappath, merr := jspath.Map(); merr == nil {
			for method, _ := range mappath {
				apiMethod := new(ApiMethod)
				apiMethod.MethodName = method

				jv := jspath.Get(method)
				if mmethod, err := jv.Map(); err == nil {
					for mk, _ := range mmethod {
						apiMethod.RequestWay = mk          //设置请求方式 get/post
						jsm, _ := jv.Get(mk).MarshalJSON() //将请求方式下的数据映射为json

						json.Unmarshal(jsm, &apiMethod) //只映射 请求所需参数字段即 method=>get=>parameters
						//jinfo, _ := json.Marshal(apiMethod)
					}
				}

				this.apiMethod[method] = apiMethod //保存方法名=>ApiInfo
			}
		} else {
			return merr
		}
	} else {
		return err
	}

	//传递到后端的，方法键名默认为method
	if len(this.apiDesign.GlobalInfo.RequestWay.RequestMethodName) <= 0 {
		this.apiDesign.GlobalInfo.RequestWay.RequestMethodName = "method"
	}

	//若后端代理方式为method方式则解析url，应为该模式下url不会发生变化
	if this.apiDesign.GlobalInfo.RequestWay.RequestMode != "path" {
		this.backUrl, err = url.Parse(this.apiDesign.BackEndUrl)
		if err != nil {
			l4g.Warn("api client init, parse backend url failed, serviceid:%s, url:%s, error:%s", serviceid, this.apiDesign.BackEndUrl, err.Error())
		}

		this.sandboxUrl, err = url.Parse(this.apiDesign.SandBoxUrl)
		if err != nil {
			l4g.Warn("api client init, parse sandbox url failed, serviceid:%s, url:%s, error:%s", serviceid, this.apiDesign.SandBoxUrl, err.Error())
		}
	}

	l4g.Info("load api success, backend url:%s, sandbox url:%s, api:%v, global:%v", this.apiDesign.BackEndUrl, this.apiDesign.SandBoxUrl, this.apiMethod, this.apiDesign.GlobalInfo)

	return nil
}

func (this *ApiClient) ServiceId() string { return this.serviceid }

func (this *ApiClient) GetAllMethod() map[string]*ApiMethod {
	return this.apiMethod
}

func (this *ApiClient) Proxy(method, requestid string, entid string, w http.ResponseWriter, req *http.Request, t *lib.TracingRecord, sandbox bool) error {

	l4g.Info("api proxy, method:%s, requestid:%s, sandbox:%t, backurl:%s", method, requestid, sandbox, this.apiDesign.BackEndUrl)
	var backendurl *url.URL
	var err error
	if sandbox { //非沙箱环境
		//若后端请求方式为 reset 方式则需要解析url
		if this.apiDesign.GlobalInfo.RequestWay.RequestMode == "path" {
			backendurl, err = url.Parse(this.apiDesign.SandBoxUrl + "/" + method)
			if err != nil {
				l4g.Warn("api client Proxy, parse backend url failed, serviceid:%s, url:%s, method:%s, error:%s", this.serviceid, this.apiDesign.BackEndUrl, method, err.Error())
				return lib.Backend_Error
			}
		} else {
			if this.sandboxUrl == nil {
				l4g.Warn("back url error")
				return lib.Backend_Error
			}
			backendurl = this.sandboxUrl
		}
	} else {
		//若后端请求方式为 reset 方式则需要解析url
		if this.apiDesign.GlobalInfo.RequestWay.RequestMode == "path" {
			backendurl, err = url.Parse(this.apiDesign.BackEndUrl + "/" + method)
			if err != nil {
				l4g.Warn("api client Proxy, parse backend url failed, serviceid:%s, url:%s, method:%s, error:%s", this.serviceid, this.apiDesign.BackEndUrl, method, err.Error())
				return lib.Backend_Error
			}
		} else {
			if this.backUrl == nil {
				l4g.Warn("back url error")
				return lib.Backend_Error
			}
			backendurl = this.backUrl
		}
	}

	//检查方法是否存在
	if apimethod, ok := this.apiMethod[method]; ok {
		//检查参数是否传递
		if err := this.checkRequestParam(apimethod, req); err != nil {
			//检查失败，则返回错误
			return err
		}
	} else {
		l4g.Warn("api proxy, method not exist, method:%s", method)
		return lib.Api_Not_Exists
	}

	this.curBackUrl = backendurl
	this.curReqEntid = entid
	proxy_transport.ResponseHeaderTimeout = time.Second * time.Duration(this.RequestTimeout)
	proxy := lib.NewSingleHostReverseProxy(backendurl)
	proxy.Director = this.director
	proxy.Transport = proxy_transport

	//l4g.Warn("req url scheme:%s, host:%s, host:%s", remote.Scheme, remote.Host, this.apiDesign.Host)

	//proxy := httputil.NewSingleHostReverseProxy(remote)
	return proxy.ServeHTTP(w, req, t)

	//	client := &http.Client{}
	//	reqest, _ := http.NewRequest("GET", "http://www.baidu.com", nil)

	//	response, _ := client.Do(reqest)
	//	if response.StatusCode == 200 {
	//		body, _ := ioutil.ReadAll(response.Body)
	//		//bodystr := string(body)
	//		//fmt.Println(bodystr)
	//		//	w.Write(body)
	//		return body, nil
	//	}

	//return []byte("test"), nil
}

//检查请求参数
func (this *ApiClient) checkRequestParam(method *ApiMethod, req *http.Request) error {
	for _, param := range method.Param {
		if param.Required && (len(req.FormValue(param.Name)) == 0 && (req.MultipartForm != nil && len(req.MultipartForm.Value[param.Name]) == 0)) {
			return lib.Errors.Get("0040005", param.Name)
		}
	}

	return nil
}

func (this *ApiClient) director(req *http.Request, tracing *lib.TracingRecord) {
	//nowunix := tracing.Time().Unix()
	//var access model.ApiAccess
	//var ok bool

	tracing.Processed = true
	req.Header.Set("User-Agent", "Prism/1.0.1")
	req.Header.Del("Cookie")
	target_url := this.curBackUrl
	//req.URL.Path = new_path
	//req.Host = req.URL.Host
	//	node_url_parsed, err := url.Parse(node_url)
	//	if err == nil {
	//		target_url = node_url_parsed
	//	}

	req.URL.Scheme = target_url.Scheme
	req.URL.Host = target_url.Host
	req.URL.Path = target_url.Path

	if req.MultipartForm != nil {
		req.PostForm = url.Values(req.MultipartForm.Value)
	}
	this.clean_var(&req.PostForm)
	this.apply_global_params(req, &this.apiDesign.GlobalInfo)
	tracing.SetPost(&req.PostForm)
	tracing.Backend = req.URL.String()

	if req.Method == "POST" || req.Method == "PUT" {
		if req.MultipartForm != nil {
			var b bytes.Buffer
			w := multipart.NewWriter(&b)
			for k, vs := range req.PostForm {
				for _, v := range vs {
					fw, _ := w.CreateFormField(k)
					fw.Write([]byte(v))
				}
			}

			for k, vs := range req.MultipartForm.File {
				for _, v := range vs {
					fw, _ := w.CreateFormFile(k, v.Filename)
					fr, _ := v.Open()
					io.Copy(fw, fr)
					fr.Close()
				}
			}

			w.Close()
			req.Header.Set("Content-Type", w.FormDataContentType())
			req.Body = &lib.ClosingBuffer{&b}
			req.ContentLength = int64(b.Len())
		} else {
			body := req.PostForm.Encode()
			req.Body = &lib.ClosingBuffer{bytes.NewBufferString(body)}
			req.ContentLength = int64(len(body))
		}
	}
}

func (me *ApiClient) dispatch(req *http.Request) (
	api_method, new_path, api_id, prefix string,
	config *map[string]string,
	req_oauth bool) {
	/*
		l4g.Warn("url path:%v", req.URL.Path)
		var apimap *model.InstanceRouter
		var ok bool
		use_sandbox := req.URL.Path[4] != '/'
		var request_path string
		if use_sandbox {
			request_path = req.URL.Path[13:]
		} else {
			request_path = req.URL.Path[5:]
		}

		prefix, path := me.split_prefix_path(request_path)

		if me.DebugApi == nil {
			dmap := model.Cache.ApiRouter.Domain(DomainId)
			apimap, ok = dmap.Api[prefix]

			if !ok {
				panic(lib.ApiError{"0040400", nil})
			}
		} else {
			apimap, _ = me.DebugApi.Router()
		}

		config = &apimap.ConfigValues
		target_url := apimap.Url

		node_id := req.FormValue("node_id")
		if node_id != "" {
			backend_node, err := model.GetBackendNode(DomainId, apimap.Id, node_id)
			if err == nil {
				config = &backend_node.Data
				node_url, ok := backend_node.Data["@url"]
				if ok && node_url != "" {
					node_url_parsed, err := url.Parse(node_url)
					if err == nil {
						target_url = node_url_parsed
					}
				}
			}
		}

		gparams = &apimap.GlobalParams
		api_id = apimap.Id

		if use_sandbox {
			target_url = apimap.SandBoxUrl
		}

		req.URL.Scheme = target_url.Scheme
		req.URL.Host = target_url.Host

		var api_method_v interface{}
		if apimap.Router.IsPathMode {
			api_method_v, ok, _ = apimap.Router.Dispatch(req.Method, path)
			new_path = lib.SingleJoiningSlash(target_url.Path, request_path[len(prefix):])
		} else {
			api_method_v, ok, _ = apimap.Router.Dispatch(req.Method, get_value(req, apimap.Router.DispatchKey))
			new_path = target_url.Path
		}

		if !ok {
			panic(lib.ApiError{"0040400", nil})
		}
		api_method = api_method_v.(string)
		req_oauth = apimap.Router.UsedOAuth[api_method]
	*/

	return
}

func (me *ApiClient) apply_global_params(req *http.Request,
	globalinfo *ApiGlobalInfo) {

	is_post := (req.Method == "POST" || req.Method == "PUT")
	query := req.URL.Query()

	me.clean_var(&query)
	req.URL.RawQuery = query.Encode()
	method := req.FormValue("method")

	me.clean_var(&req.Form)
	needSign := false
	signParam := ApiGlobalParam{}
	config := make(map[string]string) //配置项
	var value string
	for _, p := range globalinfo.Param {
		switch p.ValueType {
		case "datetime":
			time := time.Now()
			if p.Format == "timestamp" {
				value = fmt.Sprintf("%d", time.Unix())
			} else {
				//value = beego.Date(time, p.Format)
			}
			break
		case "sign":
			needSign = true
			signParam = p
			break
		case "config":
			value = p.Value
			break
		default:
			value = p.Value
		}
		if p.RequestType != "none" {

			if is_post && p.RequestType == "request" {
				req.PostForm.Set(p.Name, value)
			} else {
				query.Set(p.Name, value)
				req.URL.RawQuery = query.Encode()
			}

			req.Form.Set(p.Name, value)
		}

		config[p.Name] = p.Value
	}
	if me.apiDesign.GlobalInfo.RequestWay.RequestMode == "param" {
		query.Set(me.apiDesign.GlobalInfo.RequestWay.RequestMethodName, method)
		//req.URL.RawQuery = query.Encode()
		req.Form.Set(me.apiDesign.GlobalInfo.RequestWay.RequestMethodName, method)
		l4g.Info("method_name:%s, method:%s", me.apiDesign.GlobalInfo.RequestWay.RequestMethodName, method)
	}

	//统一将entid传入后端
	query.Set("eid", me.curReqEntid)
	req.URL.RawQuery = query.Encode()
	l4g.Info("back end eid:%s", me.curReqEntid)

	if needSign {
		//value = lib.Sign(req, config)
		value = lib.SignFactory.Sign(signParam.Format, req, &config)
		if is_post && signParam.RequestType == "request" {
			req.PostForm.Set(signParam.Name, value)
		} else {
			query.Set(signParam.Name, value)
			req.URL.RawQuery = query.Encode()
		}
		req.Form.Set(signParam.Name, value)
	}

	//设置方法名

}

func (this *ApiClient) clean_var(v *url.Values) *url.Values {
	//v.Del("app_key")//appid传递到服务
	v.Del("sign_method")
	v.Del("sign_time")
	v.Del("sign")
	v.Del("client_secret")
	v.Del("method")
	return v
}
