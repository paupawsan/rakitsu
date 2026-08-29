import type { Component } from 'vue';
import type { DebugTreeNode } from '../../../types';

export interface VisualizationStyleMeta {
  id: string;
  label: string;
  icon: string;
  description: string;
}

export interface VisualizationStyle {
  meta: VisualizationStyleMeta;
  component: Component | (() => Promise<{ default: Component }>);
}

export interface VisualizationStyleProps {
  roots: DebugTreeNode[];
  activeIds: Set<string>;
  selectedId?: string;
  visibleUpTo: number;
  totalEvents: number;
  isHistory: boolean;
  visible: boolean;
}
