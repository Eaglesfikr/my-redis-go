package main

import (
	"bufio"
	"flag" //解析 --port 参数
	"fmt"
	"log"
	"net"
	"os"
	"strings"
)

// 服务器配置
type ServerConfig struct {
	Port             int
	ReplicaOf        string // 为空表示是 master，否则是 slave
	MasterReplID     string // 40字符伪随机ID，硬编码
	MasterReplOffset int64  // 复制偏移量，初始为 0
}

var config ServerConfig

// init 函数用于初始化配置，程序执行前隐式自动调用
func init() {
	// 解析命令行参数
	flag.IntVar(&config.Port, "p", 6379, "Port number for the Redis server")
	flag.StringVar(&config.ReplicaOf, "replicaof", "", "Master host and port for replication")
	flag.Parse()

	// 设置复制 ID 和偏移量（主节点）
	config.MasterReplID = "8371b4fb1155b71f4a04d3e1bc3e18c4a990aeeb"
	config.MasterReplOffset = 0
}

// 启动 Redis 服务器
func main() {
	//init隐式调用
	// 如果是slave，先与主服务器握手
	if config.ReplicaOf != "" {
		masterHost, masterPort := parseReplicaOf(config.ReplicaOf) // 解析--replicaof参数，提取master的host和端口
		handshakeWithMaster(masterHost, masterPort)                // 连接主服务器并发送PING命令
	}

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

	fmt.Printf("Mini Redis running on port %d as %s...\n", config.Port, getRole())

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

// 解析--replicaof参数，提取主机和端口
func parseReplicaOf(replicaOf string) (string, string) {
	parts := strings.Split(replicaOf, " ")
	if len(parts) != 2 {
		fmt.Println("Invalid replicaof format. Expected format: <master_host> <master_port>")
		os.Exit(1)
	}
	return parts[0], parts[1]
}

// 连接主服务器并发送PING命令以及REPLCONF命令
func handshakeWithMaster(masterHost string, masterPort string) {
	// 建立与主服务器的连接
	conn, err := net.Dial("tcp", masterHost+":"+masterPort)
	if err != nil {
		fmt.Println("Error connecting to master:", err)
		os.Exit(1)
	}
	defer conn.Close()

	// 构造并发送PING命令
	pingCmd := "*1\r\n$4\r\nPING\r\n"
	_, err = conn.Write([]byte(pingCmd))
	if err != nil {
		fmt.Println("Error sending PING command:", err)
		os.Exit(1)
	}

	// 读取PING响应并检查
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading response:", err)
		os.Exit(1)
	}
	if strings.TrimSpace(response) != "+PONG" {
		fmt.Println("Unexpected response to PING requset:", response)
		os.Exit(1)
	}
	fmt.Println("Received response from master:", response)

	// 发送REPLCONF listening-port <PORT>
	listeningPortCmd := "*3\r\n$8\r\nREPLCONF\r\n$14\r\nlistening-port\r\n$4\r\n" + fmt.Sprintf("%d", config.Port) + "\r\n"
	_, err = conn.Write([]byte(listeningPortCmd))
	if err != nil {
		fmt.Println("Error sending REPLCONF listening-port command:", err)
		os.Exit(1)
	}

	// 读取REPLCONF listening-port 响应并验证
	response, err = reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading response:", err)
		os.Exit(1)
	}
	if strings.TrimSpace(response) != "+OK" {
		fmt.Println("Unexpected response to REPLCONF listening-port command:", response)
		os.Exit(1)
	}
	fmt.Println("Received valid REPLCONF listening-port response from master:", response)

	// 发送REPLCONF capa psync2
	capaPsync2Cmd := "*3\r\n$8\r\nREPLCONF\r\n$4\r\ncapa\r\n$6\r\npsync2\r\n"
	_, err = conn.Write([]byte(capaPsync2Cmd))
	if err != nil {
		fmt.Println("Error sending REPLCONF capa psync2 command:", err)
		os.Exit(1)
	}

	// 读取REPLCONF capa psync2 响应并验证
	response, err = reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading response:", err)
		os.Exit(1)
	}
	if strings.TrimSpace(response) != "+OK" {
		fmt.Println("Unexpected response to REPLCONF capa psync2 command:", response)
		os.Exit(1)
	}
	fmt.Println("Received valid REPLCONF capa psync2 response from master:", response)

	// 发送PSYNC ? -1,由于这是副本第一次连接到主服务器，因此复制 ID 将为（问号）?
	// 这是 replica 第一次连接到 master，因此偏移量将为-1
	psyncCmd := "*3\r\n$5\r\nPSYNC\r\n$1\r\n?\r\n$2\r\n-1\r\n"
	_, err = conn.Write([]byte(psyncCmd))
	if err != nil {
		fmt.Println("Error sending PSYNC ? -1 command:", err)
		os.Exit(1)
	}

	// 读取PSYNC响应并检查
	response, err = reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading response:", err)
		os.Exit(1)
	}
	if !strings.HasPrefix(response, "+FULLRESYNC") {
		fmt.Println("Unexpected response to PSYNC command:", response)
		os.Exit(1)
	}
	// 解析 FULLRESYNC 响应，获取 REPL_ID
	parts := strings.Fields(response)
	if len(parts) < 2 {
		fmt.Println("Invalid FULLRESYNC response:", response)
		os.Exit(1)
	}
	replID := parts[1]
	fmt.Printf("Received FULLRESYNC response from master, REPL_ID: %s\n", replID)
}
