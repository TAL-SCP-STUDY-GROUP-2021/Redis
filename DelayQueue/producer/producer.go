package producer

import (
	"DelayQueue/redis"
	"context"
	"encoding/json"
	"fmt"
	redis2 "github.com/go-redis/redis/v8"
	"math/rand"
	"time"
)

type Producer struct {
	delayTime time.Duration
	ctx       context.Context
}

func NewProducer(delayTime time.Duration) *Producer {
	obj := new(Producer)
	obj.delayTime = delayTime
	obj.ctx = context.Background()
	return obj
}

type Task struct {
	ID      int    `json:"id"`
	Content string `json:"content"`
}

func (p *Producer) Produce(taskId int, content string) {
	task := Task{
		ID:      taskId,
		Content: content,
	}
	jsonTask, err := json.Marshal(task)
	if err != nil { //
		println(err)
	}
	dao := redis.CreateRedisClient()
	executionTime := time.Now().Add(p.delayTime).Unix()
	ret, err := dao.ZAdd(p.ctx, "delay-queue", &redis2.Z{
		Score:  float64(executionTime),
		Member: string(jsonTask),
	}).Result()
	if err == nil && ret == 1 {
		fmt.Printf("send msg success,msg: %s \n", string(jsonTask))
	}
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
var n = 12

func GenerateSubId() string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.New(rand.NewSource(time.Now().UnixNano())).Intn(len(letterRunes))]
	}
	return string(b)
}

func Production() {
	p := NewProducer(30 * time.Second)
	for {
		rand.Seed(time.Now().UnixNano())
		p.Produce(rand.Int(), GenerateSubId())
		time.Sleep(time.Second)
		if rand.Int()%9 == 0 {
			time.Sleep(2 * time.Second)
		}
	}
}
