<script context="module">
	// Type declarations for Turnstile
	declare global {
		interface Window {
			turnstile?: {
				render: (
					container: HTMLElement | string,
					options: {
						sitekey: string;
						theme?: 'light' | 'dark';
						callback?: (token: string) => void;
					}
				) => string;
				reset: (container?: HTMLElement | string) => void;
				remove: (container?: HTMLElement | string) => void;
				getResponse: (container?: HTMLElement | string) => string | undefined;
			};
		}
	}
</script>

<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { env } from '$env/dynamic/public';
	import { authStore } from '$lib/stores/auth.svelte';
	import { auth } from '$lib/api/client';
	import { Eye, EyeOff, Loader } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { onMount } from 'svelte';

	let email = $state('');
	let password = $state('');
	let showPassword = $state(false);
	let challengeToken = $state('');
	let twoFactorCode = $state('');
	let recoveryCode = $state('');
	let useRecoveryCode = $state(false);
	let loading = $state(false);
	let error = $state('');
	let turnstileToken = $state('');
	let turnstileContainerRef = $state<HTMLDivElement | null>(null);
	let turnstileReady = $state(false);
	let turnstileSiteKey = env.PUBLIC_TURNSTILE_SITE_KEY || '';

	function getAuthenticatedRedirectTarget() {
		const redirectParam = page.url.searchParams.get('redirect')?.trim() ?? '';
		const returnToParam = page.url.searchParams.get('return_to')?.trim() ?? '';
		const candidate = redirectParam || returnToParam || '/';

		try {
			const parsed = new URL(candidate, page.url.origin);
			if (parsed.origin !== page.url.origin) return '/';
			if (parsed.pathname === '/login' || parsed.pathname === '/signup') return '/';
			return `${parsed.pathname}${parsed.search}${parsed.hash}` || '/';
		} catch {
			return '/';
		}
	}

	function initTurnstile() {
		if (!turnstileContainerRef || !window.turnstile || !turnstileSiteKey) return;
		window.turnstile.render(turnstileContainerRef, {
			sitekey: turnstileSiteKey,
			theme: document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light',
			callback: (token: string) => {
				turnstileToken = token;
			}
		});
		turnstileReady = true;
	}

	onMount(() => {
		// Load Turnstile script
		if (!window.turnstile && turnstileSiteKey) {
			const script = document.createElement('script');
			script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js';
			script.async = true;
			script.defer = true;
			script.onload = initTurnstile;
			document.head.appendChild(script);
		} else if (turnstileSiteKey) {
			initTurnstile();
		}

		return () => {
			// Cleanup: reset Turnstile when component unmounts
			if (turnstileContainerRef && window.turnstile) {
				try {
					window.turnstile.reset(turnstileContainerRef);
				} catch (e) {
					// Ignore errors during cleanup
				}
			}
		};
	});

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (loading) return;
		error = '';
		loading = true;
		try {
			if (!challengeToken) {
				if (turnstileSiteKey && !turnstileToken && turnstileContainerRef && window.turnstile) {
					turnstileToken = window.turnstile.getResponse(turnstileContainerRef) ?? '';
				}
				if (turnstileSiteKey && !turnstileToken) {
					error = 'Please complete the CAPTCHA verification.';
					return;
				}
				const result = await auth.login({ email, password, turnstile_token: turnstileToken });
				if (result.requires_2fa && result.two_factor_challenge_token) {
					challengeToken = result.two_factor_challenge_token;
					password = '';
					turnstileToken = '';
					// Reset Turnstile after successful first step
					if (turnstileContainerRef && window.turnstile) {
						window.turnstile.reset(turnstileContainerRef);
					}
					return;
				}
				if (!result.token || !result.user) {
					throw new Error('Invalid login response');
				}
				authStore.setAuth(result.token, result.user);
				goto('/');
				return;
			}

			const result = await auth.login({
				challenge_token: challengeToken,
				...(useRecoveryCode ? { two_factor_recovery_code: recoveryCode } : { two_factor_code: twoFactorCode })
			});
			if (!result.token || !result.user) {
				throw new Error('Invalid two-factor response');
			}
			authStore.setAuth(result.token, result.user);
			goto('/');
		} catch (e: any) {
			error = e.message ?? 'Sign in failed. Please try again.';
			// Reset Turnstile on error so user can try again
			if (!challengeToken && turnstileContainerRef && window.turnstile) {
				window.turnstile.reset(turnstileContainerRef);
				turnstileToken = '';
			}
		} finally {
			loading = false;
		}
	}

	function resetTwoFactorStep() {
		challengeToken = '';
		twoFactorCode = '';
		recoveryCode = '';
		useRecoveryCode = false;
		error = '';
		// Reset Turnstile and re-initialize it
		if (turnstileContainerRef && window.turnstile) {
			window.turnstile.reset(turnstileContainerRef);
			turnstileToken = '';
		}
	}

	// Handle Turnstile token updates
	$effect(() => {
		if (!authStore.loading && authStore.isAuthenticated) {
			goto(getAuthenticatedRedirectTarget(), { replaceState: true });
			return;
		}
	});

	$effect(() => {
		if (turnstileSiteKey && typeof window !== 'undefined' && window.turnstile && turnstileContainerRef && !challengeToken) {
			const container = turnstileContainerRef;
			// Update Turnstile callback to store token
			const observer = new MutationObserver(() => {
				if (window.turnstile) {
					const token = window.turnstile.getResponse(container);
					if (token) {
						turnstileToken = token;
					}
				}
			});

			observer.observe(turnstileContainerRef, { attributes: true, subtree: true });

			return () => {
				observer.disconnect();
			};
		}
	});
</script>

<svelte:head>
	<title>Sign in to GitPier</title>
</svelte:head>

<div class="auth-login-page">
	<div class="grid min-h-[95vh] w-full lg:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)]">
		<section class="flex items-center justify-center p-6 sm:p-8 lg:p-10">
			<div class="w-full max-w-md">
				<a href="/" class="mb-8 inline-flex">
					<img src="/images/logo.png" alt="GitPier" class="h-10 w-10 object-contain" />
				</a>

				<h1 class="text-3xl font-semibold tracking-tight text-foreground">{challengeToken ? 'Two-factor authentication' : 'Sign in to GitPier'}</h1>
				<p class="mt-2 text-sm text-muted-foreground">
					{#if challengeToken}
						Enter your verification code to continue.
					{:else}
						Access your repositories, pull requests, and workflows.
					{/if}
				</p>

				{#if error}
					<div class="mt-6 rounded-xl border border-red-800/50 bg-red-900/30 px-4 py-3 text-sm text-red-400">{error}</div>
				{/if}

				<form onsubmit={handleSubmit} class="mt-6 space-y-4">
					{#if !challengeToken}
						<div>
							<label for="email" class="mb-1.5 block text-sm font-semibold text-foreground">Username or email address</label>
							<input
								id="email"
								type="text"
								bind:value={email}
								required
								autofocus
								class="h-10 w-full rounded-xl border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground transition-all focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
							/>
						</div>

						<div>
							<div class="mb-1.5 flex items-center justify-between">
								<label for="password" class="text-sm font-semibold text-foreground">Password</label>
								<a href="/auth/forgot-password" class="text-xs text-muted-foreground transition-colors hover:text-foreground">Forgot password?</a>
							</div>
							<div class="relative">
								<input
									id="password"
									type={showPassword ? 'text' : 'password'}
									bind:value={password}
									required
									class="h-10 w-full rounded-xl border border-border bg-background px-3 pr-10 text-sm text-foreground placeholder:text-muted-foreground transition-all focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
								/>
								<button
									type="button"
									onclick={() => (showPassword = !showPassword)}
									class="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
								>
									{#if showPassword}<EyeOff class="h-4 w-4" />{:else}<Eye class="h-4 w-4" />{/if}
								</button>
							</div>
						</div>

						{#if turnstileSiteKey}
							<div class="flex justify-center py-1">
								<div bind:this={turnstileContainerRef} class="cf-turnstile" data-sitekey={turnstileSiteKey}></div>
							</div>
						{/if}

						<Button variant="brand" type="submit" class="mt-1 h-10 w-full rounded-xl" disabled={loading || (!!turnstileSiteKey && !turnstileReady)}>
							{#if loading}<Loader class="h-4 w-4 animate-spin" />{/if}
							Sign in
						</Button>
					{:else}
						<p class="text-xs text-muted-foreground">Enter the 6-digit code from your authenticator app. If you lost access, use one of your recovery codes.</p>
						{#if !useRecoveryCode}
							<div>
								<label for="two_factor_code" class="mb-1.5 block text-sm font-semibold text-foreground">Authenticator code</label>
								<input
									id="two_factor_code"
									type="text"
									inputmode="numeric"
									maxlength={6}
									bind:value={twoFactorCode}
									autofocus
									required
									class="h-10 w-full rounded-xl border border-border bg-background px-3 text-sm tracking-[0.2em] text-foreground placeholder:text-muted-foreground transition-all focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
								/>
							</div>
						{:else}
							<div>
								<label for="recovery_code" class="mb-1.5 block text-sm font-semibold text-foreground">Recovery code</label>
								<input
									id="recovery_code"
									type="text"
									bind:value={recoveryCode}
									autofocus
									required
									placeholder="XXXX-XXXX"
									class="h-10 w-full rounded-xl border border-border bg-background px-3 text-sm tracking-[0.15em] text-foreground uppercase placeholder:text-muted-foreground transition-all focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
								/>
							</div>
						{/if}

						<div class="flex flex-col gap-2">
							<Button variant="brand" type="submit" class="h-10 rounded-xl" disabled={loading || (!useRecoveryCode && !twoFactorCode) || (useRecoveryCode && !recoveryCode)}>
								{#if loading}<Loader class="h-4 w-4 animate-spin" />{/if}
								Verify and sign in
							</Button>
							<Button variant="outline" type="button" class="h-10 rounded-xl" onclick={() => (useRecoveryCode = !useRecoveryCode)}>
								{useRecoveryCode ? 'Use authenticator code instead' : 'Use recovery code instead'}
							</Button>
							<Button variant="ghost" type="button" class="h-10 rounded-xl" onclick={resetTwoFactorStep}>Back to password sign in</Button>
						</div>
					{/if}
				</form>

				<div class="mt-6 text-center text-sm text-muted-foreground">
					New to GitPier?
					<a href="/signup" class="ml-1 font-semibold text-primary hover:underline">Create an account</a>
				</div>
			</div>
		</section>

		<aside class="auth-login-hero relative hidden overflow-hidden p-10 lg:flex lg:flex-col lg:justify-between">
			<div class="relative z-10">
				<p class="text-3xl font-semibold tracking-tight text-white">GitPier</p>
			</div>
			<div class="auth-login-bars" aria-hidden="true">
				<span style="--x:7%; --h:20%; --t:20%; --d:0.12s;"></span>
				<span style="--x:16%; --h:31%; --t:48%; --d:0.24s;"></span>
				<span style="--x:26%; --h:17%; --t:18%; --d:0.3s;"></span>
				<span style="--x:36%; --h:43%; --t:34%; --d:0.42s;"></span>
				<span style="--x:47%; --h:30%; --t:13%; --d:0.52s;"></span>
				<span style="--x:57%; --h:37%; --t:44%; --d:0.65s;"></span>
				<span style="--x:67%; --h:49%; --t:18%; --d:0.76s;"></span>
				<span style="--x:77%; --h:25%; --t:12%; --d:0.87s;"></span>
				<span style="--x:86%; --h:58%; --t:50%; --d:0.96s;"></span>
			</div>
			<div class="relative z-10 max-w-md">
				<h2 class="text-5xl font-semibold leading-[1.02] tracking-tight text-white">Continue where your code left off.</h2>
				<p class="mt-4 text-lg leading-relaxed text-white/88">Sign in to review pull requests, track issues, and run workflows across your GitPier repositories.</p>
			</div>
		</aside>
	</div>
</div>

<style>
	.auth-login-page {
		background-image:
			radial-gradient(90rem 36rem at -20% -25%, color-mix(in oklch, var(--brand) 14%, transparent), transparent 65%),
			radial-gradient(80rem 24rem at 120% 115%, color-mix(in oklch, var(--brand) 8%, transparent), transparent 70%);
	}

	.auth-login-hero {
		background: linear-gradient(
			162deg,
			color-mix(in oklch, var(--brand) 48%, white 38%) 0%,
			color-mix(in oklch, var(--brand) 78%, black 20%) 58%,
			color-mix(in oklch, var(--brand) 88%, black 52%) 100%
		);
	}

	.auth-login-hero::before {
		content: '';
		position: absolute;
		inset: 0;
		background:
			radial-gradient(34rem 24rem at 20% 28%, color-mix(in oklch, white 20%, transparent), transparent 72%), linear-gradient(to top, rgb(0 0 0 / 44%), rgb(0 0 0 / 12%) 40%, transparent 66%);
		pointer-events: none;
	}

	.auth-login-bars {
		position: absolute;
		inset: 0;
	}

	.auth-login-bars span {
		position: absolute;
		left: var(--x);
		top: var(--t);
		width: 1.2rem;
		height: var(--h);
		border-radius: 0.55rem;
		background: linear-gradient(to bottom, rgb(255 255 255 / 30%), rgb(255 255 255 / 2%));
		box-shadow: 0 0 1.8rem color-mix(in oklch, var(--brand) 28%, transparent);
		animation: pulse-login-bar 4.8s ease-in-out infinite;
		animation-delay: var(--d);
	}

	.auth-login-bars span::before {
		content: '';
		position: absolute;
		left: 50%;
		top: -18%;
		width: 2px;
		height: 136%;
		transform: translateX(-50%);
		background: linear-gradient(to bottom, rgb(255 255 255 / 42%), transparent);
	}

	@keyframes pulse-login-bar {
		0%,
		100% {
			opacity: 0.34;
			transform: translateY(0);
		}
		50% {
			opacity: 0.92;
			transform: translateY(-0.5rem);
		}
	}
</style>
