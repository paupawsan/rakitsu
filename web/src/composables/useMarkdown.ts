import MarkdownIt from 'markdown-it';
import type { RenderRule } from 'markdown-it/lib/renderer.mjs';

// Singleton Markdown renderer for chat messages.
//
// Security: `html: false` makes markdown-it escape any raw HTML in model
// output rather than emit it, and markdown-it's built-in `validateLink`
// rejects `javascript:` / `vbscript:` / `data:` URIs. The rendered string is
// therefore safe to pass to `v-html` without a separate sanitizer (DOMPurify).
const md: MarkdownIt = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
});

// Open links in a new tab, defensively (noopener/noreferrer).
const defaultLinkOpen: RenderRule =
  md.renderer.rules.link_open ??
  ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options));

md.renderer.rules.link_open = (tokens, idx, options, env, self) => {
  const token = tokens[idx];
  if (token) {
    token.attrSet('target', '_blank');
    token.attrSet('rel', 'noopener noreferrer');
  }
  return defaultLinkOpen(tokens, idx, options, env, self);
};

// Wrap every fenced code block with a copy affordance. The button carries no
// handler — ChatPanel delegates clicks on `.md-copy` and reads the rendered
// `<code>` text. The default fence renderer still escapes the code content.
const defaultFence: RenderRule =
  md.renderer.rules.fence ??
  ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options));

md.renderer.rules.fence = (tokens, idx, options, env, self) => {
  const code = defaultFence(tokens, idx, options, env, self);
  return `<div class="md-code"><button class="md-copy" type="button" title="Copy code">Copy</button>${code}</div>`;
};

// renderMarkdown turns a Markdown string into sanitized HTML for `v-html`.
export function renderMarkdown(src: string): string {
  return md.render(src ?? '');
}
