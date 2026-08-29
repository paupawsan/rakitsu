import { registerVisualizationStyle } from './registry';
import TreeStyle from './TreeStyle.vue';

// Eager: Tree (lightweight, always the default)
registerVisualizationStyle({
  meta: { id: 'tree', label: 'Tree', icon: 'T', description: 'Collapsible hierarchy tree' },
  component: TreeStyle,
});

// Disabled for alpha — Tree is the only stable view. Re-enable when ready.
registerVisualizationStyle({
  meta: { id: 'block-diagram', label: 'Block', icon: 'B', description: 'Nested block diagram' },
  component: () => import('./BlockDiagramStyle.vue'),
});
//
// registerVisualizationStyle({
//   meta: { id: 'timeline', label: 'Timeline', icon: 'L', description: 'Chronological timeline with agent swim lanes' },
//   component: () => import('./TimelineStyle.vue'),
// });
//
// registerVisualizationStyle({
//   meta: { id: 'mind-map', label: 'Mind Map', icon: 'M', description: 'Organic force-directed mind map' },
//   component: () => import('./MindMapStyle.vue'),
// });

export { getAllVisualizationStyles, getVisualizationStyle } from './registry';
export type { VisualizationStyle, VisualizationStyleMeta, VisualizationStyleProps } from './types';
