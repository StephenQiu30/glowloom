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
    prd = (DOCS / "prd/008-创作生产模块PRD任务.md").read_text(encoding="utf-8")
    prd_index = (DOCS / "prd/产品需求索引.md").read_text(encoding="utf-8")
    plan_index = (DOCS / "plan/计划索引.md").read_text(encoding="utf-8")

    for source in (prd, prd_index, plan_index):
        assert "PT-SBD-001～006/PT-AST-004 本地工程证据完成" in source
        assert "S2/S3 产品保持 `in_progress`" in source
        assert "真实 DeepSeek" in source

    assert "S3 本地实现 in_progress" not in prd_index
    assert "PT-SBD-001～006/PT-AST-004 本地实现 in_progress" not in prd_index
