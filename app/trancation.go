package main

import (
    "sync"
)

// type Multicmd struct {
//     isInMulti bool
//     queue     [][]string
// }

// var Multicmds = make(map[string]*Multicmd)
// var mu sync.Mutex

// 线程安全锁
var mu sync.Mutex

// 事务状态
var isInTransaction bool = false
var transactionQueue [][]string // 存储事务中的命令和参数



