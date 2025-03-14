package main

import (
	"bufio"
	"fmt"
	"net"
)

// 启动 Redis 服务器
func main() {
	ln, err := net.Listen("tcp", ":6379")
	if err != nil {
		fmt.Println("Failed to bind port 6379:", err)
		return
	}
	defer ln.Close()
	
	fmt.Println("Mini Redis running on port 6379...")

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
