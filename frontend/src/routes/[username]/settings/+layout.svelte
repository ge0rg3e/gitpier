<script lang="ts">
	import { page } from '$app/state';
	import { getContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { type Organization } from '$lib/api/client';
	import OrgSidebar from '$lib/components/org/OrgSidebar.svelte';
	import { Menu, X } from '@lucide/svelte';
	import { fade, fly } from 'svelte/transition';
	import Button from '$lib/components/ui/button/button.svelte';

	let { children } = $props();

	const handle = $derived(page.params.username as string);
	const activeTab = $derived.by(() => {
		const path = page.url.pathname;
		if (path.includes('/settings/teams')) return 'teams' as const;
		if (path.includes('/settings/moderation')) return 'moderation' as const;
		if (path.includes('/settings/installed-apps')) return 'installed-apps' as const;
		if (path.includes('/settings/apps')) return 'apps' as const;
		if (path.includes('/settings/oauth-apps')) return 'oauth-apps' as const;
		return 'settings' as const;
	});

	const ctx = getContext<{
		org: Organization | null;
		isOwner: boolean;
		isMember: boolean;
		loading: boolean;
	}>('org');

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
		if (!ctx.loading && !ctx.isMember) {
			goto(`/${handle}`);
		}
	});

	$effect(() => {
		page.url.pathname;
		closeMobileSidebar();
	});
</script>

<div class="bg-background min-h-screen py-6 px-4">
	<div class="mx-auto max-w-7xl">
		{#if ctx.loading}
			<div class="text-center py-12 text-muted-foreground">Loading…</div>
		{:else}
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
					id="org-settings-sidebar"
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
					<OrgSidebar {handle} active={activeTab} isOwner={ctx.isOwner} mobile />
				</div>
			{/if}

			<div class="flex flex-col gap-6 lg:flex-row lg:gap-8">
				<OrgSidebar {handle} active={activeTab} isOwner={ctx.isOwner} />
				<div class="flex-1 min-w-0">
					{@render children()}
				</div>
			</div>
		{/if}
	</div>
</div>
