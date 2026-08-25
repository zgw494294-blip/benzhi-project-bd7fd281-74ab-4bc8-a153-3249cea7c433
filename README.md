# bioacoustic-release-hub

`bioacoustic-release-hub` 是面向野生动物声学研究团队的 JSON HTTP 服务。它只登记录音来源元数据和 SHA-256 摘要，不接收大型录音文件；系统在一个数据集聚合内完成信号质检、物种与叫声标注、专家争议处理、整改复验、候选冻结和研究放行。

## 状态与一致性

数据集依次经过 `draft`、`screened`、`annotated`/`under_review`、`ready`、`frozen` 和 `released`。每次成功写入都会递增 `revision`，调用方必须通过 `If-Match-Revision` 提交当前版本。每条命令还必须带 `X-Request-Id`，相同数据集内重复的 requestId 会返回第一次提交的结果，不会重复执行。

角色通过 `X-Role` 指定：资料管理员为 `manager`，标注员为 `annotator`，专家为 `expert`，负责人为 `lead`。操作人由 `X-Actor` 指定。候选冻结后禁止修改纳入内容；负责人可以先撤销候选，完成整改后重新冻结。放行凭据不可修改，可使用校验码独立验证。

SQLite 使用 WAL 模式保存规范化实体、不可变标注修订、审查问题、冻结清单、幂等结果和追加式审计事件。启动时会执行 schemaVersion 迁移、SQLite 完整性检查、悬空引用检查和冻结摘要一致性检查。

## 构建、运行与测试

```text
go build ./cmd/server
go run ./cmd/server -addr=127.0.0.1:19081
go test ./...
```

默认地址是 `127.0.0.1:19081`。可以显式传入 `-addr=127.0.0.1:<port>`；没有传入 `-addr` 时，也可以设置 `PORT`，服务会绑定 `127.0.0.1:<PORT>`。服务拒绝非回环监听地址。数据库默认写入 `bioacoustic-release-hub.db`，可用 `-db=<path>` 修改。

有界 selfcheck 会启动真实监听，通过 HTTP 完成创建、登记、质检、标注、冻结和放行，然后主动关闭：

```text
go run ./cmd/server -addr=127.0.0.1:19081 -selfcheck
```

## API 流程

创建数据集：

```text
curl -X POST http://127.0.0.1:19081/api/v1/datasets -H 'Content-Type: application/json' -H 'X-Role: manager' -H 'X-Actor: alice' -H 'X-Request-Id: create-001' --data '{"title":"华东林鸟声纹","researchGoal":"研究季节性叫声","targetTaxa":["aves"],"recordingRegion":"华东","qualityRuleVersion":"bio-v1"}'
```

后续写请求使用返回的 `dataset.id` 和 `dataset.revision`：

- `POST /api/v1/datasets/{id}/samples`：批量登记样本。
- `POST /api/v1/datasets/{id}/assessments`：记录单个样本的信噪比、削波率和静音比例。
- `POST /api/v1/datasets/{id}/assessments/batch`：通过 `items` 原子提交 1 到 100 个样本的质检或阻断项复验；批次只递增一次 revision。
- `POST /api/v1/datasets/{id}/annotations`：提交不可变标注修订；低置信度或标签变化会创建问题。
- `POST /api/v1/datasets/{id}/issues/{issueId}/decision`：专家执行 `confirm`、`override` 或 `return`。
- `POST /api/v1/datasets/{id}/freeze`：负责人冻结确定性候选清单。
- `POST /api/v1/datasets/{id}/revoke`：负责人撤销尚未放行的冻结候选。
- `POST /api/v1/datasets/{id}/approve`：负责人签发不可变凭据。

查询端点为 `GET /api/v1/datasets/{id}`、`GET /samples`、`GET /issues`、`GET /timeline` 和 `GET /credential`。新增查询包括：

- `GET /api/v1/datasets/{id}/freeze/readiness`：只读汇总冻结前置条件、逐样本阻断项及确定性摘要预览。
- `GET /api/v1/datasets/{id}/samples/{sampleId}/annotations`：按修订号查询不可变标注履历和关联裁决。
- `GET /api/v1/datasets/{id}/freeze/items`：分页读取 frozen 或 released 候选清单，并使用全量清单复核摘要与放行凭据。
- `GET /api/v1/datasets/{id}/issues`：除 `limit`、`offset` 外支持 `status`、`kind`、`severity`、`sampleId` 组合筛选，响应包含筛选后的 `total` 以及不受分页影响的状态和类型汇总。

分页查询接受 `limit` 与 `offset`。`GET /api/v1/credentials/{code}/verify` 校验凭据，`GET /healthz` 提供健康检查。所有响应均为 JSON；错误包含稳定 `code`、中文 `message`，发生版本冲突时还包含当前 revision 和状态。
