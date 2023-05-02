// Copyright 2017 fatedier, fatedier@gmail.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"

	"github.com/fatedier/frp/models/config"
	"github.com/fatedier/frp/models/msg"
	"github.com/fatedier/frp/server/controller"
	"github.com/fatedier/frp/server/stats"
	frpNet "github.com/fatedier/frp/utils/net"
	"github.com/fatedier/frp/utils/xlog"

	frpIo "github.com/fatedier/golib/io"
)

type GetWorkConnFn func() (net.Conn, error)

type Proxy interface {
	Context() context.Context
	Run() (remoteAddr string, err error)
	GetName() string
	GetConf() config.ProxyConf
	GetWorkConnFromPool(src, dst net.Addr) (workConn net.Conn, err error)
	GetUsedPortsNum() int
	Close()
}

type BaseProxy struct {
	name           string
	rc             *controller.ResourceController
	statsCollector stats.Collector
	listeners      []net.Listener
	usedPortsNum   int
	poolCount      int
	getWorkConnFn  GetWorkConnFn
	serverCfg      config.ServerCommonConf

	mu  sync.RWMutex
	xl  *xlog.Logger
	ctx context.Context
}

func (pxy *BaseProxy) GetName() string {
	return pxy.name
}

func (pxy *BaseProxy) Context() context.Context {
	return pxy.ctx
}

func (pxy *BaseProxy) GetUsedPortsNum() int {
	return pxy.usedPortsNum
}

func (pxy *BaseProxy) Close() {
	xl := xlog.FromContextSafe(pxy.ctx)
	xl.Info("proxy closing")
	for _, l := range pxy.listeners {
		l.Close()
	}
}

// GetWorkConnFromPool try to get a new work connections from pool
// for quickly response, we immediately send the StartWorkConn message to frpc after take out one from pool
func (pxy *BaseProxy) GetWorkConnFromPool(src, dst net.Addr) (workConn net.Conn, err error) {
	xl := xlog.FromContextSafe(pxy.ctx)
	// try all connections from the pool
	for i := 0; i < pxy.poolCount+1; i++ {
		if workConn, err = pxy.getWorkConnFn(); err != nil {
			xl.Warn("failed to get work connection: %v", err)
			return
		}
		xl.Info("get a new work connection: [%s]", workConn.RemoteAddr().String())
		xl.Spawn().AppendPrefix(pxy.GetName())
		workConn = frpNet.NewContextConn(workConn, pxy.ctx)

		var (
			srcAddr    string
			dstAddr    string
			srcPortStr string
			dstPortStr string
			srcPort    int
			dstPort    int
		)

		if src != nil {
			srcAddr, srcPortStr, _ = net.SplitHostPort(src.String())
			srcPort, _ = strconv.Atoi(srcPortStr)
		}
		if dst != nil {
			dstAddr, dstPortStr, _ = net.SplitHostPort(dst.String())
			dstPort, _ = strconv.Atoi(dstPortStr)
		}
		err := msg.WriteMsg(workConn, &msg.StartWorkConn{
			ProxyName: pxy.GetName(),
			SrcAddr:   srcAddr,
			SrcPort:   uint16(srcPort),
			DstAddr:   dstAddr,
			DstPort:   uint16(dstPort),
		})
		if err != nil {
			xl.Warn("failed to send message to work connection from pool: %v, times: %d", err, i)
			workConn.Close()
		} else {
			break
		}
	}

	if err != nil {
		xl.Error("try to get work connection failed in the end")
		return
	}
	return
}

// startListenHandler start a goroutine handler for each listener.
// p: p will just be passed to handler(Proxy, frpNet.Conn).
// handler: each proxy type can set different handler function to deal with connections accepted from listeners.
func (pxy *BaseProxy) startListenHandler(p Proxy, handler func(Proxy, net.Conn, stats.Collector, config.ServerCommonConf)) {
	xl := xlog.FromContextSafe(pxy.ctx)
	for _, listener := range pxy.listeners {
		go func(l net.Listener) {
			for {
				// block
				// if listener is closed, err returned
				c, err := l.Accept()
				if err != nil {
					xl.Info("listener is closed")
					return
				}
				xl.Debug("get a user connection [%s]", c.RemoteAddr().String())
				go handler(p, c, pxy.statsCollector, pxy.serverCfg)
			}
		}(listener)
	}
}

func NewProxy(ctx context.Context, runId string, rc *controller.ResourceController, statsCollector stats.Collector, poolCount int,
	getWorkConnFn GetWorkConnFn, pxyConf config.ProxyConf, serverCfg config.ServerCommonConf) (pxy Proxy, err error) {

	xl := xlog.FromContextSafe(ctx).Spawn().AppendPrefix(pxyConf.GetBaseInfo().ProxyName)
	basePxy := BaseProxy{
		name:           pxyConf.GetBaseInfo().ProxyName,
		rc:             rc,
		statsCollector: statsCollector,
		listeners:      make([]net.Listener, 0),
		poolCount:      poolCount,
		getWorkConnFn:  getWorkConnFn,
		serverCfg:      serverCfg,
		xl:             xl,
		ctx:            xlog.NewContext(ctx, xl),
	}
	switch cfg := pxyConf.(type) {
	case *config.TcpProxyConf:
		basePxy.usedPortsNum = 1
		pxy = &TcpProxy{
			BaseProxy: &basePxy,
			cfg:       cfg,
		}
	case *config.HttpProxyConf:
		pxy = &HttpProxy{
			BaseProxy: &basePxy,
			cfg:       cfg,
		}
	case *config.HttpsProxyConf:
		pxy = &HttpsProxy{
			BaseProxy: &basePxy,
			cfg:       cfg,
		}
	case *config.UdpProxyConf:
		basePxy.usedPortsNum = 1
		pxy = &UdpProxy{
			BaseProxy: &basePxy,
			cfg:       cfg,
		}
	case *config.StcpProxyConf:
		pxy = &StcpProxy{
			BaseProxy: &basePxy,
			cfg:       cfg,
		}
	case *config.XtcpProxyConf:
		pxy = &XtcpProxy{
			BaseProxy: &basePxy,
			cfg:       cfg,
		}
	default:
		return pxy, fmt.Errorf("proxy type not support")
	}
	return
}

// HandleUserTcpConnection is used for incoming tcp user connections.
// It can be used for tcp, http, https type.
func HandleUserTcpConnection(pxy Proxy, userConn net.Conn, statsCollector stats.Collector, serverCfg config.ServerCommonConf) {
	xl := xlog.FromContextSafe(pxy.Context())
	defer userConn.Close()

	// try all connections from the pool
	workConn, err := pxy.GetWorkConnFromPool(userConn.RemoteAddr(), userConn.LocalAddr())
	if err != nil {
		return
	}
	defer workConn.Close()

	var local io.ReadWriteCloser = workConn
	cfg := pxy.GetConf().GetBaseInfo()
	xl.Trace("handler user tcp connection, use_encryption: %t, use_compression: %t", cfg.UseEncryption, cfg.UseCompression)
	if cfg.UseEncryption {
		local, err = frpIo.WithEncryption(local, []byte(serverCfg.Token))
		if err != nil {
			xl.Error("create encryption stream error: %v", err)
			return
		}
	}
	if cfg.UseCompression {
		local = frpIo.WithCompression(local)
	}
	xl.Debug("join connections, workConn(l[%s] r[%s]) userConn(l[%s] r[%s])", workConn.LocalAddr().String(),
		workConn.RemoteAddr().String(), userConn.LocalAddr().String(), userConn.RemoteAddr().String())

	statsCollector.Mark(stats.TypeOpenConnection, &stats.OpenConnectionPayload{ProxyName: pxy.GetName()})
	inCount, outCount := frpIo.Join(local, userConn)
	statsCollector.Mark(stats.TypeCloseConnection, &stats.CloseConnectionPayload{ProxyName: pxy.GetName()})
	statsCollector.Mark(stats.TypeAddTrafficIn, &stats.AddTrafficInPayload{
		ProxyName:    pxy.GetName(),
		TrafficBytes: inCount,
	})
	statsCollector.Mark(stats.TypeAddTrafficOut, &stats.AddTrafficOutPayload{
		ProxyName:    pxy.GetName(),
		TrafficBytes: outCount,
	})
	xl.Debug("join connections closed")
}

type ProxyManager struct {
	// proxies indexed by proxy name
	pxys map[string]Proxy

	mu sync.RWMutex
}

func NewProxyManager() *ProxyManager {
	return &ProxyManager{
		pxys: make(map[string]Proxy),
	}
}

func (pm *ProxyManager) Add(name string, pxy Proxy) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if _, ok := pm.pxys[name]; ok {
		return fmt.Errorf("proxy name [%s] is already in use", name)
	}

	pm.pxys[name] = pxy
	return nil
}

func (pm *ProxyManager) Del(name string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.pxys, name)
}

func (pm *ProxyManager) GetByName(name string) (pxy Proxy, ok bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	pxy, ok = pm.pxys[name]
	return
}
