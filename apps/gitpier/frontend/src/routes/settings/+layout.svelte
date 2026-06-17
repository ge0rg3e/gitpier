<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import AccountSettingsSidebar from '$lib/components/settings/AccountSettingsSidebar.svelte';
	import { Menu, X } from '@lucide/svelte';
	import { fade, fly } from 'svelte/transition';
	import Button from '$lib/components/ui/button/button.svelte';

	let { children } = $props();
	let isMobileSidebarOpen = $state(false);

	const closeMobileSidebar = () => {
		isMobileSidebarOpen = false;
	};

	const toggleMobileSidebar = () => {
		isMobileSidebarOpen = !isMobileSidebarOpen;
	};

	const handleMobileSidebarClick = (event: MouseEvent) => {
		const target = event.target as HTMLElement | null;
		if (target?.closest('a')) {
			closeMobileSidebar();
		}
	};

	$effect(() => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
		}
	});

	$effect(() => {
		page.url.pathname;
		closeMobileSidebar();
	});
</script>

<div class="bg-background min-h-screen py-8 px-4">
	<div class="mx-auto max-w-7xl">
		<div class="mb-4 lg:hidden">
			<Button
				onclick={toggleMobileSidebar}
				aria-expanded={isMobileSidebarOpen}
				aria-controls="org-settings-sidebar"
				title={isMobileSidebarOpen ? 'Close settings menu' : 'Open settings menu'}
				aria-label={isMobileSidebarOpen ? 'Close settings menu' : 'Open settings menu'}
				variant="outline"
				size="icon"
			>
				{#if isMobileSidebarOpen}
					<X class="h-4 w-4" />
				{:else}
					<Menu class="h-4 w-4" />
				{/if}
			</Button>
		</div>

		{#if isMobileSidebarOpen}
			<div class="fixed inset-x-0 bottom-0 top-15 z-30 bg-black/50 lg:hidden" onclick={closeMobileSidebar} aria-hidden="true" transition:fade={{ duration: 180 }}></div>
			<div
				id="account-settings-sidebar"
				class="fixed bottom-0 left-0 top-15 z-40 w-70 overflow-y-auto border-r border-border bg-background p-4 lg:hidden"
				onclick={handleMobileSidebarClick}
				transition:fly={{ x: -18, duration: 220, opacity: 0.25 }}
			>
				<div class="mb-4 flex justify-end">
					<button
						type="button"
						onclick={closeMobileSidebar}
						class="inline-flex items-center justify-center rounded-md p-2 text-muted-foreground hover:bg-secondary hover:text-foreground transition-colors"
						aria-label="Close settings menu"
					>
						<X class="h-4 w-4" />
					</button>
				</div>
				<AccountSettingsSidebar activePath={page.url.pathname} mobile />
			</div>
		{/if}

		<div class="flex flex-col gap-6 lg:flex-row lg:gap-8">
			<AccountSettingsSidebar activePath={page.url.pathname} />
			<div class="flex-1 min-w-0">{@render children()}</div>
		</div>
	</div>
</div>
