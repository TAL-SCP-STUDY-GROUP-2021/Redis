package redis

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
)

type Client struct {
	*redis.Client
}

func CreateRedisClient() (client *Client) {
	client = new(Client)
	client.Client = redis.NewClient(&redis.Options{
		Addr:     "82.156.15.16:6379",
		Password: "gm123456", // no password set
		DB:       0,          // use default DB
	})
	res, err := client.Ping(context.Background()).Result()
	if err != nil {
		fmt.Printf("%s\n", res)
	} else {
		fmt.Printf("%s\n", err)

	}
	return client
}
