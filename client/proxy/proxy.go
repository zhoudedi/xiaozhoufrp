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
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatedier/frp/g"
	"github.com/fatedier/frp/models/config"
	"github.com/fatedier/frp/models/msg"
	"github.com/fatedier/frp/models/plugin"
	"github.com/fatedier/frp/models/proto/udp"
	"github.com/fatedier/frp/utils/log"
	frpNet "github.com/fatedier/frp/utils/net"

	"github.com/fatedier/golib/errors"
	frpIo "github.com/fatedier/golib/io"
	"github.com/fatedier/golib/pool"
	fmux "github.com/hashicorp/yamux"
	pp "github.com/pires/go-proxyproto"
)

// Proxy defines how to handle work connections for different proxy type.
type Proxy interface {
	Run() error

	// InWorkConn accept work connections registered to server.
	InWorkConn(frpNet.Conn, *msg.StartWorkConn)

	Close()
	log.Logger
}

func NewProxy(pxyConf config.ProxyConf) (pxy Proxy) {
	baseProxy := BaseProxy{
		Logger: log.NewPrefixLogger(pxyConf.GetBaseInfo().ProxyName),
	}
	switch cfg := pxyConf.(type) {
	case *config.TcpProxyConf:
		pxy = &TcpProxy{
			BaseProxy: &baseProxy,
			cfg:       cfg,
		}
	case *config.UdpProxyConf:
		pxy = &UdpProxy{
			BaseProxy: &baseProxy,
			cfg:       cfg,
		}
	case *config.HttpProxyConf:
		pxy = &HttpProxy{
			BaseProxy: &baseProxy,
			cfg:       cfg,
		}
	case *config.HttpsProxyConf:
		pxy = &HttpsProxy{
			BaseProxy: &baseProxy,
			cfg:       cfg,
		}
	case *config.StcpProxyConf:
		pxy = &StcpProxy{
			BaseProxy: &baseProxy,
			cfg:       cfg,
		}
	case *config.XtcpProxyConf:
		pxy = &XtcpProxy{
			BaseProxy: &baseProxy,
			cfg:       cfg,
		}
	}
	return
}

type BaseProxy struct {
	closed bool
	mu     sync.RWMutex
	log.Logger
}

// TCP
type TcpProxy struct {
	*BaseProxy

	cfg         *config.TcpProxyConf
	proxyPlugin plugin.Plugin
}

func (pxy *TcpProxy) Run() (err error) {
	if pxy.cfg.Plugin != "" {
		pxy.proxyPlugin, err = plugin.Create(pxy.cfg.Plugin, pxy.cfg.PluginParams)
		if err != nil {
			return
		}
	}
	return
}

func (pxy *TcpProxy) Close() {
	if pxy.proxyPlugin != nil {
		pxy.proxyPlugin.Close()
	}
}

func (pxy *TcpProxy) InWorkConn(conn frpNet.Conn, m *msg.StartWorkConn) {
	HandleTcpWorkConnection(&pxy.cfg.LocalSvrConf, pxy.proxyPlugin, &pxy.cfg.BaseProxyConf, conn,
		[]byte(g.GlbClientCfg.Token), m)
}

// HTTP
type HttpProxy struct {
	*BaseProxy

	cfg         *config.HttpProxyConf
	proxyPlugin plugin.Plugin
}

func (pxy *HttpProxy) Run() (err error) {
	if pxy.cfg.Plugin != "" {
		pxy.proxyPlugin, err = plugin.Create(pxy.cfg.Plugin, pxy.cfg.PluginParams)
		if err != nil {
			return
		}
	}
	return
}

func (pxy *HttpProxy) Close() {
	if pxy.proxyPlugin != nil {
		pxy.proxyPlugin.Close()
	}
}

func (pxy *HttpProxy) InWorkConn(conn frpNet.Conn, m *msg.StartWorkConn) {
	HandleTcpWorkConnection(&pxy.cfg.LocalSvrConf, pxy.proxyPlugin, &pxy.cfg.BaseProxyConf, conn,
		[]byte(g.GlbClientCfg.Token), m)
}

// HTTPS
type HttpsProxy struct {
	*BaseProxy

	cfg         *config.HttpsProxyConf
	proxyPlugin plugin.Plugin
}

func (pxy *HttpsProxy) Run() (err error) {
	if pxy.cfg.Plugin != "" {
		pxy.proxyPlugin, err = plugin.Create(pxy.cfg.Plugin, pxy.cfg.PluginParams)
		if err != nil {
			return
		}
	}
	return
}

func (pxy *HttpsProxy) Close() {
	if pxy.proxyPlugin != nil {
		pxy.proxyPlugin.Close()
	}
}

func (pxy *HttpsProxy) InWorkConn(conn frpNet.Conn, m *msg.StartWorkConn) {
	HandleTcpWorkConnection(&pxy.cfg.LocalSvrConf, pxy.proxyPlugin, &pxy.cfg.BaseProxyConf, conn,
		[]byte(g.GlbClientCfg.Token), m)
}

// STCP
type StcpProxy struct {
	*BaseProxy

	cfg         *config.StcpProxyConf
	proxyPlugin plugin.Plugin
}

func (pxy *StcpProxy) Run() (err error) {
	if pxy.cfg.Plugin != "" {
		pxy.proxyPlugin, err = plugin.Create(pxy.cfg.Plugin, pxy.cfg.PluginParams)
		if err != nil {
			return
		}
	}
	return
}

func (pxy *StcpProxy) Close() {
	if pxy.proxyPlugin != nil {
		pxy.proxyPlugin.Close()
	}
}

func (pxy *StcpProxy) InWorkConn(conn frpNet.Conn, m *msg.StartWorkConn) {
	HandleTcpWorkConnection(&pxy.cfg.LocalSvrConf, pxy.proxyPlugin, &pxy.cfg.BaseProxyConf, conn,
		[]byte(g.GlbClientCfg.Token), m)
}

// XTCP
type XtcpProxy struct {
	*BaseProxy

	cfg         *config.XtcpProxyConf
	proxyPlugin plugin.Plugin
}

func (pxy *XtcpProxy) Run() (err error) {
	if pxy.cfg.Plugin != "" {
		pxy.proxyPlugin, err = plugin.Create(pxy.cfg.Plugin, pxy.cfg.PluginParams)
		if err != nil {
			return
		}
	}
	return
}

func (pxy *XtcpProxy) Close() {
	if pxy.proxyPlugin != nil {
		pxy.proxyPlugin.Close()
	}
}

func (pxy *XtcpProxy) InWorkConn(conn frpNet.Conn, m *msg.StartWorkConn) {
	defer conn.Close()
	var natHoleSidMsg msg.NatHoleSid
	err := msg.ReadMsgInto(conn, &natHoleSidMsg)
	if err != nil {
		pxy.Error("xtcp read from workConn error: %v", err)
		return
	}

	natHoleClientMsg := &msg.NatHoleClient{
		ProxyName: pxy.cfg.ProxyName,
		Sid:       natHoleSidMsg.Sid,
	}
	raddr, _ := net.ResolveUDPAddr("udp",
		fmt.Sprintf("%s:%d", g.GlbClientCfg.ServerAddr, g.GlbClientCfg.ServerUdpPort))
	clientConn, err := net.DialUDP("udp", nil, raddr)
	defer clientConn.Close()

	err = msg.WriteMsg(clientConn, natHoleClientMsg)
	if err != nil {
		pxy.Error("send natHoleClientMsg to server error: %v", err)
		return
	}

	// Wait for client address at most 5 seconds.
	var natHoleRespMsg msg.NatHoleResp
	clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))

	buf := pool.GetBuf(1024)
	n, err := clientConn.Read(buf)
	if err != nil {
		pxy.Error("get natHoleRespMsg error: %v", err)
		return
	}
	err = msg.ReadMsgInto(bytes.NewReader(buf[:n]), &natHoleRespMsg)
	if err != nil {
		pxy.Error("get natHoleRespMsg error: %v", err)
		return
	}
	clientConn.SetReadDeadline(time.Time{})
	clientConn.Close()

	if natHoleRespMsg.Error != "" {
		pxy.Error("natHoleRespMsg get error info: %s", natHoleRespMsg.Error)
		return
	}

	pxy.Trace("get natHoleRespMsg, sid [%s], client address [%s] visitor address [%s]", natHoleRespMsg.Sid, natHoleRespMsg.ClientAddr, natHoleRespMsg.VisitorAddr)

	// Send detect message
	array := strings.Split(natHoleRespMsg.VisitorAddr, ":")
	if len(array) <= 1 {
		pxy.Error("get NatHoleResp visitor address error: %v", natHoleRespMsg.VisitorAddr)
	}
	laddr, _ := net.ResolveUDPAddr("udp", clientConn.LocalAddr().String())
	/*
		for i := 1000; i < 65000; i++ {
			pxy.sendDetectMsg(array[0], int64(i), laddr, "a")
		}
	*/
	port, err := strconv.ParseInt(array[1], 10, 64)
	if err != nil {
		pxy.Error("get natHoleResp visitor address error: %v", natHoleRespMsg.VisitorAddr)
		return
	}
	pxy.sendDetectMsg(array[0], int(port), laddr, []byte(natHoleRespMsg.Sid))
	pxy.Trace("send all detect msg done")

	msg.WriteMsg(conn, &msg.NatHoleClientDetectOK{})

	// Listen for clientConn's address and wait for visitor connection
	lConn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		pxy.Error("listen on visitorConn's local adress error: %v", err)
		return
	}
	defer lConn.Close()

	lConn.SetReadDeadline(time.Now().Add(8 * time.Second))
	sidBuf := pool.GetBuf(1024)
	var uAddr *net.UDPAddr
	n, uAddr, err = lConn.ReadFromUDP(sidBuf)
	if err != nil {
		pxy.Warn("get sid from visitor error: %v", err)
		return
	}
	lConn.SetReadDeadline(time.Time{})
	if string(sidBuf[:n]) != natHoleRespMsg.Sid {
		pxy.Warn("incorrect sid from visitor")
		return
	}
	pool.PutBuf(sidBuf)
	pxy.Info("nat hole connection make success, sid [%s]", natHoleRespMsg.Sid)

	lConn.WriteToUDP(sidBuf[:n], uAddr)

	kcpConn, err := frpNet.NewKcpConnFromUdp(lConn, false, natHoleRespMsg.VisitorAddr)
	if err != nil {
		pxy.Error("create kcp connection from udp connection error: %v", err)
		return
	}

	fmuxCfg := fmux.DefaultConfig()
	fmuxCfg.KeepAliveInterval = 5 * time.Second
	fmuxCfg.LogOutput = ioutil.Discard
	sess, err := fmux.Server(kcpConn, fmuxCfg)
	if err != nil {
		pxy.Error("create yamux server from kcp connection error: %v", err)
		return
	}
	defer sess.Close()
	muxConn, err := sess.Accept()
	if err != nil {
		pxy.Error("accept for yamux connection error: %v", err)
		return
	}

	HandleTcpWorkConnection(&pxy.cfg.LocalSvrConf, pxy.proxyPlugin, &pxy.cfg.BaseProxyConf,
		frpNet.WrapConn(muxConn), []byte(pxy.cfg.Sk), m)
}

func (pxy *XtcpProxy) sendDetectMsg(addr string, port int, laddr *net.UDPAddr, content []byte) (err error) {
	daddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", addr, port))
	if err != nil {
		return err
	}

	tConn, err := net.DialUDP("udp", laddr, daddr)
	if err != nil {
		return err
	}

	//uConn := ipv4.NewConn(tConn)
	//uConn.SetTTL(3)

	tConn.Write(content)
	tConn.Close()
	return nil
}

// UDP
type UdpProxy struct {
	*BaseProxy

	cfg *config.UdpProxyConf

	localAddr *net.UDPAddr
	readCh    chan *msg.UdpPacket

	// include msg.UdpPacket and msg.Ping
	sendCh   chan msg.Message
	workConn frpNet.Conn
}

func (pxy *UdpProxy) Run() (err error) {
	pxy.localAddr, err = net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", pxy.cfg.LocalIp, pxy.cfg.LocalPort))
	if err != nil {
		return
	}
	return
}

func (pxy *UdpProxy) Close() {
	pxy.mu.Lock()
	defer pxy.mu.Unlock()

	if !pxy.closed {
		pxy.closed = true
		if pxy.workConn != nil {
			pxy.workConn.Close()
		}
		if pxy.readCh != nil {
			close(pxy.readCh)
		}
		if pxy.sendCh != nil {
			close(pxy.sendCh)
		}
	}
}

func (pxy *UdpProxy) InWorkConn(conn frpNet.Conn, m *msg.StartWorkConn) {
	pxy.Info("incoming a new work connection for udp proxy, %s", conn.RemoteAddr().String())
	// close resources releated with old workConn
	pxy.Close()

	pxy.mu.Lock()
	pxy.workConn = conn
	pxy.readCh = make(chan *msg.UdpPacket, 1024)
	pxy.sendCh = make(chan msg.Message, 1024)
	pxy.closed = false
	pxy.mu.Unlock()

	workConnReaderFn := func(conn net.Conn, readCh chan *msg.UdpPacket) {
		for {
			var udpMsg msg.UdpPacket
			if errRet := msg.ReadMsgInto(conn, &udpMsg); errRet != nil {
				pxy.Warn("read from workConn for udp error: %v", errRet)
				return
			}
			if errRet := errors.PanicToError(func() {
				pxy.Trace("get udp package from workConn: %s", udpMsg.Content)
				readCh <- &udpMsg
			}); errRet != nil {
				pxy.Info("reader goroutine for udp work connection closed: %v", errRet)
				return
			}
		}
	}
	workConnSenderFn := func(conn net.Conn, sendCh chan msg.Message) {
		defer func() {
			pxy.Info("writer goroutine for udp work connection closed")
		}()
		var errRet error
		for rawMsg := range sendCh {
			switch m := rawMsg.(type) {
			case *msg.UdpPacket:
				pxy.Trace("send udp package to workConn: %s", m.Content)
			case *msg.Ping:
				pxy.Trace("send ping message to udp workConn")
			}
			if errRet = msg.WriteMsg(conn, rawMsg); errRet != nil {
				pxy.Error("udp work write error: %v", errRet)
				return
			}
		}
	}
	heartbeatFn := func(conn net.Conn, sendCh chan msg.Message) {
		var errRet error
		for {
			time.Sleep(time.Duration(30) * time.Second)
			if errRet = errors.PanicToError(func() {
				sendCh <- &msg.Ping{}
			}); errRet != nil {
				pxy.Trace("heartbeat goroutine for udp work connection closed")
				break
			}
		}
	}

	go workConnSenderFn(pxy.workConn, pxy.sendCh)
	go workConnReaderFn(pxy.workConn, pxy.readCh)
	go heartbeatFn(pxy.workConn, pxy.sendCh)
	udp.Forwarder(pxy.localAddr, pxy.readCh, pxy.sendCh)
}

// Common handler for tcp work connections.
func HandleTcpWorkConnection(localInfo *config.LocalSvrConf, proxyPlugin plugin.Plugin,
	baseInfo *config.BaseProxyConf, workConn frpNet.Conn, encKey []byte, m *msg.StartWorkConn) {

	var (
		remote io.ReadWriteCloser
		err    error
	)
	remote = workConn

	if baseInfo.UseEncryption {
		remote, err = frpIo.WithEncryption(remote, encKey)
		if err != nil {
			workConn.Close()
			workConn.Error("create encryption stream error: %v", err)
			return
		}
	}
	if baseInfo.UseCompression {
		remote = frpIo.WithCompression(remote)
	}

	// check if we need to send proxy protocol info
	var extraInfo []byte
	if baseInfo.ProxyProtocolVersion != "" {
		if m.SrcAddr != "" && m.SrcPort != 0 {
			if m.DstAddr == "" {
				m.DstAddr = "127.0.0.1"
			}
			h := &pp.Header{
				Command:            pp.PROXY,
				SourceAddress:      net.ParseIP(m.SrcAddr),
				SourcePort:         m.SrcPort,
				DestinationAddress: net.ParseIP(m.DstAddr),
				DestinationPort:    m.DstPort,
			}

			if h.SourceAddress.To16() == nil {
				h.TransportProtocol = pp.TCPv4
			} else {
				h.TransportProtocol = pp.TCPv6
			}

			if baseInfo.ProxyProtocolVersion == "v1" {
				h.Version = 1
			} else if baseInfo.ProxyProtocolVersion == "v2" {
				h.Version = 2
			}

			buf := bytes.NewBuffer(nil)
			h.WriteTo(buf)
			extraInfo = buf.Bytes()
		}
	}

	if proxyPlugin != nil {
		// if plugin is set, let plugin handle connections first
		workConn.Debug("handle by plugin: %s", proxyPlugin.Name())
		proxyPlugin.Handle(remote, workConn, extraInfo)
		workConn.Debug("handle by plugin finished")
		return
	} else {
		localConn, err := frpNet.ConnectServer("tcp", fmt.Sprintf("%s:%d", localInfo.LocalIp, localInfo.LocalPort))
		if err != nil {
			workConn.Close()
			workConn.Error("connect to local service [%s:%d] error: %v", localInfo.LocalIp, localInfo.LocalPort, err)
			return
		}

		workConn.Debug("join connections, localConn(l[%s] r[%s]) workConn(l[%s] r[%s])", localConn.LocalAddr().String(),
			localConn.RemoteAddr().String(), workConn.LocalAddr().String(), workConn.RemoteAddr().String())

		if len(extraInfo) > 0 {
			localConn.Write(extraInfo)
		}

		frpIo.Join(localConn, remote)
		workConn.Debug("join connections closed")
	}
}
