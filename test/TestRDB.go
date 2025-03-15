package main

import (
	"encoding/binary"
	"errors"
	"os"
	"fmt"
)

// 读取 RDB 文件的键值对
func readRDBFile(dir, dbfilename string) (map[string]string, error) {
	// 打开 RDB 文件
	filePath := dir + "/" + dbfilename
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// 检查文件头是否为 "REDIS0011"
	var header [9]byte
	_, err = file.Read(header[:])
	if err != nil {
		return nil, err
	}
	if string(header[:]) != "REDIS0011" {
		return nil, errors.New("invalid RDB header")
	}

	// 跳过元数据部分，直到找到数据库部分
	var database map[string]string
	database = make(map[string]string)

	for {
		// 读取每个部分
		partHeader := make([]byte, 1)
		_, err := file.Read(partHeader)
		if err != nil {
			return nil, err
		}

		if partHeader[0] == 0xFE {
			// 数据库部分
			// 跳过数据库索引（只有一个数据库）
			var dbIndex uint8
			binary.Read(file, binary.LittleEndian, &dbIndex)

			// 读取每个键值对
			for {
				var keySize uint8
				err := binary.Read(file, binary.LittleEndian, &keySize)
				if err != nil {
					return nil, err
				}

				// 如果读取到 0xFF，表示结束
				if keySize == 0 {
					break
				}

				// 读取键
				key := make([]byte, keySize)
				_, err = file.Read(key)
				if err != nil {
					return nil, err
				}

				// 读取值
				var valueSize uint8
				err = binary.Read(file, binary.LittleEndian, &valueSize)
				if err != nil {
					return nil, err
				}

				value := make([]byte, valueSize)
				_, err = file.Read(value)
				if err != nil {
					return nil, err
				}

				// 将键值对添加到 map 中
				database[string(key)] = string(value)
			}
		} else if partHeader[0] == 0xFF {
			// 文件结束部分
			break
		}
	}

	return database, nil
}

func main() {
	// 使用读取函数并打印读取的键值对
	// dir := "../app/data"  // 设定数据目录
	// dbfilename := "dump.rdb" // RDB 文件名

	// data, err := readRDBFile(dir, dbfilename)
	// if err != nil {
	// 	fmt.Println("Error reading RDB file:", err)
	// 	return
	// }

	// // 输出读取的键值对
	// for key, value := range data {
	// 	fmt.Printf("Key: %s, Value: %s\n", key, value)
	// }



	err := SaveRDB(rdbConfig.dir,rdbConfig.dbfilename)
	if err != nil {
		fmt.Println("Error writing RDB file:", err)
	} else {
		fmt.Println("RDB file written successfully")
	}

}