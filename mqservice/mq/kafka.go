package mq

import (
	"container/list"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	msg "git.ishopex.cn/teegon/mqservice/message"
	"github.com/Shopify/sarama"
	l4g "github.com/alecthomas/log4go"
	//	"github.com/wvanbergen/kafka/consumergroup"
	//kazoo "github.com/wvanbergen/kazoo-go"
)

type KafkaConsumerConfig struct {
	ReturnError   bool  `json:"ret_error,omitempty"`
	DeleteTimeOut int32 `json:"delete_timeout,omitempty"` //consumer 删除间隔 单位秒，多长时间未使用则删除,
}

type KafkaProducerConfig struct {
	ReturnError     bool `json:"ret_error,omitempty"`
	ReturnSuccess   bool `json:"ret_success,omitempty"`
	MaxMessageBytes int  `json:"max_msg_byte,omitempty"`
}

type KafkaNetConfig struct {
	DialTimeout     int `json:"dial_timeout,omitempty"`
	ReadTimeout     int `json:"read_timeout,omitempty"`
	WriteTimeout    int `json:"write_timeout,omitempty"`
	KeepAlive       int `json:"keep_alive,omitempty"`
	MaxOpenRequests int `json:"max_open_request,omitempty"`
}

type KafkaConfig struct {
	Brokers  string              `json:"brokers,omitempty"`
	Consumer KafkaConsumerConfig `json:"consumer,omitempty"`
	Producer KafkaProducerConfig `json:"producer,omitempty"`
	Net      KafkaNetConfig      `json:"net,omitempty"`
}

type SubMsgChan struct {
	num     uint32
	msgChan chan *msg.MsgData
	exitCh  chan bool
}

type Kafka struct {
	consumer       sarama.Consumer
	producerPool   *sync.Pool
	consumerPool   *sync.Pool
	config         *sarama.Config
	BrokerAddrs    []string
	brokerClient   *sarama.Client
	zklist         string
	group_consumer map[string]map[string]*list.Element //存储所有的Consumer对象，group=>topic=>Consumer
	consumerMgr    ConsumerManager
	subMsgChan     map[string]map[string]*SubMsgChan
	subMsgMutex    sync.RWMutex
}

func (this *Kafka) Init(config interface{}, raddr string, zklist string) error {
	sarama.Logger = log.New(os.Stderr, "[sarama] ", log.LstdFlags)
	cfg := config.(KafkaConfig)

	this.zklist = zklist

	this.config = this.initConfig(cfg)
	this.BrokerAddrs = strings.Split(cfg.Brokers, ",")
	l4g.Info("kafka init, broker list:%s", this.BrokerAddrs)
	//sarama.NewSyncProducer()
	this.producerPool = &sync.Pool{}
	this.consumerPool = &sync.Pool{}
	this.consumerMgr.Init(this.config, this.zklist, raddr, cfg.Consumer.DeleteTimeOut)

	this.subMsgChan = make(map[string]map[string]*SubMsgChan)

	this.producerPool.New = func() interface{} {
		l4g.Info("get producer")
		producer, err := sarama.NewSyncProducer(this.BrokerAddrs, this.config)
		if err == nil {
			return producer
		} else {
			l4g.Warn("new producer failed, error:%s", err.Error())
			return nil
		}
	}

	this.consumerPool.New = func() interface{} {
		consumer, err := sarama.NewConsumer(this.BrokerAddrs, this.config)
		if err == nil {
			return consumer
		} else {
			return nil
		}
	}

	//	if client, err := sarama.NewClient(this.BrokerAddrs, this.config); err == nil {
	//		this.brokerClient = &client
	//	} else {
	//		l4g.Warn("new kafka client failed, error:%s", err.Error())
	//		return err
	//	}
	return nil
}

func (this *Kafka) initConfig(cfg KafkaConfig) *sarama.Config {
	this.config = sarama.NewConfig()
	//	this.config.Consumer.Return.Errors = cfg.Consumer.ReturnError

	//this.config.Producer.Partitioner = sarama.NewManualPartitioner //发送消息时分区的计算方式
	this.config.Producer.Return.Errors = cfg.Producer.ReturnError
	this.config.Producer.Return.Successes = cfg.Producer.ReturnSuccess
	this.config.Producer.Compression = sarama.CompressionSnappy
	this.config.Producer.MaxMessageBytes = cfg.Producer.MaxMessageBytes
	this.config.Producer.RequiredAcks = sarama.WaitForAll

	this.config.Net.DialTimeout = time.Second * time.Duration(cfg.Net.DialTimeout)
	this.config.Net.ReadTimeout = time.Second * time.Duration(cfg.Net.ReadTimeout)
	this.config.Net.WriteTimeout = time.Second * time.Duration(cfg.Net.WriteTimeout)
	this.config.Net.KeepAlive = time.Second * time.Duration(cfg.Net.KeepAlive)
	this.config.Net.MaxOpenRequests = cfg.Net.MaxOpenRequests

	return this.config
}

func (this *Kafka) getProducer(topic string) (sarama.SyncProducer, error) {

	pr := this.producerPool.Get()
	if pr == nil {
		return nil, errors.New("get producer failed!")
	}
	return pr.(sarama.SyncProducer), nil

}

//创建队列接口，go开发包不支持创建队列，故先向队列写入数据，创建好队列在从队列将数据读出以此创建
func (this *Kafka) Create(qname string) error {
	l4g.Info("create queue, topic:%s", qname)
	if _, _, err := this.Write(qname, "hello world!", ""); err != nil {
		l4g.Warn("create queue, write to queue failed, error:%s", err.Error())
		return err
	}

	l4g.Info("create queue, queue create success, topic:%s", qname)
	rdata, err := this.Read("", qname, true, 1)
	if err != nil {
		l4g.Warn("create queue, read from queue failed, error:%s", err.Error())
	}

	if len(*rdata) <= 0 {
		l4g.Warn("create queue, read from queue failed, read empty")
	}

	return nil
}

func (this *Kafka) Write(topic string, value string, key string) (partition int32, offset int64, err error) {
	if producer, err := this.getProducer(topic); err == nil {
		defer this.Close(producer, "producer")
		message := &sarama.ProducerMessage{
			Key:   sarama.StringEncoder(key),
			Value: sarama.StringEncoder(value),
			Topic: topic,
			//Partition: 3,
		}

		return producer.SendMessage(message)
	} else {
		return 0, 0, err
	}

	return 0, 0, nil
}

func (me *Kafka) getPartition(topic string) int32 {
	return 0
}

func (this *Kafka) getOffset(topic string, partition int32) int64 {
	broker := sarama.NewBroker(this.BrokerAddrs[0])
	err := broker.Open(nil)
	if err != nil {
		return sarama.OffsetOldest
	}

	request := &sarama.OffsetFetchRequest{}
	request.AddPartition(topic, partition)
	//request.AddBlock(topic, partition, sarama.ReceiveTime, -1)

	defer broker.Close()

	if response, err := broker.FetchOffset(request); err == nil {

		l4g.Info("recv offset succes:", response.GetBlock(topic, partition).Offset)
		return response.GetBlock(topic, partition).Offset + 1
	} else {
		l4g.Warn("GetAvailableOffsets  " + err.Error())
	}

	return sarama.OffsetOldest
}

func (this *Kafka) getGroupName(group string, topic string) string {
	if len(group) <= 0 {
		md5Ctx := md5.New()
		md5Ctx.Write([]byte(topic))
		cipherStr := md5Ctx.Sum(nil)
		str_md5 := hex.EncodeToString(cipherStr)
		group = "cg_" + topic + string(str_md5[len(str_md5)-2])
	}

	return group
}

func (this *Kafka) Read(group string, topic string, drop bool, num int) (*[]msg.MsgData, error) {
	result := &[]msg.MsgData{}
	consumerItem, err := this.consumerMgr.GetConsumer(this.getGroupName(group, topic), topic)
	if err != nil {
		return nil, err
	}
	defer this.consumerMgr.MoveToBack(consumerItem)
	consumer := consumerItem.Consumer
	t := time.Tick(time.Second * 1)
FOR:
	for {
		select {
		case message := <-consumer.Messages():
			if message == nil {
				break FOR
			}
			r := msg.MsgData{
				Group:     group,
				Topic:     topic,
				Offset:    message.Offset,
				Data:      string(message.Value),
				Partition: message.Partition,
			}
			*result = append(*result, r)
			num = num - 1
			if drop {
				consumer.CommitUpto(message)
			}

			if num == 0 {
				break FOR
			}
		case <-t:
			break FOR
		}
	}

	return result, err
}

func (this *Kafka) Subscribe(group string, topic string) (<-chan *msg.MsgData, error) {
	consumerItem, err := this.consumerMgr.GetConsumer(this.getGroupName(group, topic), topic)
	if err != nil {
		return nil, err
	}
	//此处需要设置consumer不删除
	//defer this.consumerMgr.MoveToBack(consumerItem)
	consumerItem.IsCheckTimeOut = false
	consumer := consumerItem.Consumer
	subch, num := this.getSubMsgChan(group, topic)
	l4g.Info("Subscribe:%d", num)
	if num > 1 { //大于1说明该队列，不止一个用户在订阅，则直接返回chan，不在创建go程
		return subch.msgChan, nil
	}
	go func() {
		for {
			select {
			case message := <-consumer.Messages():
				l4g.Info("Kafka sub read msg", message)
				if message == nil {
					return
				}
				msg := &msg.MsgData{
					Group:     group,
					Topic:     topic,
					Offset:    message.Offset,
					Data:      string(message.Value),
					Partition: message.Partition,
				}

				subch.msgChan <- msg
			case <-subch.exitCh:
				l4g.Info("exit Subscribe, group:%s, topic:%s", group, topic)
				return
			}
		}
	}()

	return subch.msgChan, nil
}

func (this *Kafka) UnSubscribe(group string, topic string) error {
	this.delSubMsgChan(group, topic)

	consumerItem, err := this.consumerMgr.GetConsumer(this.getGroupName(group, topic), topic)
	if err != nil {
		return err
	}
	//此处需要设置consumer删除
	consumerItem.IsCheckTimeOut = true
	defer this.consumerMgr.MoveToBack(consumerItem)

	return nil
}

func (this *Kafka) SetOffset(group string, topic string, partition int32, offset int64) error {
	consumerItem, err := this.consumerMgr.GetConsumer(this.getGroupName(group, topic), topic)
	if err != nil {
		return err
	}
	defer this.consumerMgr.MoveToBack(consumerItem)
	consumer := consumerItem.Consumer

	msg := sarama.ConsumerMessage{
		Topic:     topic,
		Partition: partition,
		Offset:    offset,
	}
	return consumer.CommitUpto(&msg)
}

func (this *Kafka) Purge(qname string) error {
	return nil
}

func (this *Kafka) Status(qname string) string {
	return ""
}

func (this *Kafka) Close(pool interface{}, t string) {
	switch t {
	case "consume":
		this.consumerPool.Put(pool)
		//KafkaIncrConsumed(topic, p, -1)
	case "producer":
		this.producerPool.Put(pool)
		//		for t, p := range me.producers {
		//			//delete(me.producers, t)
		//			this.producerPool.Put(p)
		//		}
	}
}

func (this *Kafka) getSubMsgChan(group string, topic string) (*SubMsgChan, uint32) {
	this.subMsgMutex.Lock()
	defer this.subMsgMutex.Unlock()

	if mptopic, ok := this.subMsgChan[group]; ok {
		if ch, chok := mptopic[topic]; chok {
			ch.num = ch.num + 1
			return ch, ch.num
		} else {

			sch := &SubMsgChan{
				num:     1,
				msgChan: make(chan *msg.MsgData),
				exitCh:  make(chan bool),
			}
			mptopic[topic] = sch
			return sch, 1
		}
	} else {
		mptopic := make(map[string]*SubMsgChan)
		sch := &SubMsgChan{
			num:     1,
			msgChan: make(chan *msg.MsgData),
			exitCh:  make(chan bool),
		}
		mptopic[topic] = sch
		this.subMsgChan[group] = mptopic
		return sch, 1
	}
	return nil, 0
}

func (this *Kafka) delSubMsgChan(group string, topic string) {
	this.subMsgMutex.Lock()
	defer this.subMsgMutex.Unlock()
	if mptopic, ok := this.subMsgChan[group]; ok {
		if ch, chok := mptopic[topic]; chok {
			ch.num = ch.num - 1
			l4g.Info("ch.num:%d", ch.num)
			if ch.num <= 0 {
				l4g.Info("delSubMsgChan")
				close(ch.exitCh)
				close(ch.msgChan)
				delete(mptopic, topic)
				l4g.Info(this.subMsgChan)
			}
		}
	}
}
