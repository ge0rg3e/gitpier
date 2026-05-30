<script lang="ts">
	import { browser } from '$app/environment';
	import { onMount } from 'svelte';

	const STORAGE_KEY = 'cookie_consent';

	let visible = $state(false);

	onMount(() => {
		if (!localStorage.getItem(STORAGE_KEY)) {
			visible = true;
		}
	});

	function accept() {
		localStorage.setItem(STORAGE_KEY, 'accepted');
		visible = false;
	}
</script>

{#if visible}
	<div role="dialog" aria-label="Cookie consent" class="fixed bottom-0 left-0 right-0 z-50 border-t border-border bg-card/95 backdrop-blur-sm px-4 py-4 shadow-lg">
		<div class="mx-auto max-w-screen-xl flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
			<p class="text-xs text-muted-foreground leading-relaxed max-w-2xl">
				We use strictly necessary cookies to keep you signed in. No advertising or tracking cookies are used.
			</p>
			<button onclick={accept} class="shrink-0 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-primary-foreground hover:bg-primary/90 transition-colors"> Accept </button>
		</div>
	</div>
{/if}
