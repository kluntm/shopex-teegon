package plugins

import (
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var oauth_mysql_regexp *regexp.Regexp
var oauth_mysql_escape_re *regexp.Regexp

func init() {
	oauth_mysql_regexp = regexp.MustCompile("\\$[a-z]+[a-z0-9\\_\\.]+[a-z]+")
	oauth_mysql_escape_re = regexp.MustCompile("[\\'\"\\`]")
}

type OAuthMysql struct {
	ok              bool
	Host            string `label:"mysql host"`
	Database        string `label:"mysql database"`
	User            string `label:"mysql user"`
	Password        string `label:"mysql password"`
	CheckAccountSQL string `label:"用户认证 SQL" type:"text" params:"$user,$password,$arg.xxx"`
	CfgIdColumn     string `label:"Id列"`
	SuccessSQL      string `label:"SQL on success" type:"text" params:"$user,$user.xxx,$password,$client.ip,$client.agent,$time"`
	FailedSQL       string `label:"SQL on failed" type:"text" params:"$user,$password,$client.ip,$client.agent,$time"`
}

var dbconns map[string]*sql.DB
var dbconns_lk sync.Mutex

func init() {
	dbconns = make(map[string]*sql.DB)
}

func (me *OAuthMysql) OnRegister() {
	me.ok = true
}

func (me *OAuthMysql) Name() string {
	return "mysql"
}

func (me *OAuthMysql) IdColumn() string {
	return me.CfgIdColumn
}

func (me *OAuthMysql) CheckUser(params *map[string]string) (*map[string]string, error) {
	ret, err := me.db_select(me.CheckAccountSQL, params)
	if err != nil {
		if err.Error() == "sql: Rows are closed" {
			err = errors.New("user/password error")
		}
		return nil, err
	}
	return ret, err
}

func (me *OAuthMysql) HandleEvent(params *map[string]string, err error) {
	var sqls []string
	if err == nil {
		sqls = strings.Split(me.SuccessSQL, ";")
	} else {
		sqls = strings.Split(me.FailedSQL, ";")
	}

	db, err := me.db()

	for _, s := range sqls {
		s = me.fixed_sql(s, params)
		db.Exec(s)
	}
}

func (me *OAuthMysql) db() (db *sql.DB, err error) {
	connstr := me.User + ":" + me.Password + "@tcp(" + me.Host + ")/" + me.Database
	var ok bool
	if db, ok = dbconns[connstr]; !ok {
		db, err = sql.Open("mysql", connstr)
		db.SetMaxIdleConns(100)

		if err == nil {
			dbconns_lk.Lock()
			if db2, ok := dbconns[connstr]; !ok {
				dbconns[connstr] = db
			} else {
				defer db.Close()
				db = db2
			}
			dbconns_lk.Unlock()
		}
	}
	return db, err
}

func (me *OAuthMysql) fixed_sql(s_sql string, params *map[string]string) string {
	return oauth_mysql_regexp.ReplaceAllStringFunc(s_sql, func(s string) string {
		v, ok := (*params)[s[1:]]
		if ok {
			return oauth_mysql_escape_re.ReplaceAllString(v, "\\$0")
		} else {
			return ""
		}
	})
}

func (me *OAuthMysql) db_select(s_sql string, params *map[string]string) (*map[string]string, error) {
	db, err := me.db()

	if err != nil {
		return nil, err
	}

	s_sql = me.fixed_sql(s_sql, params)

	rows, err := db.Query(s_sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rows.Next()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	valmap := make([]interface{}, len(columns))
	values := make([]interface{}, len(columns))
	for i, _ := range columns {
		values[i] = &valmap[i]
	}

	err = rows.Scan(values...)
	ret := map[string]string{}

	for i, k := range columns {
		switch v := valmap[i].(type) {
		case string:
			ret[k] = v
		case []byte:
			ret[k] = string(v)
		case int:
			ret[k] = strconv.Itoa(v)
		case nil:
			ret[k] = ""
		default:
			ret[k] = fmt.Sprintf("%s", v)
		}
	}

	return &ret, err
}

func (me *OAuthMysql) CheckAccess(params *map[string]string) (*map[string]string, error) {
	attr := make(map[string]string)
	return &attr, nil
}
