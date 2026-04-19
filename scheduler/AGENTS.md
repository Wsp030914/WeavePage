# scheduler 接力说明

## 模块职责

`scheduler/` 是独立调度服务，不直接承载业务规则；它负责：

- 接收一次性定时任务
- 将任务写入 Redis
- 到时间后发起 HTTP 回调
- 失败后延迟重试
- 提供取消任务接口和健康检查接口

## 当前运行方式

- 入口文件：`scheduler/main.go`
- 默认监听：`0.0.0.0:9090`
- 依赖 Redis
- 通过 `X-Scheduler-Token` 保护调度接口

## 当前接口

- `POST /api/v1/jobs/once`
  - 创建一次性任务
- `POST /api/v1/jobs/cancel`
  - 取消任务
- `GET /health`
  - 健康检查

## 当前数据存储

- ZSet：`scheduler:jobs`
- Job 明细 key：`scheduler:job:{jobID}`
- 处理锁 key：`scheduler:lock:{jobID}`

## 当前非显而易见约束

- 调度器只允许回调到白名单前缀  
  症状：任务创建成功率异常或接口被拒绝  
  原因：`SCHEDULER_ALLOWED_CALLBACK_PREFIXES` 不匹配  
  结论：修改 server 或内网地址时要同步检查白名单

- 当前只允许 `POST` 回调  
  结论：如果后端回调接口改成其他方法，调度器也要一起改

- 当前 processing lock 只有 30 秒 TTL  
  结论：如果未来回调耗时显著增长，需要重新评估这个值

- 回调失败会按固定延迟重试  
  结论：它不是指数退避，也没有复杂失败策略

## 与 server 的关系

- `server/service/due_scheduler.go`
  - 通过 HTTP 调 scheduler 创建或取消到期提醒任务
- `server/handler/task.go`
  - 通过 `/api/internal/scheduler/task-due` 接收回调

当前到期提醒链路是：

1. server 创建或更新任务
2. server 调 scheduler 创建定时 job
3. scheduler 到时回调 server 内部接口
4. server 再决定是否发提醒邮件

## 改动时必须联动检查

- 改回调 URL：检查 `server/config.yml`、`scheduler` 白名单、docker compose
- 改 callback token：检查 server 和 scheduler 同步
- 改 job ID 规则：检查取消逻辑是否仍可命中
