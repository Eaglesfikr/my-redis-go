package main

import (
	"path/filepath"
	"sync"
	"time"
	// "honnef.co/go/tools/pattern"
)

// 内存存储 key-value 数据
var store = struct {
	sync.RWMutex
	data    map[string]string
	expires map[string]int64 // 过期时间（毫秒时间戳）
}{data: make(map[string]string), expires: make(map[string]int64)}



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

// 返回所有的 key（处理 KEYS (pattern) 命令）
func storeKeys(pattern string) []string {
	store.RLock()
	defer store.RUnlock()

	var keys []string
	for key := range store.data {
		match, _ := filepath.Match(pattern, key)
		if match {
			keys = append(keys, key)
		}
	}
	return keys
}
