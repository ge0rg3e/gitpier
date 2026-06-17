<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	const username = $derived(page.params.username!);
	const repoName = $derived(page.params.repo!);
	const q = $derived(page.url.searchParams.get('q') ?? '');
	const type = $derived(page.url.searchParams.get('type') ?? 'code');

	onMount(() => {
		const params: Record<string, string> = {
			repo: `${username}/${repoName}`,
			type: type === 'files' ? 'files' : 'code'
		};
		if (q) params.q = q;
		goto(`/search?${new URLSearchParams(params)}`, { replaceState: true });
	});
</script>
