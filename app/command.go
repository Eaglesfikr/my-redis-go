package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// 命令处理函数类型
type commandHandler func(args []string) string

// 命令映射
var commandHandlers = map[string]commandHandler{
	"PING":     handlePING,
	"SET":      handleSET,
	"GET":      handleGET,
	"TYPE":     handleType,
	"ECHO":     handleECHO,
	"CONFIG":   handleCONFIG,   // CONFIG GET 命令先以CONFIG处理
	"KEYS":     handleKEYS,     // 添加 KEYS 命令
	"SAVE":     handleSAVE,     // 添加 SAVE 命令
	"INFO":     handleInfo,     // 添加 INFO 命令
	"REPLCONF": handleREPLCONF, // 添加 REPLCONF 命令
	"PSYNC":    handlePSYNC,    // 添加 PSYNC 命令处理
	"XADD":     handleXADD,     // 添加 XADD 命令处理
	"XRANGE":   handleXRANGE,   // 添加 XRANGE 命令处理
	"XREAD":	handleXREAD,		// 添加 XREAD 命令处理
	"INCR":     handleINCR,     // 添加 INCR 命令处理
	// "MULTI":	handleMULTI,		// 添加 MULTI 命令处理
	// "EXEC":		handleEXEC,		// 添加 EXEC 命令处理
}

// 解析 RESP 协议
// func parseRESP(reader *bufio.Reader) (string, []string, error) {
// 	line, err := reader.ReadString('\n')
// 	if err != nil {
// 		return "", nil, err
// 	}
// 	line = strings.TrimSpace(line)
// 	if strings.HasPrefix(line, "*") {
// 		count := 0
// 		fmt.Sscanf(line, "*%d", &count)
// 		var args []string
// 		for i := 0; i < count; i++ {
// 			reader.ReadString('\n') // 读取 $N
// 			arg, _ := reader.ReadString('\n')
// 			args = append(args, strings.TrimSpace(arg))
// 		}
// 		
// 		return strings.ToUpper(args[0]), args[1:], nil
// 	}
// 	return "", nil, nil
// }

// 解析 RESP 协议
func parseRESP(reader *bufio.Reader) (string, []string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", nil, err
	}
	line = strings.TrimSpace(line)
	if getRole() == "master" {
		if strings.HasPrefix(line, "*") {
			count := 0
			fmt.Sscanf(line, "*%d", &count)
			var args []string
			totalBytes := len(line) // `*N\r\n` 的长度

			for i := 0; i < count; i++ {
				lenLine, _ := reader.ReadString('\n') // 读取 `$N\r\n`
				totalBytes += len(lenLine) // 计算 `$N\r\n` 长度

				arg, _ := reader.ReadString('\n') // 读取参数值
				args = append(args, strings.TrimSpace(arg))
				totalBytes += len(arg) // `VALUE\r\n`
			}

			// **增加 offset**
			toSlaveCommands := []string{"SET", "REPLCONF"}
			for _, scmd := range toSlaveCommands {
				if strings.ToUpper(args[0]) == scmd {
					config.IncrementOffset(int64(totalBytes))
				}
			}

			return strings.ToUpper(args[0]), args[1:], nil
		}
		return "", nil, nil
	}else{	//master不需要考虑offset
		if strings.HasPrefix(line, "*") {
			count := 0
			fmt.Sscanf(line, "*%d", &count)
			var args []string
			for i := 0; i < count; i++ {
				reader.ReadString('\n') // 读取 $N
				arg, _ := reader.ReadString('\n')
				args = append(args, strings.TrimSpace(arg))
			}
			
			return strings.ToUpper(args[0]), args[1:], nil
		}
		return "", nil, nil
	}		
}	
	


// 处理 CONFIG 命令
func handleCONFIG(args []string) string {
	if len(args) < 2 || strings.ToUpper(args[0]) != "GET" {
		return "-ERR syntax error\r\n"
	}

	configKey := strings.ToLower(args[1])
	var value string

	// 读取 RDB 配置
	rdbConfig.RLock()
	switch configKey {
	case "dir":
		value = rdbConfig.dir
	case "dbfilename":
		value = rdbConfig.dbfilename
	default:
		rdbConfig.RUnlock()
		return "$-1\r\n" // 未知配置项
	}
	rdbConfig.RUnlock()

	return fmt.Sprintf("*2\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(args[1]), args[1], len(value), value)
}

// 处理 PING
func handlePING(args []string) string {
	return "+PONG\r\n"
}

// 处理 ECHO
func handleECHO(args []string) string {
	if len(args) < 1 {
		return "-ERR wrong number of arguments for 'echo' command\r\n"
	}
	message := args[0]

	return fmt.Sprintf("$%d\r\n%s\r\n", len(message), message)
}

// 处理 SET
func handleSET(args []string) string {
	if len(args) < 2 {
		return "-ERR wrong number of arguments for 'set' command\r\n"
	}
	key, value := args[0], args[1]
	var ttl int64 = 0 // 默认不过期

	// 处理可选参数 PX
	if len(args) > 2 && strings.ToUpper(args[2]) == "PX" && len(args) > 3 {
		if px, err := strconv.ParseInt(args[3], 10, 64); err == nil {
			ttl = time.Now().UnixNano()/1e6 + px // 计算过期时间
		} else {
			return "-ERR PX argument must be an integer\r\n"
		}
	}

	storeSet(key, value, ttl)
	// 发送给所有 slave 节点
	if getRole() == "master" {
		propagateToSlaves(fmt.Sprintf("*3\r\n$3\r\nSET\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n",
			len(key), key, len(value), value))
	}
	return "+OK\r\n"
}

// 处理 GET
func handleGET(args []string) string {
	if len(args) < 1 {
		return "-ERR wrong number of arguments for 'get' command\r\n"
	}
	key := args[0]

	value, exists := storeGet(key)
	if exists {
		return fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)
	}
	return "$-1\r\n"
}

// 假设有一个全局的存储数据结构，可以模拟 Redis 存储
var redisDatabase = map[string]interface{}{
	"some_key": "foo", // 示例数据
}

// 处理 TYPE 命令的函数
func handleType(args []string) string {
	// 确保传入的参数正确
	if len(args) != 1 {
		return "-ERR wrong number of arguments for 'TYPE' command\r\n"
	}

	// 获取传入的键
	key := args[0]

	// 检查键是否存在于 store.data 或 store.streams 中
	store.RLock() // 使用读锁
	defer store.RUnlock()

	// 检查键是否在 streams 中，表示是 stream 类型
	if _, exists := store.streams[key]; exists {
		return "+stream\r\nstream"
	}

	// 检查键是否在 data 中，表示是 string 类型
	if _, exists := store.data[key]; exists {
		return "+string\r\nstring"
	}

	// 键不存在时，返回 "none"
	return "+none\r\nnone"
}

// 处理 KEYS 命令，添加规则匹配
func handleKEYS(args []string) string {
	if len(args) < 1 {
		return "-ERR wrong number of arguments for 'keys' command\r\n"
	}
	pattern := args[0]

	// 获取所有 keys
	keys := storeKeys(pattern)

	if len(keys) == 0 {
		return "*0\r\n" // 没有匹配项
	}

	var result string
	result += fmt.Sprintf("*%d\r\n", len(keys)) // 返回 key 数量
	for _, key := range keys {
		result += fmt.Sprintf("$%d\r\n%s\r\n", len(key), key)
	}

	return result
}

// 处理 SAVE 命令
func handleSAVE(args []string) string {
	if len(args) > 0 {
		return "-ERR wrong number of arguments for 'save' command\r\n"
	}

	// 保存 RDB 文件
	err := SaveRDB(rdbConfig.dir, rdbConfig.dbfilename)
	if err != nil {
		return "-ERR " + err.Error() + "\r\n"
	}

	return "+OK\r\n"
}

// 处理 INFO replication 命令
func handleInfo(args []string) string {
	if len(args) > 0 && strings.ToLower(args[0]) == "replication" {
		// RESP Bulk String 响应格式
		response := fmt.Sprintf(
			"role:%s\r\nmaster_replid:%s\r\nmaster_repl_offset:%d",
			getRole(),
			config.MasterReplID,
			config.ReplOffset,
		)
		return fmt.Sprintf("$%d\r\n%s\r\n", len(response), response)
	}
	return "-ERR invalid INFO section\r\n"
}

// 处理 REPLCONF 命令
func handleREPLCONF(args []string) string {
	if len(args) >= 2 {
		if args[0] == "listening-port" {
			// 对应 REPLCONF listening-port（master接受）
			return "+OK\r\n"
		} else if args[0] == "capa" && args[1] == "psync2" {
			// 对应 REPLCONF capa psync2（master接受）
			return "+OK\r\n"
		} else if args[0] == "getack" && args[1] == "*" {
			// 处理 REPLCONF GETACK *(slave接受后返回)
			offsetStr := strconv.FormatInt(config.ReplOffset, 10)	//将 int64 转换为 10 进制字符串。
			return fmt.Sprintf("*3\r\n$8\r\nREPLCONF\r\n$3\r\nACK\r\n$%d\r\n%s\r\n", len(offsetStr), offsetStr)
		}else if args[0] == "ACK" {
			// 处理 REPLCONF ACK <REPL_ID> <OFFSET>（master接受后打印就行，不操作，后面在server里加上net信息）
			return fmt.Sprintf("*3\r\n$3\r\nACK\r\n$2\r\nis\r\n$%d\r\n%s\r\n", len(args[1]), args[1])
		} else {
			return "-ERR unknown REPLCONF command 's args\r\n"
		}
	}
	return "-ERR invalid REPLCONF command\r\n"
}

// 处理 PSYNC 命令
func handlePSYNC(args []string) string {
	// 当收到 PSYNC ? -1 请求时，返回 FULLRESYNC <REPL_ID> 0
	if len(args) == 2 && args[0] == "?" && args[1] == "-1" {
		// 1. 发送 FULLRESYNC 响应
		fullResyncResponse := fmt.Sprintf("+FULLRESYNC %s 0\r\n", config.MasterReplID)

		// 2. 空 RDB 文件（Hex 格式）
		emptyRDBHex := "524544495330303131fa0972656469732d76657205372e322e30fa0a72656469732d62697473c040fa056374696d65c26d08bc65fa08757365642d6d656dc2b0c41000fa08616f662d62617365c000fff06e3bfec0ff5aa2"
		emptyRDB, _ := hex.DecodeString(emptyRDBHex)
		fmt.Println("emptyRDB is:", len(emptyRDB), string(emptyRDB))
		// 3. 使用 bytes.Buffer 来处理二进制数据
		var buffer bytes.Buffer

		// 4. 向缓冲区写入 FULLRESYNC 响应和 RDB 文件响应
		buffer.WriteString(fullResyncResponse)                    // 写入 FULLRESYNC 响应
		buffer.WriteString(fmt.Sprintf("$%d\r\n", len(emptyRDB))) // 写入长度信息
		buffer.Write(emptyRDB)                                    // 写入空的 RDB 数据

		// 返回完整响应
		return buffer.String()

	}
	return "-ERR invalid PSYNC command\r\n"
}

// 解析 XADD 命令
func handleXADD(args []string) string {
	if len(args) < 3 || len(args)%2 == 1 {
		return "-ERR wrong number of arguments for 'XADD' command"
	}
	
	stream := args[0]
	id := args[1]
	fields := make(map[string]string)

	for i := 2; i < len(args); i += 2 {
		fields[args[i]] = args[i+1]
	}

	 // 假设 xadd 函数返回流的 ID
	 result := xadd(stream, id, fields)

	// 通知所有等待 `XREAD` 的客户端
	notifyClients(stream) 

	 // 返回批量字符串格式的 ID，格式为：$length\r\nID\r\n
	 return "$" + strconv.Itoa(len(result)) + "\r\n" + result+"\r\n"
}

// 为命令XRANGE解析 stream ID
func parseStreamID(id string) (int64, int64) {
	parts := strings.Split(id, "-")
	if len(parts) != 2 {
		return 0, 0
	}
	timestamp, err1 := strconv.ParseInt(parts[0], 10, 64)
	sequence, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return timestamp, sequence
}

func handleXRANGE(args []string) string {
	if len(args) < 1 {
		return "-ERR wrong number of arguments for 'XRANGE' command\r\n"
	}

	streamKey := args[0]
	startID := "0-0"
	endID := "9223372036854775807-9223372036854775807" // 默认最大

	if len(args) >= 2 {
		startID = args[1]
		if startID == "-" {
			startID = "0-0"
		} else if !strings.Contains(startID, "-") {
			startID += "-0" // 补全 -0
		}
	}

	if len(args) >= 3 {
		endID = args[2]
		if endID == "+" {
			endID = "9223372036854775807-9223372036854775807" // 最大值
		} else if !strings.Contains(endID, "-") {
			endID += "-9223372036854775807" // 序列号设为最大
		}
	}

	startTS, startSeq := parseStreamID(startID)
	endTS, endSeq := parseStreamID(endID)

	store.RLock()
	defer store.RUnlock()

	entries, exists := store.streams[streamKey]
	if !exists {
		return "*0\r\n" // 空列表
	}

	var result []string
	var count int

	for _, entry := range entries {
		entryTS, entrySeq := parseStreamID(entry.ID)
		if (entryTS > startTS || (entryTS == startTS && entrySeq >= startSeq)) &&
			(entryTS < endTS || (entryTS == endTS && entrySeq <= endSeq)) {
			count++
			// ID
			entryData := []string{fmt.Sprintf("*2\r\n$%d\r\n%s\r\n", len(entry.ID), entry.ID)}

			// Fields
			fieldCount := len(entry.Fields) * 2
			entryData = append(entryData, fmt.Sprintf("*%d\r\n", fieldCount))
			for k, v := range entry.Fields {
				entryData = append(entryData, fmt.Sprintf("$%d\r\n%s\r\n", len(k), k))
				entryData = append(entryData, fmt.Sprintf("$%d\r\n%s\r\n", len(v), v))
			}

			result = append(result, strings.Join(entryData, ""))
		}
	}

	if count == 0 {
		return "*0\r\n" // 空列表
	}

	return fmt.Sprintf("*%d\r\n%s", count, strings.Join(result, ""))
}


// 处理XREAD命令
var waitingClients = make(map[string][]chan struct{})

func handleXREAD(args []string) string {
    if len(args) < 3 {
        return "-ERR syntax error\r\n"
    }

    var blockTime int
    blocking := false
    infiniteBlock := false

    // 解析 BLOCK 参数
    if args[0] == "block" {
        if len(args) < 5 {
            return "-ERR syntax error\r\n"
        }
        blocking = true
        var err error
        blockTime, err = strconv.Atoi(args[1])
        if err != nil || blockTime < 0 {
            return "-ERR invalid block time\r\n"
        }
        if blockTime == 0 {
            infiniteBlock = true
        }
        args = args[2:] // 移除 BLOCK 参数
    }

    // 确保 `streams` 关键字正确
    if args[0] != "streams" || len(args) < 3 || len(args)%2 != 1 {
        return "-ERR syntax error\r\n"
    }

    // // 解析流及其起始 ID
    // streams := make(map[string]string)
    // for i := 1; i < len(args); i += 2 {
    //     streams[args[i]] = args[i+1]
    // }

	//解析流及其起始 ID以支持$
    streams := make(map[string]string)
    store.RLock()
    for i := 1; i < len(args); i += 2 {
        streamKey := args[i]
        lastReadID := args[i+1]

        // 处理 `$` 作为 ID，获取当前流的最新 ID
        if lastReadID == "$" {
            if entries, exists := store.streams[streamKey]; exists && len(entries) > 0 {
                lastReadID = entries[len(entries)-1].ID
            } else {
                lastReadID = "0-0" // 若流为空，则等待新条目
            }
        }
        streams[streamKey] = lastReadID
    }
    store.RUnlock()



    for {
        store.RLock()
        result := make([]string, 0)

        for streamKey, lastReadID := range streams {
            if !strings.Contains(lastReadID, "-") {
                lastReadID += "-0" // 确保 ID 格式正确
            }
            lastTS, lastSeq := parseStreamID(lastReadID)

            entries, exists := store.streams[streamKey]
            if !exists {
                continue
            }

            streamResult := []string{fmt.Sprintf("*2\r\n$%d\r\n%s\r\n", len(streamKey), streamKey)}
            entryCount := 0
            entryData := make([]string, 0)

            for _, entry := range entries {
                ts, seq := parseStreamID(entry.ID)
                if ts > lastTS || (ts == lastTS && seq > lastSeq) {
                    entryCount++
                    entryPart := []string{fmt.Sprintf("*2\r\n$%d\r\n%s\r\n", len(entry.ID), entry.ID)}
                    fieldCount := len(entry.Fields) * 2
                    entryPart = append(entryPart, fmt.Sprintf("*%d\r\n", fieldCount))
                    for k, v := range entry.Fields {
                        entryPart = append(entryPart, fmt.Sprintf("$%d\r\n%s\r\n", len(k), k))
                        entryPart = append(entryPart, fmt.Sprintf("$%d\r\n%s\r\n", len(v), v))
                    }
                    entryData = append(entryData, strings.Join(entryPart, ""))
                }
            }

            if entryCount > 0 {
                streamResult = append(streamResult, fmt.Sprintf("*%d\r\n%s", entryCount, strings.Join(entryData, "")))
                result = append(result, strings.Join(streamResult, ""))
            }
        }
        store.RUnlock()

        if len(result) > 0 {
            return fmt.Sprintf("*%d\r\n%s", len(result), strings.Join(result, ""))
        }

        if !blocking {
            return "$-1\r\n"
        }

        // 使用 channel 等待新数据
        waitChan := make(chan struct{})
        store.Lock()
        for streamKey := range streams {
            waitingClients[streamKey] = append(waitingClients[streamKey], waitChan)
        }
        store.Unlock()

        if infiniteBlock {
            <-waitChan // 无限阻塞，直到新数据到来
        } else {
            select {
            case <-waitChan:
                // 有数据更新
            case <-time.After(time.Duration(blockTime) * time.Millisecond):
                // 超时返回 NULL
                return "$-1\r\n"
            }
        }
    }
}

// XADD 时通知等待的 XREAD,以支持XREADBLOCK 0参数取消阻塞
func notifyClients(streamKey string) {
    store.Lock()
    if clients, ok := waitingClients[streamKey]; ok {
        for _, ch := range clients {
            close(ch) // 通知所有等待的 XREAD
        }
        delete(waitingClients, streamKey) // 清除已通知的 channel
    }
    store.Unlock()
}


// 这个是没实现 BLOACK 0之前的，那时候也没有var waitingClients 和 notifyClients 函数
// func handleXREAD(args []string) string {
//     if len(args) < 3 {
//         return "-ERR syntax error\r\n"
//     }
//     var blockTime int
//     blocking := false
//     // 解析 BLOCK 参数
//     if args[0] == "block" {
//         if len(args) < 5 {
//             return "-ERR syntax error\r\n"
//         }
//         blocking = true
//         var err error
//         blockTime, err = strconv.Atoi(args[1])
//         if err != nil || blockTime < 0 {
//             return "-ERR invalid block time\r\n"
//         }
//         args = args[2:] // 移除 BLOCK 参数，继续处理 STREAMS
//     }
//     // 确保 `streams` 关键字正确
//     if args[0] != "streams" || len(args) < 3 || len(args)%2 != 1 {
//         return "-ERR syntax error\r\n"
//     }
//     // 解析流及其起始 ID
//     streams := make(map[string]string)
//     store.RLock()
//     for i := 1; i < len(args); i += 2 {
//         streamKey := args[i]
//         lastReadID := args[i+1]
//         // 处理 `$` 作为 ID，获取当前流的最新 ID
//         if lastReadID == "$" {
//             if entries, exists := store.streams[streamKey]; exists && len(entries) > 0 {
//                 lastReadID = entries[len(entries)-1].ID
//             } else {
//                 lastReadID = "0-0" // 若流为空，则等待新条目
//             }
//         }
//         streams[streamKey] = lastReadID
//     }
//     store.RUnlock()
//     for {
//         store.RLock()
//         result := make([]string, 0)
//         for streamKey, lastReadID := range streams {
//             if !strings.Contains(lastReadID, "-") {
//                 lastReadID += "-0" // 确保 ID 格式正确
//             }
//             lastTS, lastSeq := parseStreamID(lastReadID)
//             entries, exists := store.streams[streamKey]
//             if !exists {
//                 continue
//             }
//             streamResult := []string{fmt.Sprintf("*2\r\n$%d\r\n%s\r\n", len(streamKey), streamKey)}
//             entryCount := 0
//             entryData := make([]string, 0)
//             for _, entry := range entries {
//                 ts, seq := parseStreamID(entry.ID)
//                 if ts > lastTS || (ts == lastTS && seq > lastSeq) {
//                     entryCount++
//                     entryPart := []string{fmt.Sprintf("*2\r\n$%d\r\n%s\r\n", len(entry.ID), entry.ID)}
//                     fieldCount := len(entry.Fields) * 2
//                     entryPart = append(entryPart, fmt.Sprintf("*%d\r\n", fieldCount))
//                     for k, v := range entry.Fields {
//                         entryPart = append(entryPart, fmt.Sprintf("$%d\r\n%s\r\n", len(k), k))
//                         entryPart = append(entryPart, fmt.Sprintf("$%d\r\n%s\r\n", len(v), v))
//                     }
//                     entryData = append(entryData, strings.Join(entryPart, ""))
//                 }
//             }
//             if entryCount > 0 {
//                 streamResult = append(streamResult, fmt.Sprintf("*%d\r\n%s", entryCount, strings.Join(entryData, "")))
//                 result = append(result, strings.Join(streamResult, ""))
//             }
//         }
//         store.RUnlock()
//         if len(result) > 0 {
//             return fmt.Sprintf("*%d\r\n%s", len(result), strings.Join(result, ""))
//         }
//         if !blocking {
//             return "$-1\r\n"
//         }
//         // 阻塞等待
//         waitChan := make(chan struct{}, 1)
//         store.Lock()
//         go func() {
//             time.Sleep(time.Duration(blockTime) * time.Millisecond)
//             waitChan <- struct{}{}
//         }()
//         store.Unlock()
//         select {
//         case <-waitChan:
//             return "$-1\r\n"
//         }
//     }
// }


// 处理 INCR 命令
func handleINCR(args []string) string {
    if len(args) != 1 {
        return "-ERR wrong number of arguments for 'INCR' command\r\n"
    }

    key := args[0]

    store.Lock()
    defer store.Unlock()

    value, exists := store.data[key]
    if !exists {
        store.data[key] = "1"
        return ":1\r\n"  // Redis 整数响应格式
    }

    num, err := strconv.Atoi(value)
    if err != nil {
        return "-ERR value is not an integer or out of range\r\n"
    }

    num++
    store.data[key] = strconv.Itoa(num)

    return fmt.Sprintf(":%d\r\n", num)  // Redis 正确的整数返回格式
}


// 处理 MULTI 命令
// func handleMULTI(args []string) string {
// 	config.inTransaction = true
// 	// 清空之前的事务队列，以防不小心留下旧的命令
// 	config.transactionQueue = []string{}
// 	return "+OK\r\n"
// }


