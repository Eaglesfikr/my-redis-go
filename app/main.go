package main

import (
	"bufio"
	"flag" //解析 --port 参数
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

// 服务器配置
type ServerConfig struct {
	sync.Mutex
	Port               int
	ReplicaOf          string     // 为空表示是 master，否则是 slave
	MasterReplID       string     // 40字符伪随机ID，硬编码
	ReplOffset         int64      // 副本已经处理的字节数,初始为 0
	replicaConnections []net.Conn // 存储所有副本的连接
	// 事务队列
	transactionQueue []string
	// 当前是否处于事务模式
	inTransaction bool
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
	config.ReplOffset = -120 // 平衡各个slave偏移量为0，因为这里的握手我时按照命令来做的
}

// 启动 Redis 服务器
func main() {
	//init隐式调用
	// 如果是slave，先与主服务器握手
	if config.ReplicaOf != "" { //代表是slave
		masterHost, masterPort := parseReplicaOf(config.ReplicaOf)                 // 解析--replicaof参数，提取master的host和端口
		conn, replID, emptyRDB, err := handshakeWithMaster(masterHost, masterPort) // 发起连接主服务器并获取最终的ID和RDB文件

		if err != nil {
			log.Fatalf("Error handshaking with master: %v", err)
		}

		// defer conn.Close()

		// 先不对得到的ID和emptyRDB做任何处理，等到后面有空再处理
		fmt.Printf("Handshaked with master, REPL_ID: %s and empty RDB: %s\n", replID, emptyRDB)

		// 在这里开始处理来自主服务器的命令
		go handleMasterCommands(conn) // 在另一个 goroutine 中处理来自 master 的命令

		//*** 让 slave 监听客户端请求 ***
		address := fmt.Sprintf(":%d", config.Port)
		ln, err := net.Listen("tcp", address)
		if err != nil {
			fmt.Printf("Failed to bind port %d: %v\n", config.Port, err)
			return
		}
		defer ln.Close()

		fmt.Printf("Slave Redis running on port %d...\n", config.Port)

		for {
			conn, err := ln.Accept()
			if err != nil {
				fmt.Println("Connection error:", err)
				continue
			}
			go handleReadOnlyClient(conn) // 只允许读取命令，不允许写入命令
		}

	} else { // 代表是master
		// 每次重启 Redis的master 服务器时，都需要读取 RDB 文件
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
}

// master处理客户端(包括slave节点)连接
func handleClient(conn net.Conn) {
	defer conn.Close()

	// 读取客户端命令
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

		if cmd == "PSYNC" {
			//网络存入master的config.replicaConnections,要保证原子性，多个slave节点下,调用方法
			config.AddReplicaConnection(conn)
		}

		fmt.Println("Received command:", cmd, args)

		// 处理 MULTI 命令
		if cmd == "MULTI" {
			fmt.Println("Received MULTI command, entering transaction mode")
			config.StartTransaction(conn)
			continue
		}

		// 处理 EXEC 命令
		if cmd == "EXEC" {
			config.ExecuteTransaction(conn)
			continue
		}

		// 在事务模式下，将命令排队
		if config.inTransaction {
			config.QueueTransactionCommand(cmd, args, conn)
			continue
		}

		// 非事务模式下，直接处理命令方法分发
		handler, exists := commandHandlers[cmd]
		if !exists {
			conn.Write([]byte("-ERR unknown command\r\n"))
			continue
		}
		response := handler(args)

		// 当 为REPLCONF ACK *时，是例外，需要返回响应给 主服务器
		if cmd == "REPLCONF" && args[0] == "GETACK" && args[1] == "*" {
			fmt.Println("Received", response, "from slave of ", conn.RemoteAddr().String())
		}
		fmt.Println("Executing :", cmd, args, "Response:", response)
		conn.Write([]byte(response))
	}
}

// slave处理来自客户端的读命令
func handleReadOnlyClient(conn net.Conn) {
	defer conn.Close()

	// 读取客户端命令
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

		// 检查是否是只读命令
		if !isReadCommand(cmd) {
			conn.Write([]byte("-ERR unknown command or not allowed in read-only mode\r\n"))
			continue
		}

		// 处理 MULTI 命令
		if cmd == "MULTI" {
			fmt.Println("Received MULTI command, entering transaction mode")
			config.StartTransaction(conn)
			continue
		}

		// 处理 EXEC 命令
		if cmd == "EXEC" {
			config.ExecuteTransaction(conn)
			continue
		}

		// 在事务模式下，将命令排队
		if config.inTransaction {
			config.QueueTransactionCommand(cmd, args, conn)
			continue
		}

		// 非事务模式下，直接处理命令方法分发
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

// slave连接master握手过程,返回主服务器的响应中的ID和空的RDB文件
func handshakeWithMaster(masterHost string, masterPort string) (net.Conn, string, []byte, error) {
	// 建立与主服务器的连接
	conn, err := net.Dial("tcp", masterHost+":"+masterPort)
	if err != nil {
		conn.Close()
		fmt.Println("Error connecting to master:", err)
		os.Exit(1)
	}
	// defer conn.Close() // 连接到主服务器后，不需要关闭

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
	// 解析 FULLRESYNC 响应
	parts := strings.Fields(response)

	// 检查解析结果是否正确
	if len(parts) < 3 {
		fmt.Println("Invalid FULLRESYNC response:", response)
		os.Exit(1) // 需要处理退出或返回的方式
	}

	// 提取 REPL_ID 和状态 (通常是0)
	replID := parts[1]
	offset := parts[2] // 这是 0，因为它应该代表偏移量
	fmt.Printf("Received FULLRESYNC response from master, REPL_ID: %s, status: %s\n", replID, offset)

	// 读取 RDB 文件长度部分
	rdbLengthLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, "", nil, fmt.Errorf("error reading RDB length: %v", err)
	}

	// 去掉末尾的 \r\n，并解析长度
	rdbLengthLine = strings.TrimSuffix(rdbLengthLine, "\r\n")
	rdbLength, err := strconv.Atoi(rdbLengthLine[1:]) // rdbLengthLine 应该是 "$<length>"
	if err != nil {
		return nil, "", nil, fmt.Errorf("invalid RDB length: %v", err)
	}

	// 读取空 RDB 文件的二进制数据
	emptyRDB := make([]byte, rdbLength)
	_, err = reader.Read(emptyRDB)
	if err != nil {
		return nil, "", nil, fmt.Errorf("error reading RDB data: %v", err)
	}
	// fmt.Printf("Received empty RDB file of length %d bytes and content: %s\n", rdbLength, emptyRDB)
	// 返回 REPL_ID 和 RDB 文件内容
	return conn, replID, emptyRDB, nil
}

// 让 Master 发送命令给 Slave
func propagateToSlaves(command string) {
	for _, slave := range config.replicaConnections {
		_, err := slave.Write([]byte(command))
		// 打印发送
		fmt.Printf("Sending command to %s: %s\n", slave.RemoteAddr().String(), command)

		if err != nil {
			fmt.Println("Failed to propagate to slave:", err)
		}
	}
}

// Slave 解析 Master 发送的命令
func handleMasterCommands(conn net.Conn) {
	// defer wg.Done() // 确保 Goroutine 执行完时通知 WaitGroup

	reader := bufio.NewReader(conn)
	for {
		command, args, err := parseRESP(reader)
		if err != nil {
			fmt.Println("Error reading command from master:", err)
			return
		}
		fmt.Println("Received command :", command, args, "from ", conn.RemoteAddr().String())
		if handler, exists := commandHandlers[command]; exists {
			response := handler(args)
			// conn.Write([]byte(response)) // slave 一般不需要返回响应给 主服务器
			// 当 为REPLCONF GETACK *时，是例外，需要返回响应给 主服务器
			if command == "REPLCONF" && args[0] == "GETACK" && args[1] == "*" {
				conn.Write([]byte(response))
			}
			fmt.Println("Executing from Master:", command, args, "Response:", response)
		} else {
			fmt.Println("Unknown command from master:", command)
		}
	}
	// fmt.Println("Executing from Master:", command, args, "Response:", response)
}
