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


def test_s3_local_engineering_and_product_acceptance_states_do_not_drift() -> None:
    slice_prd = (DOCS / "prd/002-MVP纵向交付切片.md").read_text(encoding="utf-8")
    prd = (DOCS / "prd/008-创作生产模块PRD任务.md").read_text(encoding="utf-8")
    prd_index = (DOCS / "prd/产品需求索引.md").read_text(encoding="utf-8")
    plan_index = (DOCS / "plan/计划索引.md").read_text(encoding="utf-8")

    for source in (slice_prd, prd, prd_index, plan_index):
        assert "PT-SBD-001～006/PT-AST-004 本地工程证据完成" in source
        assert "S2/S3 产品保持 `in_progress`" in source
        assert "真实 DeepSeek" in source

    assert "S3 本地实现 in_progress" not in slice_prd
    assert "S3 本地实现 in_progress" not in prd_index
    assert "PT-SBD-001～006/PT-AST-004 本地实现 in_progress" not in prd_index


def test_ark_public_contract_is_revalidated_without_premature_sdk_install() -> None:
    admission = (DOCS / "design/005-S2供应商与合规准入设计.md").read_text(
        encoding="utf-8"
    )
    production = (DOCS / "design/模块设计/009-生产模块详细设计.md").read_text(
        encoding="utf-8"
    )
    execution = (DOCS / "prd/003-MVP执行计划与追踪矩阵.md").read_text(
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


def test_minio_acceptance_evidence_tracks_the_latest_compose_decision() -> None:
    compose_acceptance = (
        DOCS / "acceptance/008-D006全栈Compose部署验收.md"
    ).read_text(encoding="utf-8")
    assert "基础镜像使用 `latest`" in compose_acceptance
    assert "不固定 tag、digest 或二进制发行版" in compose_acceptance

    for filename in (
        "003-S2授权治理增量验收.md",
        "004-S2资产增量验收.md",
        "005-S2统一前端增量验收.md",
        "006-S3分镜本地工程增量验收.md",
    ):
        source = (DOCS / "acceptance" / filename).read_text(encoding="utf-8")
        assert "ACC-008" in source
        assert "minio-version-check" not in source
        assert "RELEASE.2025-09-07T16-13-09Z" not in source
        assert "固定版本 MinIO" not in source
        assert "精确版本守卫" not in source


def test_media_version_lifecycle_traceability_is_explicit() -> None:
    requirement = (DOCS / "requirement/004-媒体模块需求.md").read_text(
        encoding="utf-8"
    )
    prd = (DOCS / "prd/007-基础业务模块PRD任务.md").read_text(encoding="utf-8")
    design = (DOCS / "design/模块设计/004-媒体模块详细设计.md").read_text(
        encoding="utf-8"
    )
    plan = (DOCS / "plan/000-MVP全栈实施总计划.md").read_text(encoding="utf-8")
    acceptance = (DOCS / "acceptance/005-S2统一前端增量验收.md").read_text(
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
    acceptance = (DOCS / "acceptance/005-S2统一前端增量验收.md").read_text(
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
    s1_acceptance = (DOCS / "acceptance/002-S1单人立项验收.md").read_text(
        encoding="utf-8"
    )
    s2_acceptance = (DOCS / "acceptance/005-S2统一前端增量验收.md").read_text(
        encoding="utf-8"
    )

    assert "Project 只有无 Episode、版本、任务、费用、审核和交付时可删" in design
    assert "HAS_TASKS" in design
    assert "有版本、任务、费用、审核或交付时返回逐项 blocker" in prd
    assert "Task 引用进入删除预检" in plan
    assert "HAS_TASKS" in s1_acceptance
    assert "单集已有 1 个任务" in s2_acceptance
