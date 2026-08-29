import { ref, type Ref } from 'vue';

export interface TokenItem {
  id: number;
  text: string;
}

export interface TokenizeResult {
  tokens: TokenItem[];
  total: number;
  encoding: string;
}

export function useTokenizer(baseUrl: Ref<string>) {
  const loading = ref(false);
  const error = ref<string | null>(null);

  function apiUrl(path: string): string {
    const base = baseUrl.value.startsWith('http') ? baseUrl.value : `http://${baseUrl.value}`;
    return `${base.replace(/\/+$/, '')}${path}`;
  }

  async function tokenize(text: string, model: string): Promise<TokenizeResult | null> {
    loading.value = true;
    error.value = null;
    try {
      const res = await fetch(apiUrl('/api/tokenize'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text, model }),
      });
      if (!res.ok) {
        error.value = `Tokenize failed: ${res.status}`;
        return null;
      }
      const data = await res.json();
      if (data.error) {
        error.value = data.error;
        return null;
      }
      return data as TokenizeResult;
    } catch (e) {
      error.value = String(e);
      return null;
    } finally {
      loading.value = false;
    }
  }

  return { tokenize, loading, error };
}
