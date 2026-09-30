package rulesdb

// Bucket 名称常量
var (
	BucketRules       = []byte("rules")
	BucketConfig      = []byte("config")       // 系统配置存储
	BucketConnectors  = []byte("connectors")   // 连接器资源存储
	BucketSnapshots   = []byte("snapshots")    // 规则快照存储
	BucketGrayRelease = []byte("gray_release") // 灰度发布配置存储
)
