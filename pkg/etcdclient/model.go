package etcdclient

import (
	"context"
	"github.com/pg-ha-controller/pkg/config"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"sync"
	"time"
)

// Client etcd客户端封装，包含连接、租约、会话等核心资源
type Client struct {
	cfg     *config.EtcdConfig
	cli     *clientv3.Client
	session *concurrency.Session
	leaseID clientv3.LeaseID
	
	mu          sync.RWMutex                            // 保护会话/租约的并发访问
	leaseCtx    context.Context                         // 租约续期上下文
	leaseCancel context.CancelFunc                      // 租约续期取消函数
	keepAliveCh <-chan *clientv3.LeaseKeepAliveResponse // 租约续期通道
}

// LeaderElectionConfig Leader选举配置
type LeaderElectionConfig struct {
	Scope           string        // 选举作用域（避免不同集群冲突）
	NodeName        string        // 当前节点名称
	CheckInterval   time.Duration // 节点状态检查间隔
	RetryInterval   time.Duration // 锁获取重试基础间隔（默认1s）
	MaxRetryBackoff time.Duration // 最大重试退避间隔（默认10s）
}

// LeaderElection Leader选举实例
type LeaderElection struct {
	cfg        *LeaderElectionConfig
	client     *Client
	mutex      *concurrency.Mutex
	LeaderChan chan bool // Leader状态变更通道（true=成为Leader，false=失去Leader）
	
	isLeader bool          // 当前是否为Leader
	mu       sync.RWMutex  // 保护isLeader的并发访问
	stopChan chan struct{} // 优雅退出通道
}
