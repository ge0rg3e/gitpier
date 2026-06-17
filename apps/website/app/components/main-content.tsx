import { useLocation } from 'react-router';

export function MainContent({ children }: { children: React.ReactNode }) {
  const location = useLocation();
  const isDocs = location.pathname.startsWith('/docs');

  return (
    <main className={`flex-1 ${isDocs ? '' : 'pt-14'}`}>
      {children}
    </main>
  );
}
