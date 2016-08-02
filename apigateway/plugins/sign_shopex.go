package plugins

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
)

type ShopexSign struct {
}

func (me *ShopexSign) Name() string {
	return "ShopEx.md5"
}

//func (me *ShopexSign) RequiredConfigValues() []lib.ConfigValue {
//	return []lib.ConfigValue{
//		lib.ConfigValue{
//			Name:        "token",
//			Description: "站点token",
//			IsSecret:    true,
//		},
//	}
//}

func (me *ShopexSign) Sign(r *http.Request, config *map[string]string) string {
	secret := (*config)["token"]
	signstr := me.sign_str(&r.Form) + secret
	h := md5.New()
	io.WriteString(h, signstr)
	return fmt.Sprintf("%032X", h.Sum(nil))
}

func (me *ShopexSign) sign_str(sets *url.Values) (ret string) {
	mk := make([]string, len(*sets))
	i := 0
	for k, _ := range *sets {
		mk[i] = k
		i++
	}
	sort.Strings(mk)

	for _, k := range mk {
		for _, v := range (*sets)[k] {
			ret += v
		}
	}
	return
}
