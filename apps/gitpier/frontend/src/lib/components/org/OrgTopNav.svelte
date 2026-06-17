<script lang="ts">
	import { Book, LayoutGrid, Package, Settings, Users } from '@lucide/svelte';

	type OrgNavTab = 'overview' | 'repos' | 'starred' | 'packages' | 'teams' | 'people' | 'settings';

	let {
		handle,
		active = 'overview',
		isMember = false,
		isOwner = false
	}: {
		handle: string;
		active?: OrgNavTab;
		isMember?: boolean;
		isOwner?: boolean;
	} = $props();
</script>

<div class="border-b border-secondary bg-background">
	<div class="mx-auto max-w-7xl px-4">
		<div class="-mx-4 overflow-x-auto px-4 lg:mx-0 lg:overflow-visible lg:px-0">
			<nav class="-mb-px flex min-w-max gap-0" aria-label="Organization tabs">
				<a
					href="/{handle}"
					class="flex shrink-0 items-center gap-1.5 whitespace-nowrap px-4 py-3 text-sm border-b-2 transition-colors {active === 'overview'
						? 'border-brand text-foreground'
						: 'border-transparent text-muted-foreground hover:text-foreground'}"
				>
					<LayoutGrid class="h-4 w-4" />
					Overview
				</a>
				<a
					href="/{handle}/people"
					class="flex shrink-0 items-center gap-1.5 whitespace-nowrap px-4 py-3 text-sm border-b-2 transition-colors {active === 'people'
						? 'border-brand text-foreground'
						: 'border-transparent text-muted-foreground hover:text-foreground'}"
				>
					<Users class="h-4 w-4" />
					People
				</a>
				<a
					href="/{handle}/packages"
					class="flex shrink-0 items-center gap-1.5 whitespace-nowrap px-4 py-3 text-sm border-b-2 transition-colors {active === 'packages'
						? 'border-brand text-foreground'
						: 'border-transparent text-muted-foreground hover:text-foreground'}"
				>
					<Package class="h-4 w-4" />
					Packages
				</a>
				{#if isOwner}
					<a
						href="/{handle}/settings"
						class="flex shrink-0 items-center gap-1.5 whitespace-nowrap px-4 py-3 text-sm border-b-2 transition-colors {active === 'settings'
							? 'border-brand text-foreground'
							: 'border-transparent text-muted-foreground hover:text-foreground'}"
					>
						<Settings class="h-4 w-4" />
						Settings
					</a>
				{/if}
			</nav>
		</div>
	</div>
</div>
