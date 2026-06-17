import type { Route } from './+types/terms';

export function meta() {
  return [
    { title: 'Terms of Service · GitPier' },
    { name: 'description', content: 'Terms for using the GitPier alpha website.' },
  ];
}

export default function TermsPage() {
  return (
    <section className="mx-auto mt-12 max-w-3xl px-4 py-24 text-foreground">
      <h1 className="text-3xl font-bold tracking-tight">Terms of Service</h1>
      <p className="mt-2 text-sm text-muted-foreground">
        Last updated: <time dateTime="2026-05-27">May 27, 2026</time>
      </p>

      <div className="mt-8 space-y-5 text-sm text-muted-foreground leading-relaxed">
        <p>These terms apply to your use of the GitPier alpha website.</p>

        <div>
          <h2 className="text-lg font-semibold text-foreground">Pre-launch status</h2>
          <p>GitPier is not fully launched yet. Features, timelines, and availability may change at any time.</p>
        </div>

        <div>
          <h2 className="text-lg font-semibold text-foreground">Acceptable use</h2>
          <ul className="mt-2 list-disc pl-5 space-y-1">
            <li>Use this website lawfully.</li>
            <li>Do not attempt to abuse, disrupt, or attack the service.</li>
          </ul>
        </div>

        <div>
          <h2 className="text-lg font-semibold text-foreground">No warranty</h2>
          <p>This website is provided &quot;as is&quot; without warranties of any kind.</p>
        </div>

        <div>
          <h2 className="text-lg font-semibold text-foreground">Limitation of liability</h2>
          <p>To the extent allowed by law, we are not liable for indirect or consequential damages from use of this alpha site.</p>
        </div>

        <div>
          <h2 className="text-lg font-semibold text-foreground">Changes</h2>
          <p>We may update these terms as GitPier develops. Continued use means you accept the latest version.</p>
        </div>
      </div>
    </section>
  );
}
