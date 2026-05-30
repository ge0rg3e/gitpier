<script lang="ts">
	import { UserRound, KeyRound, ShieldCheck, Shield, AppWindow, Puzzle, Smartphone, Palette, MonitorSmartphone } from '@lucide/svelte';

	let {
		activePath,
		mobile = false
	}: {
		activePath: string;
		mobile?: boolean;
	} = $props();

	const isActive = (matchers: Array<string | RegExp>) => matchers.some((matcher) => (typeof matcher === 'string' ? activePath === matcher : matcher.test(activePath)));
</script>

<aside class={`w-65 shrink-0 ${mobile ? 'block' : 'hidden lg:block'}`}>
	<div class="space-y-6">
		<section>
			<h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2 px-3">General</h2>
			<nav class="space-y-0.5">
				<a
					href="/settings/profile"
					class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
						isActive(['/settings/profile']) ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
					}`}
				>
					<UserRound class="h-3.5 w-3.5" />
					Profile
				</a>
			</nav>
		</section>

		<section>
			<h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2 px-3">Access</h2>
			<nav class="space-y-0.5">
				<a
					href="/settings/security"
					class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
						isActive(['/settings/security']) ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
					}`}
				>
					<Smartphone class="h-3.5 w-3.5" />
					Password and authentication
				</a>
				<a
					href="/settings/sessions"
					class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
						isActive(['/settings/sessions']) ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
					}`}
				>
					<MonitorSmartphone class="h-3.5 w-3.5" />
					Sessions
				</a>
				<a
					href="/settings/keys"
					class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
						isActive(['/settings/keys']) ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
					}`}
				>
					<KeyRound class="h-3.5 w-3.5" />
					SSH and GPG keys
				</a>
				<a
					href="/settings/privacy"
					class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
						isActive(['/settings/privacy']) ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
					}`}
				>
					<ShieldCheck class="h-3.5 w-3.5" />
					Privacy and data
				</a>
				<a
					href="/settings/applications"
					class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
						isActive(['/settings/applications']) ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
					}`}
				>
					<AppWindow class="h-3.5 w-3.5" />
					Authorized apps
				</a>
			</nav>
		</section>

		<section>
			<h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2 px-3">Code and automation</h2>
			<nav class="space-y-0.5">
				<a
					href="/settings/developer-settings/apps"
					class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
						isActive([/^\/settings\/developer-settings\/apps/]) ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
					}`}
				>
					<Puzzle class="h-3.5 w-3.5" />
					GitPier Apps
				</a>
				<a
					href="/settings/developer-settings/oauth-apps"
					class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
						isActive([/^\/settings\/developer-settings\/oauth-apps$/, /^\/settings\/developer-settings\/oauth-apps\/new$/, /^\/settings\/developer-settings\/oauth-apps\/\d+$/])
							? 'font-semibold bg-brand text-white'
							: 'text-foreground hover:bg-secondary transition-colors'
					}`}
				>
					<AppWindow class="h-3.5 w-3.5" />
					OAuth Apps
				</a>
			</nav>
		</section>

		<section>
			<h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2 px-3">Security and quality</h2>
			<nav class="space-y-0.5">
				<a
					href="/settings/moderation"
					class={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm ${
						isActive(['/settings/moderation']) ? 'font-semibold bg-brand text-white' : 'text-foreground hover:bg-secondary transition-colors'
					}`}
				>
					<Shield class="h-3.5 w-3.5" />
					Moderation
				</a>
			</nav>
		</section>

	</div>
</aside>
