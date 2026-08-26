from __future__ import annotations

import hashlib
import json
from copy import deepcopy
from pathlib import Path
from typing import Any

FIXTURE_PATH = (
    Path(__file__).resolve().parents[3]
    / "backend"
    / "tests"
    / "fixtures"
    / "storygraph"
    / "contract-v1.json"
)


def canonical_hash(value: Any) -> str:
    encoded = json.dumps(
        value,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    ).encode()
    return hashlib.sha256(encoded).hexdigest()


def owner_ref(
    kind: str, logical_id: str, fragment: str, version: str, content: str
) -> dict[str, Any]:
    return {
        "owner_kind": kind,
        "owner_logical_id": logical_id,
        "fragment_key": fragment,
        "owner_version_id": version,
        "owner_revision": 1,
        "content_hash": content + "0" * 63,
    }


def node_key(node_type: str, owner: dict[str, Any]) -> str:
    material = {
        "schema": "story-node-key-v1",
        "node_type": node_type,
        "owner_kind": owner["owner_kind"],
        "owner_logical_id": owner["owner_logical_id"],
        "fragment_key": owner["fragment_key"],
    }
    return "sgn_" + canonical_hash(material)


def canonical_node(
    node_type: str,
    owner: dict[str, Any],
    payload: dict[str, Any],
    evidence: list[dict[str, Any]] | None = None,
) -> dict[str, Any]:
    value = {
        "story_node_key": node_key(node_type, owner),
        "node_type": node_type,
        "owner_ref": owner,
        "evidence_refs": sorted(
            evidence or [],
            key=lambda item: (
                item["document_revision_id"],
                item["absolute_start"],
                item["absolute_end"],
                item["text_hash"],
            ),
        ),
        "payload": payload,
    }
    value["content_hash"] = canonical_hash(value)
    return value


def canonical_edge(
    edge_type: str,
    source: str,
    target: str,
    qualifier: dict[str, str],
) -> dict[str, Any]:
    key_material = {
        "schema": "story-edge-key-v1",
        "edge_type": edge_type,
        "from_node_key": source,
        "to_node_key": target,
        "qualifier": qualifier,
    }
    value = {
        "edge_key": "sge_" + canonical_hash(key_material),
        "edge_type": edge_type,
        "from_node_key": source,
        "to_node_key": target,
        "qualifier": qualifier,
    }
    value["content_hash"] = canonical_hash(value)
    return value


def test_python_recomputes_go_storygraph_canonical_hash_golden() -> None:
    fixture = json.loads(FIXTURE_PATH.read_text(encoding="utf-8"))
    asset_a_owner = owner_ref(
        "asset",
        "character-a",
        "identity",
        "70000000-0000-0000-0000-000000000001",
        "a",
    )
    asset_b_owner = owner_ref(
        "asset",
        "character-b",
        "identity",
        "70000000-0000-0000-0000-000000000002",
        "b",
    )
    scene_owner = owner_ref(
        "production/planning",
        "episode-1",
        "scene-1",
        "70000000-0000-0000-0000-000000000003",
        "c",
    )
    claim_owner = owner_ref(
        "production/bible",
        "bible-main",
        "relationship-1",
        "70000000-0000-0000-0000-000000000004",
        "d",
    )
    asset_a_key = node_key("asset_identity", asset_a_owner)
    asset_b_key = node_key("asset_identity", asset_b_owner)
    scene_key = node_key("scene", scene_owner)
    claim_payload = deepcopy(fixture["claim"])
    claim_payload["participants"][0]["story_node_key"] = asset_a_key
    claim_payload["participants"][1]["story_node_key"] = asset_b_key
    claim_payload["anchors"] = [scene_key]
    claim = canonical_node(
        "relationship_claim",
        claim_owner,
        claim_payload,
        [
            {
                "document_revision_id": "80000000-0000-0000-0000-000000000001",
                "absolute_start": 4,
                "absolute_end": 8,
                "text_hash": "e" + "0" * 63,
            }
        ],
    )
    nodes = sorted(
        [
            claim,
            canonical_node("scene", scene_owner, {}),
            canonical_node("asset_identity", asset_b_owner, {"asset_kind": "character"}),
            canonical_node("asset_identity", asset_a_owner, {"asset_kind": "character"}),
        ],
        key=lambda item: item["story_node_key"],
    )
    edges = sorted(
        [
            canonical_edge(
                "claim_anchor", scene_key, claim["story_node_key"], {"anchor_role": "scene"}
            ),
            canonical_edge(
                "claim_participant",
                asset_b_key,
                claim["story_node_key"],
                {"participant_role": "object"},
            ),
            canonical_edge(
                "claim_participant",
                asset_a_key,
                claim["story_node_key"],
                {"participant_role": "subject"},
            ),
        ],
        key=lambda item: item["edge_key"],
    )
    topology = {
        "schema_version": fixture["schema_version"],
        "nodes": [
            {"story_node_key": node["story_node_key"], "node_type": node["node_type"]}
            for node in nodes
        ],
        "edges": [
            {
                key: edge[key]
                for key in ("edge_key", "edge_type", "from_node_key", "to_node_key", "qualifier")
            }
            for edge in edges
        ],
    }
    content = {"schema_version": fixture["schema_version"], "nodes": nodes, "edges": edges}

    assert canonical_hash(topology) == fixture["expected_topology_hash"]
    assert canonical_hash(content) == fixture["expected_content_hash"]
