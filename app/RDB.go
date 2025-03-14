package main

import (
	// "bytes"
	"encoding/binary"
	"errors"
	"os"
)

// 读取 RDB 文件：只读出database部分就行
func readRDBKeys(dir, dbfilename string) ([]string, error) {
	// 打开 RDB 文件
	filePath := dir + "/" + dbfilename
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var keys []string
	var header [9]byte
	_, err = file.Read(header[:])
	if err != nil {
		return nil, err
	}

	// 检查 header 是否匹配 REDIS0011
	if string(header[:]) != "REDIS0011" {
		return nil, errors.New("invalid RDB header")
	}

	// 跳过元数据部分，直到找到数据库部分
	for {
		// 读取每个部分
		partHeader := make([]byte, 1)
		_, err := file.Read(partHeader)
		if err != nil {
			return nil, err
		}

		if partHeader[0] == 0xFE {
			// 读取数据库部分
			var dbIndex uint8
			binary.Read(file, binary.LittleEndian, &dbIndex)

			// 读取键值对数量（哈希表大小）
			var hashSize, expireSize uint8
			binary.Read(file, binary.LittleEndian, &hashSize)
			binary.Read(file, binary.LittleEndian, &expireSize)

			// 读取每个键值对
			for i := 0; i < int(hashSize); i++ {
				var keySize uint8
				binary.Read(file, binary.LittleEndian, &keySize)

				key := make([]byte, keySize)
				_, err := file.Read(key)
				if err != nil {
					return nil, err
				}

				keys = append(keys, string(key))
			}
		} else if partHeader[0] == 0xFF {
			// 文件结束部分
			break
		}
	}

	return keys, nil
}


// 保存 RDB 文件，下面4个小函数使用
func saveRDBFile(dir, dbfilename string) error {
    filePath := dir + "/" + dbfilename
    file, err := os.Create(filePath)
    if err != nil {
        return err
    }
    defer file.Close()

    // 写入 RDB 文件的内容
    writeRDBHeader(file)
    writeRDBDatabase(file)
    writeRDBFooter(file)

    return nil
}

// 1-写入 RDB 文件的头部
func writeRDBHeader(file *os.File) {
    header := []byte("REDIS0011")
    file.Write(header)
}

// 2-写入 RDB 数据部分
func writeRDBDatabase(file *os.File) {
    file.Write([]byte{0xFE}) // 数据段开始标志
    // 这里只处理一个数据库
    file.Write([]byte{0x00}) // 数据库索引

    // 写入所有的 key-value
    for key, value := range store.data {
        writeString(file, key)
        writeString(file, value)
    }
}

// 3-写入字符串数据
func writeString(file *os.File, str string) {
    size := len(str)
    file.Write([]byte{byte(size)}) // 字符串长度
    file.Write([]byte(str))         // 字符串内容
}

// 4-写入 RDB 文件的尾部
func writeRDBFooter(file *os.File) {
    file.Write([]byte{0xFF}) // 文件结束标志
}