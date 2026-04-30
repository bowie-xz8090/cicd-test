# 自动部署平台

一个轻量级的 Web 部署平台，通过浏览器页面一键触发项目的构建与部署。支持从 Gitea 拉取代码，自动构建，并通过 SSH 密码认证将产物部署到不同环境的目标服务器。

## 功能特性

- **免登录使用** — 打开页面即可操作，无需账号密码
- **Gitea 集成** — 自动获取 Gitea 上的项目列表和分支列表
- **多环境部署** — 支持 dev（开发）、sit（测试）、prod（生产）三个环境，每个环境对应不同的目标服务器
- **SSH 密码认证** — 通过密码登录目标服务器，无需配置免密登录
- **自动化流程** — 代码拉取 → 构建 → 上传产物 → 执行部署脚本，全程自动
- **实时状态跟踪** — 页面每 5 秒轮询，实时展示部署进度
- **部署历史** — 记录所有部署操作，支持按项目和环境筛选，可查看执行日志

## 技术栈

| 组件 | 技术 |
|------|------|
| 后端 | Go + Gin |
| 前端 | Vue 3 + Vite + TypeScript |
| 数据库 | SQLite（零配置，自动创建） |
| SSH 连接 | golang.org/x/crypto/ssh（密码认证） |
| 代码管理 | Gitea REST API |

## 环境要求

部署平台所在的机器需要安装以下软件：

| 软件 | 版本要求 | 用途 |
|------|----------|------|
| Go | 1.21+ | 编译后端 |
| Node.js | 18+ | 构建前端 |
| Git | 任意版本 | 部署时拉取项目代码 |
| GCC | 任意版本 | 编译 SQLite 驱动（CGO 依赖） |

> **注意**：目标服务器不需要安装任何额外软件，只需要能通过 SSH 登录即可。

## 快速开始

### 1. 获取代码

```bash
git clone <仓库地址>
cd auto-deploy-platform
```

### 2. 构建前端

```bash
cd web
npm install
npm run build
cd ..
```

构建产物会输出到 `web/dist/` 目录。

### 3. 编译后端

```bash
CGO_ENABLED=1 go build -o auto-deploy-platform ./cmd/
```

> `CGO_ENABLED=1` 是必须的，因为 SQLite 驱动依赖 CGO。

### 4. 创建配置文件

```bash
cp config.example.yaml config.yaml
```

编辑 `config.yaml`，填入实际的 Gitea 地址、服务器信息等（详见下方配置说明）。

### 5. 启动服务

```bash
./auto-deploy-platform
```

打开浏览器访问 `http://localhost:8080` 即可使用。

## 配置说明

所有配置集中在一个 `config.yaml` 文件中，默认放在可执行文件同级目录。也可以通过环境变量指定路径。

### 完整配置示例

```yaml
# ============================================================
# 服务配置
# ============================================================
server:
  port: 8080              # 平台监听端口，浏览器访问此端口
  workspace: "./workspace" # 代码克隆和构建的工作目录（需要写权限）

# ============================================================
# Gitea 配置
# ============================================================
gitea:
  url: "http://192.168.1.100:3000"    # Gitea 服务地址（不带尾部斜杠）
  token: "abc123def456ghi789"          # Gitea API Access Token

# ============================================================
# 环境配置 — 每个环境对应一台目标服务器
# 支持的环境 key：dev、sit、prod
# ============================================================
environments:
  dev:
    label: "开发环境"                   # 页面上显示的中文名称
    server:
      host: "192.168.1.10"            # 目标服务器 IP 地址
      port: 22                         # SSH 端口
      user: "root"                     # SSH 登录用户名
      password: "Dev@2024"             # SSH 登录密码
      deploy_path: "/opt/apps"         # 产物上传到服务器的目标目录

  sit:
    label: "集成测试环境"
    server:
      host: "192.168.1.20"
      port: 22
      user: "root"
      password: "Sit@2024"
      deploy_path: "/opt/apps"

  prod:
    label: "生产环境"
    server:
      host: "192.168.1.30"
      port: 22
      user: "root"
      password: "Prod@2024"
      deploy_path: "/opt/apps"

# ============================================================
# 项目配置 — 每个项目的构建和部署方式
# key 必须和 Gitea 上的仓库名一致
# ============================================================
projects:
  my-frontend:
    build_cmd: "npm install && npm run build"   # 构建命令
    build_output: "./dist"                       # 构建产物路径（相对于项目根目录）
    deploy_script: |                             # 部署后在目标服务器上执行的脚本
      cd /opt/apps/my-frontend
      tar -xzf dist.tar.gz
      nginx -s reload

  my-backend:
    build_cmd: "go build -o server ./cmd/"
    build_output: "./server"
    deploy_script: |
      cd /opt/apps/my-backend
      chmod +x server
      systemctl restart my-backend

  my-java-app:
    build_cmd: "mvn clean package -DskipTests"
    build_output: "./target/app.jar"
    deploy_script: |
      cd /opt/apps/my-java-app
      systemctl stop my-java-app
      cp app.jar app-running.jar
      systemctl start my-java-app
```

### 配置项详解

#### server（服务配置）

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `port` | int | `8080` | 平台 Web 服务监听端口 |
| `workspace` | string | `./workspace` | 代码克隆和构建的工作目录，需要有足够磁盘空间和写权限 |

#### gitea（Gitea 配置）

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `url` | string | 是 | Gitea 服务地址，如 `http://192.168.1.100:3000` |
| `token` | string | 是 | Gitea API Access Token，在 Gitea 的 `设置 → 应用 → 管理 Access Token` 中生成 |

**生成 Gitea Token 的步骤：**
1. 登录 Gitea → 点击右上角头像 → 设置
2. 左侧菜单选择「应用」
3. 在「管理 Access Token」中输入名称，点击「生成令牌」
4. 复制生成的 Token 填入配置文件

#### environments（环境配置）

每个环境的 `server` 配置：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `host` | string | 是 | 目标服务器 IP 地址或域名 |
| `port` | int | 是 | SSH 端口，通常为 `22` |
| `user` | string | 是 | SSH 登录用户名 |
| `password` | string | 是 | SSH 登录密码 |
| `deploy_path` | string | 是 | 产物上传到服务器的目标目录，需要提前创建且有写权限 |

> **安全提示**：配置文件中包含服务器密码，请确保 `config.yaml` 的文件权限设置为仅所有者可读（`chmod 600 config.yaml`）。

#### projects（项目配置）

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `build_cmd` | string | 是 | 构建命令，在项目根目录下执行。会设置环境变量 `BUILD_ENV=dev/sit/prod` |
| `build_output` | string | 是 | 构建产物路径，相对于项目根目录 |
| `deploy_script` | string | 是 | 部署脚本，产物上传后在目标服务器上通过 SSH 执行 |

**重要**：`projects` 下的 key（如 `my-frontend`）必须和 Gitea 上的仓库名完全一致，否则系统找不到对应的构建配置。

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `CONFIG_PATH` | `config.yaml`（当前目录） | 配置文件路径 |
| `DB_PATH` | `deploy.db`（当前目录） | SQLite 数据库文件路径 |
| `GIN_MODE` | `debug` | 设为 `release` 可关闭 Gin 的调试日志 |

## 部署到服务器

### 目录结构

建议在服务器上按以下结构部署：

```
/opt/auto-deploy-platform/
├── auto-deploy-platform          # 可执行文件
├── config.yaml                   # 配置文件（chmod 600）
├── deploy.db                     # SQLite 数据库（自动创建）
├── workspace/                    # 代码工作目录（自动创建）
└── web/
    └── dist/                     # 前端构建产物
        ├── index.html
        └── assets/
```

### 部署步骤

```bash
# 1. 在开发机上编译（也可以在服务器上编译）
CGO_ENABLED=1 go build -o auto-deploy-platform ./cmd/
cd web && npm install && npm run build && cd ..

# 2. 将文件传输到服务器
scp auto-deploy-platform root@your-server:/opt/auto-deploy-platform/
scp config.example.yaml root@your-server:/opt/auto-deploy-platform/config.yaml
scp -r web/dist root@your-server:/opt/auto-deploy-platform/web/dist

# 3. 在服务器上编辑配置
ssh root@your-server
cd /opt/auto-deploy-platform
vim config.yaml    # 填入实际的 Gitea 地址、服务器密码等

# 4. 设置配置文件权限（包含密码，仅所有者可读）
chmod 600 config.yaml

# 5. 启动
./auto-deploy-platform
```

### 使用 systemd 管理（推荐）

创建服务文件 `/etc/systemd/system/auto-deploy.service`：

```ini
[Unit]
Description=Auto Deploy Platform
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/auto-deploy-platform
ExecStart=/opt/auto-deploy-platform/auto-deploy-platform
Environment=GIN_MODE=release
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

管理命令：

```bash
# 加载配置
sudo systemctl daemon-reload

# 设置开机自启
sudo systemctl enable auto-deploy

# 启动服务
sudo systemctl start auto-deploy

# 查看状态
sudo systemctl status auto-deploy

# 查看日志
sudo journalctl -u auto-deploy -f

# 重启服务（修改配置后）
sudo systemctl restart auto-deploy
```

## 使用方式

启动后打开浏览器访问 `http://服务器IP:8080`。

### 部署操作（首页）

1. **选择项目** — 下拉框会自动从 Gitea 加载所有仓库
2. **选择分支** — 选择项目后自动加载该项目的分支列表
3. **选择环境** — 选择 dev / sit / prod
4. **点击「部署」** — 三项全选后按钮才可点击

点击部署后，页面会实时显示任务状态：

| 状态 | 含义 |
|------|------|
| 等待中 | 任务已创建，等待执行 |
| 拉取代码中 | 正在从 Gitea 克隆/拉取代码 |
| 构建中 | 正在执行构建命令 |
| 部署中 | 正在上传产物并执行部署脚本 |
| 成功 | 部署完成 |
| 失败 | 某个阶段出错，可查看日志排查 |

### 部署历史（/history）

- 查看所有历史部署记录
- 支持按项目名称和环境筛选
- 点击任意记录可展开查看详细执行日志

## 部署流程原理

当用户点击「部署」后，后台自动执行以下流程：

```
1. 拉取代码
   从 Gitea 克隆或拉取指定项目的指定分支到 workspace 目录

2. 执行构建
   在项目目录中执行 config.yaml 中配置的 build_cmd
   环境变量 BUILD_ENV 会被设置为 dev/sit/prod

3. 上传产物
   通过 SSH（密码认证）将 build_output 指定的文件上传到目标服务器的 deploy_path

4. 执行部署脚本
   通过 SSH 在目标服务器上执行 deploy_script 中配置的命令
```

任何一个阶段失败，任务会标记为「失败」并记录错误信息。

## 常见问题

### 启动时报 "config file not found"

确保 `config.yaml` 文件存在于当前工作目录，或通过 `CONFIG_PATH` 环境变量指定路径：

```bash
CONFIG_PATH=/etc/deploy/config.yaml ./auto-deploy-platform
```

### 启动时报 "gitea.url must not be empty"

`config.yaml` 中的 `gitea.url` 和 `gitea.token` 是必填项，不能为空。

### 页面上获取项目列表失败

检查 Gitea 配置是否正确：
- `gitea.url` 是否能从部署平台机器访问
- `gitea.token` 是否有效（在 Gitea 上测试：`curl -H "Authorization: token YOUR_TOKEN" http://your-gitea/api/v1/repos/search`）

### 部署失败：SSH connection failed

检查目标服务器配置：
- `host` 和 `port` 是否正确
- `user` 和 `password` 是否能正常 SSH 登录
- 目标服务器的 SSH 服务是否开启了密码认证（`/etc/ssh/sshd_config` 中 `PasswordAuthentication yes`）

### 部署失败：产物上传失败

检查目标服务器的 `deploy_path` 目录是否存在且有写权限：

```bash
ssh user@target-server "ls -la /opt/apps"
```

### 编译时报 CGO 相关错误

SQLite 驱动需要 CGO，确保安装了 GCC：

```bash
# macOS
xcode-select --install

# Ubuntu/Debian
sudo apt install gcc

# CentOS/RHEL
sudo yum install gcc
```

## 项目结构

```
auto-deploy-platform/
├── cmd/
│   └── main.go                 # 程序入口，组件初始化和注入
├── internal/
│   ├── api/                    # HTTP API 路由和处理函数
│   ├── builder/                # 代码拉取和构建执行器
│   ├── config/                 # YAML 配置加载和解析
│   ├── db/                     # SQLite 数据库操作
│   ├── deployer/               # SSH 密码认证部署器
│   ├── gitea/                  # Gitea REST API 客户端
│   └── task/                   # 部署任务管理和异步执行
├── web/                        # Vue 3 前端项目
│   ├── src/
│   │   ├── api/                # API 请求封装
│   │   ├── components/         # 通用组件（DeployStatus、LogViewer）
│   │   ├── views/              # 页面（DeployPage、HistoryPage）
│   │   └── router/             # 路由配置
│   └── dist/                   # 前端构建产物
├── config.example.yaml         # 配置文件示例
├── go.mod                      # Go 依赖管理
└── README.md                   # 本文档
```

## API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/health` | 健康检查 |
| GET | `/api/projects` | 获取 Gitea 项目列表 |
| GET | `/api/projects/:owner/:repo/branches` | 获取项目分支列表 |
| GET | `/api/environments` | 获取环境列表 |
| POST | `/api/deploy` | 触发部署任务 |
| GET | `/api/deploy/:id/status` | 获取任务状态 |
| GET | `/api/deploy/:id/logs` | 获取任务日志 |
| GET | `/api/deploy/records` | 获取部署历史（支持筛选和分页） |
