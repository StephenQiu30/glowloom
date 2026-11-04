# 生成引用与COS复核

2026-11-04。本轮先限定34文件提交`e7e376b46a9123fc6c4f6fafedacffbc72449d75`并推送main，包含连线代码、设计/PRD/原需求及证据；共享BACKLOG仅提交连线段落，未包含并行探索/顶栏工作。提交前6文件22项定向前端测试通过。CI[41751714383](https://github.com/StephenQiu30/glowloom/actions/runs/41751714383)已终结：frontend成功；backend失败于已有`TestPostgresCheckpointerSaveAndResume`缺`agent.graph_checkpoint`表，SQLSTATE42P01，与ca56fa06的前次失败相同。

## 原任务根因

- 原任务`0d7fd96a-2ee4-49be-9147-e1eec908a378`，冻结模型doubao-seedream-5-0-flash-260915，image.generate/image2image，单张、size2K。
- 直接只读取得原Temporal历史，provider.managed返回`state=not_submitted,error_code=invalid_provider_request`。调用表原先只保留state，因此页面显示通用provider_not_submitted，不能把它解释为供应商余额不足或HTTP拒绝。
- 当前数据库主路由为cos-main/COS、签名为HTTPS，真实只读HeadBucket成功。本项目当前没有已就绪COS图片可供完整对象读取核验；桶连通不等于COS素材读取/模型生成通过。
- 原引用的物理位置是legacy-main/MinIO，location e4cd6cee-1274-44d8-aceb-2efc3a5d1b84，5442字节。走现有项目权限/输入证明后，从这个确切位置读取并核验SHA成功，签名为http、127.0.0.1:9000。方舟适配器的HTTPS门禁在本地拒绝它，尚未到供应商请求阶段。
- 这证明本次问题是旧位置图片的供应商输入传输，不证明COS主路由被切回，也不允许把旧记录直接改成cos-main假装完成迁移。

## 修复合同与实现

[方舟官方图片API](https://console.volcengine.com/ark/region%3Acn-beijing/docs/image-generation-api)支持URL或Base64图片输入，要求URL可访问。项目签名器在原对象位置读取，保留现有权限/冻结输入/大小/SHA校验；当图片URL为HTTP、已知内网或回环地址时，保留最多30MiB已核验图片字节。字段ImageBytes仅在worker内，JSON明确排除，不进入workflow历史。公网HTTPS引用不复制图片字节；COS路径、bucket、profile和签名不改。

方舟图片适配器只从上述字节构造带真实图片MIME的Base64，仍拒绝非图片、超界和没有核验字节的HTTP引用。其它协议、主体及视频/音频的本地引用没有因此取得新的输入支持，不把本次图片修复扩展成全量多模态接受。

通用invalid_provider_request映射为安全的provider_invalid_parameters。新的managed工作流保留not_submitted的错误分类，调用表只保存白名单分类，不保存任意上游消息；未知分类仍退回provider_not_submitted。通过Temporal版本门保留旧历史的调用和结算命令，本次旧任务状态不重写、预算不重置、不重试。

## 验证

1. Red：真实签名器＋本地S3 fixture＋方舟离线transport组合，旧代码未到transport（calls0而期望1）。首次fixture缺Last-Modified导致前置错误，补齐后才记录该业务Red。安全分类测试旧代码得到通用错误；workflow测试旧代码丢ErrorCode；隔离PG测试旧条件只保存state、没有failure_code，均按预期失败。
2. Green：8组顶层定向Race测试及子场景通过，包括公网COS URL原样、私有图片编码、非图片和超界零请求、SHA失配拒绝、JSON不包含图片字节、旧/新workflow命令、隔离PG保存分类/重复回执仅一条、未知/已发送/未发送的再次派发门禁。测试只用fixture供应商，未调用真实生成。
3. 实际旧素材：在现有容器和真实对象配置中，通过原权限/位置读取这5442字节并校验。新适配器连到完全替换的离线RoundTripper一次，图片为内嵌PNG且不含回环URL；未发网络生成请求。这是实际素材＋离线供应商验证，不是模型成功。
4. 原任务49事件用新OperationWorkflow代码离线回放通过；replayer不执行Activity、不更改任务或账本。原始history只在内存中使用，未导出包含输入的完整历史。
5. goimports/gofmt、范围go vet及后端正式入口构建通过。govulncheck未发现可达漏洞；另有1个依赖模块漏洞未被当前代码调用。未安装golangci-lint，未将其计为通过。
6. 隔离PostgreSQL18由本轮独立临时容器提供，完整schema事务装载。首次缺预置glowloom_agent_runtime角色，事务回滚；补足测试角色后重建成功。未对业务库运行schema或测试数据写入。
7. 构建成功后仅重启本地glowloom-backend载入修复；容器healthy，localhost:1123/readyz返回status ok。实际入口二进制与提前验证构建的SHA256同为9252d6caee2982963149c69be8f206342e7bd536011bfdbf9ede7153d732bb1c。重启前后只读核对原任务均failed、调用数1、reserved0/settled0/limit120000，没有确认、重试或新预算。诊断程序和独立PG容器在结束时删除。

## 设计与剩余边界

原Figma板3159:54491、说明3159:54598原位同步COS/旧引用根因和离线修复边界。字体沿用Noto Sans SC；说明2300×392、画板2392×1130，24TEXT/20FRAME/86VECTOR、无图片填充，截图无裁切。见figma-readback.json及figma-motion-contract.png。

用户明确“仅修复，暂不再生成”。本轮不新建/确认/重试任务、不增加积分预算。供应商生成中、成功、结果归档、自动采纳与生成中拖动/缩放仍未获得真实接受；不能报告和即梦全量一致。后端根因修复及本补记属于e7e376b4之后的独立交付，提交与推送状态以Git记录为准。
