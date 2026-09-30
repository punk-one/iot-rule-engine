# 2026-09-18 审计隔离复现

关联报告：[前后端、代码与文档复审](reaudit-2026-09-18.md)。

## 安全边界与执行方式

以下 Go 程序仅使用自行创建的临时 SQLite 和空插件目录，通过内存 HTTP handler 调用验证问题，不启动监听端口、不读现有 data/rules.db、不运行真实插件或访问外部设备。结束时只清理自身创建的临时目录。SDK 解析沿用项目已有 Go workspace。

本轮执行的本地副本是 `docs/reaudit-2026-09-18-repro.go`，带 ignore 构建约束，不进入普通构建。当前仓库忽略 docs 下非 Markdown 文件，因此**以本 Markdown 内的完整源码为版本化证据**；从干净检出复现时，将下方 Go 代码保存到该位置：

```bash
go run docs/reaudit-2026-09-18-repro.go
```

本轮关键输出：

```text
execute_dry_false_returned_nil=true node_calls=0
execute_dry_true_returned_nil=true node_calls=0
connector_list_status=200 contains_synthetic_secret=true
empty_enabled_rule_create_status=201 reported_deployed_without_runtime=true
stale_write_version_before=4 after=3 stale_content_won=true
bootstrap_edit_status=200 manager_port=12012 db_path_changed=true
failed_watch_callbacks_after_more_than_3_polls=1
```

这些输出是整改前观察值，不是期望通过条件。修复后应将对应行为反转为拒绝泄露/冲突、真实执行、运行态正确、配置字段保护及自动重试。配置复现只检查 API 接受和持久化，不会真正迁移数据库或重启服务；连接秘密复现直接调用 handler，权限等级另由路由审查确认。

## Go 源码

```go
//go:build ignore

package main

import (
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "sync/atomic"
 "time"

 "github.com/gin-gonic/gin"
 "github.com/goccy/go-yaml"
 "github.com/punk-one/iot-rule-engine/internal/core/engine"
 "github.com/punk-one/iot-rule-engine/internal/core/runtime"
 "github.com/punk-one/iot-rule-engine/internal/infra/config"
 "github.com/punk-one/iot-rule-engine/internal/infra/storage/rulesdb"
 "github.com/punk-one/iot-rule-engine/internal/server/manager/handlers"
 sdk "github.com/punk-one/iot-rule-plugin-sdk"
 "github.com/punk-one/iot-rule-plugin-sdk/schema"
)

type probe struct { sdk.BasePlugin; calls int }
func (p *probe) Info() sdk.PluginInfo { return sdk.PluginInfo{ID:"audit-probe", Type:sdk.PluginTypeSink} }
func (p *probe) OnEvent(sdk.Event) error { p.calls++; return nil }
func (p *probe) OnEvents([]sdk.Event) error { p.calls++; return nil }
func must(err error) { if err != nil { panic(err) } }
func request(r http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
 w := httptest.NewRecorder()
 req := httptest.NewRequest(method, path, bytes.NewReader(body))
 req.Header.Set("Content-Type","application/json")
 r.ServeHTTP(w,req)
 return w
}
func main() {
 gin.SetMode(gin.TestMode)
 dir, err := os.MkdirTemp("", "iot-engine-audit-20260918-"); must(err)
 defer os.RemoveAll(dir) // Only this program's newly created temporary directory.
 db, err := rulesdb.OpenSQLite(rulesdb.Config{DataDir:dir}); must(err)
 defer db.Close()
 rt := runtime.NewManager()
 m := handlers.NewManager(db, dir, "", rt)
 r := gin.New()
 r.GET("/connectors",m.ListConnectorResourcesHandler)
 r.GET("/rules",m.ListRulesHandler)
 r.POST("/rules",m.CreateRuleHandler)
 r.PUT("/config",m.UpdateConfigHandler)

 // No network, no loaded binary: prove the current Execute implementation does not invoke nodes.
 ex, err := engine.NewExecutor(); must(err)
 defer ex.Close()
 p := &probe{}
 must(ex.RegisterDAG(&engine.DAG{
   RuleID:"audit-execute", RuleFlow:&schema.RuleFlow{ID:"audit-execute",Enabled:true},
   Plugins:map[string]sdk.Plugin{"sink":p},
 }))
 for _, dry := range []bool{false,true} {
   before := p.calls
   err := ex.Execute(context.Background(), &schema.RuleMessage{
     RuleID:"audit-execute",IsDryRun:dry,Payload:[]map[string]interface{}{{"value":1}},
   })
   fmt.Printf("execute_dry_%t_returned_nil=%t node_calls=%d\n",dry,err==nil,p.calls-before)
 }

 // Synthetic marker only: never read any production resource or password.
 marker := "AUDIT_SYNTHETIC_SECRET"
 must(db.SaveConnectorResource(&sdk.ConnectorResource{
   ID:"audit-resource",PluginID:"missing-audit-plugin",Name:"audit",Enabled:true,
   Connection:json.RawMessage(`{"password":"AUDIT_SYNTHETIC_SECRET","host":"192.0.2.1"}`),
 }))
 w := request(r,"GET","/connectors",nil)
 fmt.Printf("connector_list_status=%d contains_synthetic_secret=%t\n",w.Code,bytes.Contains(w.Body.Bytes(),[]byte(marker)))

 w = request(r,"POST","/rules",[]byte(`{"id":"audit-rule","name":"audit","enabled":true,"version":3,"nodes":[]}`))
 listed := request(r,"GET","/rules",nil)
 fmt.Printf("empty_enabled_rule_create_status=%d reported_deployed_without_runtime=%t\n",w.Code,
   bytes.Contains(listed.Body.Bytes(),[]byte(`"status":"deployed"`)) && len(rt.ListRuleInstances())==0)

 // Two clients read the same version; saving a stale client can move version backwards.
 current, err := db.GetRule("audit-rule"); must(err)
 stale, err := db.GetRule("audit-rule"); must(err)
 current.Name="newer-edit"; must(db.SaveRule(current))
 versionBefore := current.Version
 stale.Name="stale-edit"; must(db.SaveRule(stale))
 final, err := db.GetRule("audit-rule"); must(err)
 fmt.Printf("stale_write_version_before=%d after=%d stale_content_won=%t\n",versionBefore,final.Version,final.Name=="stale-edit")

 cfg := &config.Config{}
 cfg.Manager.Port=11011
 cfg.Manager.DBPath=dir
 must(config.NormalizeAndValidate(cfg))
 raw, err := yaml.Marshal(cfg); must(err); must(db.SaveConfig(raw))
 cfg.Manager.Port=12012
 cfg.Manager.DBPath=dir+"/new-location"
 raw, err = json.Marshal(map[string]interface{}{"config":cfg}); must(err)
 w = request(r,"PUT","/config",raw)
 persisted, err := db.GetConfig(); must(err)
 var saved config.Config; must(yaml.Unmarshal(persisted,&saved))
 fmt.Printf("bootstrap_edit_status=%d manager_port=%d db_path_changed=%t\n",w.Code,saved.Manager.Port,saved.Manager.DBPath!=dir)

 var callbacks atomic.Int32
 notified := make(chan struct{},1)
 cancel, err := db.WatchRules(func(_ string,_ []byte) error {
   callbacks.Add(1)
   select { case notified<-struct{}{}: default: }
   return fmt.Errorf("synthetic transient failure")
 },nil); must(err)
 final.Name="watch-change"; must(db.SaveRule(final))
 select { case <-notified: case <-time.After(5*time.Second): panic("watch callback did not arrive") }
 time.Sleep(3200*time.Millisecond)
 cancel()
 fmt.Printf("failed_watch_callbacks_after_more_than_3_polls=%d\n",callbacks.Load())
}
```

## UI store 复现

在 `ui` 目录下，用 Node 执行下面代码。它读取真实 ruleStore.ts，以 TypeScript 转译后加载真实 Zustand/zundo；只把 API 和规则序列化替换为隔离模拟，不连接服务、不写源码：

```javascript
const fs=require('fs'),ts=require('typescript'),vm=require('vm'); const source=fs.readFileSync('src/store/ruleStore.ts','utf8'); const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2020}}).outputText; const mod={exports:{}}; const api={get:async()=>{throw new Error('synthetic 404')}}; vm.runInNewContext(compiled,{exports:mod.exports,module:mod,require:(name)=>name==='../services/api'?{rulesAPI:api}:name==='../utils/ruleUtils'?{hydrateRule:x=>x,dehydrateRule:x=>x}:require(name)}); const store=mod.exports.useRuleStore; store.getState().setCurrentRule({id:'rule-a',name:'A',nodes:[]}); store.getState().fetchRule('rule-b').then(()=>{const s=store.getState();console.log(JSON.stringify({requested:'rule-b',retainedRule:s.currentRule?.id,error:s.error,loading:s.loading}));});
```

输出：

```json
{"requested":"rule-b","retainedRule":"rule-a","error":"synthetic 404","loading":false}
```

该程序只证明 store 保留旧规则。编辑器没有以路由 ID 和加载错误阻止显示/保存的结论来自 RuleEditorPage.tsx 的代码审查；未做真实浏览器误保存操作。

## 上轮问题复核

```bash
go run docs/audit-repros-2026-09-09/session_repro.go
go run docs/audit-repros-2026-09-09/dedup_repro.go
```

本轮输出：

```text
old_token_accepted_after_password_reset=false
old_token_accepted_after_disable=false
fresh_login_blocked_after_disable=true
fill_ok=true first_attempt_rejected=true retry_falsely_accepted=false
```
