package main

import (
	"bufio"
	"flag" //解析 --port 参数
	"fmt"
	"log"
	"net"
)

// 服务器配置
type ServerConfig struct {
	Port       int
	ReplicaOf  string // 为空表示是 master，否则是 slave
}

var config ServerConfig

// 启动 Redis 服务器
func main() {
	flag.IntVar(&config.Port, "p", 6379, "Port number for the Redis server")
	flag.StringVar(&config.ReplicaOf, "replicaof", "", "Master host and port for replication")
	flag.Parse()

	// 每次重启 Redis 服务器时，都需要读取 RDB 文件
	err := LoadRDB(rdbConfig.dir, rdbConfig.dbfilename)
	if err != nil {
		log.Fatalf("Error reading RDB file: %v", err)
	}

	// 监听端口
	address := fmt.Sprintf(":%d", config.Port)
	ln, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Printf("Failed to bind port %d: %v\n", config.Port, err)
		return
	}

	defer ln.Close()

	fmt.Println("Mini Redis running on port:", config.Port)

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Connection error:", err)
			continue
		}
		go handleClient(conn)
	}
}

// 处理客户端连接
func handleClient(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		cmd, args, err := parseRESP(reader)
		if err != nil {
			fmt.Println("Error parsing command:", err)
			return
		}
		if cmd == "" {
			fmt.Println("Client disconnected.")
			return
		}

		fmt.Println("Received command:", cmd, args)

		// 方法分发
		handler, exists := commandHandlers[cmd]
		if !exists {
			conn.Write([]byte("-ERR unknown command\r\n"))
			continue
		}

		response := handler(args)
		conn.Write([]byte(response))
	}
}

// 获取服务器角色
func getRole() string {
	if config.ReplicaOf == "" {
		return "master"
	}
	return "slave"
}