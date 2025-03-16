package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"fmt"
	"strings"
)
	
func writeString(buf *bytes.Buffer, str string) {
	// 计算字符串长度
	strLen := byte(len(str))
	// 写入字符串长度和字符串内容
	buf.Write([]byte{strLen})
	buf.Write([]byte(str))
}

func writeLengthEncodedInt(buf *bytes.Buffer, value int) {
	// 将整数按照长度编码规则写入文件
	// 此处假设值小于 64, 使用简单的字节表示
	buf.Write([]byte{byte(value)})
}

func writeUint32(buf *bytes.Buffer, value uint32) {
	// 写入 4 字节无符号整数
	bufBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bufBytes, value)
	buf.Write(bufBytes)
}


// 读取大小编码的数值
func readSizeEncoded(file *os.File) (uint32, error) {
	var sizeByte byte
	err := binary.Read(file, binary.LittleEndian, &sizeByte)
	if err != nil {
		return 0, err
	}

	

	// 小于 64，直接使用一个字节
	if sizeByte&0xC0 == 0x00 {
		return uint32(sizeByte & 0x3F), nil
	}

	// 大于等于 64，使用两个字节
	if sizeByte&0xC0 == 0x40 {
		var nextByte byte
		err = binary.Read(file, binary.LittleEndian, &nextByte)
		if err != nil {
			return 0, err
		}
		// 调试输出，查看第二个字节
		fmt.Printf("Read next byte: 0x%X\n", nextByte)

		return uint32((uint32(sizeByte&0x3F) << 8) | uint32(nextByte)), nil
	}

	// 不支持的编码类型
	return 0, errors.New("unsupported size encoding")
}

// 读取字符串（4byte）
func readString(file *os.File) (string, error) {
	var keySize byte
	err := binary.Read(file, binary.LittleEndian, &keySize)
	if err != nil {
		return "", err
	}

	key := make([]byte, keySize)
	_, err = file.Read(key)
	if err != nil {
		return "", err
	}

	return string(key), nil
}

// 判断是否是写命令
func isReadCommand(cmd string) bool {
    ReadCommands := []string{"GET", "ECHO", "KEYS"}
    for _, rcmd := range ReadCommands {
        if strings.HasPrefix(strings.ToUpper(cmd), rcmd) {
            return true
        }
    }
    return false
}