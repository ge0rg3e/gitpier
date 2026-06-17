<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { webhooks, type Webhook } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { Plus, Webhook as WebhookIcon, CheckCircle, XCircle, Trash2, ChevronRight } from '@lucide/svelte';

	const username = $derived(page.params.username as string);
	const repoName = $derived(page.params.repo as string);

	let hookList = $state<Webhook[]>([]);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}
		try {
			const data = await webhooks.list(username, repoName);
			hookList = data.webhooks ?? [];
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	async function deleteHook(id: number) {
		if (!confirm('Delete this webhook?')) return;
		try {
			await webhooks.delete(username, repoName, id);
			hookList = hookList.filter((h) => h.id !== id);
		} catch (e: any) {
			alert(e.message);
		}
	}
</script>

<div class="container mx-auto max-w-4xl px-4 py-8">
	<!-- Header -->
	<div class="mb-6 flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold">Webhooks</h1>
			<p class="text-muted-foreground mt-1 text-sm">
				Webhooks allow external services to be notified when certain events happen. When the specified events occur, we'll send a POST request to each of the URLs you provide.
			</p>
		</div>
		<Button href={`/${username}/${repoName}/settings/webhooks/new`}>
			<Plus class="mr-2 h-4 w-4" />
			Add webhook
		</Button>
	</div>

	{#if loading}
		<div class="text-muted-foreground py-12 text-center text-sm">Loading webhooks…</div>
	{:else if error}
		<div class="text-destructive py-12 text-center text-sm">{error}</div>
	{:else if hookList.length === 0}
		<div class="rounded-lg border border-dashed p-12 text-center">
			<WebhookIcon class="text-muted-foreground mx-auto mb-4 h-12 w-12" />
			<h3 class="mb-2 font-semibold">No webhooks</h3>
			<p class="text-muted-foreground mb-4 text-sm">Add a webhook to receive HTTP POST payloads for events in this repository.</p>
			<Button href={`/${username}/${repoName}/settings/webhooks/new`}>
				<Plus class="mr-2 h-4 w-4" />
				Add webhook
			</Button>
		</div>
	{:else}
		<div class="divide-y rounded-lg border">
			{#each hookList as hook (hook.id)}
				<div class="flex items-center gap-4 p-4">
					<!-- Status indicator -->
					{#if hook.active}
						<CheckCircle class="h-5 w-5 shrink-0 text-green-500" />
					{:else}
						<XCircle class="text-muted-foreground h-5 w-5 shrink-0" />
					{/if}

					<!-- Payload URL + events -->
					<a href={`/${username}/${repoName}/settings/webhooks/${hook.id}`} class="hover:text-primary min-w-0 flex-1 truncate font-mono text-sm font-medium">
						{hook.payload_url}
					</a>

					<!-- Actions -->
					<div class="flex shrink-0 items-center gap-2">
						<Button variant="ghost" size="sm" href={`/${username}/${repoName}/settings/webhooks/${hook.id}`}>
							<ChevronRight class="h-4 w-4" />
						</Button>
						<Button variant="ghost" size="sm" onclick={() => deleteHook(hook.id)} class="text-destructive hover:text-destructive">
							<Trash2 class="h-4 w-4" />
						</Button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
