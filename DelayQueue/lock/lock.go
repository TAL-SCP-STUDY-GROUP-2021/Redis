package lock

import (
	"DelayQueue/redis"
	"context"
	"fmt"
)

var lockLua = "local lockKey = KEYS[1]  local lockValue = KEYS[2]  local result_1 = redis.call('SETNX', lockKey, lockValue)  if result_1 == 1    then    local result_4 = redis.call('SETEX', lockKey,30, lockValue)    if (result_4)      then return 1    else      return 0    end  else    local result_3= redis.call('GET', lockKey)    if (result_3==lockValue)      then      local result_5 = redis.call('SETEX', lockKey,30, lockValue)      if (result_5)        then return 1      else        return 0      end    else      return 0    end  end"

var unLockLua = "local lockKey = KEYS[1] " +
	"local lockValue = KEYS[2] " +
	"local result_0 = redis.call('get', lockKey) " +
	"if result_0 == lockValue " +
	"then " +
	"redis.call('DEL',lockKey) " +
	"return 1 " +
	"else " +
	"return 0 " +
	"end"

type RedisLock struct {
	client *redis.Client
	ctx    context.Context
}

func NewRedisLock() *RedisLock {
	obj := new(RedisLock)
	obj.ctx = context.Background()
	return obj
}
func (r *RedisLock) Lock(uuid string) (ok bool) {
	r.client = redis.CreateRedisClient()
	defer r.client.Close()
	val, _ := r.client.Eval(r.ctx, lockLua, []string{"lock", uuid}).Int()
	if val == 1 {
		fmt.Printf("%s get lock ", uuid)
		ok = true
		return
	}
	return
}
func (r *RedisLock) UnLock(uuid string) (ok bool) {
	r.client = redis.CreateRedisClient()
	defer r.client.Close()
	//v,err:=r.client.Get(r.ctx,"lock").Result()
	//if err != nil {
	//	fmt.Printf("del lock err:%s ", err.Error())
	//}
	//if v==uuid{
	//	val,_:=r.client.Del(r.ctx,"lock").Result()
	//	if val==1{fmt.Printf("%s unlock ", uuid)}
	//}
	val, _ := r.client.Eval(r.ctx, unLockLua, []string{"lock", uuid}).Int()
	if val == 1 {
		fmt.Printf("%s unlock ", uuid)
		ok = true
	}
	return
}
