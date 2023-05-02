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

package client

import (
	"crypto/tls"
	"fmt"
	"io"
	"runtime/debug"
	"sync"
	"time"

	"github.com/fatedier/frp/client/proxy"
	"github.com/fatedier/frp/g"
	"github.com/fatedier/frp/models/config"
	"github.com/fatedier/frp/models/msg"
	"github.com/fatedier/frp/utils/log"
	frpNet "github.com/fatedier/frp/utils/net"

	"github.com/fatedier/golib/control/shutdown"
	"github.com/fatedier/golib/crypto"
	fmux "github.com/hashicorp/yamux"
)

type Control struct {
	// uniq id got from frps, attach it in loginMsg
	runId string

	// manage all proxies
	pxyCfgs map[string]config.ProxyConf
	pm      *proxy.ProxyManager

	// manage all visitors
	vm *VisitorManager

	// control connection
	conn frpNet.Conn

	// tcp stream multiplexing, if enabled
	session *fmux.Session

	// put a message in this channel to send it over control connection to server
	sendCh chan (msg.Message)

	// read from this channel to get the next message sent by server
	readCh chan (msg.Message)

	// goroutines can block by reading from this channel, it will be closed only in reader() when control connection is closed
	closedCh chan struct{}

	closedDoneCh chan struct{}

	// last time got the Pong message
	lastPong time.Time

	readerShutdown     *shutdown.Shutdown
	writerShutdown     *shutdown.Shutdown
	msgHandlerShutdown *shutdown.Shutdown

	mu sync.RWMutex

	log.Logger
}

func NewControl(runId string, conn frpNet.Conn, session *fmux.Session, pxyCfgs map[string]config.ProxyConf, visitorCfgs map[string]config.VisitorConf) *Control {
	ctl := &Control{
		runId:              runId,
		conn:               conn,
		session:            session,
		pxyCfgs:            pxyCfgs,
		sendCh:             make(chan msg.Message, 100),
		readCh:             make(chan msg.Message, 100),
		closedCh:           make(chan struct{}),
		closedDoneCh:       make(chan struct{}),
		readerShutdown:     shutdown.New(),
		writerShutdown:     shutdown.New(),
		msgHandlerShutdown: shutdown.New(),
		Logger:             log.NewPrefixLogger(""),
	}
	ctl.pm = proxy.NewProxyManager(ctl.sendCh, runId)

	ctl.vm = NewVisitorManager(ctl)
	ctl.vm.Reload(visitorCfgs)
	return ctl
}

func (ctl *Control) Run() {
	go ctl.worker()

	// start all proxies
	ctl.pm.Reload(ctl.pxyCfgs)

	// start all visitors
	go ctl.vm.Run()
	return
}

func (ctl *Control) HandleReqWorkConn(inMsg *msg.ReqWorkConn) {
	workConn, err := ctl.connectServer()
	if err != nil {
		return
	}

	m := &msg.NewWorkConn{
		RunId: ctl.runId,
	}
	if err = msg.WriteMsg(workConn, m); err != nil {
		ctl.Warn("work connection write to server error: %v", err)
		workConn.Close()
		return
	}

	var startMsg msg.StartWorkConn
	if err = msg.ReadMsgInto(workConn, &startMsg); err != nil {
		ctl.Error("work connection closed, %v", err)
		workConn.Close()
		return
	}
	workConn.AddLogPrefix(startMsg.ProxyName)

	// dispatch this work connection to related proxy
	ctl.pm.HandleWorkConn(startMsg.ProxyName, workConn, &startMsg)
}

func (ctl *Control) HandleNewProxyResp(inMsg *msg.NewProxyResp) {
	// Server will return NewProxyResp message to each NewProxy message.
	// Start a new proxy handler if no error got
	err := ctl.pm.StartProxy(inMsg.ProxyName, inMsg.RemoteAddr, inMsg.Error)
	if err != nil {
		ctl.Warn("[%s] start error: %v", inMsg.ProxyName, err)
	} else {
		ctl.Info("[%s] start proxy success", inMsg.ProxyName)
	}
}

func (ctl *Control) Close() error {
	ctl.pm.Close()
	ctl.conn.Close()
	if ctl.session != nil {
		ctl.session.Close()
	}
	return nil
}

// ClosedDoneCh returns a channel which will be closed after all resources are released
func (ctl *Control) ClosedDoneCh() <-chan struct{} {
	return ctl.closedDoneCh
}

// connectServer return a new connection to frps
func (ctl *Control) connectServer() (conn frpNet.Conn, err error) {
	if g.GlbClientCfg.TcpMux {
		stream, errRet := ctl.session.OpenStream()
		if errRet != nil {
			err = errRet
			ctl.Warn("start new connection to server error: %v", err)
			return
		}
		conn = frpNet.WrapConn(stream)
	} else {
		var tlsConfig *tls.Config
		if g.GlbClientCfg.TLSEnable {
			tlsConfig = &tls.Config{
				InsecureSkipVerify: true,
			}
		}
		conn, err = frpNet.ConnectServerByProxyWithTLS(g.GlbClientCfg.HttpProxy, g.GlbClientCfg.Protocol,
			fmt.Sprintf("%s:%d", g.GlbClientCfg.ServerAddr, g.GlbClientCfg.ServerPort), tlsConfig)
		if err != nil {
			ctl.Warn("start new connection to server error: %v", err)
			return
		}
	}
	return
}

// reader read all messages from frps and send to readCh
func (ctl *Control) reader() {
	defer func() {
		if err := recover(); err != nil {
			ctl.Error("panic error: %v", err)
			ctl.Error(string(debug.Stack()))
		}
	}()
	defer ctl.readerShutdown.Done()
	defer close(ctl.closedCh)

	encReader := crypto.NewReader(ctl.conn, []byte(g.GlbClientCfg.Token))
	for {
		if m, err := msg.ReadMsg(encReader); err != nil {
			if err == io.EOF {
				ctl.Debug("read from control connection EOF")
				return
			} else {
				ctl.Warn("read error: %v", err)
				ctl.conn.Close()
				return
			}
		} else {
			ctl.readCh <- m
		}
	}
}

// writer writes messages got from sendCh to frps
func (ctl *Control) writer() {
	defer ctl.writerShutdown.Done()
	encWriter, err := crypto.NewWriter(ctl.conn, []byte(g.GlbClientCfg.Token))
	if err != nil {
		ctl.conn.Error("crypto new writer error: %v", err)
		ctl.conn.Close()
		return
	}
	for {
		if m, ok := <-ctl.sendCh; !ok {
			ctl.Info("control writer is closing")
			return
		} else {
			if err := msg.WriteMsg(encWriter, m); err != nil {
				ctl.Warn("write message to control connection error: %v", err)
				return
			}
		}
	}
}

// msgHandler handles all channel events and do corresponding operations.
func (ctl *Control) msgHandler() {
	defer func() {
		if err := recover(); err != nil {
			ctl.Error("panic error: %v", err)
			ctl.Error(string(debug.Stack()))
		}
	}()
	defer ctl.msgHandlerShutdown.Done()

	hbSend := time.NewTicker(time.Duration(g.GlbClientCfg.HeartBeatInterval) * time.Second)
	defer hbSend.Stop()
	hbCheck := time.NewTicker(time.Second)
	defer hbCheck.Stop()

	ctl.lastPong = time.Now()

	for {
		select {
		case <-hbSend.C:
			// send heartbeat to server
			ctl.Debug("send heartbeat to server")
			ctl.sendCh <- &msg.Ping{}
		case <-hbCheck.C:
			if time.Since(ctl.lastPong) > time.Duration(g.GlbClientCfg.HeartBeatTimeout)*time.Second {
				ctl.Warn("heartbeat timeout")
				// let reader() stop
				ctl.conn.Close()
				return
			}
		case rawMsg, ok := <-ctl.readCh:
			if !ok {
				return
			}

			switch m := rawMsg.(type) {
			case *msg.ReqWorkConn:
				go ctl.HandleReqWorkConn(m)
			case *msg.NewProxyResp:
				ctl.HandleNewProxyResp(m)
			case *msg.Pong:
				ctl.lastPong = time.Now()
				ctl.Debug("receive heartbeat from server")
			}
		}
	}
}

// If controler is notified by closedCh, reader and writer and handler will exit
func (ctl *Control) worker() {
	go ctl.msgHandler()
	go ctl.reader()
	go ctl.writer()

	select {
	case <-ctl.closedCh:
		// close related channels and wait until other goroutines done
		close(ctl.readCh)
		ctl.readerShutdown.WaitDone()
		ctl.msgHandlerShutdown.WaitDone()

		close(ctl.sendCh)
		ctl.writerShutdown.WaitDone()

		ctl.pm.Close()
		ctl.vm.Close()

		close(ctl.closedDoneCh)
		if ctl.session != nil {
			ctl.session.Close()
		}
		return
	}
}

func (ctl *Control) ReloadConf(pxyCfgs map[string]config.ProxyConf, visitorCfgs map[string]config.VisitorConf) error {
	ctl.vm.Reload(visitorCfgs)
	ctl.pm.Reload(pxyCfgs)
	return nil
}
