# 功能开发

从需求理解到实现、验证、接受和回写。

## 阶段合同

```workspace-workflow
{
  "id": "feature",
  "stages": [
    {
      "id": "understand",
      "label": "理解范围",
      "needs": [],
      "outputs": [
        "artifact"
      ],
      "optional": false
    },
    {
      "id": "design",
      "label": "方案核对",
      "needs": [
        "understand"
      ],
      "outputs": [
        "artifact"
      ],
      "optional": true
    },
    {
      "id": "implement",
      "label": "实现",
      "needs": [
        "design"
      ],
      "outputs": [
        "artifact"
      ],
      "optional": false
    },
    {
      "id": "validate",
      "label": "验证",
      "needs": [
        "implement"
      ],
      "outputs": [
        "evidence"
      ],
      "optional": false
    },
    {
      "id": "accept",
      "label": "接受",
      "needs": [
        "validate"
      ],
      "outputs": [
        "acceptance"
      ],
      "optional": false
    },
    {
      "id": "writeback",
      "label": "知识回写",
      "needs": [
        "accept"
      ],
      "outputs": [
        "artifact"
      ],
      "optional": false
    }
  ]
}
```

## 执行与恢复

每次运行固定输入版本、TaskContext、Skill与流程修订。阶段按依赖顺序推进；可省略阶段须记录理由，验证与接受不能跳过。接手先查询原句柄，未知不自动重派；暂停阻止后续动作，取消不撤销已发生副作用。输入或授权变化后旧证据需复核。

## 接受与回写

接受者核对实际产物和对应输入；passed、failed、skipped、not_run分别记录。流程成功不等于提交、合并或部署。长期结论更新所属文件，实施状态仅在根BACKLOG维护。
