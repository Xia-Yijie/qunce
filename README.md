# 群策

群策是一个中文优先的多智能体群聊工作台。

当前版本采用“远端服务端 + 本地守护进程”的结构：

- 服务端部署在远端服务器
- 前端页面由服务端直接提供
- 本地 `agentd` 部署在客户电脑上，主动连接服务端
- 首版执行引擎先只接入 `codex`

主界面目标不是传统单助手聊天，而是类似微信群的群聊界面：用户先配置多个智能体身份，再让它们在同一个房间里协作发言。

## 当前技术栈

- 服务端：`Flask` + `Flask-Sock`
- 前端：`React` + `Vite` + `Ant Design`
- 本地守护进程：`Go`
- 依赖管理：`pixi`
- 服务进程管理：`pm2`

## 项目结构

```text
群策/
  agent/      本地守护进程 agentd
  console/    React 前端
  docs/       设计文档
  server/     Flask 服务端
  pixi.toml   pixi 任务和依赖
  pm2.config.cjs
```

## 已实现内容

- Flask API 骨架
- Flask 直接托管 React 构建产物
- `/ws/agent` 和 `/ws/console` 两条 WebSocket 通道
- 群聊工作台基础界面
- 本地 `agentd` 握手、鉴权、状态上报、心跳
- `pixi run server` 通过 `pm2` 后台启动服务

## 开发要求

本项目使用 `pixi` 管理依赖环境。

如果本机还没有 `pixi`，先安装它，再在项目目录执行下面的命令。

## 常用命令

安装根目录 `pm2` 依赖：

```bash
pixi run pm2-install
```

安装前端依赖：

```bash
pixi run console-install
```

构建前端：

```bash
pixi run console-build
```

后台启动服务端：

```bash
pixi run server
```

查看服务端状态：

```bash
pixi run server-status
```

停止服务端：

```bash
pixi run server-stop
```

前端开发模式：

```bash
pixi run console-dev
```

构建本地 agent：

```bash
pixi run agent-build
```

运行本地 agent：

```bash
pixi run --manifest-path . sh -lc 'cd agent && CGO_ENABLED=0 go run ./cmd/agentd --link 127.0.0.1:8000'
```

或者进入 `agent/` 目录直接运行：

```bash
CGO_ENABLED=0 go run ./cmd/agentd --link 127.0.0.1:8000
```

## agentd 启动参数

`agentd` 现在必须通过 `--link` 显式传入服务端地址，格式可以是 `host:port`，也兼容完整的 WebSocket URL。

工作目录通过 `--workspace` 传入，可选；如果不传，默认使用 `~/.qunce`。启动时会自动创建该目录，并在其中写入本地 SQLite 数据库 `agent.db`。

用法：

```bash
agentd --link <host:port|ws://server-host:8000/ws/agent> [--workspace <workdir>] [--hello "<message>"]
```

例如：

```bash
agentd --link 127.0.0.1:8000
agentd --link 127.0.0.1:8000 --workspace ~/.qunce
```

如果不传 `--link`，会直接输出 usage 并退出。

## 默认行为

- 默认配对码：`dev-pair-token`
- 默认服务端端口：`8000`
- Flask 根路径 `/` 直接返回前端页面
- `agentd` 默认工作目录：`~/.qunce`
- `agentd` 会把节点 ID、节点名、配对码、最近使用的服务端地址等信息持久化到本地 SQLite
- `agentd` 启动时优先读取环境变量；如果没有，再读取本地 SQLite 持久化配置

可用环境变量：

- `QUNCE_PAIR_TOKEN`
- `QUNCE_NODE_ID`
- `QUNCE_NODE_NAME`

## 设计文档

详细设计见 `docs/`：

- `docs/页面信息架构.md`
- `docs/WebSocket消息协议.md`
- `docs/数据库表结构.md`

## 当前限制

- 当前状态数据仍然是内存态，没有正式数据库接入
- 还没有打通“发送消息 -> 调度执行 -> 流式回显”的完整链路
- `codex` 执行层目前还是最小骨架
- 前端生产包体积偏大，后续需要做拆包
