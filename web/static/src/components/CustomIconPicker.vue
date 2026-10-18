<template>
  <div class="form-control">
    <label class="label" for="install-custom-icon">
      <span class="label-text">{{ $t('install.form.custom_icon.label') }}</span>
    </label>
    <input id="install-custom-icon" type="file" accept="image/png,image/jpeg"
      class="file-input file-input-bordered w-full" :disabled="disabled" @change="selectIcon" />
    <div v-if="modelValue" class="flex items-center gap-3 mt-3">
      <img :src="modelValue" :alt="$t('install.form.custom_icon.preview')"
        class="rounded object-cover" :style="{ width: isTv ? '100px' : '72px', height: isTv ? '60px' : '72px', background: '#000' }" />
      <button type="button" class="btn btn-sm btn-ghost" :disabled="disabled" @click="clearIcon">
        {{ $t('install.form.custom_icon.reset') }}
      </button>
    </div>
    <details class="mt-3 rounded border border-base-300 p-3">
      <summary class="cursor-pointer text-sm font-medium">{{ $t('install.form.custom_icon.apple_search') }}</summary>
      <div class="flex flex-col gap-2 mt-3">
        <label class="text-sm" for="apple-icon-term">{{ $t('install.form.custom_icon.app_name') }}</label>
        <input id="apple-icon-term" v-model="query" type="search" maxlength="200"
          class="input input-bordered w-full" :disabled="disabled" :placeholder="$t('install.form.custom_icon.search_placeholder')"
          @keydown.enter.prevent="searchApple" @input="invalidateSearch" />
        <div class="flex items-end gap-2">
          <div>
            <label class="block text-sm mb-1" for="apple-icon-country">{{ $t('install.form.custom_icon.country') }}</label>
            <input id="apple-icon-country" v-model="country" class="input input-bordered w-24 uppercase" maxlength="2"
              :disabled="disabled" @input="invalidateSearch" @keydown.enter.prevent="searchApple" />
          </div>
          <button type="button" class="btn btn-sm" :disabled="disabled || searching || query.trim().length < 2" @click="searchApple">
            <span v-if="searching" class="loading loading-spinner loading-xs"></span>
            {{ $t('install.form.custom_icon.search') }}
          </button>
        </div>
        <p class="text-xs opacity-70">{{ $t(isTv ? 'install.form.custom_icon.tv_search_hint' : 'install.form.custom_icon.search_hint') }}</p>
        <p v-if="searched && !searching && !results.length" class="text-sm" role="status">{{ $t('install.form.custom_icon.no_results') }}</p>
        <div v-if="results.length" class="grid grid-cols-2 sm:grid-cols-3 gap-2 max-h-80 overflow-y-auto" :aria-label="$t('install.form.custom_icon.results')">
          <div v-for="app in results" :key="app.id" class="rounded border border-base-300 p-2 flex flex-col gap-1">
            <button type="button" class="text-left flex flex-col gap-1" :disabled="disabled || selecting" @click="chooseAppleIcon(app)">
              <img :src="app.artwork_url" :alt="app.name" loading="lazy" referrerpolicy="no-referrer"
                class="w-full rounded object-contain bg-black" :style="{ aspectRatio: isTv ? '5 / 3' : '1' }" />
              <span class="text-sm font-medium break-words">{{ app.name }}</span>
              <span class="text-xs opacity-70 break-words">{{ app.artist }}</span>
              <span class="text-xs text-primary">{{ $t(selecting === app.id ? 'install.form.custom_icon.downloading' : 'install.form.custom_icon.use_icon') }}</span>
            </button>
            <a :href="app.store_url" target="_blank" rel="noopener noreferrer" class="text-xs link">{{ $t('install.form.custom_icon.view_store') }}</a>
          </div>
        </div>
      </div>
    </details>
    <p class="label-text-alt mt-2">{{ $t('install.form.custom_icon.tips') }}</p>
    <p v-if="error" class="text-error text-sm mt-2" role="alert">{{ error }}</p>
  </div>
</template>

<script>
import api from "@/api/api";
export default {
  props: {
    modelValue: { type: String, default: '' },
    disabled: Boolean,
    isTv: Boolean,
  },
  emits: ['update:modelValue', 'busy'],
  data: () => ({ error: '', sequence: 0, query: '', country: 'US', results: [], searching: false, searched: false, searchSequence: 0, selecting: 0 }),
  watch: { isTv() { this.invalidateSearch(); } },
  beforeUnmount() { this.sequence++; this.searchSequence++; },
  methods: {
    invalidateSearch() {
      this.searchSequence++;
      this.searching = false;
      this.searched = false;
      this.results = [];
    },
    async searchApple() {
      if (this.disabled || this.query.trim().length < 2) return;
      const sequence = ++this.searchSequence;
      this.searching = true;
      this.searched = false;
      this.error = '';
      this.results = [];
      try {
        const response = await api.searchAppleIcons({ term: this.query.trim(), country: this.country.trim(), platform: this.isTv ? 'tvos' : 'ios' });
        if (sequence === this.searchSequence) {
          this.results = response.data || [];
          this.searched = true;
        }
      } catch (error) {
        if (sequence === this.searchSequence) this.error = error.message || this.$t('install.form.custom_icon.search_failed');
      } finally {
        if (sequence === this.searchSequence) this.searching = false;
      }
    },
    async chooseAppleIcon(app) {
      if (this.disabled || this.selecting) return;
      const sequence = ++this.sequence;
      this.selecting = app.id;
      this.error = '';
      this.$emit('busy', true);
      try {
        const response = await api.downloadAppleIcon(app.artwork_url);
        if (sequence === this.sequence) this.$emit('update:modelValue', response.data);
      } catch (error) {
        if (sequence === this.sequence) this.error = error.message || this.$t('install.form.custom_icon.search_failed');
      } finally {
        if (sequence === this.sequence) {
          this.selecting = 0;
          this.$emit('busy', false);
        }
      }
    },
    clearIcon() {
      this.sequence++;
      this.error = '';
      this.selecting = 0;
      this.$el.querySelector('input').value = '';
      this.$emit('update:modelValue', '');
      this.$emit('busy', false);
    },
    async selectIcon(event) {
      const sequence = ++this.sequence;
      this.selecting = 0;
      const file = event.target.files?.[0];
      this.error = '';
      if (!file) { this.$emit('busy', false); return; }
      this.$emit('busy', true);
      try {
        if (!['image/png', 'image/jpeg'].includes(file.type) || file.size > 4 * 1024 * 1024) {
          throw new Error(this.$t('install.form.custom_icon.invalid'));
        }
        const value = await new Promise((resolve, reject) => {
          const reader = new FileReader();
          reader.onload = () => resolve(reader.result);
          reader.onerror = () => reject(new Error(this.$t('install.form.custom_icon.invalid')));
          reader.readAsDataURL(file);
        });
        const image = new Image();
        image.src = value;
        await image.decode();
        if (image.naturalWidth > 4096 || image.naturalHeight > 4096) {
          throw new Error(this.$t('install.form.custom_icon.invalid'));
        }
        if (sequence === this.sequence) this.$emit('update:modelValue', value);
      } catch (error) {
        if (sequence === this.sequence) {
          this.error = error.message || this.$t('install.form.custom_icon.invalid');
          event.target.value = '';
        }
      } finally {
        if (sequence === this.sequence) this.$emit('busy', false);
      }
    },
  },
};
</script>
