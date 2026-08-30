import DebugPipelineNode from './DebugPipelineNode.vue';
import DebugAgentNode from './DebugAgentNode.vue';
import DebugToolNode from './DebugToolNode.vue';
import DebugStepNode from './DebugStepNode.vue';
import DebugGenericNode from './DebugGenericNode.vue';
import ExecIterationNode from './ExecIterationNode.vue';
import ExecThoughtNode from './ExecThoughtNode.vue';

export const debugNodeTypes = {
  'debug-pipeline': DebugPipelineNode,
  'debug-agent': DebugAgentNode,
  'debug-tool': DebugToolNode,
  'debug-step': DebugStepNode,
  'debug-generic': DebugGenericNode,
} as Record<string, any>;

// Execution tree node types — used on Visual Builder canvas during debug/results
export const execNodeTypes = {
  'exec-iteration': ExecIterationNode,
  'exec-thought': ExecThoughtNode,
  'debug-pipeline': DebugPipelineNode,
  'debug-agent': DebugAgentNode,
  'debug-step': DebugStepNode,
  'debug-tool': DebugToolNode,
  'debug-generic': DebugGenericNode,
} as Record<string, any>;

export {
  DebugPipelineNode,
  DebugAgentNode,
  DebugToolNode,
  DebugStepNode,
  DebugGenericNode,
  ExecIterationNode,
  ExecThoughtNode,
};
