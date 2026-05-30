<script lang="ts">
	import { Settings, Users, Boxes, Webhook, Shield } from '@lucide/svelte';

	let {
		handle,
		repo,
		activePath,
		mobile = false
	}: {
		handle: string;
		repo: string;
		activePath: string;
		mobile?: boolean;
	} = $props();

	const base = $derived.by(() => `/${handle}/${repo}/settings`);
	const isActive = (matchers: Array<string | RegExp>) => matchers.some((matcher) => (typeof matcher === 'string' ? activePath === matcher : matcher.test(activePath)));
</script>

<aside class={`w-65 shrink-0 ${mobile ? 'block' : 'hidden lg:block'}`}>
	<h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2 px-3">General</h2>
	<nav class="space-y-0.5 mb-4">
		<a
			href={base}
			class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${isActive([base]) ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'}`}
		>
			<Settings class="h-3.5 w-3.5" />
			General
		</a>
		<a
			href={`${base}/collaborators`}
			class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
				isActive([`${base}/collaborators`]) ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
			}`}
		>
			<Users class="h-3.5 w-3.5" />
			Collaborators
		</a>
	</nav>

	<h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2 px-3">Code and automation</h2>
	<nav class="space-y-0.5 mb-4">
		<a
			href={`${base}/environments`}
			class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
				isActive([`${base}/environments`]) ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
			}`}
		>
			<Boxes class="h-3.5 w-3.5" />
			Environments
		</a>
		<a
			href={`${base}/webhooks`}
			class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
				isActive([/^.+\/settings\/webhooks(?:\/.*)?$/]) ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
			}`}
		>
			<Webhook class="h-3.5 w-3.5" />
			Webhooks
		</a>
	</nav>

	<h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2 px-3">Security and quality</h2>
	<nav class="space-y-0.5">
		<a
			href={`${base}/moderation`}
			class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
				isActive([`${base}/moderation`]) ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
			}`}
		>
			<Shield class="h-3.5 w-3.5" />
			Moderation
		</a>
	</nav>
</aside>
