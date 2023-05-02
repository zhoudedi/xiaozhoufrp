// Copyright 2016 fatedier, fatedier@gmail.com
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

package config

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/fatedier/frp/models/consts"
	"github.com/fatedier/frp/models/msg"
	"github.com/fatedier/frp/utils/util"

	ini "github.com/vaughan0/go-ini"
)

var (
	proxyConfTypeMap map[string]reflect.Type
)

func init() {
	proxyConfTypeMap = make(map[string]reflect.Type)
	proxyConfTypeMap[consts.TcpProxy] = reflect.TypeOf(TcpProxyConf{})
	proxyConfTypeMap[consts.UdpProxy] = reflect.TypeOf(UdpProxyConf{})
	proxyConfTypeMap[consts.HttpProxy] = reflect.TypeOf(HttpProxyConf{})
	proxyConfTypeMap[consts.HttpsProxy] = reflect.TypeOf(HttpsProxyConf{})
	proxyConfTypeMap[consts.StcpProxy] = reflect.TypeOf(StcpProxyConf{})
	proxyConfTypeMap[consts.XtcpProxy] = reflect.TypeOf(XtcpProxyConf{})
}

// NewConfByType creates a empty ProxyConf object by proxyType.
// If proxyType isn't exist, return nil.
func NewConfByType(proxyType string) ProxyConf {
	v, ok := proxyConfTypeMap[proxyType]
	if !ok {
		return nil
	}
	cfg := reflect.New(v).Interface().(ProxyConf)
	return cfg
}

type ProxyConf interface {
	GetBaseInfo() *BaseProxyConf
	UnmarshalFromMsg(pMsg *msg.NewProxy)
	UnmarshalFromIni(prefix string, name string, conf ini.Section) error
	MarshalToMsg(pMsg *msg.NewProxy)
	CheckForCli() error
	CheckForSvr() error
	Compare(conf ProxyConf) bool
}

func NewProxyConfFromMsg(pMsg *msg.NewProxy) (cfg ProxyConf, err error) {
	if pMsg.ProxyType == "" {
		pMsg.ProxyType = consts.TcpProxy
	}

	cfg = NewConfByType(pMsg.ProxyType)
	if cfg == nil {
		err = fmt.Errorf("proxy [%s] type [%s] error", pMsg.ProxyName, pMsg.ProxyType)
		return
	}
	cfg.UnmarshalFromMsg(pMsg)
	err = cfg.CheckForSvr()
	return
}

func NewProxyConfFromIni(prefix string, name string, section ini.Section) (cfg ProxyConf, err error) {
	proxyType := section["type"]
	if proxyType == "" {
		proxyType = consts.TcpProxy
		section["type"] = consts.TcpProxy
	}
	cfg = NewConfByType(proxyType)
	if cfg == nil {
		err = fmt.Errorf("proxy [%s] type [%s] error", name, proxyType)
		return
	}
	if err = cfg.UnmarshalFromIni(prefix, name, section); err != nil {
		return
	}
	if err = cfg.CheckForCli(); err != nil {
		return
	}
	return
}

// BaseProxy info
type BaseProxyConf struct {
	ProxyName string `json:"proxy_name"`
	ProxyType string `json:"proxy_type"`

	UseEncryption  bool   `json:"use_encryption"`
	UseCompression bool   `json:"use_compression"`
	Group          string `json:"group"`
	GroupKey       string `json:"group_key"`

	// only used for client
	ProxyProtocolVersion string `json:"proxy_protocol_version"`
	LocalSvrConf
	HealthCheckConf
}

func (cfg *BaseProxyConf) GetBaseInfo() *BaseProxyConf {
	return cfg
}

func (cfg *BaseProxyConf) compare(cmp *BaseProxyConf) bool {
	if cfg.ProxyName != cmp.ProxyName ||
		cfg.ProxyType != cmp.ProxyType ||
		cfg.UseEncryption != cmp.UseEncryption ||
		cfg.UseCompression != cmp.UseCompression ||
		cfg.Group != cmp.Group ||
		cfg.GroupKey != cmp.GroupKey ||
		cfg.ProxyProtocolVersion != cmp.ProxyProtocolVersion {
		return false
	}
	if !cfg.LocalSvrConf.compare(&cmp.LocalSvrConf) {
		return false
	}
	if !cfg.HealthCheckConf.compare(&cmp.HealthCheckConf) {
		return false
	}
	return true
}

func (cfg *BaseProxyConf) UnmarshalFromMsg(pMsg *msg.NewProxy) {
	cfg.ProxyName = pMsg.ProxyName
	cfg.ProxyType = pMsg.ProxyType
	cfg.UseEncryption = pMsg.UseEncryption
	cfg.UseCompression = pMsg.UseCompression
	cfg.Group = pMsg.Group
	cfg.GroupKey = pMsg.GroupKey
}

func (cfg *BaseProxyConf) UnmarshalFromIni(prefix string, name string, section ini.Section) error {
	var (
		tmpStr string
		ok     bool
	)
	cfg.ProxyName = prefix + name
	cfg.ProxyType = section["type"]

	tmpStr, ok = section["use_encryption"]
	if ok && tmpStr == "true" {
		cfg.UseEncryption = true
	}

	tmpStr, ok = section["use_compression"]
	if ok && tmpStr == "true" {
		cfg.UseCompression = true
	}

	cfg.Group = section["group"]
	cfg.GroupKey = section["group_key"]
	cfg.ProxyProtocolVersion = section["proxy_protocol_version"]

	if err := cfg.LocalSvrConf.UnmarshalFromIni(prefix, name, section); err != nil {
		return err
	}

	if err := cfg.HealthCheckConf.UnmarshalFromIni(prefix, name, section); err != nil {
		return err
	}

	if cfg.HealthCheckType == "tcp" && cfg.Plugin == "" {
		cfg.HealthCheckAddr = cfg.LocalIp + fmt.Sprintf(":%d", cfg.LocalPort)
	}
	if cfg.HealthCheckType == "http" && cfg.Plugin == "" && cfg.HealthCheckUrl != "" {
		s := fmt.Sprintf("http://%s:%d", cfg.LocalIp, cfg.LocalPort)
		if !strings.HasPrefix(cfg.HealthCheckUrl, "/") {
			s += "/"
		}
		cfg.HealthCheckUrl = s + cfg.HealthCheckUrl
	}
	return nil
}

func (cfg *BaseProxyConf) MarshalToMsg(pMsg *msg.NewProxy) {
	pMsg.ProxyName = cfg.ProxyName
	pMsg.ProxyType = cfg.ProxyType
	pMsg.UseEncryption = cfg.UseEncryption
	pMsg.UseCompression = cfg.UseCompression
	pMsg.Group = cfg.Group
	pMsg.GroupKey = cfg.GroupKey
}

func (cfg *BaseProxyConf) checkForCli() (err error) {
	if cfg.ProxyProtocolVersion != "" {
		if cfg.ProxyProtocolVersion != "v1" && cfg.ProxyProtocolVersion != "v2" {
			return fmt.Errorf("no support proxy protocol version: %s", cfg.ProxyProtocolVersion)
		}
	}

	if err = cfg.LocalSvrConf.checkForCli(); err != nil {
		return
	}
	if err = cfg.HealthCheckConf.checkForCli(); err != nil {
		return
	}
	return nil
}

// Bind info
type BindInfoConf struct {
	RemotePort int `json:"remote_port"`
}

func (cfg *BindInfoConf) compare(cmp *BindInfoConf) bool {
	if cfg.RemotePort != cmp.RemotePort {
		return false
	}
	return true
}

func (cfg *BindInfoConf) UnmarshalFromMsg(pMsg *msg.NewProxy) {
	cfg.RemotePort = pMsg.RemotePort
}

func (cfg *BindInfoConf) UnmarshalFromIni(prefix string, name string, section ini.Section) (err error) {
	var (
		tmpStr string
		ok     bool
		v      int64
	)
	if tmpStr, ok = section["remote_port"]; ok {
		if v, err = strconv.ParseInt(tmpStr, 10, 64); err != nil {
			return fmt.Errorf("Parse conf error: proxy [%s] remote_port error", name)
		} else {
			cfg.RemotePort = int(v)
		}
	} else {
		return fmt.Errorf("Parse conf error: proxy [%s] remote_port not found", name)
	}
	return nil
}

func (cfg *BindInfoConf) MarshalToMsg(pMsg *msg.NewProxy) {
	pMsg.RemotePort = cfg.RemotePort
}

// Domain info
type DomainConf struct {
	CustomDomains []string `json:"custom_domains"`
	SubDomain     string   `json:"sub_domain"`
}

func (cfg *DomainConf) compare(cmp *DomainConf) bool {
	if strings.Join(cfg.CustomDomains, " ") != strings.Join(cmp.CustomDomains, " ") ||
		cfg.SubDomain != cmp.SubDomain {
		return false
	}
	return true
}

func (cfg *DomainConf) UnmarshalFromMsg(pMsg *msg.NewProxy) {
	cfg.CustomDomains = pMsg.CustomDomains
	cfg.SubDomain = pMsg.SubDomain
}

func (cfg *DomainConf) UnmarshalFromIni(prefix string, name string, section ini.Section) (err error) {
	var (
		tmpStr string
		ok     bool
	)
	if tmpStr, ok = section["custom_domains"]; ok {
		cfg.CustomDomains = strings.Split(tmpStr, ",")
		for i, domain := range cfg.CustomDomains {
			cfg.CustomDomains[i] = strings.ToLower(strings.TrimSpace(domain))
		}
	}

	if tmpStr, ok = section["subdomain"]; ok {
		cfg.SubDomain = tmpStr
	}
	return
}

func (cfg *DomainConf) MarshalToMsg(pMsg *msg.NewProxy) {
	pMsg.CustomDomains = cfg.CustomDomains
	pMsg.SubDomain = cfg.SubDomain
}

func (cfg *DomainConf) check() (err error) {
	if len(cfg.CustomDomains) == 0 && cfg.SubDomain == "" {
		err = fmt.Errorf("custom_domains and subdomain should set at least one of them")
		return
	}
	return
}

func (cfg *DomainConf) checkForCli() (err error) {
	if err = cfg.check(); err != nil {
		return
	}
	return
}

func (cfg *DomainConf) checkForSvr() (err error) {
	if err = cfg.check(); err != nil {
		return
	}

	for _, domain := range cfg.CustomDomains {
		if subDomainHost != "" && len(strings.Split(subDomainHost, ".")) < len(strings.Split(domain, ".")) {
			if strings.Contains(domain, subDomainHost) {
				return fmt.Errorf("custom domain [%s] should not belong to subdomain_host [%s]", domain, subDomainHost)
			}
		}
	}

	if cfg.SubDomain != "" {
		if subDomainHost == "" {
			return fmt.Errorf("subdomain is not supported because this feature is not enabled in remote frps")
		}
		if strings.Contains(cfg.SubDomain, ".") || strings.Contains(cfg.SubDomain, "*") {
			return fmt.Errorf("'.' and '*' is not supported in subdomain")
		}
	}
	return
}

// Local service info
type LocalSvrConf struct {
	LocalIp   string `json:"local_ip"`
	LocalPort int    `json:"local_port"`

	Plugin       string            `json:"plugin"`
	PluginParams map[string]string `json:"plugin_params"`
}

func (cfg *LocalSvrConf) compare(cmp *LocalSvrConf) bool {
	if cfg.LocalIp != cmp.LocalIp ||
		cfg.LocalPort != cmp.LocalPort {
		return false
	}
	if cfg.Plugin != cmp.Plugin ||
		len(cfg.PluginParams) != len(cmp.PluginParams) {
		return false
	}
	for k, v := range cfg.PluginParams {
		value, ok := cmp.PluginParams[k]
		if !ok || v != value {
			return false
		}
	}
	return true
}

func (cfg *LocalSvrConf) UnmarshalFromIni(prefix string, name string, section ini.Section) (err error) {
	cfg.Plugin = section["plugin"]
	cfg.PluginParams = make(map[string]string)
	if cfg.Plugin != "" {
		// get params begin with "plugin_"
		for k, v := range section {
			if strings.HasPrefix(k, "plugin_") {
				cfg.PluginParams[k] = v
			}
		}
	} else {
		if cfg.LocalIp = section["local_ip"]; cfg.LocalIp == "" {
			cfg.LocalIp = "127.0.0.1"
		}

		if tmpStr, ok := section["local_port"]; ok {
			if cfg.LocalPort, err = strconv.Atoi(tmpStr); err != nil {
				return fmt.Errorf("Parse conf error: proxy [%s] local_port error", name)
			}
		} else {
			return fmt.Errorf("Parse conf error: proxy [%s] local_port not found", name)
		}
	}
	return
}

func (cfg *LocalSvrConf) checkForCli() (err error) {
	if cfg.Plugin == "" {
		if cfg.LocalIp == "" {
			err = fmt.Errorf("local ip or plugin is required")
			return
		}
		if cfg.LocalPort <= 0 {
			err = fmt.Errorf("error local_port")
			return
		}
	}
	return
}

// Health check info
type HealthCheckConf struct {
	HealthCheckType      string `json:"health_check_type"` // tcp | http
	HealthCheckTimeoutS  int    `json:"health_check_timeout_s"`
	HealthCheckMaxFailed int    `json:"health_check_max_failed"`
	HealthCheckIntervalS int    `json:"health_check_interval_s"`
	HealthCheckUrl       string `json:"health_check_url"`

	// local_ip + local_port
	HealthCheckAddr string `json:"-"`
}

func (cfg *HealthCheckConf) compare(cmp *HealthCheckConf) bool {
	if cfg.HealthCheckType != cmp.HealthCheckType ||
		cfg.HealthCheckTimeoutS != cmp.HealthCheckTimeoutS ||
		cfg.HealthCheckMaxFailed != cmp.HealthCheckMaxFailed ||
		cfg.HealthCheckIntervalS != cmp.HealthCheckIntervalS ||
		cfg.HealthCheckUrl != cmp.HealthCheckUrl {
		return false
	}
	return true
}

func (cfg *HealthCheckConf) UnmarshalFromIni(prefix string, name string, section ini.Section) (err error) {
	cfg.HealthCheckType = section["health_check_type"]
	cfg.HealthCheckUrl = section["health_check_url"]

	if tmpStr, ok := section["health_check_timeout_s"]; ok {
		if cfg.HealthCheckTimeoutS, err = strconv.Atoi(tmpStr); err != nil {
			return fmt.Errorf("Parse conf error: proxy [%s] health_check_timeout_s error", name)
		}
	}

	if tmpStr, ok := section["health_check_max_failed"]; ok {
		if cfg.HealthCheckMaxFailed, err = strconv.Atoi(tmpStr); err != nil {
			return fmt.Errorf("Parse conf error: proxy [%s] health_check_max_failed error", name)
		}
	}

	if tmpStr, ok := section["health_check_interval_s"]; ok {
		if cfg.HealthCheckIntervalS, err = strconv.Atoi(tmpStr); err != nil {
			return fmt.Errorf("Parse conf error: proxy [%s] health_check_interval_s error", name)
		}
	}
	return
}

func (cfg *HealthCheckConf) checkForCli() error {
	if cfg.HealthCheckType != "" && cfg.HealthCheckType != "tcp" && cfg.HealthCheckType != "http" {
		return fmt.Errorf("unsupport health check type")
	}
	if cfg.HealthCheckType != "" {
		if cfg.HealthCheckType == "http" && cfg.HealthCheckUrl == "" {
			return fmt.Errorf("health_check_url is required for health check type 'http'")
		}
	}
	return nil
}

// TCP
type TcpProxyConf struct {
	BaseProxyConf
	BindInfoConf
}

func (cfg *TcpProxyConf) Compare(cmp ProxyConf) bool {
	cmpConf, ok := cmp.(*TcpProxyConf)
	if !ok {
		return false
	}

	if !cfg.BaseProxyConf.compare(&cmpConf.BaseProxyConf) ||
		!cfg.BindInfoConf.compare(&cmpConf.BindInfoConf) {
		return false
	}
	return true
}

func (cfg *TcpProxyConf) UnmarshalFromMsg(pMsg *msg.NewProxy) {
	cfg.BaseProxyConf.UnmarshalFromMsg(pMsg)
	cfg.BindInfoConf.UnmarshalFromMsg(pMsg)
}

func (cfg *TcpProxyConf) UnmarshalFromIni(prefix string, name string, section ini.Section) (err error) {
	if err = cfg.BaseProxyConf.UnmarshalFromIni(prefix, name, section); err != nil {
		return
	}
	if err = cfg.BindInfoConf.UnmarshalFromIni(prefix, name, section); err != nil {
		return
	}
	return
}

func (cfg *TcpProxyConf) MarshalToMsg(pMsg *msg.NewProxy) {
	cfg.BaseProxyConf.MarshalToMsg(pMsg)
	cfg.BindInfoConf.MarshalToMsg(pMsg)
}

func (cfg *TcpProxyConf) CheckForCli() (err error) {
	if err = cfg.BaseProxyConf.checkForCli(); err != nil {
		return err
	}
	return
}

func (cfg *TcpProxyConf) CheckForSvr() error { return nil }

// UDP
type UdpProxyConf struct {
	BaseProxyConf
	BindInfoConf
}

func (cfg *UdpProxyConf) Compare(cmp ProxyConf) bool {
	cmpConf, ok := cmp.(*UdpProxyConf)
	if !ok {
		return false
	}

	if !cfg.BaseProxyConf.compare(&cmpConf.BaseProxyConf) ||
		!cfg.BindInfoConf.compare(&cmpConf.BindInfoConf) {
		return false
	}
	return true
}

func (cfg *UdpProxyConf) UnmarshalFromMsg(pMsg *msg.NewProxy) {
	cfg.BaseProxyConf.UnmarshalFromMsg(pMsg)
	cfg.BindInfoConf.UnmarshalFromMsg(pMsg)
}

func (cfg *UdpProxyConf) UnmarshalFromIni(prefix string, name string, section ini.Section) (err error) {
	if err = cfg.BaseProxyConf.UnmarshalFromIni(prefix, name, section); err != nil {
		return
	}
	if err = cfg.BindInfoConf.UnmarshalFromIni(prefix, name, section); err != nil {
		return
	}
	return
}

func (cfg *UdpProxyConf) MarshalToMsg(pMsg *msg.NewProxy) {
	cfg.BaseProxyConf.MarshalToMsg(pMsg)
	cfg.BindInfoConf.MarshalToMsg(pMsg)
}

func (cfg *UdpProxyConf) CheckForCli() (err error) {
	if err = cfg.BaseProxyConf.checkForCli(); err != nil {
		return
	}
	return
}

func (cfg *UdpProxyConf) CheckForSvr() error { return nil }

// HTTP
type HttpProxyConf struct {
	BaseProxyConf
	DomainConf

	Locations         []string          `json:"locations"`
	HttpUser          string            `json:"http_user"`
	HttpPwd           string            `json:"http_pwd"`
	HostHeaderRewrite string            `json:"host_header_rewrite"`
	Headers           map[string]string `json:"headers"`
}

func (cfg *HttpProxyConf) Compare(cmp ProxyConf) bool {
	cmpConf, ok := cmp.(*HttpProxyConf)
	if !ok {
		return false
	}

	if !cfg.BaseProxyConf.compare(&cmpConf.BaseProxyConf) ||
		!cfg.DomainConf.compare(&cmpConf.DomainConf) ||
		strings.Join(cfg.Locations, " ") != strings.Join(cmpConf.Locations, " ") ||
		cfg.HostHeaderRewrite != cmpConf.HostHeaderRewrite ||
		cfg.HttpUser != cmpConf.HttpUser ||
		cfg.HttpPwd != cmpConf.HttpPwd ||
		len(cfg.Headers) != len(cmpConf.Headers) {
		return false
	}

	for k, v := range cfg.Headers {
		if v2, ok := cmpConf.Headers[k]; !ok {
			return false
		} else {
			if v != v2 {
				return false
			}
		}
	}
	return true
}

func (cfg *HttpProxyConf) UnmarshalFromMsg(pMsg *msg.NewProxy) {
	cfg.BaseProxyConf.UnmarshalFromMsg(pMsg)
	cfg.DomainConf.UnmarshalFromMsg(pMsg)

	cfg.Locations = pMsg.Locations
	cfg.HostHeaderRewrite = pMsg.HostHeaderRewrite
	cfg.HttpUser = pMsg.HttpUser
	cfg.HttpPwd = pMsg.HttpPwd
	cfg.Headers = pMsg.Headers
}

func (cfg *HttpProxyConf) UnmarshalFromIni(prefix string, name string, section ini.Section) (err error) {
	if err = cfg.BaseProxyConf.UnmarshalFromIni(prefix, name, section); err != nil {
		return
	}
	if err = cfg.DomainConf.UnmarshalFromIni(prefix, name, section); err != nil {
		return
	}

	var (
		tmpStr string
		ok     bool
	)
	if tmpStr, ok = section["locations"]; ok {
		cfg.Locations = strings.Split(tmpStr, ",")
	} else {
		cfg.Locations = []string{""}
	}

	cfg.HostHeaderRewrite = section["host_header_rewrite"]
	cfg.HttpUser = section["http_user"]
	cfg.HttpPwd = section["http_pwd"]
	cfg.Headers = make(map[string]string)

	for k, v := range section {
		if strings.HasPrefix(k, "header_") {
			cfg.Headers[strings.TrimPrefix(k, "header_")] = v
		}
	}
	return
}

func (cfg *HttpProxyConf) MarshalToMsg(pMsg *msg.NewProxy) {
	cfg.BaseProxyConf.MarshalToMsg(pMsg)
	cfg.DomainConf.MarshalToMsg(pMsg)

	pMsg.Locations = cfg.Locations
	pMsg.HostHeaderRewrite = cfg.HostHeaderRewrite
	pMsg.HttpUser = cfg.HttpUser
	pMsg.HttpPwd = cfg.HttpPwd
	pMsg.Headers = cfg.Headers
}

func (cfg *HttpProxyConf) CheckForCli() (err error) {
	if err = cfg.BaseProxyConf.checkForCli(); err != nil {
		return
	}
	if err = cfg.DomainConf.checkForCli(); err != nil {
		return
	}
	return
}

func (cfg *HttpProxyConf) CheckForSvr() (err error) {
	if vhostHttpPort == 0 {
		return fmt.Errorf("type [http] not support when vhost_http_port is not set")
	}
	if err = cfg.DomainConf.checkForSvr(); err != nil {
		err = fmt.Errorf("proxy [%s] domain conf check error: %v", cfg.ProxyName, err)
		return
	}
	return
}

// HTTPS
type HttpsProxyConf struct {
	BaseProxyConf
	DomainConf
}

func (cfg *HttpsProxyConf) Compare(cmp ProxyConf) bool {
	cmpConf, ok := cmp.(*HttpsProxyConf)
	if !ok {
		return false
	}

	if !cfg.BaseProxyConf.compare(&cmpConf.BaseProxyConf) ||
		!cfg.DomainConf.compare(&cmpConf.DomainConf) {
		return false
	}
	return true
}

func (cfg *HttpsProxyConf) UnmarshalFromMsg(pMsg *msg.NewProxy) {
	cfg.BaseProxyConf.UnmarshalFromMsg(pMsg)
	cfg.DomainConf.UnmarshalFromMsg(pMsg)
}

func (cfg *HttpsProxyConf) UnmarshalFromIni(prefix string, name string, section ini.Section) (err error) {
	if err = cfg.BaseProxyConf.UnmarshalFromIni(prefix, name, section); err != nil {
		return
	}
	if err = cfg.DomainConf.UnmarshalFromIni(prefix, name, section); err != nil {
		return
	}
	return
}

func (cfg *HttpsProxyConf) MarshalToMsg(pMsg *msg.NewProxy) {
	cfg.BaseProxyConf.MarshalToMsg(pMsg)
	cfg.DomainConf.MarshalToMsg(pMsg)
}

func (cfg *HttpsProxyConf) CheckForCli() (err error) {
	if err = cfg.BaseProxyConf.checkForCli(); err != nil {
		return
	}
	if err = cfg.DomainConf.checkForCli(); err != nil {
		return
	}
	return
}

func (cfg *HttpsProxyConf) CheckForSvr() (err error) {
	if vhostHttpsPort == 0 {
		return fmt.Errorf("type [https] not support when vhost_https_port is not set")
	}
	if err = cfg.DomainConf.checkForSvr(); err != nil {
		err = fmt.Errorf("proxy [%s] domain conf check error: %v", cfg.ProxyName, err)
		return
	}
	return
}

// STCP
type StcpProxyConf struct {
	BaseProxyConf

	Role string `json:"role"`
	Sk   string `json:"sk"`
}

func (cfg *StcpProxyConf) Compare(cmp ProxyConf) bool {
	cmpConf, ok := cmp.(*StcpProxyConf)
	if !ok {
		return false
	}

	if !cfg.BaseProxyConf.compare(&cmpConf.BaseProxyConf) ||
		cfg.Role != cmpConf.Role ||
		cfg.Sk != cmpConf.Sk {
		return false
	}
	return true
}

// Only for role server.
func (cfg *StcpProxyConf) UnmarshalFromMsg(pMsg *msg.NewProxy) {
	cfg.BaseProxyConf.UnmarshalFromMsg(pMsg)
	cfg.Sk = pMsg.Sk
}

func (cfg *StcpProxyConf) UnmarshalFromIni(prefix string, name string, section ini.Section) (err error) {
	if err = cfg.BaseProxyConf.UnmarshalFromIni(prefix, name, section); err != nil {
		return
	}

	cfg.Role = section["role"]
	if cfg.Role != "server" {
		return fmt.Errorf("Parse conf error: proxy [%s] incorrect role [%s]", name, cfg.Role)
	}

	cfg.Sk = section["sk"]

	if err = cfg.LocalSvrConf.UnmarshalFromIni(prefix, name, section); err != nil {
		return
	}
	return
}

func (cfg *StcpProxyConf) MarshalToMsg(pMsg *msg.NewProxy) {
	cfg.BaseProxyConf.MarshalToMsg(pMsg)
	pMsg.Sk = cfg.Sk
}

func (cfg *StcpProxyConf) CheckForCli() (err error) {
	if err = cfg.BaseProxyConf.checkForCli(); err != nil {
		return
	}
	if cfg.Role != "server" {
		err = fmt.Errorf("role should be 'server'")
		return
	}
	return
}

func (cfg *StcpProxyConf) CheckForSvr() (err error) {
	return
}

// XTCP
type XtcpProxyConf struct {
	BaseProxyConf

	Role string `json:"role"`
	Sk   string `json:"sk"`
}

func (cfg *XtcpProxyConf) Compare(cmp ProxyConf) bool {
	cmpConf, ok := cmp.(*XtcpProxyConf)
	if !ok {
		return false
	}

	if !cfg.BaseProxyConf.compare(&cmpConf.BaseProxyConf) ||
		!cfg.LocalSvrConf.compare(&cmpConf.LocalSvrConf) ||
		cfg.Role != cmpConf.Role ||
		cfg.Sk != cmpConf.Sk {
		return false
	}
	return true
}

// Only for role server.
func (cfg *XtcpProxyConf) UnmarshalFromMsg(pMsg *msg.NewProxy) {
	cfg.BaseProxyConf.UnmarshalFromMsg(pMsg)
	cfg.Sk = pMsg.Sk
}

func (cfg *XtcpProxyConf) UnmarshalFromIni(prefix string, name string, section ini.Section) (err error) {
	if err = cfg.BaseProxyConf.UnmarshalFromIni(prefix, name, section); err != nil {
		return
	}

	cfg.Role = section["role"]
	if cfg.Role != "server" {
		return fmt.Errorf("Parse conf error: proxy [%s] incorrect role [%s]", name, cfg.Role)
	}

	cfg.Sk = section["sk"]

	if err = cfg.LocalSvrConf.UnmarshalFromIni(prefix, name, section); err != nil {
		return
	}
	return
}

func (cfg *XtcpProxyConf) MarshalToMsg(pMsg *msg.NewProxy) {
	cfg.BaseProxyConf.MarshalToMsg(pMsg)
	pMsg.Sk = cfg.Sk
}

func (cfg *XtcpProxyConf) CheckForCli() (err error) {
	if err = cfg.BaseProxyConf.checkForCli(); err != nil {
		return
	}
	if cfg.Role != "server" {
		err = fmt.Errorf("role should be 'server'")
		return
	}
	return
}

func (cfg *XtcpProxyConf) CheckForSvr() (err error) {
	return
}

func ParseRangeSection(name string, section ini.Section) (sections map[string]ini.Section, err error) {
	localPorts, errRet := util.ParseRangeNumbers(section["local_port"])
	if errRet != nil {
		err = fmt.Errorf("Parse conf error: range section [%s] local_port invalid, %v", name, errRet)
		return
	}

	remotePorts, errRet := util.ParseRangeNumbers(section["remote_port"])
	if errRet != nil {
		err = fmt.Errorf("Parse conf error: range section [%s] remote_port invalid, %v", name, errRet)
		return
	}
	if len(localPorts) != len(remotePorts) {
		err = fmt.Errorf("Parse conf error: range section [%s] local ports number should be same with remote ports number", name)
		return
	}
	if len(localPorts) == 0 {
		err = fmt.Errorf("Parse conf error: range section [%s] local_port and remote_port is necessary", name)
		return
	}

	sections = make(map[string]ini.Section)
	for i, port := range localPorts {
		subName := fmt.Sprintf("%s_%d", name, i)
		subSection := copySection(section)
		subSection["local_port"] = fmt.Sprintf("%d", port)
		subSection["remote_port"] = fmt.Sprintf("%d", remotePorts[i])
		sections[subName] = subSection
	}
	return
}

// if len(startProxy) is 0, start all
// otherwise just start proxies in startProxy map
func LoadAllConfFromIni(prefix string, content string, startProxy map[string]struct{}) (
	proxyConfs map[string]ProxyConf, visitorConfs map[string]VisitorConf, err error) {

	conf, errRet := ini.Load(strings.NewReader(content))
	if errRet != nil {
		err = errRet
		return
	}

	if prefix != "" {
		prefix += "."
	}

	startAll := true
	if len(startProxy) > 0 {
		startAll = false
	}
	proxyConfs = make(map[string]ProxyConf)
	visitorConfs = make(map[string]VisitorConf)
	for name, section := range conf {
		if name == "common" {
			continue
		}

		_, shouldStart := startProxy[name]
		if !startAll && !shouldStart {
			continue
		}

		subSections := make(map[string]ini.Section)

		if strings.HasPrefix(name, "range:") {
			// range section
			rangePrefix := strings.TrimSpace(strings.TrimPrefix(name, "range:"))
			subSections, err = ParseRangeSection(rangePrefix, section)
			if err != nil {
				return
			}
		} else {
			subSections[name] = section
		}

		for subName, subSection := range subSections {
			if subSection["role"] == "" {
				subSection["role"] = "server"
			}
			role := subSection["role"]
			if role == "server" {
				cfg, errRet := NewProxyConfFromIni(prefix, subName, subSection)
				if errRet != nil {
					err = errRet
					return
				}
				proxyConfs[prefix+subName] = cfg
			} else if role == "visitor" {
				cfg, errRet := NewVisitorConfFromIni(prefix, subName, subSection)
				if errRet != nil {
					err = errRet
					return
				}
				visitorConfs[prefix+subName] = cfg
			} else {
				err = fmt.Errorf("role should be 'server' or 'visitor'")
				return
			}
		}
	}
	return
}

func copySection(section ini.Section) (out ini.Section) {
	out = make(ini.Section)
	for k, v := range section {
		out[k] = v
	}
	return
}
