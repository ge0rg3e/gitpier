import type { Route } from './+types/privacy';

export function meta() {
  return [
    { title: 'Privacy Policy · GitPier' },
    { name: 'description', content: 'Privacy policy for the GitPier alpha website.' },
  ];
}

export default function PrivacyPage() {
  return (
    <section className="mx-auto mt-12 max-w-3xl px-4 py-24 text-foreground">
      <h1 className="text-3xl font-bold tracking-tight">Privacy Policy</h1>
      <p className="mt-2 text-sm text-muted-foreground">
        Last updated: <time dateTime="2026-05-31">May 31, 2026</time>
      </p>

      <div className="mt-8 space-y-5 text-sm text-muted-foreground leading-relaxed">
        <p>GitPier is currently in an alpha stage. This policy explains what data is processed when you use this website.</p>

        <div>
          <h2 className="text-lg font-semibold text-foreground">Data we collect now</h2>
          <ul className="mt-2 list-disc pl-5 space-y-1">
            <li>Basic technical logs (for example, IP address and browser details) for security and abuse prevention.</li>
          </ul>
        </div>

        <div>
          <h2 className="text-lg font-semibold text-foreground">How we use it</h2>
          <ul className="mt-2 list-disc pl-5 space-y-1">
            <li>To operate and secure this website.</li>
          </ul>
        </div>

        <div>
          <h2 className="text-lg font-semibold text-foreground">Cookies and consent</h2>
          <p>We only enable analytics after you accept the cookie banner.</p>
          <ul className="mt-2 list-disc pl-5 space-y-1">
            <li><code>_trk_uid</code>: visitor identifier used for analytics visit tracking (stored for up to 365 days).</li>
            <li><code>_trk_ses</code>: session identifier used to group page views in a single session (stored for about 30 minutes and refreshed while active).</li>
          </ul>
          <p className="mt-2">These cookies are first-party analytics cookies used by GitPier for traffic measurement.</p>
        </div>

        <div>
          <h2 className="text-lg font-semibold text-foreground">Sharing</h2>
          <p>We do not sell your personal data. We only share data when required by law or to run the service.</p>
        </div>

        <div>
          <h2 className="text-lg font-semibold text-foreground">Retention</h2>
          <p>We retain technical logs only as long as needed for security, abuse prevention, and operational reliability.</p>
        </div>

        <div>
          <h2 className="text-lg font-semibold text-foreground">Contact</h2>
          <p>For privacy requests, contact <a className="text-brand underline underline-offset-2" href="mailto:privacy@gitpier.com">privacy@gitpier.com</a>.</p>
        </div>

        <div>
          <h2 className="text-lg font-semibold text-foreground">Changes</h2>
          <p>We may update this policy as GitPier evolves. The latest version will always be posted here.</p>
        </div>
      </div>
    </section>
  );
}
