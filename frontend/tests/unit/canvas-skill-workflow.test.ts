import { expect, it } from "vitest";
import {
  compileSkillGroup,
  createSkillGroup,
  skillGroups,
} from "@/components/canvas/skill-workflow/skill-workflow-graph";
import templates from "@/components/canvas/skill-workflow/skill-workflow-templates.json";

it.each(templates)(
  "imports and compiles $title through the canvas document",
  (template) => {
    const { graph, groupId } = createSkillGroup(
      {
        nodes: [],
        connections: [],
        backgroundMode: "dots",
        viewport: { x: 0, y: 0, k: 1 },
        showImageInfo: false,
      },
      template,
    );
    const [group] = skillGroups(graph);
    expect(group).toMatchObject({ id: groupId, title: template.title });
    const original = compileSkillGroup(graph, groupId);
    expect(original.nodes).toHaveLength(template.nodes.length);
    expect(original.nodes.map((node) => node.skill)).toEqual(
      template.nodes.map((node) => node.skill),
    );
    const edited = { ...graph, connections: [] };
    const compiled = compileSkillGroup(edited, groupId);
    expect(compiled.nodes.every((node) => node.after.length === 0)).toBe(true);
    expect(original.nodes.some((node) => node.after.length > 0)).toBe(true);
  },
);
