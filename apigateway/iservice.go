package main

//"net/http"

type HttpHandler struct {
}

/*
func (this *HttpHandler) dispatch(req *http.Request) (
	api_method, new_path, api_id, prefix string,
	config *map[string]string,
	gparams *[]lib.GlobalParam, req_oauth bool) {

	var apimap *model.InstanceRouter
	var ok bool
	use_sandbox := req.URL.Path[4] != '/'
	var request_path string
	if use_sandbox {
		request_path = req.URL.Path[13:]
	} else {
		request_path = req.URL.Path[5:]
	}

	prefix, path := this.split_prefix_path(request_path)

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

	return
}

func get_value(req *http.Request, key string) (val string) {
	val = req.FormValue(key)

	if val == "" {
		panic(lib.ApiError{"0040005", key})
	}

	return val
}

func ValidateApiRequest(DomainId string, req *http.Request, nowunix int64, t *lib.TracingRecord) (*model.Key, *model.Client) {
	app_key := get_value(req, "app_key")

	key := &model.Key{}
	err := key.Load(DomainId, app_key)
	if err != nil {
		panic(lib.ApiError{"0040302", nil})
	}

	if !try_validate_params(key, req, t) {
		if !try_validate_header(key, req, t) {
			if !try_validate_sign(key, req, t, nowunix) {
				panic(lib.ApiError{"0040301", nil})
			}
		}
	}

	if (key.Expired > 0) && (key.Expired < nowunix) {
		t.Debug = "app_key has expired"
		panic(lib.ApiError{"0040303", nil})
	}

	app := &model.Client{}
	err = app.Load(DomainId, key.ClientId)

	if err != nil {
		panic(lib.ApiError{"0040305", key.ClientId})
	} else {
		if app.Status == "stop" {
			panic(lib.ApiError{"0040306", key.ClientId})
		}
	}

	return key, app
}

func try_validate_params(key *model.Key, req *http.Request, t *lib.TracingRecord) bool {
	// 暂定不该，以防影响业务
	//const https = "https"
	//if req.Header.Get("X-Forwarded-Proto") != https && req.URL.Scheme != https {
	//return false
	//}
	secret := req.FormValue("client_secret")
	if secret != "" {
		if secret == key.Secret {
			return true
		} else {
			panic(lib.ApiError{"0040304", nil})
		}
	}
	return false
}

func try_validate_sign(key *model.Key, req *http.Request, t *lib.TracingRecord, nowunix int64) bool {

	sign_time_str := req.FormValue("sign_time")
	if sign_time_str == "" {
		return false
	}

	sign_time, err := strconv.Atoi(sign_time_str)
	if err != nil {
		panic(lib.ApiError{"0040307", sign_time_str})
	}
	if nowunix-int64(sign_time) > 90000 {
		panic(lib.ApiError{"0040308", sign_time_str})
	}
	is_post := (req.Method == "POST" || req.Method == "PUT")
	if is_post && req.ContentLength > 0 && len(req.Form) == 0 {
		req.ParseForm()
	}
	sign := lib.SignFactory.Sign("prism", req, &map[string]string{"client_secret": key.Secret})
	if req.FormValue("sign") != sign {
		if t != nil {
			t.Debug = map[string]string{
				"your":    req.FormValue("sign"),
				"server":  sign,
				"signstr": (&lib.PrismSign{}).SignStr(req, "*secret*"),
			}
		}
		return false
	}
	return true
}

// try_validate_header check the basic auth
// if the username == key.Key and password == key.Secret then pass
//		Authorization: Basic xxxxxxxxx== (base64)
// but prism has own auth header like:
//		Authorization: Bearer xxxxxxxxxx (oauth token)
func try_validate_header(key *model.Key, req *http.Request, t *lib.TracingRecord) bool {
	if k, s, ok := req.BasicAuth(); ok {
		if key.Key == k && key.Secret == s {
			return true
		}
	}
	return false
}
*/
