<script lang="ts">
	import './layout.css';
	import Navbar from '$lib/components/Navbar.svelte';
	import AlphaBanner from '$lib/components/AlphaBanner.svelte';
	import { authStore } from '$lib/stores/auth.svelte';
	import { onMount } from 'svelte';

	let { children } = $props();

	onMount(() => {
		authStore.init();

		if ('serviceWorker' in navigator && window.isSecureContext) {
			navigator.serviceWorker.register('/sw.js').catch((error) => {
				console.error('Service worker registration failed:', error);
			});
		}
	});
</script>

<svelte:head>
	<link rel="icon" href="/images/logo.png" />
	<link rel="manifest" href="/manifest.webmanifest" />
	<link rel="apple-touch-icon" href="/icons/apple-touch-icon.png" />
	<meta name="theme-color" content="#0f172a" />
</svelte:head>

<div class="min-h-screen flex flex-col bg-background">
	<Navbar />
	<AlphaBanner />
	<main class="flex-1">
		{@render children()}
	</main>

	<footer class="border-t border-secondary bg-background py-6">
		<div class="mx-auto max-w-screen-xl px-4">
			<div class="flex flex-col gap-4 text-xs text-muted-foreground">
				<div class="flex flex-col sm:flex-row items-center justify-between gap-4">
					<div class="flex items-center gap-1">
						<img src="/images/logo.png" alt="GitPier" class="h-5 w-5 object-contain opacity-70" />
						<span>© 2026 GitPier - All rights reserved.</span>
					</div>
				</div>
			</div>
		</div>
	</footer>
</div>
