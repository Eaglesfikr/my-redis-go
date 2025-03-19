package main

import (
	"fmt"
	"strings"
    "net"
)


// 启动事务，清空队列并设置 inTransaction 标志
func (server *ServerConfig) StartTransaction(conn net.Conn) {
	server.Lock()
	defer server.Unlock()

	server.inTransaction = true
	server.transactionQueue = []string{} // 清空之前的队列
	conn.Write([]byte("+OK\r\n"))
}

// 执行事务中的所有命令
func (server *ServerConfig) ExecuteTransaction(conn net.Conn) {
	server.Lock()
	defer server.Unlock()

	if !server.inTransaction {
		conn.Write([]byte("-ERR NOT in MULTI mode\r\n"))
		return
	}

	// 先构造 RESP 数组的头部
	responseLines := []string{}
	responseLines = append(responseLines, fmt.Sprintf("*%d", len(server.transactionQueue)))

	// 执行所有排队的命令
	for _, queuedCmd := range server.transactionQueue {
		fmt.Println("Executing queued command:", queuedCmd)

		// 解析命令
		cmdArgs := strings.Split(queuedCmd, " ")
		cmd := cmdArgs[0]
		args := cmdArgs[1:]

		handler, exists := commandHandlers[cmd]
		if !exists {
			responseLines = append(responseLines, "-ERR unknown command")
			continue
		}

		// 执行命令并获取 RESP 响应
		response := handler(args)
		responseLines = append(responseLines, strings.TrimSpace(response)) // 可能需要 trim 掉 \r\n，避免重复
	}

	// 事务结束，清空队列并退出事务模式
	server.transactionQueue = []string{}
	server.inTransaction = false

	// 组装 RESP 返回给客户端
	finalResponse := strings.Join(responseLines, "\r\n") + "\r\n"
	conn.Write([]byte(finalResponse))
}

// 在事务模式下将命令排队
func (server *ServerConfig) QueueTransactionCommand(cmd string, args []string, conn net.Conn) {
	server.Lock()
	defer server.Unlock()

	if server.inTransaction {
		cmdLine := fmt.Sprintf("%s %s", cmd, strings.Join(args, " "))
		server.transactionQueue = append(server.transactionQueue, cmdLine)
		conn.Write([]byte("+QUEUED\r\n"))
	}
}