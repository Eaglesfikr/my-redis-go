package test

import (
    "encoding/binary"
    "hash/crc64"
    "os"
    "fmt"
)

// CRC64 polynomial
var crc64Table = crc64.MakeTable(crc64.ECMA)

// 保存 RDB 文件
func saveRDBFile(dir, dbfilename string) error {
    filePath := dir + "/" + dbfilename
    file, err := os.Create(filePath)
    if err != nil {
        return err
    }
    defer file.Close()

    // 创建一个 CRC64 校验和计算器
    crc := crc64.New(crc64Table)

    // 写入 RDB 文件的内容
    writeRDBHeader(file, crc)
    writeRDBMetadata(file, crc) // 添加写入元数据部分
    writeRDBDatabase(file, crc)
    writeRDBFooter(file, crc)   // 添加写入文件结束部分并计算 CRC64 校验和

    return nil
}

// 1-写入 RDB 文件的头部
func writeRDBHeader(file *os.File, crc hash.Hash64) {
    header := []byte("REDIS0011")
    file.Write(header)
    crc.Write(header) // 更新 CRC64 校验和
}

// 2-写入 RDB 文件的元数据部分
func writeRDBMetadata(file *os.File, crc hash.Hash64) {
    // 示例：写入 Redis 版本号
    file.Write([]byte{0xFA}) // Metadata section flag
    file.Write([]byte{0x09}) // Length of the key "redis-ver"
    file.Write([]byte("redis-ver")) // Key
    file.Write([]byte{0x06}) // Length of the value "6.0.16"
    file.Write([]byte("6.0.16")) // Value
    crc.Write([]byte{0xFA, 0x09, 'r', 'e', 'd', 'i', 's', '-', 'v', 'e', 'r', 0x06, '6', '.', '0', '.', '1', '6}) // 更新 CRC64 校验和
}

// 3-写入 RDB 数据部分
func writeRDBDatabase(file *os.File, crc hash.Hash64) {
    file.Write([]byte{0xFE}) // 数据段开始标志
    crc.Write([]byte{0xFE})  // 更新 CRC64 校验和

    // 数据库索引
    file.Write([]byte{0x00}) // 假设只有一个数据库
    crc.Write([]byte{0x00})  // 更新 CRC64 校验和

    // 数据库哈希表大小
    file.Write([]byte{0xFB}) // 哈希表大小标志
    file.Write([]byte{0x03}) // 哈希表大小
    file.Write([]byte{0x02}) // 过期时间哈希表大小
    file.Write([]byte{0x00}) // 代表值类型为字符串
    crc.Write([]byte{0xFB, 0x03, 0x02, 0x00}) // 更新 CRC64 校验和

    // 写入键值对
    writeString(file, crc, "foobar")
    writeString(file, crc, "bazqux")

    // 键值对过期时间戳示例（毫秒）
    file.Write([]byte{0xFC})
    file.Write([]byte{0x15, 0x72, 0xE7, 0x07, 0x8F, 0x01, 0x00, 0x00})
    crc.Write([]byte{0xFC, 0x15, 0x72, 0xE7, 0x07, 0x8F, 0x01, 0x00, 0x00}) // 更新 CRC64 校验和
    writeString(file, crc, "foo")
    writeString(file, crc, "bar")

    // 键值对过期时间戳示例（秒）
    file.Write([]byte{0xFD})
    file.Write([]byte{0x52, 0xED, 0x2A, 0x66})
    crc.Write([]byte{0xFD, 0x52, 0xED, 0x2A, 0x66}) // 更新 CRC64 校验和
    writeString(file, crc, "baz")
    writeString(file, crc, "qux")
}

// 4-写入 RDB 文件的字符串
func writeString(file *os.File, crc hash.Hash64, str string) {
    size := len(str)
    file.Write([]byte{byte(size)}) // 字符串长度
    file.Write([]byte(str))         // 字符串内容
    crc.Write([]byte{byte(size)})  // 更新 CRC64 校验和
    crc.Write([]byte(str))         // 更新 CRC64 校验和
}

// 5-写入文件结束部分
func writeRDBFooter(file *os.File, crc hash.Hash64) {
    file.Write([]byte{0xFF}) // 文件结束标志
    crc.Write([]byte{0xFF})  // 更新 CRC64 校验和

    // 写入 CRC64 校验和（低字节在前，高字节在后）
    crc64Checksum := crc.Sum(nil)
    binary.Write(file, binary.LittleEndian, crc64Checksum) // 写入 CRC64 校验和
    fmt.Printf("CRC64 Checksum: %x\n", crc64Checksum)       // 打印 CRC64 校验和
}

func main() {
    err := saveRDBFile("./app/data", "dump.rdb")
    if err != nil {
        fmt.Println("Error saving RDB file:", err)
    } else {
        fmt.Println("RDB file saved successfully.")
    }
}