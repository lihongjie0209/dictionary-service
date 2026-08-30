# dictionary-service

平台统一数据字典服务。它拥有静态字典的草稿、发布快照和版本记录，同时作为动态字典 Provider 的注册中心与查询网关；动态数据始终保留在对应业务服务中，dictionary-service 不跨 schema 查询其他服务的表。

## 能力

- 静态字典：定义、草稿项、乐观锁更新、不可变发布版本、分页查询、树和批量编码解析。
- 动态字典：Provider 能力注册、随机租约 Token、心跳、注销、DNS 目标白名单和租约失效隔离。
- 统一数据面：调用共享 `platform.dictionary.v1.DictionaryProviderService`，支持 PSK/mTLS、连接复用、独立超时、重试、熔断、指标、Trace 和 Redis 结果缓存。
- 可靠事件：状态与 Protobuf `EventEnvelope` 在同一数据库事务写入 outbox，再投递到 NATS JetStream 的 `PLATFORM_EVENTS` stream。
- 数据库：PostgreSQL/Kingbase schema 隔离和 MySQL database 隔离；迁移表默认为 `dictionary_schema_migrations`。
- 传输：前端 POST+JSON 统一响应；内部 gRPC 独立端口；JWT 由 identity-service JWKS 验证，Provider 控制面和数据面使用可配置 PSK/mTLS。

共享契约来自 [`platform-protos`](https://github.com/lihongjie0209/platform-protos)，Provider 生命周期组件来自 [`microservice-platform-go/dictionaryprovider`](https://github.com/lihongjie0209/microservice-platform-go/tree/main/dictionaryprovider)。本仓库不复制业务服务 Proto。

## API

前端接口统一返回：

```json
{"code":0,"message":"success","body":{},"request_id":"..."}
```

主要接口：

- `/api/v1/dictionaries/create|update|get|list`
- `/api/v1/dictionaries/items/upsert|delete`
- `/api/v1/dictionaries/publish|query|tree|resolve`
- `/api/v1/dictionaries/providers/register|heartbeat|unregister|list`
- `/live`、`/ready`、`/metrics`、`/swagger/index.html`

内部 gRPC 提供 `DictionaryService`；业务服务实现 `DictionaryProviderService.Describe|Query|Tree|ResolveCodes`。Provider 查询只接受注册时声明的过滤和排序字段，树查询强制深度与节点上限，避免透传任意 SQL 或无界结果。

完整请求/响应模型、JWT/PSK 说明和错误码见生成的 [OpenAPI 文档](docs/swagger.yaml)。

## 配置与运行

配置按 `config.yaml` → `config-{environment}.yaml` → `APP_*` 环境变量覆盖，`APP_ENV` 或 `-env` 选择环境。开发、测试和生产配置不保存真实密钥。

```bash
make test-race
make test-integration
make lint
make swagger-check
make build

# 独立开发栈
make dev-up
make dev-logs
make dev-down
```

平台工作区中使用：

```bash
cd /root/code/microservice-platform
make dev-up
make system-test
```

本地平台默认端口为 HTTP `18091`、gRPC `19091`，数据库为 `platform`，schema 为 `dictionary`。进程启动会在 HTTP/gRPC 监听前执行待处理迁移；生产也保留独立 migration Job 以支持受控发布。

## Provider 注册流程

1. 业务服务实现共享 `DictionaryProviderService`。
2. `platform-go/dictionaryprovider` 的 Redis/Redsync 领导租约在多副本中选出一个注册协调者。
3. 协调者向 `DictionaryService.RegisterProvider` 上报稳定的 Kubernetes Service DNS、能力、缓存和超时，并仅在内存保存返回的租约 Token。
4. SDK 每隔租约时长的三分之一续租；领导权丢失或优雅停机时立即注销。
5. dictionary-service 校验 Provider 状态和租约，再通过共享 gRPC Client 转发查询；Kubernetes Service 负责数据面实例负载均衡。

Provider 目标必须是允许后缀内的 DNS 名称和端口；IP、localhost、URL 用户信息和任意外部主机会被拒绝。生产配置要求 TLS，PSK 和证书由 Secret 管理。

## 数据与并发规则

- 所有可变表包含 `version`、`created_at`、`updated_at`、`created_by`、`updated_by`。
- PostgreSQL/Kingbase 使用 `TEXT` 和 `TIMESTAMPTZ`，会话时区为 `Asia/Shanghai`。
- 普通更新使用期望版本的原子乐观锁；字典发布另使用最小粒度 `dictionary:publish:<id>` 分布式锁。
- 静态发布项按 release 形成不可变快照；Provider 变更和发布事件使用事务 outbox。
- 线上服务不得读取其他服务 schema；报表场景通过事件、CDC 或独立只读模型处理。

## 验证与发布

CI 执行 race detector、vet、golangci-lint、Swagger 一致性、Kubernetes manifest 校验、PostgreSQL/MySQL Testcontainers 集成测试和多架构镜像构建。`make build` 与 Docker 构建会注入版本、完整 Git commit 和 UTC 构建时间：

```bash
./bin/api -version
curl -X POST http://127.0.0.1:18091/api/v1/version -H 'Content-Type: application/json' -d '{}'
```
