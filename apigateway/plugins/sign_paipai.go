package plugins

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"

	"git.ishopex.cn/teegon/apigateway/lib"
)

type PaipaiSign struct {
}

func (me *PaipaiSign) Name() string {
	return "Paipai"
}

//func (me *PaipaiSign) RequiredConfigValues() []lib.ConfigValue {
//	return []lib.ConfigValue{
//		lib.ConfigValue{
//			Name:        "appOAuthKey",
//			Description: "签名密钥",
//			IsSecret:    true,
//		},
//	}
//}

func (me *PaipaiSign) Sign(r *http.Request, config *map[string]string) string {
	key := (*config)["appOAuthKey"]
	signstr := r.Method + "&" + me.urlencode(r.URL.Path) + "&" +
		me.urlencode(lib.Sortedstr(&r.Form, "&", "=", "sign"))
	h := hmac.New(sha1.New, []byte(key+"&"))
	io.WriteString(h, signstr)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func (me *PaipaiSign) urlencode(s string) string {
	return strings.Replace(url.QueryEscape(s), `+`, `%20`, -1)
}
