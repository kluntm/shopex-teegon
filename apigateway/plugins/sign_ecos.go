package plugins

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"

	"git.ishopex.cn/teegon/apigateway/lib"
)

type EcosSign struct {
}

func (me *EcosSign) Name() string {
	return "ECOS.md5"
}

//func (me *EcosSign) RequiredConfigValues() []lib.ConfigValue {
//	return []lib.ConfigValue{
//		lib.ConfigValue{
//			Name:        "token",
//			Description: "站点token",
//			IsSecret:    true,
//		},
//	}
//}

func (me *EcosSign) Sign(r *http.Request, config *map[string]string) string {
	str := lib.Sortedstr(&r.Form, "", "", "sign")
	str = me.md5(str) + (*config)["token"]
	return me.md5(str)
}

func (me *EcosSign) md5(s string) string {
	h := md5.New()
	io.WriteString(h, s)
	return fmt.Sprintf("%032X", h.Sum(nil))
}
