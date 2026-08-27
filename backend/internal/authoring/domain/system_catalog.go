package domain

import "encoding/json"

func SystemCatalog() (Catalog, error) {
	return NewCatalog("lanverse.production", "5.0.0", []NodeDefinition{
		systemNodeDefinition(
			"input.script_revision", "Script Revision", "input", "workflow.input.script_revision", "never", "low",
			nil, []PortDefinition{requiredPort("script", "script_revision")},
			json.RawMessage(`{"type":"object","properties":{"document_revision_id":{"type":"string","format":"uuid"}},"required":["document_revision_id"],"additionalProperties":false}`),
		),
		systemNodeDefinition(
			"agent.source_evidence", "Source Evidence Candidate", "agent", "activity.source_evidence", "by_inputs", "external_ai",
			[]PortDefinition{requiredPort("script", "script_revision")}, []PortDefinition{requiredPort("evidence", "source_evidence_candidate")}, emptyNodeConfig(),
		),
		systemNodeDefinition(
			"agent.story_analysis", "Story Reconciliation Candidate", "agent", "activity.story_analysis", "by_inputs", "external_ai",
			[]PortDefinition{requiredPort("evidence", "source_evidence_candidate")}, []PortDefinition{requiredPort("candidate", "story_reconciliation_candidate")}, emptyNodeConfig(),
		),
		systemNodeDefinition(
			"agent.story_review", "Story Candidate Review", "agent", "activity.story_review", "by_inputs", "external_ai",
			[]PortDefinition{requiredPort("candidate", "story_reconciliation_candidate")}, []PortDefinition{requiredPort("candidate", "story_reconciliation_candidate")},
			json.RawMessage(`{"type":"object","properties":{"max_repair_rounds":{"type":"integer","minimum":1,"maximum":3}},"required":["max_repair_rounds"],"additionalProperties":false}`),
		),
		systemNodeDefinition(
			"agent.production_bible", "Production Bible Candidate", "agent", "activity.production_bible", "by_inputs", "external_ai",
			[]PortDefinition{requiredPort("script", "script_revision")}, []PortDefinition{requiredPort("candidate", "production_bible_candidate")}, emptyNodeConfig(),
		),
		systemNodeDefinition(
			"human.production_bible_review", "Production Bible Review", "human", "gate.production_bible_review", "never", "human_gate",
			[]PortDefinition{requiredPort("candidate", "production_bible_candidate")}, []PortDefinition{requiredPort("bible", "production_bible")}, emptyNodeConfig(),
		),
		versionedSystemNode("2.0.0", systemNodeDefinition(
			"production.episode_plan", "Episode Plan", "production", "activity.episode_plan", "by_inputs", "low",
			[]PortDefinition{requiredPort("script", "script_revision"), requiredPort("bible", "production_bible")}, []PortDefinition{requiredPort("candidate", "episode_plan_candidate")},
			json.RawMessage(`{"type":"object","properties":{"episode_count":{"type":"integer","minimum":1,"maximum":100}},"required":["episode_count"],"additionalProperties":false}`),
		)),
		systemNodeDefinition(
			"human.episode_plan_review", "Episode Plan Review", "human", "gate.episode_plan_review", "never", "human_gate",
			[]PortDefinition{requiredPort("candidate", "episode_plan_candidate")}, []PortDefinition{requiredPort("episodes", "episode_plan")}, emptyNodeConfig(),
		),
		systemNodeDefinition(
			"production.episode_structure", "Episode Structure Candidate", "production", "activity.episode_structure", "by_inputs", "low",
			[]PortDefinition{requiredPort("episodes", "episode_plan")}, []PortDefinition{requiredPort("candidate", "episode_structure_candidate")}, emptyNodeConfig(),
		),
		systemNodeDefinition(
			"human.episode_structure_review", "Episode Structure Review", "human", "gate.episode_structure_review", "never", "human_gate",
			[]PortDefinition{requiredPort("candidate", "episode_structure_candidate")}, []PortDefinition{requiredPort("structures", "episode_structures")}, emptyNodeConfig(),
		),
		systemNodeDefinition(
			"agent.storyboard_draft", "Storyboard Candidate", "agent", "activity.storyboard_draft", "by_inputs", "external_ai",
			[]PortDefinition{requiredPort("structures", "episode_structures")}, []PortDefinition{requiredPort("candidate", "storyboard_candidate")}, emptyNodeConfig(),
		),
		systemNodeDefinition(
			"human.storyboard_review", "Storyboard Review", "human", "gate.storyboard_review", "never", "human_gate",
			[]PortDefinition{requiredPort("candidate", "storyboard_candidate")}, []PortDefinition{requiredPort("storyboards", "storyboards")}, emptyNodeConfig(),
		),
		systemNodeDefinition(
			"production.storyboard_export", "Storyboard Export", "production", "activity.storyboard_export", "by_inputs", "low",
			[]PortDefinition{requiredPort("storyboards", "storyboards")}, []PortDefinition{requiredPort("export", "storyboard_export")}, emptyNodeConfig(),
		),
	})
}

func SystemShotCatalog() (Catalog, error) {
	return NewCatalog("lanverse.shot", "2.0.0", []NodeDefinition{
		versionedSystemNode("2.0.0", systemNodeDefinition(
			"input.production_shot", "Production Shot", "input", "workflow.input.production_shot_binding_target", "never", "low",
			nil, []PortDefinition{
				requiredPort("shot", "production_shot"),
				requiredPort("binding_target", "production_shot_image_binding_target"),
			},
			json.RawMessage(`{"type":"object","properties":{"shot_id":{"type":"string","format":"uuid"}},"required":["shot_id"],"additionalProperties":false}`),
		)),
		systemNodeDefinition(
			"input.generation_candidate_set", "Generation Candidate Set", "input", "workflow.input.generation_candidate_set", "never", "low",
			[]PortDefinition{requiredPort("shot", "production_shot")}, []PortDefinition{requiredPort("candidates", "generation_candidate_set")},
			json.RawMessage(`{"type":"object","properties":{"provider_job_id":{"type":"string","format":"uuid"}},"required":["provider_job_id"],"additionalProperties":false}`),
		),
		systemNodeDefinition(
			"human.generation_image_review", "Generation Image Review", "human", "gate.generation_image_review", "never", "human_gate",
			[]PortDefinition{requiredPort("candidates", "generation_candidate_set")},
			[]PortDefinition{requiredPort("selection", "generation_candidate_selection")}, emptyNodeConfig(),
		),
		versionedSystemNode("2.0.0", systemNodeDefinition(
			"production.shot_image_binding", "Shot Image Binding", "production", "activity.production_shot_image_binding_at_target", "never", "low",
			[]PortDefinition{
				requiredPort("shot", "production_shot"),
				requiredPort("selection", "generation_candidate_selection"),
				requiredPort("binding_target", "production_shot_image_binding_target"),
			},
			[]PortDefinition{requiredPort("binding", "production_shot_image_binding")},
			emptyNodeConfig(),
		)),
	})
}

func requiredPort(key, valueType string) PortDefinition {
	return PortDefinition{Key: key, ValueType: valueType, Required: true}
}

func emptyNodeConfig() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false}`)
}

func systemNodeDefinition(
	key, name, category, executor, cachePolicy, riskLevel string,
	inputs, outputs []PortDefinition,
	config json.RawMessage,
) NodeDefinition {
	return NodeDefinition{
		Key: key, Version: "1.0.0", Name: name, Category: category, Executor: executor,
		InputPorts: inputs, OutputPorts: outputs, ConfigSchema: config,
		CachePolicy: cachePolicy, RiskLevel: riskLevel, Executable: true,
	}
}

func versionedSystemNode(version string, value NodeDefinition) NodeDefinition {
	value.Version = version
	return value
}
