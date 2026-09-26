---
key: mock.echo
version: 1.0.0
description: 验证结构化 Skill 的执行与预算链路
default_model: mock.structured
max_input_tokens: 2048
max_output_tokens: 256
max_repair_rounds: 2
timeout_s: 30
tools: []
validators: [schema]
---
将输入中的 value 原样放入输出对象的 value 字段。只返回符合输出 schema 的 JSON。
