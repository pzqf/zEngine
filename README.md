# zEngine

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## 项目概述

zEngine 是一个轻量级但功能强大的**分布式游戏服务器引擎框架**，采用 Go 语言开发，提供了多种实用模块，适用于各种服务器应用场景，从游戏服务器到 Web 应用后端。

## 技术栈

| 类别 | 技术 |
|------|------|
| **开发语言** | Go 1.25+ |
| **网络协议** | TCP / UDP / WebSocket / HTTP |
| **数据格式** | Protobuf / JSON / XML |
| **加密技术** | AES-GCM / ECDH (Elliptic Curve Diffie-Hellman) |
| **日志框架** | zap |
| **并发模型** | Actor / Event-Driven |

## 核心模块

```
zEngine/
├── zNet/          # 网络层 - TCP/UDP/WebSocket/HTTP 服务器和客户端
├── zLog/          # 日志系统 - 基于 zap 的结构化日志
├── zEvent/        # 事件总线 - 发布-订阅模式
├── zActor/        # Actor 并发模型
├── zObject/       # 对象管理和对象池
├── zService/      # 服务管理 - 服务发现、依赖管理
├── zInject/       # 依赖注入容器
├── zScript/       # 脚本系统 - 行为树支持
├── zNavigationMap/# 导航寻路 - A* 算法
├── zEtcd/         # Etcd 客户端
├── zSignal/       # 信号处理
└── zSystem/       # 系统管理框架
```

## 主要特性

- 🏗️ **模块化设计** - 各模块独立，可选择性使用
- ⚡ **高性能** - 基于 Go 的并发特性，支持高并发
- 🔧 **可扩展** - 清晰的接口设计，便于扩展新功能
- 📖 **易维护** - 模块化架构，代码清晰易读
- 🎯 **丰富功能** - 包含网络、事件、服务、日志等核心模块

## 快速开始

### 安装

```bash
go get github.com/pzqf/zEngine
```

### 使用示例

#### TCP 服务器

```go
package main

import (
    "github.com/pzqf/zEngine/zNet"
    "github.com/pzqf/zEngine/zLog"
)

func main() {
    zLog.PrintLogo("My Server", "1.0.0")

    config := zNet.TcpConfig{
        ListenAddress: ":8080",
    }

    server := zNet.NewTcpServer(config)
    server.Start()
}
```

#### 日志系统

```go
package main

import "github.com/pzqf/zEngine/zLog"

func main() {
    logger := zLog.NewLogger(zLog.Config{
        LogLevel: zLog.DebugLevel,
        LogFile:  "server.log",
    })

    logger.Info("Server started")
}
```

## 开发状态

| 模块 | 状态 | 说明 |
|------|------|------|
| zNet | ✅ 完成 | TCP/UDP/WebSocket/HTTP 完整实现 |
| zLog | ✅ 完成 | 日志系统 + Logo 显示 |
| zEvent | ✅ 完成 | 事件总线 |
| zActor | ✅ 完成 | Actor 模型 |
| zObject | ✅ 完成 | 对象池和对象管理 |
| zService | ✅ 完成 | 服务管理 |
| zInject | ✅ 完成 | 依赖注入 |
| zScript | ✅ 完成 | 行为树脚本 |
| zNavigationMap | ✅ 完成 | A* 寻路算法 |

## 许可证

MIT License

---

**最后更新**: 2026-02-10
