<svelte:head>
	<title>Cookie Policy · GitPier</title>
	<meta name="description" content="GitPier Cookie Policy — what cookies we use and why." />
</svelte:head>

<div class="bg-background min-h-screen py-12 px-4">
	<div class="mx-auto max-w-3xl">
		<header class="mb-10">
			<p class="text-xs font-semibold uppercase tracking-widest text-primary mb-2">Legal</p>
			<h1 class="text-3xl font-bold tracking-tight text-foreground">Cookie Policy</h1>
			<p class="mt-2 text-sm text-muted-foreground">Last updated: <time datetime="2026-05-05">May 5, 2026</time></p>
		</header>

		<div
			class="prose prose-sm max-w-none text-foreground
			[&_h2]:text-lg [&_h2]:font-semibold [&_h2]:text-foreground [&_h2]:mt-10 [&_h2]:mb-3 [&_h2]:pb-1 [&_h2]:border-b [&_h2]:border-border
			[&_h3]:text-base [&_h3]:font-semibold [&_h3]:text-foreground [&_h3]:mt-6 [&_h3]:mb-2
			[&_p]:text-sm [&_p]:text-muted-foreground [&_p]:leading-relaxed [&_p]:mb-3
			[&_ul]:text-sm [&_ul]:text-muted-foreground [&_ul]:pl-5 [&_ul]:mb-3 [&_ul]:space-y-1 [&_ul>li]:list-disc
			[&_a]:text-primary [&_a]:underline [&_a]:underline-offset-2 [&_a:hover]:text-primary/80
			[&_strong]:text-foreground [&_strong]:font-semibold
			[&_table]:w-full [&_table]:text-sm [&_table]:mb-4
			[&_th]:text-left [&_th]:font-semibold [&_th]:text-foreground [&_th]:pb-1 [&_th]:border-b [&_th]:border-border
			[&_td]:py-2 [&_td]:pr-3 [&_td]:align-top [&_td]:text-muted-foreground [&_td]:border-b [&_td]:border-border/50
		"
		>
			<!-- 1 -->
			<h2>1. What Are Cookies?</h2>
			<p>Cookies are small text files placed on your device when you visit a website. They allow the website to remember information about your visit, such as whether you are logged in.</p>
			<p>
				Under the EU ePrivacy Directive (Directive 2002/58/EC, as amended) and national implementations thereof, <strong>strictly necessary cookies</strong> may be set without prior consent because
				they are essential for the service you have explicitly requested. All other cookies require your informed consent before being placed.
			</p>

			<!-- 2 -->
			<h2>2. Cookies We Use</h2>
			<p>
				GitPier uses <strong>only strictly necessary cookies</strong>. We do not set advertising, analytics, or tracking cookies of any kind.
			</p>

			<table>
				<thead>
					<tr>
						<th>Cookie name</th>
						<th>Type</th>
						<th>Purpose</th>
						<th>Duration</th>
					</tr>
				</thead>
				<tbody>
					<tr>
						<td><code class="text-xs bg-muted px-1 py-0.5 rounded font-mono">GitPier_session</code> (or JWT stored in HttpOnly cookie)</td>
						<td>Strictly necessary</td>
						<td>Keeps you authenticated between page loads. Contains or references a signed JWT issued on login.</td>
						<td>Session / until logout</td>
					</tr>
					<tr>
						<td><code class="text-xs bg-muted px-1 py-0.5 rounded font-mono">_csrf</code></td>
						<td>Strictly necessary</td>
						<td>Prevents Cross-Site Request Forgery (CSRF) attacks by linking your browser session to server-side state.</td>
						<td>Session</td>
					</tr>
					<tr>
						<td><code class="text-xs bg-muted px-1 py-0.5 rounded font-mono">cookie_consent</code></td>
						<td>Strictly necessary</td>
						<td>Stores your acknowledgement that you have seen the cookie consent notice, so it is not shown on every page.</td>
						<td>1 year</td>
					</tr>
				</tbody>
			</table>

			<p>
				<strong>No third-party cookies are set by GitPier.</strong> If repositories hosted here embed third-party content (e.g. images from external sources), those third parties may set their
				own cookies subject to their own policies — this is outside GitPier's control.
			</p>

			<!-- 3 -->
			<h2>3. localStorage &amp; sessionStorage</h2>
			<p>
				In addition to cookies, GitPier uses the browser's <code class="text-xs bg-muted px-1 py-0.5 rounded font-mono">localStorage</code> API to persist a small number of client-side preferences:
			</p>
			<table>
				<thead>
					<tr>
						<th>Key</th>
						<th>Purpose</th>
					</tr>
				</thead>
				<tbody>
					<tr>
						<td><code class="text-xs bg-muted px-1 py-0.5 rounded font-mono">theme</code></td>
						<td>Remembers your light/dark mode preference.</td>
					</tr>
					<tr>
						<td><code class="text-xs bg-muted px-1 py-0.5 rounded font-mono">cookie_consent</code></td>
						<td>Mirrors the consent cookie so the banner is not re-shown after clearing session cookies.</td>
					</tr>
				</tbody>
			</table>
			<p>localStorage data never leaves your browser and is not transmitted to our servers.</p>

			<!-- 4 -->
			<h2>4. Legal Basis</h2>
			<p>
				All cookies and localStorage entries listed above are strictly necessary for the operation of the Service as explicitly requested by you. They are placed on the basis of Art. 6(1)(b)
				GDPR (performance of a contract / provision of a requested service) and do not require separate consent under the ePrivacy Directive. No consent-dependent cookies are used.
			</p>

			<!-- 5 -->
			<h2>5. How to Control or Delete Cookies</h2>
			<p>
				Because all cookies set by GitPier are strictly necessary, disabling them through your browser will prevent the platform from functioning correctly (you will be unable to remain logged
				in).
			</p>
			<p>You can manage or delete cookies through your browser settings. Below are links to instructions for common browsers:</p>
			<ul>
				<li><a href="https://support.google.com/chrome/answer/95647" target="_blank" rel="noopener noreferrer">Google Chrome</a></li>
				<li><a href="https://support.mozilla.org/en-US/kb/clear-cookies-and-site-data-firefox" target="_blank" rel="noopener noreferrer">Mozilla Firefox</a></li>
				<li><a href="https://support.apple.com/guide/safari/manage-cookies-sfri11471/mac" target="_blank" rel="noopener noreferrer">Apple Safari</a></li>
				<li>
					<a href="https://support.microsoft.com/en-us/microsoft-edge/delete-cookies-in-microsoft-edge-63947406-40ac-c3b8-57b9-2a946a29ae09" target="_blank" rel="noopener noreferrer"
						>Microsoft Edge</a
					>
				</li>
			</ul>
			<p>To delete localStorage data, use the "Clear site data" option in your browser's developer tools or privacy settings.</p>

			<!-- 6 -->
			<h2>6. Changes to This Policy</h2>
			<p>
				If we introduce new cookies or change how we use existing ones, we will update this Cookie Policy and, where required by law, obtain fresh consent. The "Last updated" date at the top
				of this page always reflects the most recent revision.
			</p>

			<!-- 7 -->
			<h2>7. Contact</h2>
			<p>
				For questions about this Cookie Policy or our use of cookies, contact us using the details in our <a href="/legal/privacy">Privacy Policy</a>.
			</p>
		</div>
	</div>
</div>
