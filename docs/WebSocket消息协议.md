# WebSocket 消息协议

## 1. 范围

群策首版需要两条实时链路：

- `Server <-> Local Agent`
- `Server <-> Web Console`

其中：

- 配置类、新建类、查询类操作优先走 HTTP API
- 实时状态、流式输出、在线状态同步走 WebSocket

本协议文档重点定义 WebSocket 层的消息格式和事件类型。

## 2. 连接划分

推荐提供两个独立 WebSocket 端点：

- `/ws/agent`
- `/ws/console`

这样做的原因：

- `Local Agent` 和 `Web Console` 的权限模型不同
- 消息语义不同
- 更容易限流和排查问题

## 3. 通用消息信封

所有 WebSocket 消息统一使用如下结构：

```json
{
  "v": 1,
  "type": "agent.hello",
  "event_id": "evt_01JNYR8JY4N6K0",
  "request_id": "req_01JNYR8JY4N6K0",
  "ts": "2026-03-11T14:00:00Z",
  "source": {
    "kind": "agent",
    "id": "node_001"
  },
  "target": {
    "kind": "server",
    "id": "main"
  },
  "data": {}
}
```

字段说明：

- `v`：协议版本，首版固定为 `1`
- `type`：消息类型
- `event_id`：当前消息唯一 ID，推荐使用 `ULID`
- `request_id`：请求关联 ID，可为空
- `ts`：发送时间，ISO 8601 UTC
- `source`：发送方
- `target`：接收方
- `data`：消息体

约束：

- 所有时间统一使用 UTC
- 所有 ID 统一使用字符串，不在协议层暴露内部自增 ID
- 流式事件必须带顺序号，避免前端拼接错乱

## 4. 鉴权与连接流程

### 4.1 Agent 连接流程

推荐握手顺序：

1. `Local Agent` 连接 `/ws/agent`
2. Agent 发送 `agent.hello`
3. Server 返回 `server.hello`
4. Agent 发送 `agent.auth`
5. Server 返回 `server.auth.ok` 或 `server.auth.reject`
6. Agent 发送 `agent.state.report`
7. Server 视情况补发未完成任务或状态同步指令

### `agent.hello`

```json
{
  "hostname": "macbook-pro-01",
  "platform": "darwin",
  "arch": "arm64",
  "agent_version": "0.1.0",
  "session_id": null
}
```

### `server.hello`

```json
{
  "server_version": "0.1.0",
  "heartbeat_sec": 15,
  "resume_supported": false
}
```

### `agent.auth`

```json
{
  "pair_token": "pair_xxx",
  "node_id": null
}
```

### `server.auth.ok`

```json
{
  "node_id": "node_001",
  "node_name": "张三的 MacBook",
  "max_workers": 4
}
```

### `server.auth.reject`

```json
{
  "code": "PAIR_TOKEN_INVALID",
  "message": "配对令牌无效或已过期"
}
```

### 4.2 Console 连接流程

推荐握手顺序：

1. 浏览器完成 HTTP 登录
2. 浏览器连接 `/ws/console`
3. Server 基于会话 Cookie 或 JWT 完成鉴权
4. 浏览器发送 `console.subscribe`
5. Server 返回快照和后续增量事件

### `console.subscribe`

```json
{
  "chat_ids": ["chat_001"],
  "watch_nodes": true
}
```

## 5. 心跳机制

Agent 链路必须有心跳，Console 链路可依赖底层连接状态。

### `agent.ping`

```json
{
  "running_turn_ids": [],
  "worker_count": 1,
  "load": 0.42,
  "last_completed_turn_id": "turn_009"
}
```

### `server.pong`

```json
{
  "server_time": "2026-03-11T14:00:15Z"
}
```

判定规则建议：

- 连续 `3` 个心跳周期无响应，Server 将节点标记为 `offline`
- 节点重新连上后状态切回 `online`

## 6. Agent 通道消息类型

### 6.1 状态上报

### `agent.state.report`

用途：上报节点当前状态，供 Server 维护在线和运行信息。

```json
{
  "status": "online",
  "max_workers": 4,
  "worker_count": 1,
  "running_turn_ids": ["turn_010"]
}
```

### 6.2 创建执行任务

### `server.turn.request`

用途：Server 要求某个节点执行一轮发言。

```json
{
  "turn_id": "turn_011",
  "chat_id": "chat_001",
  "content": "请从架构角度评价这个方案。",
  "sender_name": "产品经理"
}
```

约束：

- `turn_id` 在系统内全局唯一
- Agent 必须把 `turn_id` 视为幂等键
- 如果重复收到同一个 `turn_id`，且本地已在执行，则本地仍按同一个任务处理

### `agent.turn.started`

```json
{
  "turn_id": "turn_011",
  "worker_count": 1,
  "running_turn_ids": ["turn_011"]
}
```

### 6.3 流式输出

当前实现还没有 `delta` / `stderr` / `status` 这类流式事件，Agent 仅在开始和结束时上报状态。

### `agent.turn.completed`

```json
{
  "turn_id": "turn_011",
  "output": "从架构上看，当前方案的分层是合理的，但需要尽快明确协议边界。",
  "worker_count": 0,
  "running_turn_ids": []
}
```

说明：

- `output` 是最终回复文本
- Server 以 `output` 为准更新消息正文

### 6.4 控制指令

当前实现还没有中断、kill 等控制指令。

## 7. Console 通道消息类型

Console 通道只做“实时同步”，不直接下发执行命令。

### 7.1 快照

### `server.chat.snapshot`

```json
{
  "chat": {
    "chat_id": "chat_001",
    "name": "架构讨论组",
    "mode": "mention"
  },
  "members": [
    {
      "persona_id": "persona_architect",
      "name": "架构师",
      "status": "idle"
    }
  ],
  "recent_messages": []
}
```

### 7.2 消息同步

### `server.message.created`

```json
{
  "chat_id": "chat_001",
  "message": {
    "message_id": "msg_1001",
    "sender_type": "user",
    "sender_name": "产品经理",
    "content": "请大家讨论一下这个方案。",
    "status": "completed",
    "created_at": "2026-03-11T14:12:00Z"
  }
}
```

当前实现还没有独立的消息增量事件，聊天更新通过重新广播 `server.chat.snapshot` 完成。

### 7.3 状态同步

### `server.node.updated`

```json
{
  "nodes": [
    {
      "node_id": "node_001",
      "name": "张三",
      "hostname": "macbook-pro-01",
      "status": "online",
      "approved": true,
      "last_seen_at": "2026-03-11T14:12:15Z"
    }
  ]
}
```

### `server.notice`

```json
{
  "level": "warning",
  "message": "本地节点已离线，后续发言已暂停。"
}
```

## 8. 顺序与幂等

首版必须遵守以下约束：

- `turn_id` 全局唯一
- `message_id` 全局唯一
- Agent 对重复的 `server.turn.request` 必须幂等处理

这样可以降低网络重试带来的重复执行风险。

## 9. 重连策略

MVP 先用简化方案：

- Agent 断线后，Server 将节点状态标记为 `offline`
- 该节点上的运行中 `turn` 标记为 `disconnected`
- Agent 重连后发送 `agent.state.report`
- 如果本地仍有运行中的 worker，则 Server 把 `turn` 恢复为 `running`
- 如果本地 worker 已退出但没有完成记录，则 Server 标记为 `failed`

先不要做复杂的事件重放和断点续流。

## 10. 错误码建议

推荐先统一以下错误码：

- `PAIR_TOKEN_INVALID`
- `AUTH_FAILED`
- `TURN_NOT_FOUND`
- `CODEX_NOT_FOUND`
- `CODEX_START_FAILED`
- `UNKNOWN_ERROR`

## 11. 实现建议

- HTTP 负责 CRUD，WebSocket 负责实时事件
- Server 是唯一权威状态源
- Browser 不直接连接 `Local Agent`
- Agent 不直接写入服务端业务状态
- 流式消息先写事件，再回填最终消息正文
