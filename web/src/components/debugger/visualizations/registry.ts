import type { VisualizationStyle } from './types';

const styles: Map<string, VisualizationStyle> = new Map();

export function registerVisualizationStyle(style: VisualizationStyle) {
  styles.set(style.meta.id, style);
}

export function getVisualizationStyle(id: string): VisualizationStyle | undefined {
  return styles.get(id);
}

export function getAllVisualizationStyles(): VisualizationStyle[] {
  return [...styles.values()];
}
