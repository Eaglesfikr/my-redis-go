[![progress-banner](https://backend.codecrafters.io/progress/redis/c345ba5f-f1c6-435d-9c1d-ba99d523bb60)](https://app.codecrafters.io/users/codecrafters-bot?r=2qF)

# MY-REDIS-GO

这是我自己去写的redis。这几天在学习redis和go，突发奇想，为什么不用去做一个基本功能的redis呢，双管齐下，既可以学习Go语言，形成流畅的编程习惯，也可以学习redis架构。





# 环境

> go 1.24.1





# 项目结构

```
main.go 负责网络通信
command.go 负责解析和执行命令
store.go 负责数据存储
```





# 特征

实现部分如PING，SET,GET等命令，方法分发机制，实现其REST协议解析

RDB数据快照实现数据持久化

主从复制






# 测试

成功安装redis-cli在WSL里并编译：

```
PS D:\workshop\Go\src\my-redis-go> cd test
PS D:\workshop\Go\src\my-redis-go\test> wsl
***:/mnt/d/workshop/Go/src/my-redis-go/test$ ls
redis-7.2.4  redis.tar.gz
***:/mnt/d/workshop/Go/src/my-redis-go/test$ cd redis-7.2.4/
***:/mnt/d/workshop/Go/src/my-redis-go/test/redis-7.2.4$ ./src/redis-cli
```





# 参见

感谢 https://app.codecrafters.io/courses/redis 提供的开始程序端和过程引导

感谢 https://github.com/redis/go-redis 提供的完整的，风格优雅的软件源代码

This is a starting point for Go solutions to the
["Build Your Own Redis" Challenge](https://codecrafters.io/challenges/redis).

In this challenge, you'll build a toy Redis clone that's capable of handling basic commands like `PING`, `SET` and `GET`. Along the way we'll learn about event loops, the Redis protocol and more.

**Note**: If you're viewing this repo on GitHub, head over to [codecrafters.io](https://codecrafters.io) to try the challenge.

