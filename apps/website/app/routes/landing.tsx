import type { Route } from './+types/index';
import { LayoutLines } from '@/components/landing/ui/layout-lines';
import { Hero } from '@/components/landing/sections/hero';
import { ItemsSection } from '@/components/landing/sections/items-section';
import { StatsSection } from '@/components/landing/sections/stats-section';
import { FAQSection } from '@/components/landing/sections/faq-section';
import { CTASection } from '@/components/landing/sections/cta-section';

export function meta() {
  return [
    { title: 'GitPier - Git collaboration for developers.' },
  ];
}

export default function LandingPage() {
  return (
    <main className="bg-background text-foreground min-h-screen w-full">
      <LayoutLines />
      <Hero />
      <ItemsSection />
      <StatsSection />
      <FAQSection />
      <CTASection />
    </main>
  );
}
