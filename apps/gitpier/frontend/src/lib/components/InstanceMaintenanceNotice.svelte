<script lang="ts">
	import { ExternalLink, Wrench } from '@lucide/svelte';

	type Props = {
		selfHostURL: string;
	};

	let { selfHostURL }: Props = $props();
</script>

<div class="rounded-lg border border-amber-800/40 bg-amber-950/20 p-6">
	<div class="flex items-start gap-3">
		<div class="rounded-md border border-amber-800/50 bg-amber-900/30 p-2">
			<Wrench class="h-4 w-4 text-amber-300" />
		</div>
		<div>
			<h2 class="text-lg font-semibold text-foreground">Maintenance-only instance</h2>
			<p class="mt-2 text-sm text-muted-foreground">Unfortunately, due to limited budget, this hosted instance is used only for maintaining the GitPier codebase.</p>
			<p class="mt-2 text-sm text-muted-foreground">To create your own repositories and organizations, please self-host GitPier.</p>
			<a href={selfHostURL} target="_blank" rel="noreferrer" class="mt-4 inline-flex items-center gap-1.5 text-sm font-semibold text-primary hover:underline">
				Click here
				<ExternalLink class="h-3.5 w-3.5" />
			</a>
		</div>
	</div>
</div>
