package mq

import (
	"container/list"
)

type IMsgQueue interface {
	Init(config interface{}) error         //初始化队列
	Create(qname string) error             //创建队列
	Drop(qname string) error               // 删除队列
	Write(qname, data string) error        //写数据
	Read(qname string, num int) *list.List //读数据
	Purge(qname string) error              // 清空队列
	Status(qname string) string            //查询队列状态
}
