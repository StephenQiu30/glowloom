import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
DOCS = ROOT / "docs"
LOCAL_LINK = re.compile(r"\[[^\]]+\]\(([^)#]+)(?:#[^)]+)?\)")


def test_document_tree_contains_only_current_layers() -> None:
    expected = {"research", "requirement", "design", "plan", "acceptance", "archive"}
    actual = {path.name for path in DOCS.iterdir() if path.is_dir()}

    assert actual == expected
    assert not (DOCS / "prd").exists()
    assert (DOCS / "design/017-Agent Harness与MVP业务闭环设计.md").is_file()
    assert (DOCS / "plan/001-Agent Harness MVP实施计划.md").is_file()


def test_markdown_local_links_resolve() -> None:
    broken: list[str] = []
    for document in DOCS.rglob("*.md"):
        for target in LOCAL_LINK.findall(document.read_text(encoding="utf-8")):
            if "://" in target or target.startswith("mailto:"):
                continue
            if not (document.parent / target).resolve().exists():
                broken.append(f"{document.relative_to(ROOT)} -> {target}")

    assert broken == []
