import AgentNode from './AgentNode.vue';
import ToolNode from './ToolNode.vue';
import SkillNode from './SkillNode.vue';
import OrchestratorNode from './OrchestratorNode.vue';
import GroupNode from './GroupNode.vue';

export const nodeTypes = {
  agent: AgentNode,
  tool: ToolNode,
  skill: SkillNode,
  orchestrator: OrchestratorNode,
  group: GroupNode,
};

export { AgentNode, ToolNode, SkillNode, OrchestratorNode, GroupNode };
