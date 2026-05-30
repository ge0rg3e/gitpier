<script lang="ts">
	import { Users, Settings, Shield, AppWindow, Boxes, Download } from '@lucide/svelte';

	type OrgSidebarTab = 'overview' | 'teams' | 'settings' | 'moderation' | 'oauth-apps' | 'apps' | 'installed-apps';

	let {
		handle,
		active = 'overview',
		isOwner = false,
		mobile = false
	}: {
		handle: string;
		active?: OrgSidebarTab;
		isOwner?: boolean;
		mobile?: boolean;
	} = $props();
</script>

<aside class={`w-65 shrink-0 ${mobile ? 'block' : 'hidden lg:block'}`}>
	<div class="space-y-6">
		<section>
			<h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2 px-3">Access</h2>
			<nav class="space-y-0.5">
				<a
					href="/{handle}/settings/teams"
					class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${active === 'teams' ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'}`}
				>
					<Users class="h-3.5 w-3.5" />
					Teams
				</a>
			</nav>
		</section>

		{#if isOwner}
			<section>
				<h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2 px-3">General</h2>
				<nav class="space-y-0.5">
					<a
						href="/{handle}/settings"
						class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
							active === 'settings' ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
						}`}
					>
						<Settings class="h-3.5 w-3.5" />
						General
					</a>
				</nav>
			</section>

			<section>
				<h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2 px-3">Code and automation</h2>
				<nav class="space-y-0.5">
					<a
						href="/{handle}/settings/oauth-apps"
						class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
							active === 'oauth-apps' ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
						}`}
					>
						<AppWindow class="h-3.5 w-3.5" />
						OAuth Apps
					</a>
					<a
						href="/{handle}/settings/apps"
						class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
							active === 'apps' ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
						}`}
					>
						<Boxes class="h-3.5 w-3.5" />
						GitPier Apps
					</a>
					<a
						href="/{handle}/settings/installed-apps"
						class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
							active === 'installed-apps' ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
						}`}
					>
						<Download class="h-3.5 w-3.5" />
						Installed Apps
					</a>
				</nav>
			</section>

			<section>
				<h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2 px-3">Security and quality</h2>
				<nav class="space-y-0.5">
					<a
						href="/{handle}/settings/moderation"
						class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
							active === 'moderation' ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
						}`}
					>
						<Shield class="h-3.5 w-3.5" />
						Moderation
					</a>
				</nav>
			</section>
		{/if}
	</div>
</aside>
