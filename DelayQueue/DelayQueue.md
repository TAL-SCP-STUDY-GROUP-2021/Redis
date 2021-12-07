# 延时队列
## 应用场景
* 订单下单之后，支付场景。订单下单过程中锁库存，如果超过一定时间未支付，需要释放库存同时对订单操作，使订单流程闭合
## 需求
* 消息持久化
* 失败可重试
* 消息至少可以被消费一次
* 不重复消费
## Redis实现延时队列方法
使用sortedset，以任务需要被消费时间戳作为score。生产者在生产消息的时候，以当前时间+延时时间作为消息的score。消费者在消费消息的时候，拉取小于当前时间的消息进行消费。

<img src="DelayQueue.assets/image-20211208001644671.png" alt="image-20211208001644671" style="zoom:50%;" />

### 实现

#### 生产者

~~~go
func (p *Producer) Produce(taskId int,content string)  {
	task:=Task{
		ID: taskId,
		Content: content,
	}
	jsonTask,err:=json.Marshal(task)
	if err != nil {
		println(err)
	}
	dao:=redis.CreateRedisClient()
	executionTime:=time.Now().Add(p.delayTime).Unix()
	ret,err:=dao.ZAdd(p.ctx,"delay-queue",&redis2.Z{
		Score: float64(executionTime),//任务目标执行时间
		Member: string(jsonTask),//任务
		}).Result()
	if err==nil&&ret==1{
		fmt.Printf("send msg success,msg: %s \n", string(jsonTask))
	}
}
~~~

#### 消费者

~~~go
func (c *Consumer) GetMessage() {
	dao := redis.CreateRedisClient()
	defer dao.Close()
	ret, err := dao.ZRangeArgsWithScores(c.ctx, redis2.ZRangeArgs{Key: "delay-queue", Start: "(0", Stop: time.Now().Unix(), ByScore: true, Offset: 0, Count: 10}).Result()
	if err == nil && len(ret) > 0 {
		zs := make([]*redis2.Z, 0)
		for _, z := range ret {
			zs = append(zs, &z)
			dao.ZRem(c.ctx,"delay-queue",z.Member).Result()
			r,_:=dao.ZAdd(c.ctx,"delay-queue-copy",&z).Result()
			println(r)
		}
		go c.Handler(ret)//异步处理消息
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
			dao.ZRem(c.ctx, "delay-queue-copy", msg.Member).Result()//删除消息
		} else {
			fmt.Printf("fail handle msg: %s\n", msg.Member)
		}
	}
	return
}
~~~

### 多消费者问题

如果同时存在多个消费者，为保证消息仅可以被一个消费者拉取，需要使用分布式锁，保证同一消息同一时刻只会被消费者拉取消费。分布式锁实现的方式有很多，这里同样使用redis实现分布式锁。

分布式锁需要具备的能力：

* 在分布式系统环境下，一个方法在同一时间只能被一个机器的一个线程执行；
* 高可用的获取锁与释放锁； 
* 高性能的获取锁与释放锁；
* 具备可重入特性；
* 具备锁失效机制，防止死锁； 
* 具备非阻塞锁特性，即没有获取到锁将直接返回获取锁失败。

redis的两个命令`setnx`和`setex`:

* setnx  key value   key如果不存在则设置成功返回1，否则返回0
* Setex key value time  设置过期时间

`setnx`保证锁同一时间仅可以被一个实例拿到
`setex`保证锁有过期时间，防止出现消费者宕机后出现死锁

为保证加锁及解锁过程的原子性，采用lua脚本实现加锁及解锁的过程。

**实现**

加锁：

~~~lua
local lockKey = KEYS[1] 
local lockValue = KEYS[2] 
local result_1 = redis.call('SETNX', lockKey, lockValue) 
if result_1 == 1 //判断lock是否已被占用
  then 
  local result_4 = redis.call('SETEX', lockKey,30, lockValue) //占用锁后加过期时间，防止死锁
  if (result_4) 
    then return 1 
  else 
    return 0 
  end 
else 
  local result_3= redis.call('GET', lockKey) //获得当前锁的持有者
  if (result_3==lockValue) //判断锁是否由自己持有
    then 
    local result_5 = redis.call('SETEX', lockKey,30, lockValue) //重入锁，并更新过期时间
    if (result_5) 
      then return 1 
    else 
      return 0 
    end 
  else 
    return 0 
  end 
end
~~~

解锁：

~~~lua
local lockKey = KEYS[1]
local lockValue = KEYS[2]
local result_0 = redis.call('get', lockKey)
if result_0 == lockValue //判断当前锁是否由自己持有
  then
  redis.call('DEL',lockKey) //删除锁
  return 1
else
  return 0
end
~~~



