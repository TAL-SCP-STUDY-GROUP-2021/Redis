# Redis key 过期机制

## 过期时间设置与查看

自 2.0.0 版本开始，Redis 提供了 `SETEX` 指令：

```shell
SETEX key seconds value
```

该指令是对 `SET` 指令的扩充，在为 key 设置 value 的同时，它的第二个参数为一个 int 变量，表明这个 key 的以秒为单位的过期时间。`SETEX` 指令与 `SET` 和 `EXPIRE` 两条指令的联合使用是等价的：

```shell
SET key value
EXPIRE key seconds
```

当设置带过期时间的 k/v 成功时 `SETEX` 返回 `"OK"`；否则返回错误信息；一个 key 过期后，它会被删除。

```shell
127.0.0.1:6379> SETEX test 5.5 hahaha
(error) ERR value is not an integer or out of range
127.0.0.1:6379> SETEX test 10 hahaha
OK
127.0.0.1:6379> GET test	# 10秒后
(nil)
```

查看某一个 key 的有效时间的指令是 `TTL`，ttl 是 time to live 的缩写，它能够查询具有实效性的 key 的剩余生存时间。这种内省功能允许 Redis 客户端检查给定 key 将继续成为数据集一部分的秒数。自从 2.8 版本后，指令的返回值的含义为：

-   当查询的 key 不存在时返回 `-2`；
-   当查询的 key 存在但并未关联过期时间则返回 `-1`；
-   当查询的 key 存在且关联了过期时间则返回 key 的剩余寿命，以秒为单位。

```shell
127.0.0.1:6379> SETEX test 10 Hello
OK
127.0.0.1:6379> TTL test
(integer) 7
127.0.0.1:6379> TTL test
(integer) 5
127.0.0.1:6379> TTL test
(integer) 3
127.0.0.1:6379> TTL test
(integer) -2
```

如果想要以更高的精度来查询 key 的寿命，则可以使用 `PTTL` 指令（高于 2.6 的版本）。它具有相似的用法，区别仅在于返回值是以毫秒为单位的。

## 过期机制

### SETEX 的实现

`SETEX` 在 `src/t_string.c` 中

```c
void setexCommand(client *c) {
    c->argv[3] = tryObjectEncoding(c->argv[3]);
    // 标识位OBJ_EX表示以秒为单位传递时间参数
    setGenericCommand(c,OBJ_EX,c->argv[1],c->argv[3],c->argv[2],UNIT_SECONDS,NULL,NULL);
}

// 同在src/t_string.c
void setGenericCommand(client *c, int flags, robj *key, robj *val, robj *expire, int unit, robj *ok_reply, robj *abort_reply) {
    long long milliseconds = 0; /* initialized to avoid any harmness warning */
    // 检查expire参数的合法性；
    // 合法则转换为精度为毫秒的到期unix时间戳，否则返回错误码
    if (expire && getExpireMillisecondsOrReply(c, expire, flags, unit, &milliseconds) != C_OK) {
        return;
    }
    
    // ...

    // 先调用通用SET函数创建这个k/v
    genericSetKey(c,c->db,key, val,flags & OBJ_KEEPTTL,1);
    
    // ...

    if (expire) {	// 如果设定过期时间不为空的话则为这个key设定过期时间
        setExpire(c,c->db,key,milliseconds);
       // ...
    }
    
    // ...
}
```

### EXPIRE 指令的实现

`EXPIRE` 指令的实现位于 `src/expire.c` 中

```c
/* EXPIRE key seconds */
void expireCommand(client *c) {
    expireGenericCommand(c,mstime(),UNIT_SECONDS);
}

void expireGenericCommand(client *c, long long basetime, int unit) {
    robj *key = c->argv[1], *param = c->argv[2];
    long long when; // 精度为毫秒的过期unix时间
    
    // ...

    // 计算毫秒精度的unix时间
    if (getLongLongFromObjectOrReply(c, param, &when, NULL) != C_OK)
        return;
	// ...
    
    // 加到当前时间上，获得key的到期时间戳
    when += basetime;
    
    // ...

    // 检查key是否超期
    if (checkAlreadyExpired(when)) {
        // 超期则删除
        robj *aux;

        int deleted = server.lazyfree_lazy_expire ? dbAsyncDelete(c->db,key) :
                                                    dbSyncDelete(c->db,key);
        serverAssertWithInfo(c,key,deleted);
        server.dirty++;

        /* Replicate/AOF this as an explicit DEL or UNLINK. */
        aux = server.lazyfree_lazy_expire ? shared.unlink : shared.del;
        rewriteClientCommandVector(c,2,aux,key);
        signalModifiedKey(c,c->db,key);
        notifyKeyspaceEvent(NOTIFY_GENERIC,"del",key,c->db->id);
        addReply(c, shared.cone);
        return;
    } else {
        // 未超期则设置超时时间
        setExpire(c,c->db,key,when);
        // ...
    }
}
```

### 设置过期时间的机制

通过 `SETEX` 和 `EXPIRE` 两个指令的代码可以看到，它们为 key 设置过期时间的方式都是通过调用 `setExpire()` 函数来实现的，这个函数位于 `src/db.c` 中，它也是两个指令的关键所在。

```c
void setExpire(client *c, redisDb *db, robj *key, long long when) {
    dictEntry *kde, *de;

    /* Reuse the sds from the main dict in the expire dict */
    kde = dictFind(db->dict,key->ptr);
    serverAssertWithInfo(NULL,key,kde != NULL);
    de = dictAddOrFind(db->expires,dictGetKey(kde));
    dictSetSignedIntegerVal(de,when);

    int writable_slave = server.masterhost && server.repl_slave_ro == 0;
    if (c && writable_slave && !(c->flags & CLIENT_MASTER))
        rememberSlaveKeyWithExpire(db,key);
}
```

 `setExpire()` 首先在 `db->dict` 中查找 key，确认其存在于 db 的 key 空间；接着在 `db->expires` 中查找或新增 hash entry；最后调用 `dictSetSignedIntegerVal()` 为对应的 hash entry 设置过期时间。

`dictAddOrFind()` 函数定义于 `src/dict.c` 中，它返回指定 key 在 db 中的 hash entry。

```c
dictEntry *dictAddOrFind(dict *d, void *key) {
    dictEntry *entry, *existing;
    entry = dictAddRaw(d,key,&existing);
    return entry ? entry : existing;
}
```

`dictSetSignedIntegerVal` 是一个宏，它定义于 `src/dict.h`；它操作的对象是一个 `dictEntry`，这一数据结构定义于同一文件。

```c
#define dictSetSignedIntegerVal(entry, _val_) do { (entry)->v.s64 = _val_; } while(0)

typedef struct dictEntry {
    void *key;
    union {
        void *val;
        uint64_t u64;
        int64_t s64;
        double d;
    } v;
    struct dictEntry *next;     /* Next entry in the same hash bucket. */
    void *metadata[];           /* An arbitrary number of bytes (starting at a
                                 * pointer-aligned address) of size as returned
                                 * by dictType's dictEntryMetadataBytes(). */
} dictEntry;
```

Redis 使用一个字典来维护 key 的有效期信息，字典的 key 与数据的 key 一致，val 是毫秒精度的到期时间 unix 时间戳。

### TTL 的实现

查看 key 的剩余有效时间的指令 `TTL` 定义于 `src/expire.c` 中

```c
/* TTL key */
void ttlCommand(client *c) {
    ttlGenericCommand(c, 0, 0);
}

void ttlGenericCommand(client *c, int output_ms, int output_abs) {
    long long expire, ttl = -1;
    
    // ...

	// 获取过期时间
    expire = getExpire(c->db,c->argv[1]);
    
    // ...
}
```

它的核心功能——查询 key 的过期时间——通过调用 `getExpire()` 函数实现，后者定义于 `src/db.c` 

```c
long long getExpire(redisDb *db, robj *key) {
    dictEntry *de;

    /* No expire? return ASAP */
    if (dictSize(db->expires) == 0 ||
       (de = dictFind(db->expires,key->ptr)) == NULL) return -1;

    // ...
}
```

查询的方式是在字典 `db->expires` 中读取对应 key 的过期时间。

### Redis 的过期策略

在指令 `EXPIRE` 的代码中可以看到，调用该指令时，如果 key 过期了则会删除它。那么如果一个 key 过期了，但客户端没有显式地执行 `EXPIRE` 指令，Redis 是如何清理过期 key 的呢？

根据 `EXPIRE` 指令的 [官方文档](https://redis.io/commands/expire#how-redis-expires-keys) 可以得知:

>   ##  How Redis expires keys
>
>   Redis keys are expired in two ways: a passive way, and an active way.
>
>   A key is passively expired simply when some client tries to access it, and the key is found to be timed out.
>
>   Of course this is not enough as there are expired keys that will never be accessed again. These keys should be expired anyway, so periodically Redis tests a few keys at random among keys with an expire set. All the keys that are already expired are deleted from the keyspace.
>
>   Specifically this is what Redis does 10 times per second:
>
>   1.  Test 20 random keys from the set of keys with an associated expire.
>   2.  Delete all the keys found expired.
>   3.  If more than 25% of keys were expired, start again from step 1.
>
>   This is a trivial probabilistic algorithm, basically the assumption is that our sample is representative of the whole key space, and we continue to expire until the percentage of keys that are likely to be expired is under 25%
>
>   This means that at any given moment the maximum amount of keys already expired that are using memory is at max equal to max amount of write operations per second divided by 4.

过期 key 的清理工作是 Redis 的一项定时任务来完成的。

```c
int serverCron(struct aeEventLoop *eventLoop, long long id, void *clientData) {
	// ...
    /* Redis 数据库的后台操作 */
    databasesCron();
	// ...
}

void databasesCron(void) {
    /* Expire keys by random sampling. Not required for slaves
     * as master will synthesize DELs for us. */
    // 通过对key进行随机取样进行清理
    // 只有主节点需要执行此函数，主节点会把DEL同步给从节点
    if (server.active_expire_enabled) {
        if (iAmMaster()) {
            activeExpireCycle(ACTIVE_EXPIRE_CYCLE_SLOW);
        } else {
            expireSlaveKeys();
        }
    }
}
```

