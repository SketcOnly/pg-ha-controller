package etcdclient

import (
	"context"
	"github.com/pg-ha-controller/pkg/config"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/server/v3/embed"
	"net/url"
	"testing"
	"time"
)

// 修复后的嵌入式 Etcd 启动函数（移除未定义字段）
func startEmbeddedEtcd(t *testing.T) *embed.Etcd {
	t.Helper()
	
	// 1. 为当前测试创建独立的context（替代t.Done()）
	testCtx, testCancel := context.WithCancel(context.Background())
	// 测试结束时取消context（触发goroutine退出）
	t.Cleanup(func() {
		testCancel()
	})
	
	// 临时数据目录
	dir := t.TempDir()
	
	// Etcd 基础配置（仅保留存在的字段）
	cfg := embed.NewConfig()
	cfg.Dir = dir
	cfg.Logger = "zap"
	cfg.LogLevel = "error"
	
	// 1. 修正：分开 Peer/Client 随机端口，避免混淆
	peerURL, err := url.Parse("http://127.0.0.1:0")
	require.NoError(t, err, "解析 Peer URL 失败")
	clientURL, err := url.Parse("http://127.0.0.1:0")
	require.NoError(t, err, "解析 Client URL 失败")
	
	// 2. 修正：URL 赋值一一对应，避免端口混乱
	cfg.ListenPeerUrls = []url.URL{*peerURL}        // Peer 监听地址（随机端口）
	cfg.ListenClientUrls = []url.URL{*clientURL}    // Client 监听地址（随机端口）
	cfg.AdvertisePeerUrls = []url.URL{*peerURL}     // Peer 广告地址
	cfg.AdvertiseClientUrls = []url.URL{*clientURL} // Client 广告地址
	cfg.InitialCluster = cfg.InitialClusterFromName(cfg.Name)
	
	// 启动 Etcd
	e, err := embed.StartEtcd(cfg)
	require.NoError(t, err, "启动嵌入式 Etcd 失败")
	
	// 3. 关键：监听 Etcd 错误通道，避免静默阻塞
	go func() {
		select {
		case err := <-e.Err():
			t.Errorf("嵌入式 Etcd 运行出错: %v", err)
		case <-testCtx.Done():
			return
		}
	}()
	
	// 等待 Etcd 启动完成（延长超时到15秒，避免偶发超时）
	select {
	case <-e.Server.ReadyNotify():
		t.Logf("嵌入式 Etcd 启动成功，Client 地址: %s", e.Config().ListenClientUrls[0].String())
	case <-testCtx.Done():
		t.Fatal("测试提前结束，Etcd启动被中断")
	
	case <-time.After(15 * time.Second):
		t.Fatal("等待 Etcd 启动超时（15秒）")
	}
	
	// 4. 修正：完整释放 Etcd 资源（先 Stop Server，再 Close 实例）
	t.Cleanup(func() {
		if e != nil {
			e.Server.Stop() // 停止 Server
			e.Close()       // 关闭整个 Etcd 实例，释放端口/目录
		}
	})
	
	return e
}

// 修复后的客户端创建测试
func TestNewClient(t *testing.T) {
	// 启动嵌入式 Etcd
	etcd := startEmbeddedEtcd(t)
	clientAddr := etcd.Config().ListenClientUrls[0].String()
	
	// 构建测试配置
	cfg := &config.EtcdConfig{
		Endpoints:   []string{clientAddr},
		CAFile:      "",
		CertFile:    "",
		KeyFile:     "",
		LeaseTTL:    10,
		DialTimeout: 5 * time.Second,
	}
	
	// 关键：创建客户端时传入带超时的上下文（避免 NewClient 内阻塞）
	_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// 若 NewClient 不支持上下文，需确保内部所有操作都有超时
	client, err := NewClient(cfg)
	require.NoError(t, err, "创建 Etcd 客户端失败")
	
	// 安全关闭客户端（增加空指针判断）
	defer func() {
		if client != nil && client.cli != nil {
			client.cli.Close()
			t.Log("Etcd 客户端已关闭")
		}
	}()
	
	// 额外验证：客户端能正常执行基础操作（避免假成功）
	ctxGet, cancelGet := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelGet()
	_, err = client.cli.Get(ctxGet, "test-key")
	require.NoError(t, err, "客户端执行 Get 操作失败（验证连接有效性）")
}
