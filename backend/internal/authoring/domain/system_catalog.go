package domain

import "encoding/json"

func SystemCatalog() (Catalog, error) {
	emptyConfig := json.RawMessage(`{"type":"object","additionalProperties":false}`)
	input := func(key, valueType string) PortDefinition {
		return PortDefinition{Key: key, ValueType: valueType, Required: true}
	}
	definition := func(key, name, category, executor, cachePolicy, riskLevel string, inputs, outputs []PortDefinition, config json.RawMessage) NodeDefinition {
		return NodeDefinition{
			Key: key, Version: "1.0.0", Name: name, Category: category, Executor: executor,
			InputPorts: inputs, OutputPorts: outputs, ConfigSchema: config,
			CachePolicy: cachePolicy, RiskLevel: riskLevel, Executable: true,
		}
	}
	return NewCatalog("lanverse.production", "1.0.0", []NodeDefinition{
		definition(
			"input.script_revision", "Script Revision", "input", "workflow.input.script_revision", "never", "low",
			nil, []PortDefinition{input("script", "script_revision")},
			json.RawMessage(`{"type":"object","properties":{"document_revision_id":{"type":"string","format":"uuid"}},"required":["document_revision_id"],"additionalProperties":false}`),
		),
		definition(
			"agent.production_bible", "Production Bible Candidate", "agent", "activity.production_bible", "by_inputs", "external_ai",
			[]PortDefinition{input("script", "script_revision")}, []PortDefinition{input("candidate", "production_bible_candidate")}, emptyConfig,
		),
		definition(
			"human.production_bible_review", "Production Bible Review", "human", "gate.production_bible_review", "never", "human_gate",
			[]PortDefinition{input("candidate", "production_bible_candidate")}, []PortDefinition{input("bible", "production_bible")}, emptyConfig,
		),
		definition(
			"production.episode_plan", "Episode Plan", "production", "activity.episode_plan", "by_inputs", "low",
			[]PortDefinition{input("script", "script_revision"), input("bible", "production_bible")}, []PortDefinition{input("episodes", "episode_plan")},
			json.RawMessage(`{"type":"object","properties":{"episode_count":{"type":"integer","minimum":1,"maximum":100}},"required":["episode_count"],"additionalProperties":false}`),
		),
		definition(
			"production.episode_structure", "Episode Structure Candidate", "production", "activity.episode_structure", "by_inputs", "low",
			[]PortDefinition{input("episodes", "episode_plan")}, []PortDefinition{input("candidate", "episode_structure_candidate")}, emptyConfig,
		),
		definition(
			"human.episode_structure_review", "Episode Structure Review", "human", "gate.episode_structure_review", "never", "human_gate",
			[]PortDefinition{input("candidate", "episode_structure_candidate")}, []PortDefinition{input("structures", "episode_structures")}, emptyConfig,
		),
		definition(
			"agent.storyboard_draft", "Storyboard Candidate", "agent", "activity.storyboard_draft", "by_inputs", "external_ai",
			[]PortDefinition{input("structures", "episode_structures")}, []PortDefinition{input("candidate", "storyboard_candidate")}, emptyConfig,
		),
		definition(
			"human.storyboard_review", "Storyboard Review", "human", "gate.storyboard_review", "never", "human_gate",
			[]PortDefinition{input("candidate", "storyboard_candidate")}, []PortDefinition{input("storyboards", "storyboards")}, emptyConfig,
		),
		definition(
			"production.storyboard_export", "Storyboard Export", "production", "activity.storyboard_export", "by_inputs", "low",
			[]PortDefinition{input("storyboards", "storyboards")}, []PortDefinition{input("export", "storyboard_export")}, emptyConfig,
		),
	})
}
