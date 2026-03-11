# Repository Guidelines

## 项目结构与模块组织

- `server/`：Flask + Flask-Sock 后端，主入口在 `server/app/`，核心文件包括 `main.py`、`state.py`、`config.py`。
- `console/`：React 19 + Vite + TypeScript 前端，应用代码位于 `console/src/`。
- `agent/`：Go 编写的本地守护进程 `agentd`，CLI 入口是 `agent/cmd/agentd/main.go`。
- `docs/`：产品、协议和数据结构设计文档。
- 根目录文件：`pixi.toml` 定义统一任务，`pm2.config.cjs` 管理服务进程，根 `package.json` 仅用于安装 `pm2`。

## 构建、测试与开发命令

- `pixi run pm2-install`：安装根目录 `pm2` 依赖。
- `pixi run console-install`：安装前端依赖。
- `pixi run console-dev`：启动 Vite 开发服务，监听 `0.0.0.0:5173`。
- `pixi run console-build`：执行前端类型检查并构建产物。
- `pixi run server`：必要时先构建前端，再通过 `pm2` 启动或重启后端。
- `pixi run server-status`：查看后端进程状态。
- `pixi run server-stop`：停止后端进程。
- `pixi run agent-build`：构建 `agent/bin/agentd`。
- `cd agent && CGO_ENABLED=0 go run ./cmd/agentd ws://127.0.0.1:8000/ws/agent`：将本地 agent 连接到开发服务端。

## 代码风格与命名约定

- TypeScript 开启严格模式（见 `console/tsconfig.json`），使用 2 空格缩进；React 组件使用 `PascalCase`，变量、hooks、工具函数使用 `camelCase`。
- Python 遵循 PEP 8，使用 4 空格缩进；函数和变量采用 `snake_case`，保持模块职责单一。
- Go 代码必须保持 `gofmt` 格式；导出标识符使用 MixedCaps，包级类型保持简洁清晰。

## 测试规范

当前仓库还没有提交自动化测试套件。在补齐测试前，至少完成以下验证：

- `pixi run console-build`
- `pixi run server` 后访问 `/api/health`
- `pixi run agent-build`，并让 agent 连到本地服务端

如果后续新增测试，前端测试优先放在相关组件旁边或 `console/src/` 下；后端和 agent 测试放在对应代码附近，方便维护。

## 提交与合并请求规范

现有提交历史采用 Conventional Commits，例如 `feat: initialize qunce project skeleton`。后续提交继续使用 `feat:`、`fix:`、`docs:`、`refactor:` 等前缀。

PR 应包含：

- 变更摘要，说明用户可见行为或协议层改动。
- 关联的 issue 或任务背景；没有则写明来源。
- `console/` 界面变更附截图。
- 本次实际执行过的验证步骤，如 `pixi run console-build`、`pixi run agent-build`。
- 新增或变更的环境变量说明，例如 `QUNCE_PAIR_TOKEN`、`QUNCE_NODE_ID`、`QUNCE_SERVER_PORT`。
