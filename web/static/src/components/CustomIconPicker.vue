<template>
  <div class="form-control">
    <label class="label" for="install-custom-icon">
      <span class="label-text">{{ $t('install.form.custom_icon.label') }}</span>
    </label>
    <input id="install-custom-icon" type="file" accept="image/png,image/jpeg"
      class="file-input file-input-bordered w-full" :disabled="disabled" @change="selectIcon" />
    <p class="label-text-alt mt-2">{{ $t('install.form.custom_icon.tips') }}</p>
    <div v-if="modelValue" class="flex items-center gap-3 mt-3">
      <img :src="modelValue" :alt="$t('install.form.custom_icon.preview')"
        class="rounded object-cover" :style="{ width: isTv ? '100px' : '72px', height: isTv ? '60px' : '72px', background: '#000' }" />
      <button type="button" class="btn btn-sm btn-ghost" :disabled="disabled" @click="clearIcon">
        {{ $t('install.form.custom_icon.reset') }}
      </button>
    </div>
    <p v-if="error" class="text-error text-sm mt-2" role="alert">{{ error }}</p>
  </div>
</template>

<script>
export default {
  props: {
    modelValue: { type: String, default: '' },
    disabled: Boolean,
    isTv: Boolean,
  },
  emits: ['update:modelValue', 'busy'],
  data: () => ({ error: '', sequence: 0 }),
  beforeUnmount() { this.sequence++; },
  methods: {
    clearIcon() {
      this.sequence++;
      this.error = '';
      this.$el.querySelector('input').value = '';
      this.$emit('update:modelValue', '');
      this.$emit('busy', false);
    },
    async selectIcon(event) {
      const sequence = ++this.sequence;
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
