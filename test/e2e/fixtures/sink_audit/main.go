// A local SQLite business sink used only to assert the real engine/broker path.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	_ "github.com/mattn/go-sqlite3"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

type sink struct{sdk.BasePlugin;path string;db *sql.DB;ctx sdk.RuntimeContext}
func(s *sink)Info()sdk.PluginInfo{return sdk.PluginInfo{ID:"sink-audit",Name:"SQLite assertion sink (test only)",Version:"1.0.0",Type:sdk.PluginTypeSink,Capabilities:sdk.PluginCapabilities{InputPorts:1,ConfigSchema:`{"type":"object","required":["path"],"properties":{"path":{"type":"string"}}}`}}}
func(s *sink)Init(c sdk.PluginConfig)error{var cfg struct{Path string `json:"path"`};if err:=json.Unmarshal(c.Raw,&cfg);err!=nil{return err};s.path=cfg.Path;return nil}
func(s *sink)Start(ctx sdk.RuntimeContext)error{
	s.ctx=ctx;if err:=os.MkdirAll(filepath.Dir(s.path),0700);err!=nil{return err};db,err:=sql.Open("sqlite3",s.path+"?_journal_mode=WAL&_busy_timeout=5000");if err!=nil{return err};db.SetMaxOpenConns(1);s.db=db
	_,err=db.Exec(`CREATE TABLE IF NOT EXISTS events(event_id TEXT PRIMARY KEY,gateway TEXT NOT NULL,device TEXT NOT NULL,payload TEXT NOT NULL)`);return err
}
func(s *sink)OnEvent(e sdk.Event)error{
	id,_:=e.Payload["event_id"].(string);if id==""{return fmt.Errorf("event_id missing")};gateway,_:=e.Payload["gateway_id"].(string);device,_:=e.Payload["device_code"].(string);raw,err:=json.Marshal(e.Payload);if err!=nil{return err}
	if _,err=s.db.Exec(`INSERT OR IGNORE INTO events VALUES(?,?,?,?)`,id,gateway,device,string(raw));err!=nil{return err}
	if record:=e.Metadata["buffer_record_id"];record!=""{return s.ctx.Emitter().PublishAck(sdk.AckMessage{RecordID:record,BufferNodeID:e.Metadata["buffer_node_id"],EventID:e.ID,Status:sdk.AckStatusSuccess})};return nil
}
func(s *sink)Stop()error{if s.db!=nil{return s.db.Close()};return nil}
func main(){sdk.ServeFactory(func()sdk.Plugin{return &sink{}},sdk.ServeOptions{})}
