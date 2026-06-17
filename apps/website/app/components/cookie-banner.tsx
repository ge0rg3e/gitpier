import { useState, useEffect } from 'react';
import { Button } from '@/components/ui/button';

const OWT_SRC = 'https://owt.gitpier.com/script.js';
const OWT_SITE_ID = '85b37ef6-d360-482b-8f0b-7b42c2ebae25';
const OWT_DOMAIN = 'gitpier.com';
const STORAGE_KEY = 'cookie_consent';

const EU_TIMEZONES = [
  'Europe/Amsterdam', 'Europe/Andorra', 'Europe/Athens', 'Europe/Belgrade',
  'Europe/Berlin', 'Europe/Bratislava', 'Europe/Brussels', 'Europe/Bucharest',
  'Europe/Budapest', 'Europe/Busingen', 'Europe/Chisinau', 'Europe/Copenhagen',
  'Europe/Dublin', 'Europe/Gibraltar', 'Europe/Guernsey', 'Europe/Helsinki',
  'Europe/Isle_of_Man', 'Europe/Istanbul', 'Europe/Jersey', 'Europe/Kaliningrad',
  'Europe/Kiev', 'Europe/Kirov', 'Europe/Kyiv', 'Europe/Lisbon',
  'Europe/Ljubljana', 'Europe/London', 'Europe/Luxembourg', 'Europe/Madrid',
  'Europe/Malta', 'Europe/Mariehamn', 'Europe/Minsk', 'Europe/Monaco',
  'Europe/Moscow', 'Europe/Nicosia', 'Europe/Oslo', 'Europe/Paris',
  'Europe/Podgorica', 'Europe/Prague', 'Europe/Riga', 'Europe/Rome',
  'Europe/Samara', 'Europe/San_Marino', 'Europe/Sarajevo', 'Europe/Saratov',
  'Europe/Simferopol', 'Europe/Skopje', 'Europe/Sofia', 'Europe/Stockholm',
  'Europe/Tallinn', 'Europe/Tirane', 'Europe/Ulyanovsk', 'Europe/Uzhgorod',
  'Europe/Vaduz', 'Europe/Vatican', 'Europe/Vienna', 'Europe/Vilnius',
  'Europe/Volgograd', 'Europe/Warsaw', 'Europe/Zagreb', 'Europe/Zaporozhye',
  'Europe/Zurich', 'Atlantic/Azores', 'Atlantic/Canary', 'Atlantic/Faroe',
  'Atlantic/Madeira', 'Arctic/Longyearbyen', 'Indian/Reunion', 'Indian/Mayotte',
  'Indian/Maldives',
];

function injectTracker() {
  if (document.querySelector(`script[src="${OWT_SRC}"]`)) return;
  const s = document.createElement('script');
  s.defer = true;
  s.src = OWT_SRC;
  s.setAttribute('data-website-id', OWT_SITE_ID);
  s.setAttribute('data-domain', OWT_DOMAIN);
  document.head.appendChild(s);
}

function isEuTimezone(): boolean {
  try {
    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
    return EU_TIMEZONES.some((prefix) => tz.startsWith(prefix));
  } catch {
    return false;
  }
}

export function CookieBanner() {
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === 'accepted') {
      injectTracker();
    } else if (stored === 'declined') {
      (window as Window & { _trk_disabled?: boolean })._trk_disabled = true;
    } else if (isEuTimezone()) {
      setVisible(true);
    } else {
      injectTracker();
    }
  }, []);

  function accept() {
    localStorage.setItem(STORAGE_KEY, 'accepted');
    setVisible(false);
    injectTracker();
  }

  function decline() {
    localStorage.setItem(STORAGE_KEY, 'declined');
    (window as Window & { _trk_disabled?: boolean })._trk_disabled = true;
    setVisible(false);
  }

  if (!visible) return null;

  return (
    <div
      className="fixed bottom-5 left-1/2 z-[9999] w-[min(calc(100vw-2rem),36rem)] -translate-x-1/2 animate-[slide-up_0.3s_cubic-bezier(0.16,1,0.3,1)_both]"
      role="dialog"
      aria-label="Cookie consent"
      aria-live="polite"
    >
      <div className="flex items-center gap-4 rounded-xl border border-[oklch(1_0_0_/_10%)] bg-[oklch(0.205_0_0)] px-4 py-3.5 shadow-[0_4px_24px_oklch(0_0_0_/_60%),0_1px_0_oklch(1_0_0_/_6%)_inset] max-[480px]:flex-col max-[480px]:items-start max-[480px]:gap-3">
        <div className="flex min-w-0 flex-1 items-center gap-2.5">
          <span className="shrink-0 text-xl leading-none">🍪</span>
          <p className="m-0 text-[0.8125rem] leading-[1.4] text-[oklch(0.708_0_0)]">
            We use analytics cookies to understand site usage.<br />
            See our{' '}
            <a href="/legal/privacy" className="underline underline-offset-2 hover:text-[oklch(0.86_0_0)]">
              Privacy Policy
            </a>
            .
          </p>
        </div>
        <div className="flex shrink-0 gap-2 max-[480px]:w-full max-[480px]:justify-end">
          <Button variant="outline" onClick={decline}>Decline</Button>
          <Button onClick={accept}>Accept</Button>
        </div>
      </div>
      <style>{`
        @keyframes slide-up {
          from {
            opacity: 0;
            transform: translateX(-50%) translateY(1rem);
          }
          to {
            opacity: 1;
            transform: translateX(-50%) translateY(0);
          }
        }
      `}</style>
    </div>
  );
}
