package lib

import (
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

var SignFactory *signFactory

type signFactory struct {
	Plugins map[string]SignPlugin
}

type SignPlugin interface {
	Name() string
	//RequiredConfigValues() []ConfigValue
	Sign(*http.Request, *map[string]string) string
}

func (me *signFactory) Register(name string, p SignPlugin) {
	me.Plugins[name] = p
}

func (me *signFactory) Sign(name string, req *http.Request, config *map[string]string) string {
	p, ok := me.Plugins[name]
	if !ok {
		panic(fmt.Sprintf("bad sign mode: %s", name))
	}
	return p.Sign(req, config)
}

func Init() {
	SignFactory = &signFactory{}
	SignFactory.Plugins = make(map[string]SignPlugin)
	SignFactory.Register("prism", &PrismSign{})
}

type PrismSign struct {
}

func (me *PrismSign) Name() string {
	return "Prism"
}

func (me *PrismSign) Sign(r *http.Request, config *map[string]string) string {
	secret := (*config)["client_secret"]
	signstr := SignStr(r, secret)

	fmt.Println("secret :", secret)

	h := md5.New()
	io.WriteString(h, signstr)
	return fmt.Sprintf("%032X", h.Sum(nil))
}

func SignStr(r *http.Request, secret string) string {
	query := r.URL.Query()

	post_form := &r.PostForm
	if r.MultipartForm != nil {
		post_form = (*url.Values)(&r.MultipartForm.Value)
	}

	// 有量需求:
	//   请求 url path 已 header 中 "X-Proxy-UP" 为准
	//   "X-Proxy-UP" 格式为 "/path/to/method"
	path := r.Header.Get("X-Proxy-UP")
	//	printHeaders(r.Header)
	if path == "" {
		path = r.URL.Path
	}
	signdata := []string{
		secret,
		r.Method,
		escape(path),
		escape(headers_str(&r.Header)),
		escape(Sortedstr(&query, "&", "=", "sign")),
		escape(Sortedstr(post_form, "&", "=", "sign")),
		secret,
	}
	return strings.Join(signdata, "&")
}

func escape(s string) string {
	return strings.Replace(url.QueryEscape(s), `+`, `%20`, -1)
}

// DEBUG:
func printHeaders(h http.Header) {
	str := ""
	for k, v := range h {
		for _, v1 := range v {
			str += fmt.Sprintf("%s: %s\n", k, v1)
		}
	}
	log.Printf("%s\n\n", str)
}

func headers_str(sets *http.Header) string {
	mk := make([]string, len(*sets))
	i := 0
	for k, _ := range *sets {
		if k == "Authorization" || strings.HasPrefix(k, "X-Api-") {
			mk[i] = k
			i++
		}
	}
	sort.Strings(mk)
	s := []string{}
	for _, k := range mk {
		for _, v := range (*sets)[k] {
			s = append(s, k+"="+v)
		}
	}
	return strings.Join(s, "&")
}

func Sortedstr(sets *url.Values, sep1 string, sep2 string, skip string) string {
	mk := make([]string, len(*sets))
	i := 0
	for k, _ := range *sets {
		mk[i] = k
		i++
	}
	sort.Strings(mk)

	s := []string{}

	for _, k := range mk {
		if k != skip {
			for _, v := range (*sets)[k] {
				s = append(s, k+sep2+v)
			}
		}
	}
	return strings.Join(s, sep1)
}
