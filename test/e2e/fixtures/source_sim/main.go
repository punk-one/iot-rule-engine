// A finite synthetic device source used only by the multi-gateway harness.
package main

import (
	"encoding/json"
	"time"
	"sync"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

type source struct{sdk.BasePlugin;cfg struct{Gateway string `json:"gateway"`;Device string `json:"device"`;Count int `json:"count"`;IntervalMS int `json:"interval_ms"`};stop chan struct{};done chan struct{};stopOnce sync.Once}
func(s *source)Info()sdk.PluginInfo{return sdk.PluginInfo{ID:"source-sim",Name:"Synthetic device (test only)",Version:"1.0.0",Type:sdk.PluginTypeSource,Capabilities:sdk.PluginCapabilities{OutputPorts:1,ConfigSchema:`{"type":"object"}`}}}
func(s *source)Init(c sdk.PluginConfig)error{return json.Unmarshal(c.Raw,&s.cfg)}
func(s *source)Start(ctx sdk.RuntimeContext)error{
	s.stop=make(chan struct{});s.done=make(chan struct{});if s.cfg.IntervalMS<=0{s.cfg.IntervalMS=100}
	go func(){defer close(s.done);for n:=0;n<s.cfg.Count;n++{
		e:=sdk.NewEvent(map[string]interface{}{"schema_version":1,"factory_id":"test-factory","gateway_id":s.cfg.Gateway,"device_code":s.cfg.Device,"time":time.Now().UnixMilli(),"data":map[string]interface{}{"sequence":n,"value":n%100}},nil);e.Payload["event_id"]=e.ID
		for{if err:=ctx.Emitter().Publish(e);err==nil{break};select{case<-s.stop:return;case<-time.After(100*time.Millisecond):}}
		select{case<-s.stop:return;case<-time.After(time.Duration(s.cfg.IntervalMS)*time.Millisecond):}
	}}();return nil
}
func(s *source)Stop()error{s.stopOnce.Do(func(){if s.stop!=nil{close(s.stop);<-s.done}});return nil}
func main(){sdk.ServeFactory(func()sdk.Plugin{return &source{}},sdk.ServeOptions{})}
