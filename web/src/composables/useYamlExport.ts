import yaml from 'js-yaml';
import type { Config, AgentConfig, ToolConfig, SkillConfig, OrchestratorConfig, Settings } from '../types';

/** Recursively strip keys starting with _ (internal fields like _id, _providerId) */
function stripInternal(obj: unknown): unknown {
  if (Array.isArray(obj)) return obj.map(stripInternal);
  if (obj && typeof obj === 'object') {
    const clean: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
      if (!k.startsWith('_')) clean[k] = stripInternal(v);
    }
    return clean;
  }
  return obj;
}

/**
 * Whether the settings block is worth writing at all. This is the third and
 * last place that enumerates settings keys by hand (the others being
 * VisualBuilder's importConfig whitelist and the Settings type) — a key
 * missing here is silently deleted on export even though import kept it.
 *
 * Test presence, not truthiness, for whole sub-blocks: `spawn: {enabled:
 * false}` and `agent_chat: {enabled: false}` are the documented ways to turn
 * those features OFF, and `s.spawn?.enabled` reads both as "no settings" —
 * dropping the one config shape a user writes to disable them.
 */
function hasSettings(s: Settings): boolean {
  return !!(
    s.default_provider ||
    Object.keys(s.providers || {}).length ||
    Object.keys(s.api_keys || {}).length ||
    Object.keys(s.base_urls || {}).length ||
    Object.keys(s.credentials_files || {}).length ||
    (s.allowed_commands && s.allowed_commands.length) ||
    s.defaults?.model ||
    s.defaults?.temperature ||
    s.defaults?.max_tokens ||
    s.execution?.max_iterations ||
    s.execution?.timeout_seconds ||
    s.logging?.level ||
    s.hub_url ||
    s.spawn ||
    s.agent_chat
  );
}

export function useYamlExport() {
  function generateYaml(
    name: string,
    projectId: string,
    agents: AgentConfig[],
    tools: ToolConfig[],
    skills: SkillConfig[],
    orchestrator?: OrchestratorConfig,
    settings?: Settings,
    orchestrators?: OrchestratorConfig[],
    version?: string,
    description?: string,
    interactive?: boolean
  ): string {
    const config: Config = {
      name,
      project_id: projectId,
      version: version || '1.0',
      tools,
      agents,
    };

    if (description) {
      config.description = description;
    }

    if (interactive) {
      config.interactive = true;
    }

    if (settings && hasSettings(settings)) {
      config.settings = settings;
    }

    if (skills.length > 0) {
      config.skills = skills;
    }

    if (orchestrators && orchestrators.length > 0) {
      config.orchestrators = orchestrators;
    }

    if (orchestrator) {
      config.orchestrator = orchestrator;
    }

    // Strip internal fields (_id, _providerId) before export
    const cleanConfig = stripInternal(config);

    return yaml.dump(cleanConfig, {
      indent: 2,
      lineWidth: -1,
      noRefs: true,
      sortKeys: false,
    });
  }

  function downloadYaml(content: string, filename: string = 'agent.yaml') {
    const blob = new Blob([content], { type: 'text/yaml' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  }

  function parseYaml(content: string): Config | null {
    try {
      return yaml.load(content) as Config;
    } catch (e) {
      console.error('Failed to parse YAML:', e);
      return null;
    }
  }

  return {
    generateYaml,
    downloadYaml,
    parseYaml,
  };
}
