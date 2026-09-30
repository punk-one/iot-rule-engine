// Package manager 提供规则管理 API
// 路由配置与 handler 实现分离
package manager

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	goruntime "runtime"

	"github.com/gin-gonic/gin"
	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
	"github.com/punk-one/iot-rule-engine/internal/infra/config"
	"github.com/punk-one/iot-rule-engine/internal/infra/storage/rulesdb"
	"github.com/punk-one/iot-rule-engine/internal/server/manager/handlers"
	"github.com/punk-one/iot-rule-engine/internal/server/manager/userauth"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// ServerConfig 服务器配置
type ServerConfig struct {
	Port                      int
	AuthEnabled               bool
	AuthToken                 string
	ExecutorURL               string
	AllowedOrigins            []string
	AllowRuntimePluginInstall bool
	TrustedPluginManifest     string
	MaxPluginUploadBytes      int64
	PProfEnabled              bool
	PProfBlockRate            int
	PProfMutexFraction        int
}

// Server Manager API 服务器
type Server struct {
	config          ServerConfig
	manager         *handlers.Manager
	runtimeHandlers *handlers.RuntimeHandlers
	runtimeMgr      *runtime.Manager
	authService     *userauth.Service
	router          *gin.Engine
	httpServer      *http.Server
	httpMu          sync.Mutex
}

// NewServer 创建新的 Manager 服务器
func NewServer(cfg ServerConfig, db rulesdb.RulesDB, pluginsDir string, runtimeMgr *runtime.Manager, authService *userauth.Service) *Server {
	return NewServerWithLoader(cfg, db, pluginsDir, runtimeMgr, authService, nil)
}

// NewServerWithLoader reuses the engine-owned loader so Manager validation and
// Executor runtime instances share one OS process per plugin binary.
func NewServerWithLoader(cfg ServerConfig, db rulesdb.RulesDB, pluginsDir string, runtimeMgr *runtime.Manager, authService *userauth.Service, pluginLoader *plugin.Loader) *Server {
	// 创建 handlers.Manager
	mgr := handlers.NewManagerWithLoader(db, pluginsDir, cfg.ExecutorURL, runtimeMgr, pluginLoader)
	mgr.SetPluginInstallPolicy(handlers.PluginInstallPolicy{
		Enabled:             cfg.AllowRuntimePluginInstall,
		TrustedManifestPath: cfg.TrustedPluginManifest,
		MaxUploadBytes:      cfg.MaxPluginUploadBytes,
	})

	// 创建 Runtime Handlers
	runtimeHandlers := handlers.NewRuntimeHandlers(runtimeMgr, mgr, cfg.ExecutorURL)

	return &Server{
		config:          cfg,
		manager:         mgr,
		runtimeHandlers: runtimeHandlers,
		runtimeMgr:      runtimeMgr,
		authService:     authService,
	}
}

// SetupRouter 配置路由
func (s *Server) SetupRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	// CORS 中间件
	router.Use(corsMiddleware(s.config.AllowedOrigins))

	// 鉴权中间件
	router.Use(authMiddlewareWrapper(s.config.AuthToken, s.config.AuthEnabled, s.authService))

	// pprof 必须经过认证并限制为管理员，避免泄露运行时与内存信息。
	if s.config.PProfEnabled {
		s.registerPProfRoutes(router)
	}

	// 注册路由
	s.registerRoutes(router)

	s.router = router
	return router
}

func (s *Server) registerPProfRoutes(router *gin.Engine) {
	if s.config.PProfBlockRate > 0 {
		goruntime.SetBlockProfileRate(s.config.PProfBlockRate)
	}
	if s.config.PProfMutexFraction > 0 {
		goruntime.SetMutexProfileFraction(s.config.PProfMutexFraction)
	}
	group := router.Group("/debug/pprof")
	group.Use(requireAdmin())
	group.GET("/", gin.WrapF(pprof.Index))
	group.GET("/cmdline", gin.WrapF(pprof.Cmdline))
	group.GET("/profile", gin.WrapF(pprof.Profile))
	group.GET("/symbol", gin.WrapF(pprof.Symbol))
	group.POST("/symbol", gin.WrapF(pprof.Symbol))
	group.GET("/trace", gin.WrapF(pprof.Trace))
	group.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))
	group.GET("/heap", gin.WrapH(pprof.Handler("heap")))
	group.GET("/allocs", gin.WrapH(pprof.Handler("allocs")))
	group.GET("/block", gin.WrapH(pprof.Handler("block")))
	group.GET("/mutex", gin.WrapH(pprof.Handler("mutex")))
	pkglog.Info("authenticated pprof enabled at /debug/pprof/")
}

// registerRoutes 注册所有路由
func (s *Server) registerRoutes(router *gin.Engine) {
	api := router.Group("/api/v1")
	{
		// 健康检查和指标
		api.GET("/health", HealthHandler)
		api.GET("/metrics", s.manager.MetricsHandler)

		// 不安全开发模式不初始化或暴露用户管理面。
		if s.config.AuthEnabled {
			s.registerAuthRoutes(api)
			s.registerUserRoutes(api)
		}

		// 规则管理
		s.registerRuleRoutes(api)

		// 插件管理
		s.registerPluginRoutes(api)

		// 连接器资源管理
		s.registerConnectorRoutes(api)

		// 配置管理
		s.registerConfigRoutes(api)

		// Service Ops API
		s.registerServiceRoutes(api)

		// Runtime API
		s.registerRuntimeRoutes(api)
	}
}

func (s *Server) registerAuthRoutes(api *gin.RouterGroup) {
	authAPI := api.Group("/auth")
	{
		authAPI.POST("/login", s.LoginHandler)
		authAPI.POST("/logout", s.LogoutHandler)
		authAPI.GET("/me", s.CurrentUserHandler)
		authAPI.PUT("/password", s.ChangeOwnPasswordHandler)
	}
}

func (s *Server) registerUserRoutes(api *gin.RouterGroup) {
	userAPI := api.Group("/users")
	userAPI.Use(requireAdmin())
	{
		userAPI.GET("", s.ListUsersHandler)
		userAPI.POST("", s.CreateUserHandler)
		userAPI.PUT("/:id/password", s.UpdateUserPasswordHandler)
		userAPI.PUT("/:id/status", s.UpdateUserStatusHandler)
	}
}

// registerRuleRoutes 注册规则相关路由
func (s *Server) registerRuleRoutes(api *gin.RouterGroup) {
	// 规则 CRUD
	api.GET("/rules", s.manager.ListRulesHandler)
	api.GET("/rules/:id", s.manager.GetRuleHandler)
	api.POST("/rules", s.manager.CreateRuleHandler)
	api.PUT("/rules/:id", s.manager.UpdateRuleHandler)
	api.DELETE("/rules/:id", s.manager.DeleteRuleHandler)

	// 规则部署
	api.POST("/rules/:id/deploy", s.manager.DeployRuleHandler)
	api.POST("/rules/:id/enable", s.manager.EnableRuleHandler)
	api.POST("/rules/:id/disable", s.manager.DisableRuleHandler)
	api.POST("/rules/validate", s.manager.ValidateRuleHandler)
	api.POST("/rules/dry-run", s.manager.DryRunHandler)

	// 快照管理
	api.GET("/rules/:id/snapshots", s.manager.ListSnapshotsHandler)
	api.GET("/rules/:id/snapshots/latest", s.manager.GetLatestSnapshotHandler)
	api.GET("/rules/:id/snapshots/:version", s.manager.GetSnapshotHandler)
	api.POST("/rules/:id/snapshots", s.manager.CreateSnapshotHandler)

	// 状态机
	api.POST("/rules/:id/transition", s.manager.TransitionStateHandler)
	api.GET("/rules/:id/transitions", s.manager.GetAllowedTransitionsHandler)

	// 灰度发布
	api.POST("/rules/:id/gray-release", s.manager.CreateGrayReleaseConfigHandler)
	api.GET("/rules/:id/gray-release", s.manager.GetGrayReleaseConfigHandler)
	api.PUT("/rules/:id/gray-release", s.manager.UpdateGrayReleaseConfigHandler)
	api.DELETE("/rules/:id/gray-release", s.manager.DeleteGrayReleaseConfigHandler)
}

// registerPluginRoutes 注册插件相关路由
func (s *Server) registerPluginRoutes(api *gin.RouterGroup) {
	api.GET("/plugins", s.manager.ListPluginsHandler)
	api.GET("/plugins/:name", s.manager.GetPluginHandler)
	api.GET("/plugins/:name/compatible-connectors", s.manager.ListCompatibleConnectorsHandler)
	api.POST("/plugins/upload", requireAdmin(), s.manager.UploadPluginHandler)
	api.POST("/plugins/:name/enable", requireAdmin(), s.manager.EnablePluginHandler)
	api.POST("/plugins/:name/disable", requireAdmin(), s.manager.DisablePluginHandler)
	api.DELETE("/plugins/:name", requireAdmin(), s.manager.DeletePluginHandler)
	api.POST("/plugins/:name/upgrade", requireAdmin(), s.manager.UpgradePluginHandler)
}

func (s *Server) registerConnectorRoutes(api *gin.RouterGroup) {
	api.GET("/connectors", s.manager.ListConnectorResourcesHandler)
	api.GET("/connectors/:id", s.manager.GetConnectorResourceHandler)
	api.POST("/connectors", s.manager.CreateConnectorResourceHandler)
	api.PUT("/connectors/:id", s.manager.UpdateConnectorResourceHandler)
	api.DELETE("/connectors/:id", s.manager.DeleteConnectorResourceHandler)

	api.POST("/executor/connectors/:connector_id/sync", s.manager.ExecutorSyncConnectorResourceHandler)
	api.DELETE("/executor/connectors/:connector_id", s.manager.ExecutorDeleteConnectorResourceHandler)
}

// registerConfigRoutes 注册配置相关路由
func (s *Server) registerConfigRoutes(api *gin.RouterGroup) {
	api.GET("/config", requireAdmin(), s.manager.GetConfigHandler)
	api.PUT("/config", requireAdmin(), s.manager.UpdateConfigHandler)
	api.POST("/config/upload", requireAdmin(), s.manager.UploadConfigCertHandler)
}

func (s *Server) registerServiceRoutes(api *gin.RouterGroup) {
	api.GET("/executor/health", s.manager.ExecutorHealthHandler)
	api.GET("/executor/vars", s.manager.ExecutorRulesHandler)
	api.GET("/executor/stats", s.manager.ExecutorStatsHandler)
	api.POST("/executor/rules/:rule_id/sync", s.manager.ExecutorSyncRuleHandler)

	api.GET("/pipeline/health", s.manager.PipelineHealthHandler)
	api.GET("/pipeline/metrics", s.manager.PipelineMetricsHandler)
}

// registerRuntimeRoutes 注册运行时相关路由
func (s *Server) registerRuntimeRoutes(api *gin.RouterGroup) {
	runtimeAPI := api.Group("/runtime")
	{
		// Rule Instances
		runtimeAPI.GET("/instances", s.runtimeHandlers.ListRuleInstancesHandler)
		runtimeAPI.GET("/instances/:instance_id", s.runtimeHandlers.GetRuleInstanceHandler)
		runtimeAPI.GET("/rules/:rule_id/instance", s.runtimeHandlers.GetRuleInstanceByRuleIDHandler)

		// Node Instances
		runtimeAPI.GET("/instances/:instance_id/nodes", s.runtimeHandlers.ListNodeInstancesHandler)
		runtimeAPI.GET("/instances/:instance_id/nodes/:node_id", s.runtimeHandlers.GetNodeInstanceHandler)

		// Edge Instances
		runtimeAPI.GET("/instances/:instance_id/edges", s.runtimeHandlers.ListEdgeInstancesHandler)
		runtimeAPI.GET("/instances/:instance_id/edges/:edge_id", s.runtimeHandlers.GetEdgeInstanceHandler)

		// Trace Events
		runtimeAPI.GET("/instances/:instance_id/trace", s.runtimeHandlers.GetTraceEventsHandler)
		runtimeAPI.GET("/instances/:instance_id/metrics", s.runtimeHandlers.GetCustomMetricsHandler)
		runtimeAPI.GET("/dlq", s.runtimeHandlers.GetDLQEventsHandler)
		runtimeAPI.POST("/dlq/replay", s.runtimeHandlers.ReplayDLQEventsHandler)
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	httpServer := s.prepareHTTPServer()
	pkglog.Info("Manager API server starting on port %d", s.config.Port)
	return httpServer.ListenAndServe()
}

func (s *Server) prepareHTTPServer() *http.Server {
	s.httpMu.Lock()
	defer s.httpMu.Unlock()
	if s.httpServer == nil {
		if s.router == nil {
			s.SetupRouter()
		}
		s.httpServer = &http.Server{
			Addr:              fmt.Sprintf(":%d", s.config.Port),
			Handler:           s.router,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    1 << 20,
		}
	}
	return s.httpServer
}

// Shutdown 关闭服务器
func (s *Server) Shutdown(ctx context.Context) error {
	s.httpMu.Lock()
	httpServer := s.httpServer
	s.httpMu.Unlock()
	if httpServer == nil {
		return nil
	}
	return httpServer.Shutdown(ctx)
}

// corsMiddleware CORS 中间件
func corsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if origin = strings.TrimSpace(origin); origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; !ok && !isSameOrigin(origin, c.Request) {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// isSameOrigin keeps an empty allow-list safe and usable: requests from the
// Manager's own origin are accepted, while every true cross-origin request
// still requires an explicit entry in allowed_origins.
func isSameOrigin(origin string, request *http.Request) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil ||
		parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}

	requestScheme := "http"
	if request.TLS != nil {
		requestScheme = "https"
	}
	return strings.EqualFold(parsed.Scheme, requestScheme) &&
		strings.EqualFold(parsed.Host, request.Host)
}

func requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, _ := c.Get("auth_username")
		legacyAdmin, _ := c.Get("auth_legacy_admin")
		if username == "admin" || legacyAdmin == true {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin privilege required"})
	}
}

// authMiddlewareWrapper 鉴权中间件包装器
func authMiddlewareWrapper(token string, enabled bool, authService *userauth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 健康检查和指标端点不需要鉴权
		if c.Request.URL.Path == "/api/v1/health" ||
			c.Request.URL.Path == "/api/v1/metrics" ||
			c.Request.URL.Path == "/api/v1/executor/health" ||
			c.Request.URL.Path == "/api/v1/pipeline/health" ||
			c.Request.URL.Path == "/api/v1/pipeline/metrics" ||
			c.Request.URL.Path == "/api/v1/auth/login" {
			c.Next()
			return
		}
		if !enabled {
			// Auth can only be disabled when configuration explicitly opted into
			// allow_insecure. Treat that development mode as an administrator so
			// admin-only operational routes remain usable.
			c.Set("auth_legacy_admin", true)
			c.Next()
			return
		}

		legacyEnabled := token != ""
		userEnabled := authService != nil && authService.Enabled()

		providedToken := extractBearerToken(c)
		if providedToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		if legacyEnabled && providedToken == token {
			c.Set("auth_legacy_admin", true)
			c.Next()
			return
		}

		if userEnabled {
			session, err := authService.ValidateToken(providedToken)
			if err == nil {
				c.Set("auth_user_id", session.UserID)
				c.Set("auth_username", session.Username)
				c.Next()
				return
			}
		}

		pkglog.Warn("Manager API: unauthorized request from %s", c.ClientIP())
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		c.Abort()
	}
}

func currentUserID(c *gin.Context) (uint, bool) {
	raw, ok := c.Get("auth_user_id")
	if !ok {
		return 0, false
	}

	switch value := raw.(type) {
	case uint:
		return value, true
	case int:
		if value >= 0 {
			return uint(value), true
		}
	case int64:
		if value >= 0 {
			return uint(value), true
		}
	case float64:
		if value >= 0 {
			return uint(value), true
		}
	case string:
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err == nil {
			return uint(parsed), true
		}
	}

	return 0, false
}

// HealthHandler 健康检查
func HealthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// RunWithServer 使用新的 Server 结构运行（推荐方式）
func RunWithServer(ctx context.Context, cfg *config.Config, runtimeMgr *runtime.Manager, db rulesdb.RulesDB) {
	if err := RunWithServerE(ctx, cfg, runtimeMgr, db); err != nil {
		pkglog.Error("Manager stopped with error: %v", err)
	}
}

// RunWithServerE runs the Manager until cancellation and propagates listener,
// initialization and shutdown failures to the engine supervisor.
func RunWithServerE(ctx context.Context, cfg *config.Config, runtimeMgr *runtime.Manager, db rulesdb.RulesDB) (runErr error) {
	return RunWithServerEAndLoader(ctx, cfg, runtimeMgr, db, nil)
}

// RunWithServerEAndLoader runs Manager with an optional engine-owned loader.
func RunWithServerEAndLoader(ctx context.Context, cfg *config.Config, runtimeMgr *runtime.Manager, db rulesdb.RulesDB, pluginLoader *plugin.Loader) (runErr error) {
	var authService *userauth.Service
	if cfg.Security.Manager.AuthEnabled {
		var err error
		authService, err = userauth.Open(cfg)
		if err != nil {
			return fmt.Errorf("initialize auth service: %w", err)
		}
		defer func() { runErr = errors.Join(runErr, authService.Close()) }()
	}

	serverCfg := ServerConfig{
		Port:                      cfg.Manager.Port,
		AuthEnabled:               cfg.Security.Manager.AuthEnabled,
		AuthToken:                 cfg.Security.Manager.Token,
		AllowedOrigins:            cfg.Security.Manager.AllowedOrigins,
		AllowRuntimePluginInstall: cfg.Security.Manager.AllowRuntimePluginInstall,
		TrustedPluginManifest:     cfg.Security.Manager.TrustedPluginManifest,
		MaxPluginUploadBytes:      cfg.Security.Manager.MaxPluginUploadMB << 20,
		PProfEnabled:              cfg.PProf.Enabled,
		PProfBlockRate:            cfg.PProf.BlockProfileRate,
		PProfMutexFraction:        cfg.PProf.MutexProfileFraction,
	}

	server := NewServerWithLoader(serverCfg, db, cfg.Executor.PluginsDir, runtimeMgr, authService, pluginLoader)
	server.SetupRouter()
	server.prepareHTTPServer()

	serverResult := make(chan error, 1)
	go func() { serverResult <- server.Start() }()

	pkglog.Info("Manager is running (v1.0.0).")

	select {
	case err := <-serverResult:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("manager HTTP server exited unexpectedly")
		}
		return fmt.Errorf("manager HTTP server: %w", err)
	case <-ctx.Done():
	}

	pkglog.Info("Shutting down Manager...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	startErr := <-serverResult
	if startErr != nil && !errors.Is(startErr, http.ErrServerClosed) {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("manager HTTP server: %w", startErr))
	}
	pkglog.Info("Manager shutdown complete")
	return shutdownErr
}
