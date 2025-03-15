package test

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

func parseRESP(reader *bufio.Reader) error {
	// 读取一行命令
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	line = strings.TrimSpace(line)

	// 如果是 RESP 协议的数组
	if strings.HasPrefix(line, "*") {
		count := line[1:]
		fmt.Printf("Array count: %s\n", count)
	} else if strings.HasPrefix(line, "$") {
		// 处理 Bulk String 类型数据
		length := line[1:]
		fmt.Printf("Bulk string length: %s\n", length)
	}

	// 打印命令本身
	fmt.Println("Received line:", line)

	return nil
}

func handleClient(conn net.Conn) {
	defer conn.Close()

	// 创建一个 Reader
	reader := bufio.NewReader(conn)

	for {
		err := parseRESP(reader)
		if err != nil {
			fmt.Println("Error parsing command:", err)
			return
		}
	}
}

func startServer(host string, port string) {
	listen, err := net.Listen("tcp", host+":"+port)
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
	defer listen.Close()

	fmt.Printf("Listening for connections on %s:%s...\n", host, port)

	// 接受客户端连接
	for {
		conn, err := listen.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}
		go handleClient(conn) // 使用 goroutine 来处理每个连接
	}
}

// func main() {
// 	// 设置服务器监听地址
// 	host := "0.0.0.0"
// 	port := "6379"
// 	startServer(host, port)
// }
