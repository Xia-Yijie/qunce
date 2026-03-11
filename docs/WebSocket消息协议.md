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
- 所有 ID 统一使用字符串，不在协议层暴露数据库自增 ID
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
  "room_ids": ["room_001"],
  "watch_nodes": true,
  "watch_turns": true
}
```

## 5. 心跳机制

Agent 链路必须有心跳，Console 链路可依赖底层连接状态。

### `agent.ping`

```json
{
  "running_turns": 1,
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
  "running_turns": [
    {
      "turn_id": "turn_010",
      "worker_id": "worker_010",
      "status": "running",
      "started_at": "2026-03-11T14:10:00Z"
    }
  ]
}
```

### 6.2 创建执行任务

### `server.turn.create`

用途：Server 要求某个节点为指定智能体执行一轮发言。

```json
{
  "turn_id": "turn_011",
  "room_id": "room_001",
  "message_id": "msg_1001",
  "persona": {
    "persona_id": "persona_architect",
    "name": "架构师",
    "system_prompt": "你是一个偏架构评审风格的智能体。",
    "style_prompt": "回答简洁，先给结论。"
  },
  "input": {
    "trigger_type": "mention",
    "prompt": "请从架构角度评价这个方案。",
    "context_messages": [
      {
        "role": "user",
        "name": "产品经理",
        "content": "请讨论一下当前系统方案。"
      }
    ]
  },
  "runtime": {
    "timeout_sec": 300,
    "max_output_chars": 20000
  }
}
```

约束：

- `turn_id` 在系统内全局唯一
- Agent 必须把 `turn_id` 视为幂等键
- 如果重复收到同一个 `turn_id`，且本地已在执行，则直接重复返回接受状态

### `agent.turn.accepted`

```json
{
  "turn_id": "turn_011",
  "worker_id": "worker_011",
  "status": "accepted"
}
```

### `agent.turn.rejected`

```json
{
  "turn_id": "turn_011",
  "code": "NODE_BUSY",
  "message": "当前节点并发已满"
}
```

### 6.3 流式输出

### `agent.turn.delta`

用途：把标准输出里的可展示内容按顺序推给 Server。

```json
{
  "turn_id": "turn_011",
  "seq": 1,
  "text": "从架构上看，当前方案的分层是合理的，"
}
```

### `agent.turn.stderr`

用途：回传错误输出，主要用于调试和运行详情。

```json
{
  "turn_id": "turn_011",
  "seq": 1,
  "text": "warning: ..."
}
```

### `agent.turn.status`

```json
{
  "turn_id": "turn_011",
  "status": "running"
}
```

状态建议枚举：

- `queued`
- `accepted`
- `running`
- `completed`
- `failed`
- `interrupted`
- `cancelled`

### `agent.turn.completed`

```json
{
  "turn_id": "turn_011",
  "final_text": "从架构上看，当前方案的分层是合理的，但需要尽快明确协议边界。",
  "exit_code": 0,
  "duration_ms": 8421
}
```

说明：

- `final_text` 是最终汇总文本
- Server 以 `final_text` 为准更新消息正文
- 如果中间丢过部分 `delta`，前端仍能通过完成事件拿到完整消息

### `agent.turn.failed`

```json
{
  "turn_id": "turn_011",
  "code": "CODEX_START_FAILED",
  "message": "codex 可执行文件未找到",
  "retryable": false
}
```

### 6.4 控制指令

### `server.turn.interrupt`

```json
{
  "turn_id": "turn_011"
}
```

语义：

- 请求本地 worker 尝试优雅中断
- 优先发送软中断，而不是直接 kill

### `server.turn.kill`

```json
{
  "turn_id": "turn_011"
}
```

语义：

- 强制终止 worker
- 只有在中断失败或用户明确终止时使用

### `agent.turn.interrupted`

```json
{
  "turn_id": "turn_011",
  "status": "interrupted"
}
```

## 7. Console 通道消息类型

Console 通道只做“实时同步”，不直接下发执行命令。

### 7.1 快照

### `server.room.snapshot`

```json
{
  "room": {
    "room_id": "room_001",
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
  "room_id": "room_001",
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

### `server.message.delta`

```json
{
  "room_id": "room_001",
  "message_id": "msg_1002",
  "turn_id": "turn_011",
  "seq": 1,
  "text": "从架构上看，"
}
```

### `server.message.completed`

```json
{
  "room_id": "room_001",
  "message_id": "msg_1002",
  "turn_id": "turn_011",
  "final_text": "从架构上看，当前方案的分层是合理的，但需要尽快明确协议边界。",
  "status": "completed"
}
```

### `server.message.failed`

```json
{
  "room_id": "room_001",
  "message_id": "msg_1002",
  "turn_id": "turn_011",
  "code": "CODEX_START_FAILED",
  "message": "本地执行失败",
  "status": "failed"
}
```

### 7.3 状态同步

### `server.turn.updated`

```json
{
  "turn_id": "turn_011",
  "room_id": "room_001",
  "persona_id": "persona_architect",
  "status": "running"
}
```

### `server.node.updated`

```json
{
  "node_id": "node_001",
  "status": "online",
  "running_turns": 1,
  "last_seen_at": "2026-03-11T14:12:15Z"
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
- 所有 `delta` 必须带 `seq`
- Server 以 `turn_id + seq` 去重
- Agent 对重复的 `server.turn.create` 必须幂等处理

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
- `NODE_BUSY`
- `NODE_OFFLINE`
- `TURN_NOT_FOUND`
- `TURN_ALREADY_FINISHED`
- `CODEX_NOT_FOUND`
- `CODEX_START_FAILED`
- `TURN_TIMEOUT`
- `TURN_INTERRUPTED`
- `UNKNOWN_ERROR`

## 11. 实现建议

- HTTP 负责 CRUD，WebSocket 负责实时事件
- Server 是唯一权威状态源
- Browser 不直接连接 `Local Agent`
- Agent 不直接落业务数据库
- 流式消息先写事件，再回填最终消息正文
