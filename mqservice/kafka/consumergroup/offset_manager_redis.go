package consumergroup

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/Shopify/sarama"
	_redis "gopkg.in/redis.v3"
)

type RedisConfig struct {
	Addr         string //ip:port
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolSize     int
}

type redisOffsetManager struct {
	config  *OffsetManagerConfig
	l       sync.RWMutex
	offsets offsetsMap
	cg      *ConsumerGroup
	redis   *_redis.Client

	closing, closed chan struct{}
}

func NewRedisConfig() *RedisConfig {
	return &RedisConfig{}
}

var GRedisClient *_redis.Client

func NewRedisClient(rconfig *RedisConfig) (*_redis.Client, error) {
	if GRedisClient != nil {
		return GRedisClient, nil
	}

	GRedisClient = _redis.NewClient(&_redis.Options{
		Addr:         rconfig.Addr,
		DialTimeout:  rconfig.DialTimeout,
		ReadTimeout:  rconfig.ReadTimeout,
		WriteTimeout: rconfig.WriteTimeout,
		PoolSize:     rconfig.PoolSize,
	})

	return GRedisClient, nil
}

// NewZookeeperOffsetManager returns an offset manager that uses Zookeeper
// to store offsets.
func NewRedisOffsetManager(cg *ConsumerGroup, config *OffsetManagerConfig) (OffsetManager, error) {
	if config == nil {
		config = NewOffsetManagerConfig()
	}

	redis, err := NewRedisClient(cg.config.Redis)
	if err != nil {
		return nil, err
	}
	zom := &redisOffsetManager{
		config:  config,
		cg:      cg,
		offsets: make(offsetsMap),
		closing: make(chan struct{}),
		closed:  make(chan struct{}),
		redis:   redis,
	}

	go zom.offsetCommitter()

	return zom, nil
}
func (zom *redisOffsetManager) FetchOffset(topic string, partition int32) (int64, error) {
	key := zom.cg.group.Name + "/" + topic
	field := fmt.Sprintf("%d", partition)
	value, err := zom.redis.HGet(key, field).Result()
	zom.cg.Logf("fetch offset, group:%s, topic:%s, partition:%d, key:%s, field:%s, offset:%v", zom.cg.group.Name, topic, partition, key, field, value)

	if err == _redis.Nil {
		return sarama.OffsetNewest, nil
	} else if err != nil {
		return sarama.OffsetNewest, nil
	} else {
		return strconv.ParseInt(value, 10, 64)
	}

}

func (zom *redisOffsetManager) CommitOffset(topic string, partition int32, offset int64) error {
	_, err := zom.redis.HSet(zom.cg.group.Name+"/"+topic, fmt.Sprintf("%d", partition), fmt.Sprintf("%d", offset)).Result()
	zom.cg.Logf("commit offset, group:%s, topic:%s, partition:%d, offset:%d", zom.cg.group.Name, topic, partition, offset)
	return err
}

func (zom *redisOffsetManager) InitializePartition(topic string, partition int32) (int64, error) {
	zom.l.Lock()
	defer zom.l.Unlock()

	if zom.offsets[topic] == nil {
		zom.offsets[topic] = make(topicOffsets)
	}

	nextOffset, err := zom.FetchOffset(topic, partition)
	if err != nil {
		return 0, err
	}

	zom.offsets[topic][partition] = &partitionOffsetTracker{
		highestProcessedOffset: nextOffset - 1,
		lastCommittedOffset:    nextOffset - 1,
		done:                   make(chan struct{}),
	}

	return nextOffset, nil
}

func (zom *redisOffsetManager) FinalizePartition(topic string, partition int32, lastOffset int64, timeout time.Duration) error {
	zom.l.RLock()
	tracker := zom.offsets[topic][partition]
	zom.l.RUnlock()

	if lastOffset >= 0 {
		if lastOffset-tracker.highestProcessedOffset > 0 {
			zom.cg.Logf("%s/%d :: Last processed offset: %d. Waiting up to %ds for another %d messages to process...", topic, partition, tracker.highestProcessedOffset, timeout/time.Second, lastOffset-tracker.highestProcessedOffset)
			if !tracker.waitForOffset(lastOffset, timeout) {
				return fmt.Errorf("TIMEOUT waiting for offset %d. Last committed offset: %d", lastOffset, tracker.lastCommittedOffset)
			}
		}

		if err := zom.commitOffset(topic, partition, tracker); err != nil {
			return fmt.Errorf("FAILED to commit offset %d to Zookeeper. Last committed offset: %d", tracker.highestProcessedOffset, tracker.lastCommittedOffset)
		}
	}

	zom.l.Lock()
	delete(zom.offsets[topic], partition)
	zom.l.Unlock()

	return nil
}

func (zom *redisOffsetManager) MarkAsProcessed(topic string, partition int32, offset int64) bool {
	zom.l.RLock()
	defer zom.l.RUnlock()
	if p, ok := zom.offsets[topic][partition]; ok {
		return p.markAsProcessed(offset)
	} else {
		return false
	}
}

func (zom *redisOffsetManager) Close() error {
	close(zom.closing)
	<-zom.closed

	zom.l.Lock()
	defer zom.l.Unlock()

	var closeError error
	for _, partitionOffsets := range zom.offsets {
		if len(partitionOffsets) > 0 {
			closeError = UncleanClose
		}
	}

	return closeError
}

func (zom *redisOffsetManager) offsetCommitter() {
	commitTicker := time.NewTicker(zom.config.CommitInterval)
	defer commitTicker.Stop()

	for {
		select {
		case <-zom.closing:
			close(zom.closed)
			return
		case <-commitTicker.C:
			zom.commitOffsets()
		}
	}
}

func (zom *redisOffsetManager) commitOffsets() error {
	zom.l.RLock()
	defer zom.l.RUnlock()

	var returnErr error
	for topic, partitionOffsets := range zom.offsets {
		for partition, offsetTracker := range partitionOffsets {
			err := zom.commitOffset(topic, partition, offsetTracker)
			switch err {
			case nil:
				// noop
			default:
				returnErr = err
			}
		}
	}
	return returnErr
}

func (zom *redisOffsetManager) commitOffset(topic string, partition int32, tracker *partitionOffsetTracker) error {
	err := tracker.commit(func(offset int64) error {
		if offset >= 0 {
			return zom.CommitOffset(topic, partition, offset+1) //zom.cg.group.CommitOffset(topic, partition, offset+1)
		} else {
			return nil
		}
	})

	if err != nil {
		zom.cg.Logf("FAILED to commit offset %d for %s/%d!", tracker.highestProcessedOffset, topic, partition)
	} else if zom.config.VerboseLogging {
		zom.cg.Logf("Committed offset %d for %s/%d!", tracker.lastCommittedOffset, topic, partition)
	}

	return err
}
