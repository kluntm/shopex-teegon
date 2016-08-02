package plugins

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"

	"git.ishopex.cn/teegon/apigateway/lib"
)

type TaobaoSign struct {
}

func (me *TaobaoSign) Name() string {
	return "Taobao.md5"
}

//func (me *TaobaoSign) RequiredConfigValues() []lib.ConfigValue {
//	return []lib.ConfigValue{
//		lib.ConfigValue{
//			Name:        "app_secret",
//			Description: "签名密钥",
//			IsSecret:    true,
//		},
//	}
//}

func (me *TaobaoSign) Sign(r *http.Request, config *map[string]string) string {
	secret := (*config)["app_secret"]
	signstr := secret + lib.Sortedstr(&r.Form, "", "", "sign") + secret
	h := md5.New()
	io.WriteString(h, signstr)
	return fmt.Sprintf("%032X", h.Sum(nil))
}
