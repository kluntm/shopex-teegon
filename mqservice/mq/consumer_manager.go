package mq

import (
	"container/list"
	"sync"
	"time"

	"git.ishopex.cn/teegon/mqservice/kafka/consumergroup"
	"github.com/Shopify/sarama"
	l4g "github.com/alecthomas/log4go"
	"github.com/wvanbergen/kazoo-go"
)

type ConsumerItem struct {
	group          string    //组名
	topic          string    //队列名字
	last_time      time.Time //最后处理时间，根据时间做清理
	Consumer       *consumergroup.ConsumerGroup
	element        *list.Element
	IsCheckTimeOut bool //是否做超时检查，并自动删除
}

type ConsumerManager struct {
	group_consumer    map[string]map[string]*ConsumerItem //存储所有的Consumer对象，group=>topic=>Consumer
	consumer_list     SafeList                            //用于存储ConsumerGroup,每次处理都移动到最前， 定时清除时间最长未收发消息的
	config            *sarama.Config                      //配置信息
	zklist            string                              //
	clearConsumerTime int32                               //Consumer 清理时间，多长时间未有消息处理，则清除
	consumerMutex     sync.RWMutex                        // consumer_list读写锁
	gp_csmer_mutex    sync.RWMutex                        //group_consumer 读写锁
	delConsumerList   list.List                           //将要删除的consumer
	ticker            *time.Ticker
	redis_addr        string
}

func (this *ConsumerItem) updateLastTime() {
	this.last_time = time.Now()
}

func (this *ConsumerManager) Init(cfg *sarama.Config, zklist string, redis_addr string, clearConsumerTime int32) error {
	this.config = cfg
	this.zklist = zklist
	this.group_consumer = make(map[string]map[string]*ConsumerItem)
	this.clearConsumerTime = clearConsumerTime
	this.redis_addr = redis_addr
	this.ticker = time.NewTicker(time.Second * 5)
	go func() {
		for {
			select {
			case <-this.ticker.C:
				num := this.consumer_list.Length()
				l4g.Info("check all consumer, consumer num:%d", num)
				if this.consumer_list.Length() > 0 {
					this.consumer_list.ForEach(this.checkConsumer)
					//删除已过期的consumer
					this.delConsumer()
				}
			}
		}
	}()

	return nil
}

func (this *ConsumerManager) UnInit() error {
	this.ticker.Stop()
	this.consumer_list.ForEach(func(item *list.Element) (bEnd bool) {
		consmItem := item.Value.(*ConsumerItem)
		if consmItem != nil && consmItem.Consumer != nil {
			consmItem.Consumer.Close()
		}

		return false
	})

	this.consumer_list.Clear()
	return nil
}

func (this *ConsumerManager) createConsumer(group string, topic string) (*consumergroup.ConsumerGroup, error) {
	config := consumergroup.NewConfig()
	config.Config = this.config
	config.Offsets.Initial = sarama.OffsetNewest
	config.Offsets.ProcessingTimeout = 0
	config.Redis.Addr = this.redis_addr

	var zookeeperNodes []string
	zookeeperNodes, config.Zookeeper.Chroot = kazoo.ParseConnectionString(this.zklist)

	//kafkaTopics := strings.Split(, ",")

	consumer, consumerErr := consumergroup.JoinConsumerGroup(group, []string{topic}, zookeeperNodes, config)
	if consumerErr != nil {
		//log.Fatalln(consumerErr)
		//l4g.Warn("join consumer group:", consumerErr)
	}

	return consumer, consumerErr
}

func (this *ConsumerManager) GetConsumer(group string, topic string) (*ConsumerItem, error) {
	this.gp_csmer_mutex.RLock()
	var retcsm *ConsumerItem
	var reterr error
	if topicmap, ok := this.group_consumer[group]; ok {
		if consumer, cok := topicmap[topic]; cok {
			//return consumer, nil

			retcsm = consumer
		} else { //若通过topic 没找到consumer 则创建并保存
			if consumer, err := this.createConsumer(group, topic); err == nil {
				//解开读锁，加写锁
				this.gp_csmer_mutex.RUnlock()
				this.gp_csmer_mutex.Lock()
				defer this.gp_csmer_mutex.Unlock()
				item := &ConsumerItem{
					group:          group,
					topic:          topic,
					last_time:      time.Now(),
					Consumer:       consumer,
					IsCheckTimeOut: true,
					//element:   listitem,
				}
				listitem := this.consumer_list.PushFront(item)
				item.element = listitem
				topicmap[topic] = item
				return item, nil
				//retcsm = consumer
			} else {
				//return nil, err
				reterr = err
			}
		}
	} else { //若通过 group找不到consumer，则创建并保存到topic下面
		if consumer, err := this.createConsumer(group, topic); err == nil {
			this.gp_csmer_mutex.RUnlock()
			this.gp_csmer_mutex.Lock()
			defer this.gp_csmer_mutex.Unlock()
			topicmap = make(map[string]*ConsumerItem)
			item := &ConsumerItem{
				group:          group,
				topic:          topic,
				last_time:      time.Now(),
				Consumer:       consumer,
				IsCheckTimeOut: true,
				//element:   listitem,
			}
			listitem := this.consumer_list.PushFront(item)
			item.element = listitem

			topicmap[topic] = item
			this.group_consumer[group] = topicmap
			return item, nil
			//retcsm = consumer
		} else {
			reterr = err
		}
	}

	this.gp_csmer_mutex.RUnlock()
	return retcsm, reterr
}

func (this *ConsumerManager) MoveToBack(item *ConsumerItem) {
	item.updateLastTime()
	this.consumer_list.MoveToBack(item.element)
}

//检查consumer是否超过设定时间未有交互，超时则放入删除列表，不能再该函数删除，容易死锁
func (this *ConsumerManager) checkConsumer(item *list.Element) (bEnd bool) {
	consmItem := item.Value.(*ConsumerItem)
	curtime := time.Now()
	interval := int32(curtime.Sub(consmItem.last_time).Seconds())
	if consmItem != nil && consmItem.IsCheckTimeOut && interval >= this.clearConsumerTime {
		this.delConsumerList.PushBack(consmItem)

		return false
	} else if interval < this.clearConsumerTime {
		return true
	}

	return false
}

func (this *ConsumerManager) delConsumer() {
	curtime := time.Now()

	for e := this.delConsumerList.Front(); e != nil; {
		temp := e
		e = e.Next()
		consmItem := temp.Value.(*ConsumerItem)
		interval := int32(curtime.Sub(consmItem.last_time).Seconds())
		if consmItem.IsCheckTimeOut && interval >= this.clearConsumerTime {
			l4g.Info("delete consumer, group:%s, topic:%s, last_time:%d, cur_time:%d", consmItem.group, consmItem.topic, consmItem.last_time.Unix(), curtime.Unix())

			l4g.Info("delete consumer1, group:%s, topic:%s, last_time:%d, cur_time:%d", consmItem.group, consmItem.topic, consmItem.last_time.Unix(), curtime.Unix())
			this.consumer_list.Remove(consmItem.element)
			this.delConsumerList.Remove(temp)
			//删除consumer 对象
			this.gp_csmer_mutex.Lock()

			l4g.Info("delete consumer2, group:%s, topic:%s, last_time:%d, cur_time:%d", consmItem.group, consmItem.topic, consmItem.last_time.Unix(), curtime.Unix())

			delete(this.group_consumer[consmItem.group], consmItem.topic)
			l4g.Info("delete consumer3, group:%s, topic:%s, last_time:%d, cur_time:%d", consmItem.group, consmItem.topic, consmItem.last_time.Unix(), curtime.Unix())
			this.gp_csmer_mutex.Unlock()
			consmItem.Consumer.Close()
		}

	}
}
