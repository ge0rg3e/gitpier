import { useLocation } from 'react-router';
import { Button } from '@/components/ui/button';

export function SiteHeader() {
  const location = useLocation();
  const isDocs = location.pathname.startsWith('/docs');

  if (isDocs) return null;

  return (
    <header className="fixed w-full top-0 bg-linear-to-b from-background/20 to-transparent backdrop-blur-sm z-50">
      <div className="mx-auto flex h-14 max-w-container items-center gap-2 px-3 sm:gap-3 sm:px-4">
        <a href="/" className="mr-0.5 flex shrink-0 items-center sm:mr-1" aria-label="GitPier home">
          <img src="/images/logo.png" alt="GitPier" className="h-7 w-7 object-contain sm:h-8 sm:w-8" />
        </a>
        <Button href="https://github.com/ge0rg3e/gitpier" variant="ghost">
          Source code
          <span className="bg-brand/15 text-brand border-brand/40 ml-1 rounded-full border px-1.5 py-0.5 text-[10px] leading-none font-semibold tracking-wide uppercase">
            New
          </span>
        </Button>
        <div className="flex-1" />
        <Button href="/docs/self-hosted/get-started">Self-host GitPier</Button>
      </div>
    </header>
  );
}

export function SiteFooter() {
  const location = useLocation();
  const isDocs = location.pathname.startsWith('/docs');

  if (isDocs) return null;

  return (
    <footer className="border-t border-secondary bg-background py-6">
      <div className="mx-auto max-w-screen-xl px-4">
        <div className="flex flex-col gap-4 text-xs text-muted-foreground">
          <div className="flex flex-col sm:flex-row items-center justify-between gap-4">
            <div className="flex items-center gap-1">
              <img src="/images/logo.png" alt="GitPier" className="h-5 w-5 object-contain opacity-70" />
              <span>&copy; 2026 GitPier - All rights reserved.</span>
            </div>
            <nav className="flex flex-wrap items-center justify-center gap-4">
              <a href="https://github.com/ge0rg3e/gitpier" target="_blank" rel="noopener noreferrer" className="hover:text-foreground transition-colors">Source code</a>
              <a href="/docs" className="hover:text-foreground transition-colors">Docs</a>
              <a href="/legal/privacy" className="hover:text-foreground transition-colors">Privacy Policy</a>
              <a href="/legal/terms" className="hover:text-foreground transition-colors">Terms of Service</a>
            </nav>
          </div>
        </div>
      </div>
    </footer>
  );
}
