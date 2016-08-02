package api

import (
	"net/http"
	"strconv"

	"git.ishopex.cn/teegon/apigateway/lib"
	l4g "github.com/alecthomas/log4go"
)

func ValidateApiRequest(key, secret string, req *http.Request, nowunix int64, t *lib.TracingRecord) error {
	//app_key, err := get_value(req, "app_key")

	if !try_validate_params(secret, req, t) {
		if !try_validate_header(key, secret, req, t) {
			if ok, err := try_validate_sign(secret, req, t, nowunix); !ok {
				return err
			}
		}
	}

	//	if (key.Expired > 0) && (key.Expired < nowunix) {
	//		t.Debug = "app_key has expired"
	//		panic(lib.ApiError{"0040303", nil})
	//	}

	return nil
}

func try_validate_params(secret string, req *http.Request, t *lib.TracingRecord) bool {
	// 暂定不该，以防影响业务
	//const https = "https"
	//if req.Header.Get("X-Forwarded-Proto") != https && req.URL.Scheme != https {
	//return false
	//}
	req_secret := req.FormValue("client_secret")
	if req_secret != "" {
		if secret == secret {
			return true
		} else {
			return false
		}
	}
	return false
}

func try_validate_sign(secret string, req *http.Request, t *lib.TracingRecord, nowunix int64) (bool, error) {

	sign_time_str := req.FormValue("sign_time")
	if sign_time_str == "" {
		return false, lib.Errors.Get("0040005", "sign_time")
	}
	//转换时间
	sign_time, err := strconv.Atoi(sign_time_str)
	if err != nil {
		return false, lib.SignTime_Error
	}
	//判断签名时间是否过期
	if nowunix-int64(sign_time) > 90000 {
		return false, lib.SingTimeOutOfDate_Error
	}
	is_post := (req.Method == "POST" || req.Method == "PUT")
	if is_post && req.ContentLength > 0 && len(req.Form) == 0 {
		req.ParseForm()
	}
	//计算签名
	sign := lib.SignFactory.Sign("prism", req, &map[string]string{"client_secret": secret})
	l4g.Warn("req_sign:%s, lib sign:%s", req.FormValue("sign"), sign)
	if req.FormValue("sign") != sign {
		if t != nil {
			t.Debug = map[string]string{
				"your":    req.FormValue("sign"),
				"server":  sign,
				"signstr": lib.SignStr(req, "*secret*"),
			}
		}

		return false, lib.Sign_Error
	}
	return true, nil
}

// try_validate_header check the basic auth
// if the username == key.Key and password == key.Secret then pass
//		Authorization: Basic xxxxxxxxx== (base64)
// but prism has own auth header like:
//		Authorization: Bearer xxxxxxxxxx (oauth token)
func try_validate_header(key, secret string, req *http.Request, t *lib.TracingRecord) bool {
	if k, s, ok := req.BasicAuth(); ok {
		if key == k && secret == s {
			return true
		}
	}
	return false
}
