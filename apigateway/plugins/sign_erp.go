package plugins

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
)

type ShopexErpSign struct {
}

func (me *ShopexErpSign) Name() string {
	return "ShopEx.ERP"
}

//func (me *ShopexErpSign) RequiredConfigValues() []lib.ConfigValue {
//	return []lib.ConfigValue{
//		lib.ConfigValue{
//			Name:        "secret",
//			Description: "站点私钥",
//			IsSecret:    true,
//		},
//	}
//}

func (me *ShopexErpSign) Sign(r *http.Request, config *map[string]string) string {
	secret := (*config)["secret"]
	signstr := me.md5(me.sign_str(&r.Form)) + secret
	return me.md5(signstr)
}

func (me *ShopexErpSign) md5(s string) string {
	h := md5.New()
	io.WriteString(h, s)
	return fmt.Sprintf("%032X", h.Sum(nil))
}

func (me *ShopexErpSign) sign_str(sets *url.Values) (ret string) {
	mk := make([]string, len(*sets))
	i := 0
	for k, _ := range *sets {
		mk[i] = k
		i++
	}
	sort.Strings(mk)

	for _, k := range mk {
		for _, v := range (*sets)[k] {
			ret += k + v
		}
	}
	return
}
