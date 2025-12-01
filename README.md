## 基于 Go 实现 Patroni 核心功能 + etcd 客户端（适配官方 PG 镜像）

不用 Python 版 Patroni，而是通过 Go 编写高可用控制器（Sidecar 模式） 适配官方 PostgreSQL 镜像，结合 etcd 实现 PG 三节点集群的选主、故障检测、主从管理等核心功能。核心思路是：官方 PG 容器 + Go 控制容器（Sidecar） ，共享网络 / 数据卷，Go 程序负责和 etcd 交互、管理 PG 实例。

### 整体架构（Sidecar 模式）

|组件|说明|
|-|-|
|官方 PG 镜像|仅运行 PostgreSQL 服务，无额外依赖，数据卷持久化|
|Go 控制程序（Sidecar）|1. etcd 客户端（选主、状态存储）；2. PG 实例管理；3. 健康检测；4. 主从同步|
|etcd 集群|分布式配置存储（DCS），存储集群状态、Leader 锁、PG 配置|
|自定义网络|PG 容器 + Go 容器共享网络，容器名直接解析|

### 核心优势

+ 复用官方 PG 镜像，无需构建自定义镜像；
+ Go 程序编译为静态二进制，体积小、性能高、跨平台；
+ 完全掌控高可用逻辑，可按需定制（比 Python Patroni 更轻量）。

|原则|设计实现|
|-|-|
|模块化|按职责拆分模块（配置、etcd、PG、选主、API、监控、日志），低耦合高内聚|
|高可用|etcd 租约自动续期、多维度健康检测、脑裂防护、自动故障切换|
|可观测性|Prometheus 监控指标、结构化日志（Zap）、HTTP 健康检查、审计日志|
|容错性|操作重试机制、连接池管理、异常自动恢复、优雅退出|
|安全性|TLS 加密（etcd/PG）、最小权限原则、密码通过环境变量 / 配置文件注入|
|可配置性|支持 YAML 配置文件 + 环境变量 + 命令行参数，优先级：命令行 > 环境变量 > 配置文件|

### 关键特性

+ 脑裂防护：通过 etcd 租约 + 节点状态双校验，确保同一时间只有一个 Leader；
+ 复制延迟检测：监控主从复制延迟，超过阈值触发告警；
+ 手动 / 自动切换：支持 HTTP API 手动切换主节点，自动检测故障并切换；
+ 配置热更新：支持通过配置文件 / API 更新 PG 参数，无需重启；
+ 结构化日志：输出 JSON 格式日志，便于日志平台采集分析；
+ 监控指标：暴露 PG 状态、选主次数、复制延迟、etcd 连接状态等指标。

### 工程目录

```plaintext
pg-ha-controller/
├── cmd/                          # 程序入口
│   └── pg-ha-controller/
│       └── main.go               # 主程序入口
├── pkg/                          # 核心业务包
│   ├── config/                   # 配置管理
│   │   ├── config.go             # 配置结构体 + 加载逻辑
│   │   └── parser.go             # 配置解析（命令行/环境变量/文件）
│   ├── etcdclient/               # etcd 客户端封装
│   │   ├── client.go             # etcd 连接 + 租约管理
│   │   └── leader.go             # Leader 选举 + 状态存储
│   ├── pgmanager/                # PG 实例管理
│   │   ├── backup.go             # 基础备份
│   │   ├── config.go             # PG 参数配置
│   │   ├── connection.go         # PG 连接池
│   │   ├── replication.go        # 复制管理
│   │   └── status.go             # PG 状态检测
│   ├── health/                   # 健康检测
│   │   ├── checker.go            # 多维度健康检测
│   │   └── repl_delay.go         # 复制延迟检测
│   ├── api/                      # HTTP API
│   │   ├── handler.go            # API 处理器
│   │   ├── middleware.go         # 中间件（日志/监控）
│   │   └── router.go             # 路由注册
│   ├── metrics/                  # Prometheus 监控
│   │   └── metrics.go            # 指标定义 + 采集
│   ├── logger/                   # 结构化日志
│   │   └── logger.go             # Zap 日志初始化
│   └── signal/                   # 信号处理
│       └── signal.go             # 优雅退出
├── configs/                      # 配置文件示例
│   └── pg-ha-controller.yaml     # 生产配置示例
├── scripts/                      # 辅助脚本
│   ├── build.sh                  # 编译脚本
│   ├── deploy.sh                 # 部署脚本
│   └── monitor_rules.yml         # Prometheus 告警规则
├── deploy/                       # 部署配置
│   └── docker-compose.yml        # 生产部署 Compose
├── go.mod                        # Go 模块依赖
└── go.sum                        # 依赖校验
```

