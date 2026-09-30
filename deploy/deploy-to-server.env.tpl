# IoT Rule Engine 远端部署配置
# 将此文件重命名为 deploy-to-server.env 并修改配置
# 与 deploy-to-server.sh 放在同一目录，脚本会自动加载

# SSH 连接配置
REMOTE_HOST=example.internal
REMOTE_PORT=22
REMOTE_USER=deploy
REMOTE_ROOT=/opt/iot-rule-engine

# 部署模式: deploy | update
#   deploy - 全量部署（上传所有产物、安装服务、启动）
#   update - 增量升级（仅上传后端二进制 + 前端 dist，重启服务）
DEPLOY_MODE=deploy

# 前端端口和后端 API 端口
FRONTEND_PORT=3101
BACKEND_PORT=11011
