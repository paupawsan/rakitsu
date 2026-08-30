import yaml from 'js-yaml';
import JSZip from 'jszip';
import type { Config, AgentConfig, ToolConfig, SkillConfig, OrchestratorConfig, Settings } from '../types';

// --- Types ---

export interface FileEntry {
  path: string;
  content: string;
}

export interface ModularFileSet {
  rootConfig: string;
  agents: FileEntry[];
  tools: FileEntry[];
  skills: FileEntry[];
  prompts: FileEntry[];
}

// --- Helpers ---

const FILE_REF_EXTENSIONS = ['.md', '.txt', '.prompt'];

function slugify(name: string): string {
  return name.toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9-_]/g, '');
}

function looksLikeFilePath(value: string): boolean {
  if (value.includes('\n')) return false;
  return FILE_REF_EXTENSIONS.some(ext => value.trim().endsWith(ext));
}

function splitFrontMatter(content: string): { frontMatter: string; body: string } | null {
  if (!content.startsWith('---')) return null;
  let rest = content.slice(3);
  if (rest.startsWith('\n')) rest = rest.slice(1);
  const idx = rest.indexOf('\n---');
  if (idx < 0) return null;
  const frontMatter = rest.slice(0, idx);
  let body = rest.slice(idx + 4);
  if (body.startsWith('\n')) body = body.slice(1);
  return { frontMatter, body };
}

function getDir(path: string): string {
  const parts = path.split('/');
  return parts.length > 1 ? (parts[0] ?? '') : '';
}

function findFileContent(files: FileEntry[], refPath: string): string | null {
  // Try exact match first, then with/without leading ./
  const normalized = refPath.replace(/^\.\//, '');
  for (const f of files) {
    const fNorm = f.path.replace(/^\.\//, '');
    if (fNorm === normalized) return f.content;
  }
  return null;
}

// --- Core Functions ---

function parseModularFiles(files: FileEntry[]): Config | null {
  // 1. Find root config
  const rootFile = files.find(f => {
    const dir = getDir(f.path);
    if (dir !== '') return false;
    if (!/\.(yaml|yml)$/i.test(f.path)) return false;
    try {
      const parsed = yaml.load(f.content) as Record<string, unknown>;
      return parsed && typeof parsed === 'object' && 'name' in parsed;
    } catch {
      return false;
    }
  });

  const config: Config = {
    name: 'Imported Project',
    tools: [],
    agents: [],
  };

  if (rootFile) {
    try {
      const parsed = yaml.load(rootFile.content) as Config;
      if (parsed) {
        config.name = parsed.name || config.name;
        config.version = parsed.version;
        config.description = parsed.description;
        config.settings = parsed.settings;
        config.orchestrator = parsed.orchestrator;
        if (parsed.orchestrators?.length) config.orchestrators = parsed.orchestrators;
        // Filter out $ref placeholders (objects with no name — they'll be resolved via auto-discovery)
        if (parsed.agents?.length) config.agents = parsed.agents.filter(a => a.name);
        if (parsed.tools?.length) config.tools = parsed.tools.filter(t => t.name);
        if (parsed.skills?.length) config.skills = (parsed.skills).filter(s => s.name);
      }
    } catch (e) {
      console.error('Failed to parse root config:', e);
    }
  }

  const inlineAgentNames = new Set(config.agents.map(a => a.name));
  const inlineToolNames = new Set(config.tools.map(t => t.name));
  const inlineSkillNames = new Set((config.skills || []).map(s => s.name));

  // 2. Discover agents
  for (const f of files) {
    if (getDir(f.path) !== 'agents') continue;

    let agent: AgentConfig | null = null;

    if (/\.md$/i.test(f.path)) {
      const parsed = splitFrontMatter(f.content);
      if (!parsed) continue;
      try {
        agent = yaml.load(parsed.frontMatter) as AgentConfig;
        if (agent && !agent.system_prompt && parsed.body.trim()) {
          agent.system_prompt = parsed.body.trim();
        }
      } catch (e) {
        console.error(`Failed to parse agent ${f.path}:`, e);
      }
    } else if (/\.(yaml|yml)$/i.test(f.path)) {
      try {
        agent = yaml.load(f.content) as AgentConfig;
      } catch (e) {
        console.error(`Failed to parse agent ${f.path}:`, e);
      }
    }

    if (agent?.name && !inlineAgentNames.has(agent.name)) {
      config.agents.push(agent);
      inlineAgentNames.add(agent.name);
    }
  }

  // 3. Discover tools
  for (const f of files) {
    if (getDir(f.path) !== 'tools') continue;
    if (!/\.(yaml|yml)$/i.test(f.path)) continue;

    try {
      const tool = yaml.load(f.content) as ToolConfig;
      if (tool?.name && !inlineToolNames.has(tool.name)) {
        config.tools.push(tool);
        inlineToolNames.add(tool.name);
      }
    } catch (e) {
      console.error(`Failed to parse tool ${f.path}:`, e);
    }
  }

  // 4. Discover skills
  for (const f of files) {
    if (getDir(f.path) !== 'skills') continue;

    let skill: SkillConfig | null = null;

    if (/\.md$/i.test(f.path)) {
      const parsed = splitFrontMatter(f.content);
      if (!parsed) continue;
      try {
        skill = yaml.load(parsed.frontMatter) as SkillConfig;
        if (skill && !skill.prompt_template && parsed.body.trim()) {
          skill.prompt_template = parsed.body.trim();
        }
      } catch (e) {
        console.error(`Failed to parse skill ${f.path}:`, e);
      }
    } else if (/\.(yaml|yml)$/i.test(f.path)) {
      try {
        skill = yaml.load(f.content) as SkillConfig;
      } catch (e) {
        console.error(`Failed to parse skill ${f.path}:`, e);
      }
    }

    if (skill?.name && !inlineSkillNames.has(skill.name)) {
      if (!config.skills) config.skills = [];
      config.skills.push(skill);
      inlineSkillNames.add(skill.name);
    }
  }

  // 5. Resolve file references in agent system_prompts
  for (const agent of config.agents) {
    if (!agent.system_prompt) continue;
    const val = agent.system_prompt;

    let refPath: string | null = null;
    if (val.startsWith('file:')) {
      refPath = val.slice(5).trim();
    } else if (looksLikeFilePath(val)) {
      refPath = val.trim();
    }

    if (refPath) {
      const content = findFileContent(files, refPath);
      if (content) {
        agent.system_prompt = content.trim();
      }
    }
  }

  // 6. Resolve file references in skill prompt_templates
  for (const skill of config.skills || []) {
    if (!skill.prompt_template) continue;
    const val = skill.prompt_template;

    let refPath: string | null = null;
    if (val.startsWith('file:')) {
      refPath = val.slice(5).trim();
    } else if (looksLikeFilePath(val)) {
      refPath = val.trim();
    }

    if (refPath) {
      const content = findFileContent(files, refPath);
      if (content) {
        skill.prompt_template = content.trim();
      }
    }
  }

  return config;
}

function generateModularFiles(
  name: string,
  agents: AgentConfig[],
  tools: ToolConfig[],
  skills: SkillConfig[],
  orchestrator?: OrchestratorConfig,
  settings?: Settings,
  orchestrators?: OrchestratorConfig[]
): ModularFileSet {
  const prompts: FileEntry[] = [];

  // Build agent files, extracting long prompts
  const agentFiles: FileEntry[] = [];
  const agentCopies: AgentConfig[] = [];

  for (const agent of agents) {
    const copy = JSON.parse(JSON.stringify(agent)) as AgentConfig;

    // Extract long system_prompts to prompts/ directory
    if (copy.system_prompt && copy.system_prompt.length > 200) {
      const slug = slugify(copy.name);
      const promptPath = `prompts/${slug}.md`;
      prompts.push({ path: promptPath, content: copy.system_prompt });
      copy.system_prompt = promptPath;
    }

    agentCopies.push(copy);
    const slug = slugify(agent.name);
    const content = yaml.dump(copy, { indent: 2, lineWidth: -1, noRefs: true, sortKeys: false });
    agentFiles.push({ path: `agents/${slug}.yaml`, content });
  }

  // Build tool files
  const toolFiles: FileEntry[] = tools.map(tool => ({
    path: `tools/${slugify(tool.name)}.yaml`,
    content: yaml.dump(tool, { indent: 2, lineWidth: -1, noRefs: true, sortKeys: false }),
  }));

  // Build skill files
  const skillFiles: FileEntry[] = skills.map(skill => ({
    path: `skills/${slugify(skill.name)}.yaml`,
    content: yaml.dump(skill, { indent: 2, lineWidth: -1, noRefs: true, sortKeys: false }),
  }));

  // Build root config (settings + orchestrator only, no agents/tools/skills)
  const rootObj: Record<string, unknown> = {
    name,
    version: '1.0',
  };
  if (settings) rootObj.settings = settings;
  if (orchestrators && orchestrators.length > 0) rootObj.orchestrators = orchestrators;
  if (orchestrator) rootObj.orchestrator = orchestrator;

  const rootConfig = yaml.dump(rootObj, { indent: 2, lineWidth: -1, noRefs: true, sortKeys: false });

  return {
    rootConfig,
    agents: agentFiles,
    tools: toolFiles,
    skills: skillFiles,
    prompts,
  };
}

async function downloadModularZip(fileSet: ModularFileSet, projectName: string): Promise<void> {
  const zip = new JSZip();

  zip.file('config.yaml', fileSet.rootConfig);

  for (const f of fileSet.agents) zip.file(f.path, f.content);
  for (const f of fileSet.tools) zip.file(f.path, f.content);
  for (const f of fileSet.skills) zip.file(f.path, f.content);
  for (const f of fileSet.prompts) zip.file(f.path, f.content);

  const blob = await zip.generateAsync({ type: 'blob' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `${slugify(projectName)}.zip`;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}

// --- Composable ---

export function useModularConfig() {
  return {
    parseModularFiles,
    generateModularFiles,
    downloadModularZip,
  };
}
