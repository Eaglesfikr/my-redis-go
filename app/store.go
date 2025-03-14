package main

import (
	"sync"
	"time"
)

// 内存存储 key-value 数据
var store = struct {
	sync.RWMutex
	data    map[string]string
	expires map[string]int64 // 过期时间（毫秒时间戳）
}{data: make(map[string]string), expires: make(map[string]int64)}

// RDB 相关配置
var rdbConfig = struct {
	sync.RWMutex
	dir        string
	dbfilename string
}{
	dir:        "./data", // 默认存储路径
	dbfilename: "dump.rdb",     // 默认 RDB 文件名
}

// 设置 key-value，并处理过期时间
func storeSet(key, value string, ttl int64) {
	store.Lock()
	store.data[key] = value
	if ttl > 0 {
		store.expires[key] = ttl
	} else {
		delete(store.expires, key) // 确保无 PX 参数时删除可能的旧过期时间
	}
	store.Unlock()
}

// 获取 key 的值（考虑过期情况）
func storeGet(key string) (string, bool) {
	store.RLock()
	value, exists := store.data[key]
	expireTime, hasExpiry := store.expires[key]
	store.RUnlock()

	if exists {
		if hasExpiry && time.Now().UnixNano()/1e6 >= expireTime {
			// 密钥已过期
			storeDelete(key)
			return "", false
		}
		return value, true
	}
	return "", false
}

// 删除 key
func storeDelete(key string) {
	store.Lock()
	delete(store.data, key)
	delete(store.expires, key)
	store.Unlock()
}
