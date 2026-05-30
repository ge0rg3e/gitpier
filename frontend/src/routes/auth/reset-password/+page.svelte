<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { auth } from '$lib/api/client';
	import { Eye, EyeOff, Loader } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	// Pre-fill the token from the URL query param ?token=...
	let token = $state(page.url.searchParams.get('token') ?? '');
	let newPassword = $state('');
	let confirmPassword = $state('');
	let showPassword = $state(false);
	let loading = $state(false);
	let error = $state('');
	let success = $state('');

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (loading) return;
		error = '';

		if (!token.trim()) {
			error = 'Reset token is required.';
			return;
		}
		if (newPassword.length < 8) {
			error = 'New password must be at least 8 characters.';
			return;
		}
		if (newPassword !== confirmPassword) {
			error = 'Passwords do not match.';
			return;
		}

		loading = true;
		try {
			await auth.resetPassword(token.trim(), newPassword);
			success = 'Password reset successfully. Redirecting to sign in…';
			setTimeout(() => goto('/login'), 2000);
		} catch (e: any) {
			error = e.message ?? 'Failed to reset password. The token may be invalid or expired.';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Reset password — GitPier</title>
</svelte:head>

<div class="bg-background min-h-screen flex flex-col items-center justify-center px-4 py-12">
	<a href="/" class="mb-6">
		<img src="/images/logo.png" alt="GitPier" class="h-12 w-12 object-contain" />
	</a>

	<div class="w-full max-w-sm">
		<div class="rounded-md border border-border bg-card px-6 py-6 mb-4">
			<h1 class="text-xl font-semibold text-foreground mb-5 text-center">Set a new password</h1>

			{#if success}
				<div class="rounded-md border border-brand/40 bg-brand/10 px-4 py-3 text-sm text-[#3fb950]">{success}</div>
			{:else}
				{#if error}
					<div class="mb-4 rounded-md border border-red-800/50 bg-red-900/30 px-4 py-3 text-sm text-red-400">{error}</div>
				{/if}

				<form onsubmit={handleSubmit} class="space-y-4">
					<div>
						<label for="reset-token" class="block text-sm font-semibold text-foreground mb-1.5">Reset token</label>
						<input
							id="reset-token"
							type="text"
							bind:value={token}
							required
							autocomplete="off"
							placeholder="Paste the token from the server console"
							class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground font-mono placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary transition-all"
						/>
					</div>

					<div>
						<label for="new-password" class="block text-sm font-semibold text-foreground mb-1.5">New password</label>
						<div class="relative">
							<input
								id="new-password"
								type={showPassword ? 'text' : 'password'}
								bind:value={newPassword}
								required
								autocomplete="new-password"
								placeholder="At least 8 characters"
								class="h-9 w-full rounded-md border border-border bg-background px-3 pr-9 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary transition-all"
							/>
							<button
								type="button"
								onclick={() => (showPassword = !showPassword)}
								class="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
							>
								{#if showPassword}<EyeOff class="h-4 w-4" />{:else}<Eye class="h-4 w-4" />{/if}
							</button>
						</div>
					</div>

					<div>
						<label for="confirm-password" class="block text-sm font-semibold text-foreground mb-1.5">Confirm new password</label>
						<input
							id="confirm-password"
							type="password"
							bind:value={confirmPassword}
							required
							autocomplete="new-password"
							class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary transition-all"
						/>
					</div>

					<Button variant="brand" type="submit" class="w-full" disabled={loading || !token || !newPassword || !confirmPassword}>
						{#if loading}<Loader class="h-4 w-4 animate-spin" />{/if}
						Reset password
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
