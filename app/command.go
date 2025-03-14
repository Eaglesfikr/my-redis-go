package main

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// 命令处理函数类型
type commandHandler func(args []string) string

// 命令映射
var commandHandlers = map[string]commandHandler{
	"PING":   handlePING,
	"SET":    handleSET,
	"GET":    handleGET,
	"ECHO":   handleECHO,
	"CONFIG": handleCONFIG, // CONFIG GET 命令先以CONFIG处理
}

// 解析 RESP 协议
func parseRESP(reader *bufio.Reader) (string, []string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", nil, err
	}
	line = strings.TrimSpace(line)

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
