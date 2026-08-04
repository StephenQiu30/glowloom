import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
DOCS = ROOT / "docs"
RANGE = re.compile(r"\b([A-Z]+-(?:ENT|FR|IF|NFR)-)(\d{3})(?:[–-](\d{3}))?\b")
LINK = re.compile(r"\[[^\]]+\]\(([^)#]+)(?:#[^)]+)?\)")


def _expand_requirement_ranges(text: str) -> set[str]:
    leaves: set[str] = set()
    for prefix, start_text, end_text in RANGE.findall(text):
        start = int(start_text)
        end = int(end_text or start_text)
        leaves.update(f"{prefix}{number:03d}" for number in range(start, end + 1))
    return leaves


def test_requirement_matrix_has_exactly_379_unique_leaves() -> None:
    matrix = (DOCS / "prd/010-需求设计与产品任务追踪矩阵.md").read_text(encoding="utf-8")
    assert len(_expand_requirement_ranges(matrix)) == 379


def test_product_tasks_have_expected_unique_distribution() -> None:
    expected = {
        "007-基础业务模块PRD任务.md": 20,
        "008-创作生产模块PRD任务.md": 25,
        "009-剪辑交付与平台保障PRD任务.md": 24,
    }
    all_tasks: set[str] = set()
    for filename, count in expected.items():
        source = (DOCS / "prd" / filename).read_text(encoding="utf-8")
        tasks = set(re.findall(r"^\| (PT-[A-Z]+-\d{3}) \|", source, re.MULTILINE))
        assert len(tasks) == count
        assert all_tasks.isdisjoint(tasks)
        all_tasks.update(tasks)
    assert len(all_tasks) == 69


def test_each_prd_links_to_same_numbered_plan() -> None:
    for number in range(1, 11):
        prefix = f"{number:03d}-"
        prd = next((DOCS / "prd").glob(f"{prefix}*.md"))
        assert f"../plan/{prefix}" in prd.read_text(encoding="utf-8")


def test_markdown_local_links_resolve() -> None:
    broken: list[str] = []
    for document in DOCS.rglob("*.md"):
        for target in LINK.findall(document.read_text(encoding="utf-8")):
            if "://" in target or target.startswith("mailto:"):
                continue
            if not (document.parent / target).resolve().exists():
                broken.append(f"{document.relative_to(ROOT)} -> {target}")
    assert broken == []


def test_scope_guards_remain_explicit() -> None:
    matrix = (DOCS / "prd/010-需求设计与产品任务追踪矩阵.md").read_text(encoding="utf-8")
    assert "IDN-FR-005" in matrix and "不创建 MVP PT" in matrix
    assert "IDN-IF-006" in matrix and "不创建 MVP PT" in matrix
    assert "PT-STO-002 条件性" in matrix


def test_workspace_cache_acceptance_tracks_real_redis_and_remaining_boundary() -> None:
    requirement = (DOCS / "requirement/013-缓存存储与任务调度需求.md").read_text(
        encoding="utf-8"
    )
    design = (
        DOCS / "design/模块设计/014-缓存对象存储与任务调度详细设计.md"
    ).read_text(encoding="utf-8")
    prd = (DOCS / "prd/009-剪辑交付与平台保障PRD任务.md").read_text(
        encoding="utf-8"
    )
    acceptance = (DOCS / "acceptance/011-工作空间详情缓存验收.md").read_text(
        encoding="utf-8"
    )

    assert "PT-CCH-001 已接受" in requirement
    assert "monotonic-revision" in design
    assert "PT-CCH-001 已以 Workspace 详情 cache-aside 接受" in prd
    assert "状态：accepted（PT-CCH-001）" in acceptance
    assert "make contract-redis" in acceptance
    assert "PT-CCH-002" in acceptance and "继续保持未完成" in acceptance


def test_s2_s3_real_provider_acceptance_and_s4_gate_do_not_drift() -> None:
    slice_prd = (DOCS / "prd/002-MVP纵向交付切片.md").read_text(encoding="utf-8")
    prd = (DOCS / "prd/008-创作生产模块PRD任务.md").read_text(encoding="utf-8")
    prd_index = (DOCS / "prd/产品需求索引.md").read_text(encoding="utf-8")
    plan_index = (DOCS / "plan/计划索引.md").read_text(encoding="utf-8")

    for source in (slice_prd, prd, prd_index, plan_index):
        assert "S2/S3" in source
        assert "accepted" in source
        assert "D-004" in source

    assert "真实 DeepSeek" in slice_prd
    assert "Ready 分镜 1/1" in slice_prd
    assert "PT-SCR-001～005" in prd and "已 accepted" in prd
    assert "S2/S3 对应 PT accepted" in prd_index
    assert "S0～S3 accepted" in plan_index
    assert "S2/S3 产品保持 `in_progress`" not in "\n".join(
        (slice_prd, prd, prd_index, plan_index)
    )


def test_ark_public_contract_is_revalidated_without_premature_sdk_install() -> None:
    admission = (DOCS / "design/005-AI供应商与合规准入设计.md").read_text(
        encoding="utf-8"
    )
    production = (DOCS / "design/模块设计/009-生产模块详细设计.md").read_text(
        encoding="utf-8"
    )
    execution = (DOCS / "prd/003-MVP执行计划与追踪矩阵.md").read_text(
        encoding="utf-8"
    )
    acceptance = (DOCS / "acceptance/007-D004方舟公开契约复核.md").read_text(
        encoding="utf-8"
    )

    assert "公开 SDK 复核日期：2026-07-31" in admission
    for contract in (
        "volcengine-python-sdk[ark]==5.0.43",
        "volcenginesdkarkruntime.AsyncArk",
        "https://ark.cn-beijing.volces.com/api/v3",
        "/images/generations",
        "/contents/generations/tasks",
    ):
        assert contract in admission

    for source in (admission, production, execution):
        assert "公开 SDK 不证明模型开通" in source
        assert "D-004 仍保持 open" in source
        assert "真实账号" in source

    dependency_sources = (
        (ROOT / "backend/pyproject.toml").read_text(encoding="utf-8"),
        (ROOT / "backend/requirements.txt").read_text(encoding="utf-8"),
        (ROOT / "backend/requirements-dev.txt").read_text(encoding="utf-8"),
    )
    assert all("volcengine-python-sdk" not in source for source in dependency_sources)
    assert "2026-08-04 精确 wheel 复核" in acceptance
    assert "pip download --no-deps" in acceptance
    assert "没有安装到项目 `.venv`" in acceptance
    assert "GitHub `master` 只作为补充阅读材料" in acceptance


def test_generation_fact_layer_is_traced_without_premature_provider_acceptance() -> None:
    requirement = (DOCS / "requirement/009-生产模块需求.md").read_text(
        encoding="utf-8"
    )
    design = (
        DOCS / "design/模块设计/009-生产模块详细设计.md"
    ).read_text(encoding="utf-8")
    prd = (DOCS / "prd/008-创作生产模块PRD任务.md").read_text(encoding="utf-8")
    plan = (DOCS / "plan/000-MVP全栈实施总计划.md").read_text(encoding="utf-8")
    acceptance = (
        DOCS / "acceptance/019-生成预检与费用预占事实层验收.md"
    ).read_text(encoding="utf-8")

    for source in (requirement, design, prd, plan, acceptance):
        assert "provider_contract_unverified" in source
    assert "空 Key 占位不能自动激活能力" in requirement
    assert "DEV-S4-01 无 Key 事实层" in design
    assert "DEV-S4-01 已完成工程验收" in prd
    assert "DEV-S4-01 | PT-PROD-002–004、PT-PROD-008 | completed" in plan
    assert "状态：accepted（仅 DEV-S4-01 无 Key 事实层" in acceptance
    assert "PT-PROD-002～004/008、S4 与 Provider 执行未接受" in acceptance
    assert "没有 Ollama、Ark SDK 或模拟 Provider 成功" in acceptance


def test_high_cost_guard_is_traced_without_premature_provider_acceptance() -> None:
    requirement = (
        DOCS / "requirement/013-缓存存储与任务调度需求.md"
    ).read_text(encoding="utf-8")
    design = (
        DOCS / "design/模块设计/014-缓存对象存储与任务调度详细设计.md"
    ).read_text(encoding="utf-8")
    prd = (
        DOCS / "prd/009-剪辑交付与平台保障PRD任务.md"
    ).read_text(encoding="utf-8")
    plan = (DOCS / "plan/000-MVP全栈实施总计划.md").read_text(encoding="utf-8")
    acceptance = (
        DOCS / "acceptance/020-高成本生成原子限流与去抖验收.md"
    ).read_text(encoding="utf-8")

    for source in (requirement, design, prd, plan, acceptance):
        assert "PT-CCH-002" in source
        assert "PostgreSQL" in source
        assert "Redis" in source
    assert "PT-CCH-002 产品任务仍 in_progress" in requirement
    assert "### 2.2 高成本生成保护" in design
    assert "PT-CCH-002 无 Provider 工程增量已完成" in prd
    assert "PT-CCH-002 无 Key 增量已完成" in plan
    assert "状态：accepted（仅 DEV-S6-01 的 PT-CCH-002 无 Provider 工程增量" in acceptance
    assert "PT-CCH-002 产品任务、PT-PROD-004 与 S4 未接受" in acceptance
    assert "没有新增容器" in acceptance


def test_queued_generation_cancellation_is_traced_without_provider_acceptance() -> None:
    requirement = (DOCS / "requirement/009-生产模块需求.md").read_text(
        encoding="utf-8"
    )
    design = (
        DOCS / "design/模块设计/009-生产模块详细设计.md"
    ).read_text(encoding="utf-8")
    prd = (DOCS / "prd/008-创作生产模块PRD任务.md").read_text(encoding="utf-8")
    plan = (DOCS / "plan/000-MVP全栈实施总计划.md").read_text(encoding="utf-8")
    acceptance = (
        DOCS / "acceptance/021-排队生成取消与预占释放验收.md"
    ).read_text(encoding="utf-8")

    for source in (requirement, design, prd, plan, acceptance):
        assert "queued" in source
        assert "release" in source
        assert "Provider" in source
    assert "### 4.1 queued 生成取消的无 Provider 子集" in design
    assert "queued 取消子集" in prd
    assert "queued 取消无 Provider 工程增量已完成" in plan
    assert "状态：accepted（仅 DEV-S4-02 的 queued 生成取消无 Provider 工程增量" in acceptance
    assert "PT-PROD-005/006/008、S4 与 Provider 执行未接受" in acceptance
    assert "DEEPSEEK_API_KEY='' ARK_API_KEY=''" in acceptance


def test_minio_acceptance_evidence_tracks_the_latest_compose_decision() -> None:
    compose_acceptance = (
        DOCS / "acceptance/008-D006全栈Compose部署验收.md"
    ).read_text(encoding="utf-8")
    assert "基础镜像使用 `latest`" in compose_acceptance
    assert "不固定 tag、digest 或二进制发行版" in compose_acceptance

    for filename in (
        "003-素材授权治理验收.md",
        "004-资产管理与准备度验收.md",
        "005-剧本资产联合工作台验收.md",
        "006-分镜制作工作台验收.md",
    ):
        source = (DOCS / "acceptance" / filename).read_text(encoding="utf-8")
        assert "ACC-008" in source
        assert "minio-version-check" not in source
        assert "RELEASE.2025-09-07T16-13-09Z" not in source
        assert "固定版本 MinIO" not in source
        assert "精确版本守卫" not in source


def test_minio_port_is_accepted_without_claiming_conditional_oss() -> None:
    requirement = (
        DOCS / "requirement/013-缓存存储与任务调度需求.md"
    ).read_text(encoding="utf-8")
    design = (
        DOCS / "design/模块设计/014-缓存对象存储与任务调度详细设计.md"
    ).read_text(encoding="utf-8")
    prd = (
        DOCS / "prd/009-剪辑交付与平台保障PRD任务.md"
    ).read_text(encoding="utf-8")
    plan = (DOCS / "plan/000-MVP全栈实施总计划.md").read_text(
        encoding="utf-8"
    )
    acceptance = (
        DOCS / "acceptance/022-MinIO对象存储端口验收.md"
    ).read_text(encoding="utf-8")

    for source in (requirement, design, prd, plan, acceptance):
        assert "PT-STO-001" in source
        assert "八项" in source
        assert "MinIO" in source
    assert "PT-STO-001 accepted" in requirement
    assert "PT-STO-001 已按上述边界完成真实 MinIO 收口" in design
    assert "PT-STO-001 已完成并 accepted" in prd
    assert "PT-STO-001 已完成真实 MinIO 收口" in plan
    assert "状态：accepted（PT-STO-001；PT-STO-002 conditional/not-applicable）" in acceptance
    assert "make contract-minio" in acceptance
    assert "make contract-media-stack" in acceptance
    assert "PT-STO-002 明确为 `conditional/not-applicable`" in acceptance
    assert "DEEPSEEK_API_KEY='' ARK_API_KEY=''" in acceptance


def test_media_version_lifecycle_traceability_is_explicit() -> None:
    requirement = (DOCS / "requirement/004-媒体模块需求.md").read_text(
        encoding="utf-8"
    )
    prd = (DOCS / "prd/007-基础业务模块PRD任务.md").read_text(encoding="utf-8")
    design = (DOCS / "design/模块设计/004-媒体模块详细设计.md").read_text(
        encoding="utf-8"
    )
    plan = (DOCS / "plan/000-MVP全栈实施总计划.md").read_text(encoding="utf-8")
    acceptance = (DOCS / "acceptance/005-剧本资产联合工作台验收.md").read_text(
        encoding="utf-8"
    )

    assert "active ↔ archived" in requirement
    assert "比较切换 current、归档、恢复" in prd
    for endpoint in (
        "/api/v1/media-objects/{id}/current-version",
        "/api/v1/media-objects/{id}/restore",
    ):
        assert endpoint in design
        assert f"`{endpoint}`" in plan
    assert "expected_current_version_id" in design
    assert "expected_revision" in design
    assert "追加 v2 → 探测 ready → current 切回 v1 → 归档 → 恢复" in acceptance


def test_audit_slices_are_traced_without_premature_acceptance() -> None:
    prd = (DOCS / "prd/007-基础业务模块PRD任务.md").read_text(encoding="utf-8")
    plan = (DOCS / "plan/000-MVP全栈实施总计划.md").read_text(encoding="utf-8")
    acceptance = (DOCS / "acceptance/005-剧本资产联合工作台验收.md").read_text(
        encoding="utf-8"
    )

    assert "| PT-GOV-006 | in_progress |" in prd
    for action in (
        "identity.registered",
        "identity.login_succeeded",
        "identity.account_deactivated",
        "workspace.created",
        "workspace.restored",
        "project.budget_updated",
        "project.deleted",
        "episode.reordered",
        "episode.deleted",
        "task.started",
        "task.succeeded",
        "task.failed",
        "task.unknown",
        "script.version_published",
        "script.current_changed",
        "asset.version_created",
        "asset.current_changed",
        "shot.spec_version_created",
        "shot.current_spec_changed",
    ):
        assert action in acceptance
    assert "审核和交付动作仍待对应 S5 真实实体接入" in acceptance
    assert (
        "身份、Workspace、Project/Episode、授权、媒体、任务、剧本、资产及 S3 分镜规格纵向切片"
        in plan
    )
    assert "PT-GOV-006 | accepted" not in prd


def test_task_references_are_traced_into_project_delete_guards() -> None:
    design = (DOCS / "design/模块设计/003-项目模块详细设计.md").read_text(
        encoding="utf-8"
    )
    prd = (DOCS / "prd/007-基础业务模块PRD任务.md").read_text(
        encoding="utf-8"
    )
    plan = (DOCS / "plan/007-基础业务模块产品任务执行计划.md").read_text(
        encoding="utf-8"
    )
    s1_acceptance = (DOCS / "acceptance/002-账号与项目立项验收.md").read_text(
        encoding="utf-8"
    )
    s2_acceptance = (DOCS / "acceptance/005-剧本资产联合工作台验收.md").read_text(
        encoding="utf-8"
    )

    assert "Project 只有无 Episode、版本、任务、费用、审核和交付时可删" in design
    assert "HAS_TASKS" in design
    assert "有版本、任务、费用、审核或交付时返回逐项 blocker" in prd
    assert "Task 引用进入删除预检" in plan
    assert "HAS_TASKS" in s1_acceptance
    assert "单集已有 1 个任务" in s2_acceptance


def test_script_versions_are_traced_into_project_delete_guards() -> None:
    design = (DOCS / "design/模块设计/003-项目模块详细设计.md").read_text(
        encoding="utf-8"
    )
    prd = (DOCS / "prd/007-基础业务模块PRD任务.md").read_text(
        encoding="utf-8"
    )
    plan = (DOCS / "plan/007-基础业务模块产品任务执行计划.md").read_text(
        encoding="utf-8"
    )
    s1_acceptance = (DOCS / "acceptance/002-账号与项目立项验收.md").read_text(
        encoding="utf-8"
    )
    s2_acceptance = (DOCS / "acceptance/005-剧本资产联合工作台验收.md").read_text(
        encoding="utf-8"
    )

    assert "HAS_SCRIPT_VERSIONS" in design
    assert "草稿和归档来源下的历史版本" in design
    assert "剧本版本引用已通过 scripts 公开批量计数" in prd
    assert "剧本版本进入删除预检" in plan
    assert "HAS_SCRIPT_VERSIONS" in s1_acceptance
    assert "单集已有 2 个剧本版本" in s2_acceptance


def test_storyboard_facts_are_traced_into_project_delete_guards() -> None:
    project_design = (
        DOCS / "design/模块设计/003-项目模块详细设计.md"
    ).read_text(encoding="utf-8")
    storyboard_design = (
        DOCS / "design/模块设计/008-分镜模块详细设计.md"
    ).read_text(encoding="utf-8")
    project_prd = (DOCS / "prd/007-基础业务模块PRD任务.md").read_text(
        encoding="utf-8"
    )
    project_plan = (
        DOCS / "plan/007-基础业务模块产品任务执行计划.md"
    ).read_text(encoding="utf-8")
    storyboard_plan = (
        DOCS / "plan/008-创作生产模块产品任务执行计划.md"
    ).read_text(encoding="utf-8")
    acceptance = (
        DOCS / "acceptance/006-分镜制作工作台验收.md"
    ).read_text(encoding="utf-8")

    assert "HAS_STORYBOARD_SHOTS" in project_design
    assert "active/archived Shot" in storyboard_design
    assert "Shot/ShotSpec 引用已通过 storyboards 公开批量摘要" in project_prd
    assert "Shot/ShotSpec 进入删除预检" in project_plan
    assert "Project/Episode 删除影响汇总" in storyboard_plan
    assert "单集已有 6 个分镜镜头（7 个规格版本）" in acceptance


def test_asset_facts_are_traced_into_project_delete_guards() -> None:
    project_design = (
        DOCS / "design/模块设计/003-项目模块详细设计.md"
    ).read_text(encoding="utf-8")
    asset_design = (
        DOCS / "design/模块设计/007-资产模块详细设计.md"
    ).read_text(encoding="utf-8")
    project_prd = (DOCS / "prd/007-基础业务模块PRD任务.md").read_text(
        encoding="utf-8"
    )
    project_plan = (
        DOCS / "plan/007-基础业务模块产品任务执行计划.md"
    ).read_text(encoding="utf-8")
    production_plan = (
        DOCS / "plan/008-创作生产模块产品任务执行计划.md"
    ).read_text(encoding="utf-8")
    acceptance = (
        DOCS / "acceptance/006-分镜制作工作台验收.md"
    ).read_text(encoding="utf-8")

    assert "HAS_ASSETS" in project_design
    assert "active/archived Asset" in asset_design
    assert "Asset/AssetVersion 引用已通过 assets 公开批量摘要" in project_prd
    assert "Asset/AssetVersion 进入删除预检" in project_plan
    assert "Project 级资产删除影响汇总" in production_plan
    assert "项目已有 5 个资产（5 个版本）" in acceptance


def test_asset_candidate_decisions_are_traced_into_asset_delete_guards() -> None:
    asset_design = (
        DOCS / "design/模块设计/007-资产模块详细设计.md"
    ).read_text(encoding="utf-8")
    script_design = (
        DOCS / "design/模块设计/006-剧本模块详细设计.md"
    ).read_text(encoding="utf-8")
    prd = (DOCS / "prd/008-创作生产模块PRD任务.md").read_text(
        encoding="utf-8"
    )
    plan = (
        DOCS / "plan/008-创作生产模块产品任务执行计划.md"
    ).read_text(encoding="utf-8")
    acceptance = (
        DOCS / "acceptance/004-资产管理与准备度验收.md"
    ).read_text(encoding="utf-8")

    assert "asset_has_candidate_decisions" in asset_design
    assert "ASSET downstream_id" in script_design
    assert "有版本或决议时不能硬删" in prd
    assert "有版本/决议不硬删" in plan
    assert "资产已被 1 条剧本候选决议关联，只能归档。" in acceptance


def test_related_asset_versions_are_traced_into_asset_delete_guards() -> None:
    asset_design = (
        DOCS / "design/模块设计/007-资产模块详细设计.md"
    ).read_text(encoding="utf-8")
    prd = (DOCS / "prd/008-创作生产模块PRD任务.md").read_text(
        encoding="utf-8"
    )
    plan = (
        DOCS / "plan/008-创作生产模块产品任务执行计划.md"
    ).read_text(encoding="utf-8")
    acceptance = (
        DOCS / "acceptance/004-资产管理与准备度验收.md"
    ).read_text(encoding="utf-8")

    assert "asset_has_related_versions" in asset_design
    assert "Project → Asset" in asset_design
    assert "被道具/服装历史版本引用时同样只能归档" in prd
    assert "被道具/服装历史版本引用时同样只能归档" in plan
    assert "资产已被 1 个道具或服装版本引用，只能归档。" in acceptance


def test_latest_real_infrastructure_and_e2e_gates_are_recorded() -> None:
    frontend_acceptance = (
        DOCS / "acceptance/005-剧本资产联合工作台验收.md"
    ).read_text(encoding="utf-8")
    storyboard_acceptance = (
        DOCS / "acceptance/006-分镜制作工作台验收.md"
    ).read_text(encoding="utf-8")

    for source in (frontend_acceptance, storyboard_acceptance):
        assert "2026-08-03 最新显式复核" in source
        assert "2/2、2/2、1/1、1/1" in source
        assert "42.7 秒" in source
        assert "2026-08-04 当前代码显式复核" in source
        assert "完整无密钥 E2E 7/7（46.3 秒）" in source
    assert "36 镜头 P95 10.93 ms" in storyboard_acceptance
    assert "120 镜头 P95 41.22 ms" in storyboard_acceptance
    assert "120 镜头连续重排 30/30 成功" in storyboard_acceptance
    assert "36 镜头 P95 10.14 ms" in storyboard_acceptance
    assert "120 镜头 P95 52.98 ms" in storyboard_acceptance


def test_storyboard_readiness_is_traced_into_production_snapshot() -> None:
    project_design = (
        DOCS / "design/模块设计/003-项目模块详细设计.md"
    ).read_text(encoding="utf-8")
    project_prd = (DOCS / "prd/007-基础业务模块PRD任务.md").read_text(
        encoding="utf-8"
    )
    project_plan = (
        DOCS / "plan/007-基础业务模块产品任务执行计划.md"
    ).read_text(encoding="utf-8")
    production_plan = (
        DOCS / "plan/008-创作生产模块产品任务执行计划.md"
    ).read_text(encoding="utf-8")
    frontend_acceptance = (
        DOCS / "acceptance/005-剧本资产联合工作台验收.md"
    ).read_text(encoding="utf-8")
    storyboard_acceptance = (
        DOCS / "acceptance/006-分镜制作工作台验收.md"
    ).read_text(encoding="utf-8")

    assert "EpisodeStoryboardSummary" in project_design
    assert "STORYBOARD_SUMMARY_UNAVAILABLE" in project_design
    assert "storyboards 的跨 Episode 批量 readiness 摘要" in project_prd
    assert "12 个 Episode" in project_plan
    assert "ProductionSnapshot 显示 Ready 分镜" in production_plan
    assert "Project 级资产数不按 Episode 重复累加" in frontend_acceptance
    assert "36/120 镜头的 ProductionSnapshot" in storyboard_acceptance
    assert "36 镜头 P95 16.96 ms" in storyboard_acceptance
    assert "120 镜头 P95 44.47 ms" in storyboard_acceptance
    assert "Ready 分镜 2/2" in storyboard_acceptance
    assert "完整无密钥 E2E 7/7（46.3 秒）" in storyboard_acceptance


def test_explicit_external_minio_reuse_is_traced() -> None:
    engineering_design = (
        DOCS / "design/000-项目顶层结构与工程规范.md"
    ).read_text(encoding="utf-8")
    compose_acceptance = (
        DOCS / "acceptance/008-D006全栈Compose部署验收.md"
    ).read_text(encoding="utf-8")

    for source in (engineering_design, compose_acceptance):
        assert "MINIO_REUSE_EXTERNAL=1" in source
        assert "默认仍拒绝" in source
    assert "MinIO 真实契约 2/2" in compose_acceptance
