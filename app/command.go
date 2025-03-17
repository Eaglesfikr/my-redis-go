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
// 		// **每次解析命令成功时，若为可以传输到slave的命令，增加 offset命令字节数**
// 		ToslaveCommands := []string{"SET", "PING", "FLUSHALL"}
//     	for _, scmd := range ToslaveCommands {
// 			if strings.ToUpper(args[0]) == scmd {
// 				config.IncrementOffset(int64(len(line)))
// 			}
// 		}	
// 		return strings.ToUpper(args[0]), args[1:], nil
// 	}
// 	return "", nil, nil
// }

// 解析 RESP 协议
func parseRESP(reader *bufio.Reader) (string, []string, error) {
	line, err := reader.ReadString('\n')
	fmt.Println("line 's length is",len(line),"and content is",line)
	if err != nil {
		return "", nil, err
	}
	line = strings.TrimSpace(line)

	if strings.HasPrefix(line, "*") {
		count := 0
		fmt.Sscanf(line, "*%d", &count)
		var args []string
		totalBytes := len(line) // `*N\r\n` 的长度

		for i := 0; i < count; i++ {
			lenLine, _ := reader.ReadString('\n') // 读取 `$N\r\n`
			fmt.Println("lenline 's length is",len(lenLine),"and content is",lenLine)
			totalBytes += len(lenLine) // 计算 `$N\r\n` 长度

			arg, _ := reader.ReadString('\n') // 读取参数值
			args = append(args, strings.TrimSpace(arg))
			fmt.Println("args 's length is",len(arg),"and content is",arg)
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

	// 检查键是否存在于 redisDatabase 中
	value, exists := redisDatabase[key]
	if !exists {
		// 键不存在时，返回 "none"
		return "+none\r\nnone"
	}

	// 判断键的类型，假设我们只处理 "string" 和 "none" 类型
	switch value.(type) {
	case string:
		// 键是 string 类型，返回 "string"
		return "+string\r\nstring"
	default:
		// 目前只处理 "string" 类型，其他类型返回 "none"
		return "+none\r\nnone"
	}
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
