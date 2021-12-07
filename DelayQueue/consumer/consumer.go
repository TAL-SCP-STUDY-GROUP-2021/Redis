package consumer

import (
	"DelayQueue/lock"
	"DelayQueue/redis"
	"bytes"
	"context"
	"fmt"
	redis2 "github.com/go-redis/redis/v8"
	"math/rand"
	"runtime"
	"time"
)

type Consumer struct {
	ctx context.Context
}

func NewConsumer() *Consumer {
	obj := new(Consumer)
	obj.ctx = context.Background()
	return obj
}
func (c *Consumer) GetMessage() {
	dao := redis.CreateRedisClient()
	defer dao.Close()
	ret, err := dao.ZRangeArgsWithScores(c.ctx, redis2.ZRangeArgs{Key: "delay-queue", Start: "(0", Stop: time.Now().Unix(), ByScore: true, Offset: 0, Count: 10}).Result()
	if err == nil && len(ret) > 0 {
		zs := make([]*redis2.Z, 0)
		for _, z := range ret {
			zs = append(zs, &z)
			dao.ZRem(c.ctx, "delay-queue", z.Member).Result()
			r, _ := dao.ZAdd(c.ctx, "delay-queue-copy", &z).Result()
			println(r)
		}
		go c.Handler(ret)
	}
}

func (c *Consumer) Handler(ret []redis2.Z) (ok bool) {
	dao := redis.CreateRedisClient()
	defer dao.Close()
	for _, msg := range ret {
		time.Sleep(time.Second)
		rand.Seed(time.Now().UnixNano())
		if rand.Int()/9 != 0 {
			fmt.Printf("success handle msg: %s\n", msg.Member)
			dao.ZRem(c.ctx, "delay-queue-copy", msg.Member).Result()
		} else {
			fmt.Printf("fail handle msg: %s\n", msg.Member)
		}
	}
	return
}
func GetGID() string {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	return string(b)
}

func consume() {
	consumer := NewConsumer()
	lock := lock.NewRedisLock()
	for {
		pid := GetGID()
		if lock.Lock(pid) {
			consumer.GetMessage()
			time.Sleep(2 * time.Second)
			lock.UnLock(pid)
		}
	}
}
