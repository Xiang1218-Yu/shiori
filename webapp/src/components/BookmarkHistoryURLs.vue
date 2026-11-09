<template>
  <div
    v-if="aliasesEnabled"
    class="mt-4 border-t border-gray-200 dark:border-gray-700 pt-4"
  >
    <h2 class="text-sm font-semibold text-gray-900 dark:text-white mb-1">
      {{ t('bookmarks.history_urls') }}
    </h2>
    <p class="text-xs text-gray-500 dark:text-gray-400 mb-3">
      {{ t('bookmarks.history_urls_description') }}
    </p>

    <ul v-if="historyUrls.length" class="space-y-2 mb-3">
      <li
        v-for="url in historyUrls"
        :key="url"
        class="flex items-center gap-2 text-sm"
      >
        <span class="text-gray-500 dark:text-gray-400">~</span>
        <a
          :href="url"
          target="_blank"
          rel="noopener noreferrer"
          class="flex-1 truncate text-red-600 dark:text-red-400 hover:underline"
          :title="url"
        >
          {{ url }}
        </a>
        <Button
          v-if="isAdmin"
          variant="ghost"
          size="sm"
          :disabled="loading"
          :title="t('bookmarks.delete_bookmark')"
          @click="onRemove(url)"
        >
          <TrashIcon class="h-4 w-4" />
        </Button>
      </li>
    </ul>

    <div v-if="isAdmin" class="flex items-center gap-2">
      <Input
        v-model="newAlias"
        type="url"
        size="sm"
        class="flex-1"
        :placeholder="t('bookmarks.history_url_placeholder')"
        :disabled="loading"
        @keyup.enter="onAdd"
      />
      <Button
        variant="primary"
        size="sm"
        :disabled="loading || !newAlias.trim()"
        @click="onAdd"
      >
        {{ t('bookmarks.add_history_url') }}
      </Button>
    </div>

    <p v-if="error" class="mt-2 text-sm text-red-600 dark:text-red-400">
      {{ error }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import Button from '@/components/ui/Button.vue';
import Input from '@/components/ui/Input.vue';
import { TrashIcon } from '@/components/icons';
import { useBookmarkAliases } from '@/composables/useBookmarkAliases';
import { useAuthStore } from '@/stores/auth';

interface Props {
  bookmarkId: number;
  historyUrls?: string[];
}

const props = withDefaults(defineProps<Props>(), {
  historyUrls: () => [],
});

const emit = defineEmits<{
  changed: [];
}>();

const { t } = useI18n();
const authStore = useAuthStore();
const { enabled, loading, error, fetchCapabilities, addAlias, removeAlias } =
  useBookmarkAliases();

const aliasesEnabled = enabled;
const isAdmin = ref(false);
const newAlias = ref('');

onMounted(async () => {
  isAdmin.value = Boolean(authStore.user?.owner);
  await fetchCapabilities();
});

const onAdd = async () => {
  const url = newAlias.value.trim();
  if (!url) {
    return;
  }
  try {
    await addAlias(props.bookmarkId, url);
    newAlias.value = '';
    emit('changed');
  } catch {
    // The composable already exposes the message through `error`.
  }
};

const onRemove = async (url: string) => {
  try {
    await removeAlias(props.bookmarkId, url);
    emit('changed');
  } catch {
    // The composable already exposes the message through `error`.
  }
};
</script>
