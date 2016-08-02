package main

import (
	"database/sql"
	"errors"
	"fmt"

	l4g "github.com/alecthomas/log4go"
	_ "github.com/go-sql-driver/mysql"
)

type DataQuery struct {
	db *sql.DB
}

func (this *DataQuery) Init(config *Config) error {
	//1、初始化mysql
	db, err := sql.Open("mysql", config.DBDns)
	if err != nil {
		l4g.Warn("open db failed, dns:%s, error:%s", config.DBDns, err.Error())
		return err
	}
	//2、设置最大连接和空闲连接
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	if err = db.Ping(); err != nil {
		l4g.Error("db ping failed, dns:%s, error:%s", config.DBDns, err.Error())
		return err
	}

	this.db = db
	return nil
}

func (this *DataQuery) UnInit() error {
	if this.db != nil {
		this.db.Close()
	}
	return nil
}

//QueryUseServiceApp 查询当前用户下，已使用该服务的app列表
func (this *DataQuery) QueryUseServiceApp(eid string, serviceid string) (*[]AppInfo, error) {
	l4g.Info("query use service app, begin, eid:%s, serviceid:%s", eid, serviceid)
	if len(serviceid) < 0 {
		return nil, errors.New("serviceid is empty")
	}

	var rows *sql.Rows
	sql := fmt.Sprintf("select a.fd_id, a.fd_uid, a.fd_name, a.fd_status, a.fd_key, a.fd_description from t_app a , t_user tu, t_app_service tas where tu.fd_eid=\"%s\" and a.fd_uid=tu.fd_uid and a.fd_id=tas.fd_app_id and tas.fd_serviceid=\"%s\"", eid, serviceid)

	var err error

	rows, err = this.db.Query(sql)

	if err != nil {
		l4g.Warn("query use service app, query failed, sql:%s, error:", sql, err.Error())
		return nil, err
	}

	defer rows.Close()

	result := &[]AppInfo{}

	l4g.Info("query use service app, query success, sql:%s", sql)

	for rows.Next() {
		appinfo := AppInfo{}

		var uid string
		err := rows.Scan(&appinfo.AppId, &uid, &appinfo.AppName, &appinfo.Status, &appinfo.Key, &appinfo.Description)
		if err != nil {
			l4g.Warn("query use service app, scan result failed, error:%s", err.Error())
			return nil, err
		}
		*result = append(*result, appinfo)
		//list_appinfo.PushBack(appinfo)
		l4g.Debug(fmt.Sprintf("query use service app, read line, id:%d, uid:%s, name:%s, status:%d, key:%s, description:%s", appinfo.AppId, uid, appinfo.AppName, appinfo.Status, appinfo.Key, appinfo.Description))
	}

	l4g.Info(fmt.Sprintf("query use service app, end, eid:%s, serviceid:%s, service num:%d", eid, serviceid, len(*result)))
	return result, nil
}

//QueryUseServiceApp 查询应用下面可用的队列
func (this *DataQuery) QueryAppQueue(eid string, appid string) (*[]AppQueue, error) {
	l4g.Info("query app queue, begin, eid:%s, appid:%s", eid, appid)
	if len(appid) < 0 {
		return nil, errors.New("appid is empty")
	}

	var rows *sql.Rows
	sql := fmt.Sprintf("select aq.fd_id, aq.fd_queue_name, aq.fd_read, aq.fd_write, aq.fd_description from t_app_queue aq, t_user tu, t_app ta where tu.fd_eid=\"%s\" and ta.fd_id=\"%s\" and ta.fd_uid=tu.fd_uid and aq.fd_app_id=ta.fd_id", eid, appid)

	var err error

	rows, err = this.db.Query(sql)

	if err != nil {
		l4g.Warn("query app queue, query failed, sql:%s, error:", sql, err.Error())
		return nil, err
	}

	defer rows.Close()

	result := &[]AppQueue{}

	l4g.Info("query app queue, query success, sql:%s", sql)

	for rows.Next() {
		appqueue := AppQueue{}

		err := rows.Scan(&appqueue.QueueId, &appqueue.QueueName, &appqueue.Read, &appqueue.Write, &appqueue.Description)
		if err != nil {
			l4g.Warn("query app queue, scan result failed, error:" + err.Error())
			return nil, err
		}
		*result = append(*result, appqueue)
		//list_appinfo.PushBack(appinfo)
		l4g.Debug(fmt.Sprintf("query app queue, read line, id:%d, queuename:%s, read:%d, write:%d, description:%s", appqueue.QueueId, appqueue.QueueName, appqueue.Read, appqueue.Write, appqueue.Description))
	}

	l4g.Info(fmt.Sprintf("query app queue, end, eid:%s, appid:%s, service num:%d", eid, appid, len(*result)))
	return result, nil
}
