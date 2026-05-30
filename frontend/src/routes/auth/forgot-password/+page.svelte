<script lang="ts">
	import { auth } from '$lib/api/client';
	import { Loader } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	let email = $state('');
	let loading = $state(false);
	let error = $state('');
	let submitted = $state(false);

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (loading) return;
		error = '';
		loading = true;
		try {
			await auth.forgotPassword(email);
			submitted = true;
		} catch (e: any) {
			error = e.message ?? 'Something went wrong. Please try again.';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Forgot password — GitPier</title>
</svelte:head>

<div class="bg-background min-h-screen flex flex-col items-center justify-center px-4 py-12">
	<a href="/" class="mb-6">
		<img src="/images/logo.png" alt="GitPier" class="h-12 w-12 object-contain" />
	</a>

	<div class="w-full max-w-sm">
		<div class="rounded-md border border-border bg-card px-6 py-6 mb-4">
			<h1 class="text-xl font-semibold text-foreground mb-2 text-center">Reset your password</h1>

			{#if submitted}
				<p class="text-sm text-muted-foreground text-center mb-4">
					If an account with that email exists, a password reset token has been generated. Check the server console for the token, then use it on the
					<a href="/auth/reset-password" class="text-primary hover:underline">reset password</a> page.
				</p>
				<Button
					variant="outline"
					class="w-full"
					onclick={() => {
						submitted = false;
						email = '';
					}}>Send again</Button
				>
			{:else}
				<p class="text-sm text-muted-foreground mb-5 text-center">Enter your account email address and we'll generate a reset token.</p>

				{#if error}
					<div class="mb-4 rounded-md border border-red-800/50 bg-red-900/30 px-4 py-3 text-sm text-red-400">{error}</div>
				{/if}

				<form onsubmit={handleSubmit} class="space-y-4">
					<div>
						<label for="email" class="block text-sm font-semibold text-foreground mb-1.5">Email address</label>
						<input
							id="email"
							type="email"
							bind:value={email}
							required
							autofocus
							autocomplete="email"
							class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary transition-all"
						/>
					</div>
					<Button variant="brand" type="submit" class="w-full" disabled={loading || !email}>
						{#if loading}<Loader class="h-4 w-4 animate-spin" />{/if}
						Send reset token
					</Button>
				</form>
			{/if}
		</div>

		<div class="rounded-md border border-border bg-card px-6 py-4 text-center text-sm text-muted-foreground">
			Remember your password?
			<a href="/login" class="font-semibold text-primary hover:underline ml-1">Sign in</a>
		</div>
	</div>
</div>
