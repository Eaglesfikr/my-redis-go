package main

import (
	// "fmt"
	"path/filepath"
	"sync"
	"time"
	// "honnef.co/go/tools/pattern"
	// "errors"
	"strconv"
	"strings"
	"fmt"
)

// 内存存储 key-value 数据
var store = struct {
	sync.RWMutex
	data    map[string]string
	expires map[string]int64 // 过期时间（毫秒时间戳）
	streams map[string][]StreamEntry
}{data: make(map[string]string), expires: make(map[string]int64), streams: make(map[string][]StreamEntry)}

// StreamEntry 代表 Redis Stream 的单个条目
type StreamEntry struct {
	ID    string
	Fields map[string]string
}


// 设置 key-value，并处理过期时间
func storeSet(key, value string, ttl int64) {
	store.Lock()
	store.data[key] = value
	// fmt.Println("storeSet key:", key, "value:", value, "ttl:", ttl)
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


// 生成新 ID 时根据时间和序列号递增
func generateStreamID(stream string) string {
	// 获取当前时间（毫秒级 Unix 时间戳）
	now := time.Now().UnixNano() / int64(time.Millisecond)
	timePart := strconv.FormatInt(now, 10)

	// 获取现有流中的最后一个条目的序列号
	lastSeq := int64(0)
	if len(store.streams[stream]) > 0 {
		lastEntry := store.streams[stream][len(store.streams[stream])-1]
		lastTime, lastSeq := parseID(lastEntry.ID)
		if lastTime == now {
			// 如果时间部分相同，则递增序列号
			lastSeq++
		} else {
			// 否则，序列号从 0 开始
			lastSeq = 0
		}
	}

	// 返回新的 ID
	return timePart + "-" + strconv.FormatInt(lastSeq, 10)
}

func generateSequenceID(entries []StreamEntry, id string) string {
	// 获取时间部分
	parts := strings.Split(id, "-")
	timePart := parts[0]

	// 查找同一时间部分的最大序列号
	maxSeq := int64(-1) // -1 表示没有条目时序列号从 0 开始
	for _, entry := range entries {
		entryTime := strings.Split(entry.ID, "-")[0]
		if entryTime == timePart {
			// 获取当前序列号
			entrySeq, _ := strconv.ParseInt(strings.Split(entry.ID, "-")[1], 10, 64)
			if entrySeq > maxSeq {
				maxSeq = entrySeq
			}
		}
	}

	// 根据最大序列号生成新的序列号
	var newSeq int64
	if timePart == "0" {
		newSeq = 1 // 如果时间部分为0，序列号应为1
	} else {
		// 序列号递增
		newSeq = maxSeq + 1
	}

	return timePart + "-" + strconv.FormatInt(newSeq, 10)
}

// xadd 函数，处理流的插入并验证 ID
func xadd(stream string, id string, fields map[string]string) string {
	store.Lock()
	defer store.Unlock()

	// 确保 key 是 stream 类型
	if _, exists := store.streams[stream]; !exists {
		store.streams[stream] = []StreamEntry{}
	}

	// 处理自动生成序列号的 ID
	if strings.Contains(id, "-*") {
		id = generateSequenceID(store.streams[stream], id)
	}

	// 处理自动 ID（时间和序列号）
	if id == "*" {
		id = generateStreamID(stream)
	}


	// 如果流不为空，验证 ID
	if len(store.streams[stream]) > 0 {
		lastEntry := store.streams[stream][len(store.streams[stream])-1]
		fmt.Println("ID IS:", id, "LAST ID IS:", lastEntry.ID)
		if !isValidID(id, lastEntry.ID) {
			return "-ERR The ID specified in XADD is equal or smaller than the target stream top item\r\n"
		}
	}

	// 添加条目
	entry := StreamEntry{
		ID:     id,
		Fields: fields,
	}
	store.streams[stream] = append(store.streams[stream], entry)

	// 返回 ID
	return id
}


// 判断 ID 是否有效
func isValidID(newID, lastID string) bool {

	// 解析 ID
	newTimestamp, newSequence := parseID(newID)
	lastTimestamp, lastSequence := parseID(lastID)

	// 比较时间部分和序列号部分

	if newTimestamp > lastTimestamp {
		return true
	} else if newTimestamp == lastTimestamp && newSequence > lastSequence {
		return true
	}
	return false
}

// 解析 ID 获取时间和序列号
func parseID(id string) (int64, int64) {
	parts := strings.Split(id, "-")
	fmt.Println("len(parts):", len(parts))
	// 处理异常情况：ID 不符合格式
	if len(parts) != 2 {
		fmt.Printf("Warning: Invalid ID format '%s', returning (0,0)\n", id)
		return 0, 0
	}
	
	timestamp, _ := strconv.ParseInt(parts[0], 10, 64)
	sequence, _ := strconv.ParseInt(parts[1], 10, 64)
	return timestamp, sequence
}
