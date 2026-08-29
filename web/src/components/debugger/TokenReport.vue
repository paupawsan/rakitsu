<script setup lang="ts">
import type { DebugTreeNode } from '../../types';

const props = defineProps<{
  treeRoots: DebugTreeNode[];
  renderKey?: number;
}>();

defineEmits<{ close: [] }>();

interface AgentRow {
  name: string;
  input: number;
  output: number;
  total: number;
}

function collectAgentTokens(): AgentRow[] {
  void props.renderKey;
  const rows: AgentRow[] = [];
  function walk(node: DebugTreeNode) {
    if (node.type === 'agent' && node.tokens && node.tokens.total_tokens > 0) {
      rows.push({
        name: node.agentName || node.label,
        input: node.tokens.input_tokens,
        output: node.tokens.output_tokens,
        total: node.tokens.total_tokens,
      });
    }
    node.children.forEach(walk);
  }
  props.treeRoots.forEach(walk);
  return rows;
}

function fmt(n: number): string {
  return n >= 1000 ? `${(n / 1000).toFixed(1)}K` : String(n);
}
</script>

<template>
  <div class="report-overlay" @click.self="$emit('close')">
    <div class="report-card">
      <div class="report-header">
        <h3>Token Usage Report</h3>
        <button class="close-btn" @click="$emit('close')">x</button>
      </div>
      <table class="report-table">
        <thead>
          <tr>
            <th>Agent</th>
            <th class="num">Input</th>
            <th class="num">Output</th>
            <th class="num">Total</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in collectAgentTokens()" :key="row.name">
            <td class="agent-name">{{ row.name }}</td>
            <td class="num">{{ fmt(row.input) }}</td>
            <td class="num">{{ fmt(row.output) }}</td>
            <td class="num total">{{ fmt(row.total) }}</td>
          </tr>
        </tbody>
        <tfoot>
          <tr>
            <td class="agent-name"><strong>Total</strong></td>
            <td class="num"><strong>{{ fmt(collectAgentTokens().reduce((s, r) => s + r.input, 0)) }}</strong></td>
            <td class="num"><strong>{{ fmt(collectAgentTokens().reduce((s, r) => s + r.output, 0)) }}</strong></td>
            <td class="num total"><strong>{{ fmt(collectAgentTokens().reduce((s, r) => s + r.total, 0)) }}</strong></td>
          </tr>
        </tfoot>
      </table>
    </div>
  </div>
</template>

<style scoped>
.report-overlay {
  position: absolute; /* was fixed — escaped tab container causing ghost overlay */
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}
.report-card {
  background: var(--surface-3);
  border-radius: var(--node-radius);
  border: 1px solid var(--border-default);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
  min-width: 400px;
  max-width: 600px;
  max-height: 80vh;
  overflow-y: auto;
}
.report-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-subtle);
}
.report-header h3 {
  margin: 0;
  font-size: 14px;
  color: var(--text-primary);
}
.close-btn {
  background: none;
  border: none;
  font-size: 16px;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 4px;
}
.close-btn:hover { background: var(--surface-4); }

.report-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.report-table th {
  text-align: left;
  padding: 8px 16px;
  font-size: 11px;
  text-transform: uppercase;
  color: var(--text-muted);
  letter-spacing: 0.5px;
  border-bottom: 1px solid var(--border-subtle);
}
.report-table td {
  padding: 8px 16px;
  border-bottom: 1px solid var(--border-subtle);
  color: var(--text-primary);
}
.report-table tfoot td {
  border-top: 2px solid var(--border-default);
  border-bottom: none;
  padding-top: 10px;
}
.num {
  text-align: right;
  font-family: var(--font-mono);
}
.total {
  color: var(--accent-agent);
  font-weight: 600;
}
.agent-name {
  color: var(--text-primary);
}
</style>
