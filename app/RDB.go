package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc64"
	"os"
	"sync"
	"time"
)

// RDB 相关配置
var rdbConfig = struct {
	sync.RWMutex
	dir        string
	dbfilename string
}{
	dir:        "./data",   // 默认存储路径
	dbfilename: "dump.rdb", // 默认 RDB 文件名
}

// 读取 RDB 文件：只读出database部分就行
func LoadRDB(dir, dbfilename string)  error {
	// 打开 RDB 文件
	filePath := dir + "/" + dbfilename
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	
	var header [9]byte
	var dbIndex uint8
	var hashSize uint32
	_, err = file.Read(header[:])
	if err != nil {
		return  err
	}

	// 检查 header 是否匹配 REDIS0011
	if string(header[:]) != "REDIS0011" {
		return  errors.New("invalid RDB header")
	}

	// 跳过元数据部分，直到找到数据库部分的键值对部分
	for {
		// 读取每个部分
		partHeader := make([]byte, 1)
		_, err := file.Read(partHeader)
		if err != nil {
			return err
		}

		if partHeader[0] == 0xFE {
			// 读取数据库编号，但未使用它

			err = binary.Read(file, binary.LittleEndian, &dbIndex)
			if err != nil {
				return err
			}
		}
		if partHeader[0] == 0xFB {
			// 读取哈希表大小信息（根据大小编码格式处理）
			hashSize, err = readSizeEncoded(file)
			if err != nil {
				return err
			}

			// 跳过过期哈希表
			_, err := readSizeEncoded(file)
			if err != nil {
				return err
			}
		}
		// 读取每个键值对
		for i := 0; i < int(hashSize); i++ {
			if partHeader[0] == 0xFD {
				// 跳过过期时间 (4 字节)
				var expiryTime uint32
				err := binary.Read(file, binary.LittleEndian, &expiryTime)
				if err != nil {
					fmt.Println("Reached EOF unexpectedly while reading expiry time.")
					return err
				}

				// 读取值类型标志（1 字节），不使用
				var valueType byte
				err = binary.Read(file, binary.LittleEndian, &valueType)
				if err != nil {
					fmt.Println("Reached EOF unexpectedly while reading value type.")
					return err
				}
				if(valueType!= 0x00){
					fmt.Println("value type must be String.",valueType)
				}
				
					// 读取key
					key, err := readString(file)
					if err != nil {
						return err
					}
					// 存储键值对和过期时间
					value, err := readString(file)
					if err != nil {
						return err
					}

					// 将键值对和过期时间存储到内存中
					// storeSet(key, value, int64(expiryTime)*1000) // 先不管过期时间，先存储键值对
					storeSet(key, value, 0)
					
			}
		}
		if partHeader[0] == 0xFF {
			// 文件结束部分
			break
		}
	}

	return nil
}

// 保存 RDB 文件，下面4个小函数使用
func SaveRDB(dir, dbfilename string) error {
	// 创建一个文件用于存储 RDB 数据
	filePath := dir + "/" + dbfilename
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// 使用 CRC64 校验和表
	// 定义一个 CRC64 的 polynomial 和一个 CRC64 校验和表
	// 用于计算整个文件的 CRC64 校验和
	// 使用的是 CRC64-ECMA 表
	table := crc64.MakeTable(crc64.ECMA)

	// 创建一个缓冲区来存储数据
	var buf bytes.Buffer

	// 写入 MAGIC 字符串和版本号
	magic := []byte{0x52, 0x45, 0x44, 0x49, 0x53} // MAGIC 字符串 "REDIS"
	version := []byte{0x30, 0x30, 0x31, 0x31}     // 版本号 "0003"
	buf.Write(magic)
	buf.Write(version)

	// 写入元数据（Redis 版本信息）
	writeMetadata(&buf)

	// 写入数据库（此例使用单个数据库）
	writeDatabase(&buf)

	// 计算 CRC64 校验和
	checksum := crc64.Checksum(buf.Bytes(), table)

	// 将缓冲区的内容写入文件
	file.Write(buf.Bytes())

	// 写入结束标志和 CRC64 校验和并写入文件
	writeEnd(file, checksum)

	return nil
}

// 2-写入 RDB 文件的元数据部分,假设我们只写 Redis 版本信息
func writeMetadata(buf *bytes.Buffer) {
	// 这里假设我们只写 Redis 版本信息
	metadata := []byte{0xFA} // 开始一个元数据字段
	buf.Write(metadata)

	// 写入元数据名：redis-ver
	redisVer := "redis-ver"
	redisVerLen := byte(len(redisVer)) // 以字节表示的字符串长度
	buf.Write([]byte{redisVerLen})     // 长度字段
	buf.Write([]byte(redisVer))        // 写入元数据名

	// 写入 Redis 版本：6.0.16
	redisVersion := "6.0.16"
	redisVersionLen := byte(len(redisVersion))
	buf.Write([]byte{redisVersionLen}) // 长度字段
	buf.Write([]byte(redisVersion))    // 写入版本号
}

// 3.1-写入 RDB 文件的数据库部分的头部，假设我们只写一个数据库
func writeDatabase(buf *bytes.Buffer) {
	// 写入数据库部分：数据库选择标识（此处为数据库0）
	buf.Write([]byte{0xFE, 0x00})

	// 写入数据库的哈希表大小和过期哈希表大小（假设为3和2）
	buf.Write([]byte{0xFB})

	// 1️⃣ 计算当前 store 中键值对的数量
    totalnums := len(store.data) 
	writeLengthEncodedInt(buf, totalnums) // 未过期哈希表大小
	writeLengthEncodedInt(buf, 0) // 过期哈希表大小

	// 写入键值对
	writeKeyValuePair(buf)
}

// 3.2-写入 RDB 文件数据库部分的键值对部分
func writeKeyValuePair(buf *bytes.Buffer) {
	// 假设键 "foo" 和值 "bar"，没有过期时间
	// 写入过期时间（FD 4字节无符号整数，秒）
	// 遍历 store.data 里的所有键值对
	for key, value := range store.data {
		buf.Write([]byte{0xFD})
		writeUint32(buf, uint32(time.Now().Unix())) // 写入过期时间戳（Unix时间戳）

		// 写入值类型标志（先固定为字符串类型，标志为 0x00）
		buf.Write([]byte{0x00})

		// 写入键和值
		writeString(buf, key)
		writeString(buf, value)
	}
}

// 4-写入 RDB 文件的尾部并写入文件
func writeEnd(file *os.File, checksum uint64) {
	// 写入文件结束标志
	file.Write([]byte{0xFF})

	// 写入 CRC64 校验和（8 字节）
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, checksum)
	file.Write(buf)
}
