package api

import (
	"encoding/json"
	"sync"
	"time"

	"errors"

	"git.ishopex.cn/teegon/apigateway/lib"
	l4g "github.com/alecthomas/log4go"
	sjson "github.com/bitly/go-simplejson" // for json get
)

//每个api的限制
type ApiCallLimit struct {
	Method string `json:"method,omitempty"` //api方法名
	Time   uint32 `json:"time,omitempty"`   //时间范围
	Num    uint32 `json:"num,omitempty"`    //调用次数
}

//api调用流量控制  {"default":{"time":60,"num":1000},"apis":{"method1":{"time":60,"num":1000},"method2":{"time":60,"num":1000}}}
type ApiTrafficControl struct {
	EnabledControl bool                    //启用控制，解析json成功，并且有default或者apis下有数据，则启用控制
	Default        ApiCallLimit            `json:"default,omitempty"` //默认调用限制
	CallLimit      map[string]ApiCallLimit `json:"apis,omitempty"`    //app下服务每个接口的调用限制
}

//api调用信息
type ApiCallInfo struct {
	Method       string       //api方法名
	LastCallTime time.Time    //时间范围
	CallNum      uint32       //调用次数
	rwmutex      sync.RWMutex //锁
}

//api调用统计
type ApiCallStat struct {
	CallStat map[string]map[string]*ApiCallInfo //每个key对api方法的调用次数统计 appkey=> map[string]*ApiCallLimit==> method=>ApiCallLimit
	rwmutex  sync.RWMutex
}

func (this *ApiCallStat) Init() error {
	this.CallStat = make(map[string]map[string]*ApiCallInfo)
	return nil
}

//将api调用信息保存
func (this *ApiCallStat) UpdateCallInfo(appkey, method string) error {
	this.rwmutex.RLock() //先读写锁
	if mapcall, ok := this.CallStat[appkey]; ok {
		if callinfo, ok := mapcall[method]; ok {
			this.rwmutex.RUnlock()

			callinfo.rwmutex.Lock()
			callinfo.CallNum += 1
			callinfo.LastCallTime = time.Now()
			callinfo.rwmutex.Unlock()

		} else {
			this.rwmutex.RUnlock() //解锁
			this.rwmutex.Lock()    //加写锁
			callinfo := &ApiCallInfo{
				Method:       method,
				LastCallTime: time.Now(),
				CallNum:      1,
			}

			mapcall[method] = callinfo
			this.rwmutex.Unlock() //解除写锁
		}
	} else {
		this.rwmutex.RUnlock() //解锁
		this.rwmutex.Lock()    //加写锁
		callinfo := &ApiCallInfo{
			Method:       method,
			LastCallTime: time.Now(),
			CallNum:      1,
		}

		mapcall := make(map[string]*ApiCallInfo)
		mapcall[method] = callinfo
		this.CallStat[appkey] = mapcall
		this.rwmutex.Unlock() //解除写锁
	}

	return nil
}

//CheckCallLimit 检查方法调用是否超过限制
//method: 方法名, timeinterval: 时间范围， num: 时间范围内的调用次数
func (this *ApiCallStat) CheckUpdateCallInfo(appkey, method string, timeinterval, num uint32) error {
	this.rwmutex.RLock() //先读写锁
	//defer this.rwmutex.Unlock()
	if mapcall, ok := this.CallStat[appkey]; ok {

		if callinfo, ok := mapcall[method]; ok {
			this.rwmutex.RUnlock()
			lastCallTime := callinfo.LastCallTime
			callinfo.UpdateCallInfo(1) //调用次数加一

			//检查是否超过调用限制, 当前时间 - 最后调用时间 < 设置的时间范围，则比较调用次数
			ret := time.Now().Sub(lastCallTime)

			//l4g.Warn(method, timeinterval, num, callinfo.CallNum, ret, callinfo.LastCallTime)
			if ret < time.Duration(timeinterval) {
				if (callinfo.CallNum - 1) < num { //前面增加了本次的调用，故此处需要-1再做比较
					return nil
				} else { //超过次数限制，则返回错误
					return lib.Errors.Get("0040311", uint64(ret.Seconds()))
				}
			} else { //超过间隔，则重置次数
				callinfo.ResetCallNum(1)
			}

		} else {
			this.rwmutex.RUnlock() //解锁
			this.rwmutex.Lock()    //加写锁
			callinfo := &ApiCallInfo{
				Method:       method,
				LastCallTime: time.Now(),
				CallNum:      1,
			}

			mapcall[method] = callinfo
			this.rwmutex.Unlock() //解除写锁
		}
	} else { //若未找到该appkey对应的调用信息，则创建并保存
		this.rwmutex.RUnlock() //解锁
		this.rwmutex.Lock()    //加写锁
		callinfo := &ApiCallInfo{
			Method:       method,
			LastCallTime: time.Now(),
			CallNum:      1,
		}

		mapcall := make(map[string]*ApiCallInfo)
		mapcall[method] = callinfo
		this.CallStat[appkey] = mapcall
		this.rwmutex.Unlock() //解除写锁
	}

	return nil
}

//初始化，解析流量控制json   {"default":{"time":60,"num":1000},"apis":{"method1":{"time":60,"num":1000},"method2":{"time":60,"num":1000}}}
func (this *ApiTrafficControl) Init(config string) error {
	this.EnabledControl = false

	//检查json内容是否正常
	sj, err := sjson.NewJson([]byte(config))
	if err != nil {
		return err
	}
	//检查是否存在default 和 apis,没有则标记该接口不限制调用
	_, ok := sj.CheckGet("default")
	_, aok := sj.CheckGet("apis")
	if !ok && !aok {
		return errors.New("check traffic json error, default or apis not exist.")
	}

	this.CallLimit = make(map[string]ApiCallLimit)

	if err := json.Unmarshal([]byte(config), this); err != nil {
		l4g.Warn("traffic control, json error:%v", err.Error())
		return err
	}

	//解析json成功则表示启用流控
	this.EnabledControl = true
	l4g.Warn("traffic control:%v", this)
	//this.Default = &ApiCallLimit{}
	return nil
}

//获取api接口的流量限制, 返回nil 表示该接口调用次数无限制
func (this *ApiTrafficControl) GetApiCallLimit(method string) *ApiCallLimit {
	if !this.EnabledControl {
		return nil
	}

	if limit, ok := this.CallLimit[method]; ok {
		return &limit
	}

	return &this.Default
}

func (this *ApiCallInfo) UpdateCallInfo(callnum uint32) error {
	this.rwmutex.Lock()
	this.CallNum += callnum
	this.LastCallTime = time.Now()
	this.rwmutex.Unlock()
	return nil
}

func (this *ApiCallInfo) ResetCallNum(newnum uint32) error {
	this.rwmutex.Lock()
	this.CallNum = newnum
	this.rwmutex.Unlock()
	return nil
}
