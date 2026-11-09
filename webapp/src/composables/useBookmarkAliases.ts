/**
 * Bookmark history URL (alias) management.
 *
 * The generated API client does not expose the history URL endpoints, so this
 * composable talks to the backend directly through fetch. The auth token and
 * API base URL follow the same conventions as the rest of the webapp.
 */
import { ref } from 'vue';
import { getApiBaseUrl } from '@/utils/env-config';

export interface BookmarkHistoryURLsState {
  enabled: boolean;
  loading: boolean;
  error: string | null;
}

function authHeaders(
  extra: Record<string, string> = {}
): Record<string, string> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...extra,
  };
  const token = localStorage.getItem('token');
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  return headers;
}

async function readError(response: Response): Promise<string> {
  try {
    const body = await response.json();
    if (body?.message) {
      return String(body.message);
    }
  } catch {
    // Fall through to the generic message.
  }
  return `Request failed with status ${response.status}`;
}

export function useBookmarkAliases() {
  const enabled = ref(false);
  const loading = ref(false);
  const error = ref<string | null>(null);

  const baseUrl = () => getApiBaseUrl();

  const fetchCapabilities = async (): Promise<boolean> => {
    const response = await fetch(`${baseUrl()}/api/v1/bookmarks/capabilities`, {
      headers: authHeaders(),
    });
    if (!response.ok) {
      enabled.value = false;
      return false;
    }
    const body = await response.json();
    enabled.value = Boolean(body?.historyUrlsEnabled);
    return enabled.value;
  };

  const addAlias = async (bookmarkId: number, url: string): Promise<void> => {
    loading.value = true;
    error.value = null;
    try {
      const response = await fetch(
        `${baseUrl()}/api/v1/bookmarks/${bookmarkId}/aliases`,
        {
          method: 'POST',
          headers: authHeaders(),
          body: JSON.stringify({ url }),
        }
      );
      if (!response.ok) {
        throw new Error(await readError(response));
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : String(err);
      throw err;
    } finally {
      loading.value = false;
    }
  };

  const removeAlias = async (
    bookmarkId: number,
    url: string
  ): Promise<void> => {
    loading.value = true;
    error.value = null;
    try {
      const response = await fetch(
        `${baseUrl()}/api/v1/bookmarks/${bookmarkId}/aliases`,
        {
          method: 'DELETE',
          headers: authHeaders(),
          body: JSON.stringify({ url }),
        }
      );
      if (!response.ok) {
        throw new Error(await readError(response));
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : String(err);
      throw err;
    } finally {
      loading.value = false;
    }
  };

  const resolveBookmark = async (url: string): Promise<unknown | null> => {
    const response = await fetch(
      `${baseUrl()}/api/v1/bookmarks/resolve?url=${encodeURIComponent(url)}`,
      { headers: authHeaders() }
    );
    if (response.status === 404) {
      return null;
    }
    if (!response.ok) {
      throw new Error(await readError(response));
    }
    return response.json();
  };

  return {
    enabled,
    loading,
    error,
    fetchCapabilities,
    addAlias,
    removeAlias,
    resolveBookmark,
  };
}
