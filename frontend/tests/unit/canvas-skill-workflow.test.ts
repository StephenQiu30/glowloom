import { expect, it } from "vitest";
import {
  compileSkillGroup,
  createSkillGroup,
  skillGroups,
} from "@/components/canvas/editor/skill-workflow-graph";
import templates from "@/components/canvas/editor/skill-workflow-templates.json";

it.each(templates)(
  "imports and compiles $title through the canvas document",
  (template) => {
    const graph = createSkillGroup(
      { schemaVersion: 2, nodes: [], connections: [] },
      template,
    );
    const [group] = skillGroups(graph);
    expect(group?.title).toBe(template.title);
    if (!group) throw new Error("Workflow group missing");
    const original = compileSkillGroup(graph, group.id);
    expect(original.nodes).toHaveLength(template.nodes.length);
    expect(original.nodes.map((node) => node.skill)).toEqual(
      template.nodes.map((node) => node.skill),
    );
    const edited = { ...graph, connections: [] };
    const compiled = compileSkillGroup(edited, group.id);
    expect(compiled.nodes.every((node) => node.after.length === 0)).toBe(true);
    expect(original.nodes.some((node) => node.after.length > 0)).toBe(true);
  },
);
