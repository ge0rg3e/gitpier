<script lang="ts">
	import { page } from '$app/state';
	import { packages, type ContainerPackageDetails } from '$lib/api/client';
	import { timeAgo } from '$lib/utils';
	import { Package, Download, Hash, Copy, Trash2 } from '@lucide/svelte';

	let loading = $state(true);
	let error = $state('');
	let details = $state<ContainerPackageDetails | null>(null);
	let copiedDigest = $state<string | null>(null);

	const username = $derived(page.params.username);
	const imageName = $derived(page.params.image);

	$effect(() => {
		const u = username;
		const i = imageName;
		loading = true;
		error = '';
		details = null;

		packages
			.get(u!, i!)
			.then((res) => {
				if (username !== u || imageName !== i) return;
				details = res;
			})
			.catch((e: any) => {
				if (username !== u || imageName !== i) return;
				error = e.message ?? 'Failed to load versions';
			})
			.finally(() => {
				if (username === u && imageName === i) loading = false;
			});
	});

	async function copyDigest(digest: string) {
		try {
			await navigator.clipboard.writeText(digest);
			copiedDigest = digest;
			setTimeout(() => (copiedDigest = null), 1500);
		} catch {
			/* ignore */
		}
	}

	function shortDigest(digest: string) {
		const hash = digest.includes(':') ? digest.split(':')[1] : digest;
		return hash.slice(0, 12);
	}
</script>

<svelte:head>
	<title>{username}/{imageName} · Versions · GitPier</title>
</svelte:head>

<div class="min-h-screen py-6 px-4">
	<div class="mx-auto max-w-3xl">
		<!-- Header -->
		<div class="mb-5 flex items-center gap-2 flex-wrap">
			<Package class="h-5 w-5 text-muted-foreground" />
			<a href="/{username}/packages/{imageName}" class="text-xl font-semibold text-foreground hover:underline">{imageName}</a>
			<span class="text-muted-foreground">/</span>
			<span class="text-xl font-semibold text-foreground">Versions</span>
		</div>

		{#if loading}
			<div class="rounded-md border border-border bg-card p-8 animate-pulse space-y-3">
				<div class="h-5 w-56 rounded bg-secondary"></div>
				<div class="h-12 w-full rounded bg-secondary"></div>
				<div class="h-12 w-full rounded bg-secondary"></div>
			</div>
		{:else if error}
			<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
		{:else if details}
			<div class="rounded-md border border-border bg-card overflow-hidden">
				<div class="px-4 py-3 border-b border-border flex items-center justify-between">
					<h2 class="text-sm font-semibold text-foreground">All tagged versions ({details.tags_count})</h2>
				</div>
				{#if details.tags.length === 0}
					<div class="px-4 py-10 text-sm text-muted-foreground text-center">No versions published yet.</div>
				{:else}
					<div class="divide-y divide-border">
						{#each details.tags as tag, i}
							<div class="px-4 py-3 flex items-start justify-between gap-4">
								<div class="min-w-0">
									<div class="flex items-center gap-1.5 flex-wrap mb-1">
										{#if i === 0}
											<span class="rounded-full bg-[#1a3a2a] text-[#3fb950] border border-[#2ea043]/50 px-2 py-0.5 text-xs font-medium">latest</span>
										{/if}
										<span class="rounded-full border border-[#30363d] px-2 py-0.5 text-xs font-medium text-[#58a6ff]">{tag.tag}</span>
									</div>
									<div class="text-xs text-muted-foreground flex items-center gap-1.5 flex-wrap">
										<span>Published {timeAgo(tag.updated_at)}</span>
										<span>·</span>
										<span>Digest</span>
										<button onclick={() => copyDigest(tag.digest)} class="font-mono hover:text-foreground transition-colors" title={tag.digest}>{shortDigest(tag.digest)}</button>
										<button onclick={() => copyDigest(tag.digest)} class="hover:text-foreground transition-colors px-1" title="Copy full digest">
											{#if copiedDigest === tag.digest}
												<span class="text-[#3fb950]">✓ Copied</span>
											{:else}
												<Copy class="h-3 w-3 inline" />
											{/if}
										</button>
									</div>
								</div>
								<div class="shrink-0 text-xs text-muted-foreground flex items-center gap-3">
									<span class="flex items-center gap-1"><Download class="h-3.5 w-3.5" />{tag.pull_count ?? 0}</span>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>
</div>
