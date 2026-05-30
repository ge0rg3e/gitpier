<script lang="ts">
	import { browser } from '$app/environment';
	import { X, FlaskConical } from '@lucide/svelte';

	const STORAGE_KEY = 'gitpier.alpha-banner.dismissed.v1';
	let visible = $state(false);

	function dismiss() {
		visible = false;
		if (!browser) return;
		try {
			window.localStorage.setItem(STORAGE_KEY, '1');
		} catch {}
	}

	$effect(() => {
		if (!browser) return;
		try {
			visible = window.localStorage.getItem(STORAGE_KEY) !== '1';
		} catch {
			visible = true;
		}
	});
</script>

{#if visible}
	<div class="relative flex items-center justify-center gap-2 border-b border-amber-500/30 bg-amber-500/15 px-4 py-2 text-sm text-amber-700 dark:text-amber-400">
		<FlaskConical class="h-4 w-4 shrink-0" />
		<span>
			<strong>Alpha release</strong> - GitPier is under active development. You may encounter bugs or missing features.
			<button onclick={() => document.dispatchEvent(new CustomEvent('open-feedback'))} class="ml-1 underline underline-offset-2 hover:text-amber-900 dark:hover:text-amber-200 transition-colors">
				Send feedback
			</button>
		</span>
		<button
			onclick={dismiss}
			aria-label="Dismiss"
			class="absolute right-3 top-1/2 -translate-y-1/2 rounded p-0.5 text-amber-700/60 hover:text-amber-700 dark:text-amber-400/60 dark:hover:text-amber-400 transition-colors"
		>
			<X class="h-3.5 w-3.5" />
		</button>
	</div>
{/if}
