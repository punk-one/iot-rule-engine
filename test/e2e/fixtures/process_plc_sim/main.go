// A simulated PLC command executor; never connects to a physical controller.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
	_ "github.com/mattn/go-sqlite3"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)
type plc struct{sdk.BasePlugin;cfg struct{Gateway string `json:"gateway"`;Path string `json:"path"`};db *sql.DB;ctx sdk.RuntimeContext}
func(p *plc)Info()sdk.PluginInfo{return sdk.PluginInfo{ID:"process-plc-sim",Name:"Simulated PLC command executor (test only)",Version:"1.0.0",Type:sdk.PluginTypeProcessor,Capabilities:sdk.PluginCapabilities{InputPorts:1,OutputPorts:1,ConfigSchema:`{"type":"object"}`}}}
func(p *plc)Init(c sdk.PluginConfig)error{return json.Unmarshal(c.Raw,&p.cfg)}
func(p *plc)Start(ctx sdk.RuntimeContext)error{p.ctx=ctx;db,err:=sql.Open("sqlite3",p.cfg.Path+"?_journal_mode=WAL&_busy_timeout=5000");if err!=nil{return err};p.db=db;p.db.SetMaxOpenConns(1);_,err=db.Exec(`CREATE TABLE IF NOT EXISTS commands(command_id TEXT PRIMARY KEY,device TEXT,value TEXT)`);return err}
func(p *plc)OnEvent(e sdk.Event)error{
	id,_:=e.Payload["command_id"].(string);gateway,_:=e.Payload["gateway_id"].(string);device,_:=e.Payload["device_code"].(string);expires,_:=e.Payload["expires_at"].(float64)
	status:="completed"
	if id==""||device==""||gateway!=p.cfg.Gateway||int64(expires)<=time.Now().UnixMilli(){status="rejected"}else{
		value,_:=json.Marshal(e.Payload["data"]);r,err:=p.db.Exec(`INSERT OR IGNORE INTO commands VALUES(?,?,?)`,id,device,string(value));if err!=nil{return err};n,_:=r.RowsAffected();if n==0{status="duplicate"}
	}
	out:=sdk.NewEvent(map[string]interface{}{"event_id":fmt.Sprintf("result-%s-%s",id,status),"command_id":id,"gateway_id":p.cfg.Gateway,"device_code":device,"status":status,"time":time.Now().UnixMilli()},nil)
	return p.ctx.Emitter().Publish(out)
}
func(p *plc)Stop()error{if p.db!=nil{return p.db.Close()};return nil}
func main(){sdk.ServeFactory(func()sdk.Plugin{return &plc{}},sdk.ServeOptions{})}
