package plugins

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.ishopex.cn/teegon/apigateway/lib"
	_ "github.com/go-sql-driver/mysql"
)

var oauth_api_regexp *regexp.Regexp
var oauth_api_escape_re *regexp.Regexp

func init() {
	oauth_api_regexp = regexp.MustCompile("\\$[a-z]+[a-z0-9\\_\\.]+[a-z]+")
	oauth_api_escape_re = regexp.MustCompile("[\\'\"\\`]")
}

type OAuthApi struct {
	ok          bool
	Host        string `label:"api url"`
	CfgIdColumn string `label:"Id列"`
	Key         string `label:"key"`
	Secret      string `label:"secret"`
	Meta        string `label:"meta json"`
	Response    string `label:"response key | defualt struct({'status':'success', 'data':{'xxx':'xxx'}})"`
}

//var dbconns map[string]*sql.DB
//var dbconns_lk sync.Mutex

func init() {
	dbconns = make(map[string]*sql.DB)
}

func (me *OAuthApi) OnRegister() {
	me.ok = true
}

func (me *OAuthApi) Name() string {
	return "Api"
}

func (me *OAuthApi) IdColumn() string {
	return me.CfgIdColumn
}

func (me *OAuthApi) CheckUser(params *map[string]string) (*map[string]string, error) {
	p := *params
	form := url.Values{}
	meta := make(map[string]string)
	json.Unmarshal([]byte(me.Meta), &meta)
	login_key := "loginname"
	pwd_key := "password"

	if meta["loginname"] != "" {
		login_key = meta["loginname"]
	}

	if meta["password"] != "" {
		pwd_key = meta["password"]
	}
	delete(meta, "loginname")
	delete(meta, "password")

	form.Add(login_key, p["user"])
	form.Add(pwd_key, p["password"])
	for k, v := range p {
		if strings.HasPrefix(k, "arg_") {
			form.Add(k, v)
		}
	}

	for k, v := range meta {
		form.Add(v, p[k])
	}

	ret, err := me.httpPost(me.Host, form, me.Secret)
	if err != nil {
		return nil, err
	}
	return &ret, err
}

func (me *OAuthApi) HandleEvent(params *map[string]string, err error) {
	log.Println("api oauth with error: ", err)
}

func (me *OAuthApi) CheckAccess(params *map[string]string) (*map[string]string, error) {
	attr := make(map[string]string)
	return &attr, nil
}

func (me *OAuthApi) httpPost(url string, form url.Values, secret string) (map[string]string, error) {
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		log.Println("oauthapi: ", url, err)
		return nil, err
	}

	form.Add("sign_time", strconv.Itoa(int(time.Now().Unix())))
	form.Add("app_key", me.Key)
	form.Add("sign_method", "md5")
	req.PostForm = form

	sign := lib.SignFactory.Sign("prism", req, &map[string]string{"client_secret": secret})

	req.PostForm.Set("sign", sign)
	c := http.Client{}
	rsp, err := c.PostForm(url, form)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()
	return me.httpParsed(rsp, err)
}

type apiResult struct {
	Status string
	Data   map[string]interface{}
}

func (me *OAuthApi) httpParsed(rsp *http.Response, err error) (map[string]string, error) {
	if err != nil {
		log.Println(err)
		return nil, err
	} else {
		d := apiResult{}
		body, err := ioutil.ReadAll(rsp.Body)
		if me.Response != "" {
			b1 := make(map[string]interface{})
			json.Unmarshal([]byte(body), &b1)
			body, _ = json.MarshalIndent(b1[me.Response], "", "\t")
		}

		if rsp.StatusCode == 200 {
			json.Unmarshal([]byte(body), &d)
			if d.Status == "success" {
				ret := map[string]string{}
				for k, v := range d.Data {
					switch v.(type) {
					case string:
						ret[k] = v.(string)
					case []byte:
						ret[k] = string(v.([]byte))
					case int:
						ret[k] = strconv.Itoa(v.(int))
					case nil:
						ret[k] = ""
					case float64:
						ret[k] = strconv.Itoa(int(v.(float64)))
					default:
						ret[k] = fmt.Sprintf("%s", v)
					}
				}
				return ret, err
			} else {
				return nil, errors.New(string(body))
			}
		} else {
			log.Println(string(body))
		}
		return nil, errors.New("connect failed code:" + rsp.Status)
	}
}
