package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
)

// Command 结构体
type Command struct {
	Name string
	Args []string
}

// 线程安全的 Redis 存储
type RedisStore struct {
	mu    sync.RWMutex
	store map[string]string
}

// 设置 key-value
func (r *RedisStore) Set(key, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store[key] = value
}

// 获取 key 对应的 value
func (r *RedisStore) Get(key string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	val, exists := r.store[key]
	return val, exists
}

// 解析 RESP
func parseRESP(reader *bufio.Reader) (*Command, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)

	if strings.HasPrefix(line, "*") {
		count, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, fmt.Errorf("invalid array count: %s", line)
		}

		var parts []string
		for i := 0; i < count; i++ {
			lengthLine, err := reader.ReadString('\n')
			if err != nil {
				return nil, err
			}
			lengthLine = strings.TrimSpace(lengthLine)

			if !strings.HasPrefix(lengthLine, "$") {
				return nil, fmt.Errorf("expected bulk string but got: %s", lengthLine)
			}

			length, err := strconv.Atoi(lengthLine[1:])
			if err != nil {
				return nil, fmt.Errorf("invalid bulk string length: %s", lengthLine)
			}

			data := make([]byte, length)
			_, err = io.ReadFull(reader, data)
			if err != nil {
				return nil, err
			}

			// 读取 \r\n
			_, err = reader.ReadString('\n')
			if err != nil {
				return nil, err
			}

			parts = append(parts, string(data))
		}

		if len(parts) == 0 {
			return nil, fmt.Errorf("empty command")
		}

		return &Command{Name: strings.ToUpper(parts[0]), Args: parts[1:]}, nil
	}

	return nil, fmt.Errorf("unknown RESP format: %s", line)
}

// 命令处理器类型
type CommandHandler func(conn net.Conn, args []string, store *RedisStore)

// 处理 PING
func handlePing(conn net.Conn, args []string, store *RedisStore) {
	conn.Write([]byte("+PONG\r\n"))
}

// 处理 ECHO
func handleEcho(conn net.Conn, args []string, store *RedisStore) {
	if len(args) > 0 {
		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(args[0]), args[0])))
	} else {
		conn.Write([]byte("-ERR missing argument\r\n"))
	}
}

// 处理 SET key value
func handleSet(conn net.Conn, args []string, store *RedisStore) {
	if len(args) < 2 {
		conn.Write([]byte("-ERR wrong number of arguments for 'SET'\r\n"))
		return
	}

	key := args[0]
	value := args[1]

	store.Set(key, value)
	conn.Write([]byte("+OK\r\n"))
}

// 处理 GET key
func handleGet(conn net.Conn, args []string, store *RedisStore) {
	if len(args) < 1 {
		conn.Write([]byte("-ERR wrong number of arguments for 'GET'\r\n"))
		return
	}

	key := args[0]
	value, exists := store.Get(key)
	if exists {
		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)))
	} else {
		conn.Write([]byte("$-1\r\n")) // RESP 规定的空值返回
	}
}

// 处理客户端连接
func handleClient(conn net.Conn, store *RedisStore, commands map[string]CommandHandler) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		cmd, err := parseRESP(reader)
		if err != nil {
			fmt.Println("Error parsing command:", err)
			conn.Write([]byte("-ERR invalid command\r\n"))
			return
		}

		fmt.Println("Received command:", cmd)

		if handler, exists := commands[cmd.Name]; exists {
			handler(conn, cmd.Args, store)
		} else {
			conn.Write([]byte("-ERR unknown command\r\n"))
		}
	}
}

// 启动 Redis 服务器
func main() {
	store := &RedisStore{store: make(map[string]string)}

	// 注册命令
	commands := map[string]CommandHandler{
		"PING": handlePing,
		"ECHO": handleEcho,
		"SET":  handleSet,
		"GET":  handleGet,
	}

	listener, err := net.Listen("tcp", "0.0.0.0:6379")
	if err != nil {
		fmt.Println("Failed to bind to port 6379")
		return
	}
	defer listener.Close()

	fmt.Println("Redis server started on port 6379")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		go handleClient(conn, store, commands)
	}
}
